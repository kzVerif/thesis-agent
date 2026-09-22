package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"ws-agent/antivirus"
	"ws-agent/download"
	"ws-agent/internal/retry"
	"ws-agent/internal/transportpolicy"
)

type Client struct {
	agentID             string
	loadPrivateKey      PrivateKeyLoader
	eventCtx            context.Context
	eventCancel         context.CancelFunc
	closeOnce           sync.Once
	stopping            atomic.Bool
	stableConnection    bool
	url                 string
	heartbeatInterval   time.Duration
	performanceInterval time.Duration
	pingTimeout         time.Duration
	downloadManager     *download.Manager
	connectionMu        sync.RWMutex
	connection          *safeConnection
	pendingResults      chan any
	scanManager         *antivirus.Manager
	powerController     PowerController
}

type MessageProvider func() (any, error)
type ProcessKiller func(pid int32) error

type processCommandResult struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	PID     int32  `json:"pid"`
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

func New(url string, heartbeatInterval, performanceInterval, pingTimeout time.Duration) *Client {
	c := &Client{
		url:                 url,
		heartbeatInterval:   heartbeatInterval,
		performanceInterval: performanceInterval,
		pingTimeout:         pingTimeout,
		pendingResults:      make(chan any, 32),
	}
	c.eventCtx, c.eventCancel = context.WithCancel(context.Background())
	c.scanManager = antivirus.NewManager(antivirus.Scan, c.sendDownloadEvent)
	return c
}

func (c *Client) ConfigureDownloads(cfg download.Config, agentID string) error {
	m, err := download.NewManager(cfg, agentID, c.sendDownloadEvent)
	if err == nil {
		c.downloadManager = m
	}
	return err
}

func (c *Client) sendDownloadEvent(message any) error {
	if c.stopping.Load() {
		return context.Canceled
	}
	c.connectionMu.RLock()
	conn := c.connection
	c.connectionMu.RUnlock()
	if conn != nil {
		ctx, cancel := context.WithTimeout(c.eventCtx, 10*time.Second)
		err := writeJSON(ctx, conn, message)
		cancel()
		if err == nil {
			return nil
		}
	}
	_, downloadResult := message.(download.Result)
	scanEvent, scanResult := message.(antivirus.Result)
	if !downloadResult && !(scanResult && scanEvent.Type == "virus_scan_result") {
		// Progress and transient statuses are safe to drop while disconnected;
		// reserve the bounded reconnect queue for final job results.
		return nil
	}
	select {
	case c.pendingResults <- message:
		return nil
	default:
		return fmt.Errorf("pending WebSocket event queue is full")
	}
}

func (c *Client) flushPending(ctx context.Context, conn *safeConnection) {
	for {
		select {
		case msg := <-c.pendingResults:
			if err := writeJSON(ctx, conn, msg); err != nil {
				select {
				case c.pendingResults <- msg:
				default:
					{
					}
				}
				return
			}
		default:
			return
		}
	}
}

func (c *Client) Run(
	ctx context.Context,
	initialMessage any,
	performanceProvider MessageProvider,
	processProvider MessageProvider,
	processKiller ProcessKiller,
	screenCapture ScreenCapture,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopOnClose := context.AfterFunc(c.eventCtx, cancel)
	defer stopOnClose()
	// Cancel pending power operations and manager I/O as soon as the runtime
	// stops, even while a provider is still unwinding in runConnection.
	closeOnCancel := context.AfterFunc(ctx, c.Close)
	defer func() { closeOnCancel(); c.Close() }()
	backoff := retry.New()
	for {
		if ctx.Err() != nil {
			return nil
		}
		c.stableConnection = false
		connectionErr := c.runConnection(ctx, initialMessage, performanceProvider, processProvider, processKiller, screenCapture)
		if ctx.Err() != nil {
			return nil
		}
		if c.stableConnection {
			backoff.Reset()
		}
		delay := backoff.Next()
		// Remote close reasons and URL-bearing errors are untrusted log input.
		reason := transportpolicy.FailureKind(connectionErr)
		var authErr *authenticationError
		if errors.As(connectionErr, &authErr) {
			reason = authErr.Error()
		}
		log.Printf("websocket disconnected (%s); reconnect scheduled in %s", reason, delay)
		if retry.Wait(ctx, delay) != nil {
			return nil
		}
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.stopping.Store(true)
		c.eventCancel()
		c.connectionMu.RLock()
		conn := c.connection
		c.connectionMu.RUnlock()
		if conn != nil {
			_ = conn.conn.CloseNow()
		}
		if closer, ok := c.powerController.(interface{ Close() }); ok {
			closer.Close()
		}
		if c.downloadManager != nil {
			c.downloadManager.Close()
		}
		c.scanManager.Close()
	})
}

