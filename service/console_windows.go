//go:build windows

package service

import "golang.org/x/sys/windows"

// DetachConsole lets the enrolled Agent continue as a background process.
// The first-run registration prompt happens before this function is called.
func DetachConsole() {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")
	_, _, _ = proc.Call()
}
