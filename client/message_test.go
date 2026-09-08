package client

import "testing"

func TestParsePerformanceCommand(t *testing.T) {
	tests := []struct {
		name string
		json string
		want performanceCommand
	}{
		{"start action", `{"action":"start_performance"}`, performanceStart},
		{"stop command", `{"command":"stop-performance"}`, performanceStop},
		{"structured start", `{"type":"performance","action":"start"}`, performanceStart},
		{"structured stop", `{"type":"performance","action":"stop"}`, performanceStop},
		{"unrelated", `{"type":"hello"}`, performanceNone},
		{"invalid JSON", `{`, performanceNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parsePerformanceCommand([]byte(tt.json)); got != tt.want {
				t.Fatalf("parsePerformanceCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseScreenCommands(t *testing.T) {
	start := parseStreamCommand([]byte(`{"type":"screen","action":"start"}`))
	if start.stream != streamScreen || !start.start {
		t.Fatalf("unexpected start command: %+v", start)
	}
	stop := parseStreamCommand([]byte(`{"type":"stop_stream"}`))
	if stop.stream != streamScreen || stop.start {
		t.Fatalf("unexpected stop command: %+v", stop)
	}
}

func TestParseProcessCommand(t *testing.T) {
	tests := []struct {
		name  string
		json  string
		start bool
	}{
		{"start action", `{"action":"start_process"}`, true},
		{"stop command", `{"command":"stop-process"}`, false},
		{"structured start", `{"type":"process","action":"start"}`, true},
		{"structured stop", `{"type":"processes","action":"stop"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStreamCommand([]byte(tt.json))
			if got.stream != streamProcess || got.start != tt.start {
				t.Fatalf("parseStreamCommand() = %+v, want process start=%v", got, tt.start)
			}
		})
	}
}

func TestParseKillProcessCommand(t *testing.T) {
	tests := []string{
		`{"type":"process","action":"kill","pid":1234}`,
		`{"action":"kill_process","pid":1234}`,
		`{"command":"terminate-process","pid":1234}`,
	}

	for _, message := range tests {
		got := parseStreamCommand([]byte(message))
		if got.stream != streamProcess || !got.kill || got.killPID != 1234 {
			t.Fatalf("parseStreamCommand(%s) = %+v, want process PID 1234", message, got)
		}
	}
}
