//go:build windows

package servicehost

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// RequireStoppedForKeyMaintenance queries SCM without stopping or modifying it.
func RequireStoppedForKeyMaintenance() error {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("cannot query SCM before private-key maintenance")
	}
	defer windows.CloseServiceHandle(manager)
	name, err := windows.UTF16PtrFromString(Name)
	if err != nil {
		return err
	}
	handle, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot query Service before private-key maintenance")
	}
	defer windows.CloseServiceHandle(handle)
	var status windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(handle, &status); err != nil {
		return fmt.Errorf("cannot query Service state")
	}
	if status.CurrentState != uint32(svc.Stopped) {
		return fmt.Errorf("Service/runtime must be stopped before private-key maintenance; stop it explicitly as Administrator")
	}
	return nil
}
