package agent

import (
	"context"
	"log"
	"ws-agent/internal/apppaths"
	"ws-agent/internal/protectedpath"
	"ws-agent/internal/servicehost"
)

type runtimeSecurity func(context.Context, apppaths.Paths, protectedpath.Reporter) (func(), error)

type startupDiagnostics struct {
	pending []string
	ready   bool
	event   func(string, bool)
	file    func(string)
}

func newStartupDiagnostics() *startupDiagnostics {
	return &startupDiagnostics{event: servicehost.ReportStartupDiagnostic, file: func(s string) { log.Print(s) }}
}

func (d *startupDiagnostics) report(event protectedpath.Diagnostic) {
	message := event.String()
	if d.ready {
		d.file(message)
		return
	}
	d.pending = append(d.pending, message)
	d.event(message, event.Action == "fail_closed")
}

func (d *startupDiagnostics) fileReady() {
	d.ready = true
	for _, message := range d.pending {
		d.file(message)
	}
	d.pending = nil
}

// A logger I/O rejection must not recursively write through that same logger's
// mutex or into the rejected tree. After startup, use Event Log directly.
func (d *startupDiagnostics) logBoundaryReport(event protectedpath.Diagnostic) {
	if d.ready {
		d.event(event.String(), true)
		return
	}
	d.report(event)
}