func (c *Client) runConnection(
	ctx context.Context,
	initialMessage any,
	performanceProvider MessageProvider,
	processProvider MessageProvider,
	processKiller ProcessKiller,
	screenCapture ScreenCapture,
) error {
	log.Printf("websocket connecting")
	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	rawConn, _, err := websocket.Dial(dialCtx, c.url, &websocket.DialOptions{HTTPClient: transportpolicy.NewClient(0)})
	dialCancel()
	if err != nil {
		return fmt.Errorf("connect WebSocket: %w", err)
	}
	defer rawConn.CloseNow()
	if err := c.authenticate(ctx, rawConn); err != nil {
		return err
	}
	log.Printf("Agent WebSocket authenticated")
	conn := &safeConnection{conn: rawConn}
	// Initial metadata must precede asynchronous queued job results.
	if err := writeJSON(ctx, conn, initialMessage); err != nil {
		return err
	}
	c.connectionMu.Lock()
	c.connection = conn
	c.connectionMu.Unlock()
	defer func() {
		c.connectionMu.Lock()
		if c.connection == conn {
			c.connection = nil
		}
		c.connectionMu.Unlock()
	}()

	log.Printf("websocket connected")
	connectedAt := time.Now()
	defer func() { c.stableConnection = time.Since(connectedAt) >= 30*time.Second }()
	c.flushPending(ctx, conn)

	runCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	start := func(fn func()) { workers.Add(1); go func() { defer workers.Done(); fn() }() }

	performanceCommands := make(chan bool, 1)
	processCommands := make(chan bool, 1)
	screenStreamer := NewScreenStreamer(screenCapture)
	defer func() { cancel(); _ = rawConn.CloseNow(); screenStreamer.Stop(); workers.Wait() }()
	start(func() { c.heartbeat(runCtx, conn) })
	if performanceProvider != nil {
		start(func() { c.publishPerformance(runCtx, conn, performanceProvider, performanceCommands) })
	}
	if processProvider != nil {
		start(func() { c.publishProcess(runCtx, conn, processProvider, processCommands) })
	}
	if err := readJSONMessages(runCtx, conn, func(data []byte) {
		if c.handleVirusScan(ctx, data) {
			return
		}
		if command, ok := parsePowerCommand(data); ok {
			start(func() { c.handlePowerCommand(runCtx, conn, command) })
			return
		}
		if c.downloadManager == nil {
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &envelope) != nil || normalizeCommand(envelope.Type) != "download_file" {
			return
		}
		var command download.Command
		if err := json.Unmarshal(data, &command); err != nil {
			c.downloadManager.Reject(command, "DOWNLOAD_FILE message is malformed")
			return
		}
		c.downloadManager.Submit(ctx, command)
	}, func(command streamCommand) {
		switch command.stream {
		case streamPerformance:
			setStreamState(performanceCommands, command.start)
		case streamProcess:
			if command.kill {
				start(func() { c.killProcess(runCtx, conn, command.killPID, processKiller) })
			} else if processProvider != nil {
				setStreamState(processCommands, command.start)
			}
		case streamScreen:
			if command.start {
				screenStreamer.Start(runCtx, conn)
			} else {
				screenStreamer.Stop()
			}
		}
	}); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) killProcess(ctx context.Context, conn *safeConnection, pid int32, killer ProcessKiller) {
	result := processCommandResult{
		Type:   "process",
		Action: "kill_result",
		PID:    pid,
	}
	if pid <= 0 {
		result.Message = "ไม่สามารถปิดโปรเซสได้"
		result.Error = "หมายเลข PID ต้องมีค่ามากกว่าศูนย์"
	} else if killer == nil {
		result.Message = "ไม่สามารถปิดโปรเซสได้"
		result.Error = "ระบบไม่รองรับการปิดโปรเซส"
	} else if err := killer(pid); err != nil {
		result.Message = "ไม่สามารถปิดโปรเซสได้"
		result.Error = fmt.Sprintf("ไม่สามารถปิดโปรเซส PID %d ได้ กรุณาตรวจสอบว่าโปรเซสยังทำงานอยู่และ agent มีสิทธิ์เพียงพอ", pid)
		log.Printf("kill process %d failed: %v", pid, err)
	} else {
		result.Success = true
		result.Message = "ปิดโปรเซสสำเร็จ"
	}
	if err := writeJSON(ctx, conn, result); err != nil {
		log.Printf("send process kill result failed: %v", err)
	}
}

func setStreamState(commands chan bool, start bool) {
	select {
	case commands <- start:
	default:
		// Only the latest requested state matters.
		select {
		case <-commands:
		default:
		}
		commands <- start
	}
}
