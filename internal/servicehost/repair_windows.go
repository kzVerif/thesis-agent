//go:build windows

package servicehost

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"ws-agent/internal/apppaths"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// RequireStoppedOwnedService only queries SCM. Repair never stops, starts,
// registers or changes a service and refuses an absent/unrelated registration.
func RequireStoppedOwnedService() error {
	paths, err := apppaths.Machine()
	if err != nil {
		return err
	}
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("cannot query SCM before ACL repair")
	}
	defer windows.CloseServiceHandle(manager)
	name, _ := windows.UTF16PtrFromString(Name)
	h, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return fmt.Errorf("ACL repair requires the installed, owned Service")
	}
	defer windows.CloseServiceHandle(h)
	s := &mgr.Service{Name: Name, Handle: h}
	cfg, err := s.Config()
	if err != nil {
		return err
	}
	executable := filepath.Join(paths.Install, "thesis-agent.exe")
	if !ownedRepairConfig(cfg, executable) {
		return fmt.Errorf("Service executable/account/type does not match the owned LocalSystem Service")
	}
	status, err := s.Query()
	if err != nil {
		return err
	}
	if status.State != svc.Stopped {
		return fmt.Errorf("ACL repair requires a stopped Service; stop the owned Service explicitly as Administrator first")
	}
	return nil
}

func ownedRepairConfig(cfg mgr.Config, executable string) bool {
	return (strings.EqualFold(cfg.BinaryPathName, `"`+executable+`" --service`) || strings.EqualFold(cfg.BinaryPathName, syscall.EscapeArg(executable)+" --service")) &&
		strings.EqualFold(cfg.ServiceStartName, "LocalSystem") && cfg.ServiceType == windows.SERVICE_WIN32_OWN_PROCESS
}
