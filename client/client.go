package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
	"ws-agent/antivirus"
	"ws-agent/download"
)

type Client struct {
	url                 string
	heartbeatInterval   time.Duration
	performanceInterval time.Duration
	pingTimeout         time.Duration
	downloadManager     *download.Manager
	connectionMu        sync.RWMutex
	connection          *safeConnection
	pendingResults      chan any
	scanManager         *antivirus.Manager
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
	c.connectionMu.RLock()
	conn := c.connection
	c.connectionMu.RUnlock()
	if conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	const reconnectDelay = 3 * time.Second
	for {
		err := c.runConnection(ctx, initialMessage, performanceProvider, processProvider, processKiller, screenCapture)
		if ctx.Err() != nil {
			return nil
		}
		fmt.Printf("WebSocket disconnected: %v; reconnecting in %s\n", err, reconnectDelay)
		timer := time.NewTimer(reconnectDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil
		}
	}
}

func (c *Client) runConnection(
	ctx context.Context,
	initialMessage any,
	performanceProvider MessageProvider,
	processProvider MessageProvider,
	processKiller ProcessKiller,
	screenCapture ScreenCapture,
) error {
	rawConn, _, err := websocket.Dial(ctx, c.url, nil)
	if err != nil {
		return fmt.Errorf("connect WebSocket: %w", err)
	}
	defer rawConn.CloseNow()
	conn := &safeConnection{conn: rawConn}
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

	fmt.Println("Connected to WebSocket Server")
	if err := writeJSON(ctx, conn, initialMessage); err != nil {
		return err
	}
	c.flushPending(ctx, conn)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	performanceCommands := make(chan bool, 1)
	processCommands := make(chan bool, 1)
	screenStreamer := NewScreenStreamer(screenCapture)
	defer screenStreamer.Stop()
	go c.heartbeat(runCtx, conn)
	go c.publishPerformance(runCtx, conn, performanceProvider, performanceCommands)
	if processProvider != nil {
		go c.publishProcess(runCtx, conn, processProvider, processCommands)
	}
	if err := readJSONMessages(runCtx, conn, func(data []byte) {
		if c.handleVirusScan(ctx, data) {
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
				go c.killProcess(runCtx, conn, command.killPID, processKiller)
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
		fmt.Printf("Kill process %d failed: %v\n", pid, err)
	} else {
		result.Success = true
		result.Message = "ปิดโปรเซสสำเร็จ"
	}
	if err := writeJSON(ctx, conn, result); err != nil {
		fmt.Println("Send process kill result failed:", err)
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
