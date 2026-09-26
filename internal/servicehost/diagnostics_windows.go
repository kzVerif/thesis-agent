//go:build windows

package servicehost

import "golang.org/x/sys/windows/svc/eventlog"

// ReportStartupDiagnostic uses the installer-owned source; startup never creates
// a source or bypasses validation when Event Log is unavailable.
func ReportStartupDiagnostic(message string, failure bool) {
	if events, err := eventlog.Open(Name); err == nil {
		defer events.Close()
		if failure {
			_ = events.Error(1, message)
		} else {
			_ = events.Info(2, message)
		}
	}
}
