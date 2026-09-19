package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

type safeConnection struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (c *safeConnection) write(ctx context.Context, typ websocket.MessageType, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.Write(ctx, typ, data)
}
func (c *safeConnection) ping(ctx context.Context) error { return c.conn.Ping(ctx) }
func (c *safeConnection) read(ctx context.Context) (websocket.MessageType, []byte, error) {
	return c.conn.Read(ctx)
}

func writeJSON(ctx context.Context, conn *safeConnection, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	if err := conn.write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("write WebSocket message: %w", err)
	}
	log.Printf("websocket message sent: %s", data)
	return nil
}

type performanceCommand uint8

const (
	performanceNone performanceCommand = iota
	performanceStart
	performanceStop
)

type serverCommand struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	Command string `json:"command"`
	PID     int32  `json:"pid"`
}

type streamKind uint8

const (
	streamUnknown streamKind = iota
	streamPerformance
	streamProcess
	streamScreen
)

type streamCommand struct {
	stream  streamKind
	start   bool
	kill    bool
	killPID int32
}

func parseStreamCommand(data []byte) streamCommand {
	var message serverCommand
	if err := json.Unmarshal(data, &message); err != nil {
		return streamCommand{}
	}

	typeValue := normalizeCommand(message.Type)
	// Power is a one-shot command and must never trigger stream aliases or kills.
	if typeValue == "power" {
		return streamCommand{}
	}
	action := normalizeCommand(message.Action)
	command := normalizeCommand(message.Command)
	for _, value := range []string{action, command, typeValue} {
		switch value {
		case "start_performance", "performance_start", "request_performance":
			return streamCommand{stream: streamPerformance, start: true}
		case "stop_performance", "performance_stop":
			return streamCommand{stream: streamPerformance}
		case "start_process", "process_start", "request_process":
			return streamCommand{stream: streamProcess, start: true}
		case "stop_process", "process_stop":
			return streamCommand{stream: streamProcess}
		case "start_stream", "start_screen", "screen_start", "request_screen":
			return streamCommand{stream: streamScreen, start: true}
		case "stop_stream", "stop_screen", "screen_stop":
			return streamCommand{stream: streamScreen}
		case "kill_process", "process_kill", "terminate_process":
			return streamCommand{stream: streamProcess, kill: true, killPID: message.PID}
		}
	}

	var stream streamKind
	switch typeValue {
	case "performance":
		stream = streamPerformance
	case "process", "processes":
		stream = streamProcess
	case "screen", "screen_stream":
		stream = streamScreen
	default:
		return streamCommand{}
	}
	if stream == streamProcess && (action == "kill" || action == "terminate") {
		return streamCommand{stream: streamProcess, kill: true, killPID: message.PID}
	}
	if action == "start" || action == "request" {
		return streamCommand{stream: stream, start: true}
	}
	if action == "stop" {
		return streamCommand{stream: stream}
	}
	return streamCommand{}
}

func parsePerformanceCommand(data []byte) performanceCommand {
	command := parseStreamCommand(data)
	if command.stream != streamPerformance {
		return performanceNone
	}
	if command.start {
		return performanceStart
	}
	return performanceStop
}

func normalizeCommand(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
}

func readJSONMessages(ctx context.Context, conn *safeConnection, onMessage func([]byte), onStreamCommand func(streamCommand)) error {
	for {
		messageType, data, err := conn.read(ctx)
		if err != nil {
			return fmt.Errorf("read WebSocket message: %w", err)
		}
		if messageType != websocket.MessageText || !json.Valid(data) {
			log.Printf("invalid server message: expected JSON text")
			continue
		}
		log.Printf("websocket message received: %s", data)
		onMessage(data)
		if command := parseStreamCommand(data); command.stream != streamUnknown {
			onStreamCommand(command)
		}
	}
}
