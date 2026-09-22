//go:build windows

package servicehost

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"ws-agent/internal/apppaths"
)

// Users can inspect the visible service, but cannot start/stop/reconfigure/delete.
const serviceSDDL = "O:BAG:BAD:(A;;GA;;;SY)(A;;GA;;;BA)(A;;CCLCSWLORC;;;BU)"

// Configure is development tooling, never invoked by the unattended runtime.
// The provisioning script establishes file ACLs and enrollment first.
func Configure() error {
	if err := RequireAdministrator(); err != nil {
		return err
	}
	paths, err := apppaths.Machine()
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	expected := filepath.Join(paths.Install, "thesis-agent.exe")
	if !strings.EqualFold(filepath.Clean(executable), expected) {
		return fmt.Errorf("configure-service must run from the protected installed executable")
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(Name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		service, err = manager.CreateService(Name, expected, mgr.Config{StartType: mgr.StartManual, ErrorControl: mgr.ErrorNormal, DisplayName: DisplayName, Description: Description, ServiceStartName: "LocalSystem"}, "--service")
	}
	if err != nil {
		return err
	}
	defer service.Close()
	state, err := service.Query()
	if err != nil {
		return err
	}
	if state.State != svc.Stopped {
		return fmt.Errorf("stop the development Service before configuring it")
	}
	cfg, err := service.Config()
	if err != nil {
		return err
	}
	command := syscall.EscapeArg(expected) + " --service"
	quotedCommand := "\"" + expected + "\" --service"
	if !strings.EqualFold(cfg.BinaryPathName, command) && !strings.EqualFold(cfg.BinaryPathName, quotedCommand) {
		return fmt.Errorf("existing Service uses a different executable; refusing to modify it")
	}
	sd, err := windows.SecurityDescriptorFromString(serviceSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	err = windows.SetSecurityInfo(service.Handle, windows.SE_SERVICE, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION, owner, nil, dacl, nil)
	runtime.KeepAlive(sd)
	if err != nil {
		return fmt.Errorf("apply Service security: %w", err)
	}
	if err = service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 30 * time.Second}, {Type: mgr.NoAction}}, 86400); err != nil {
		return err
	}
	if err = service.SetRecoveryActionsOnNonCrashFailures(false); err != nil {
		return err
	}
	cfg.StartType = mgr.StartAutomatic
	cfg.BinaryPathName = quotedCommand
	cfg.ServiceType = windows.SERVICE_WIN32_OWN_PROCESS
	cfg.DelayedAutoStart = false
	cfg.ServiceStartName = "LocalSystem"
	cfg.DisplayName = DisplayName
	cfg.Description = Description
	return service.UpdateConfig(cfg)
}
