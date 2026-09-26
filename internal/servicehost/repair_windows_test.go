//go:build windows

package servicehost

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
	"testing"
)

func TestRepairServiceOwnership(t *testing.T) {
	executable := `D:\Program Files\ThesisAgentDev\thesis-agent.exe`
	good := mgr.Config{BinaryPathName: `"` + executable + `" --service`, ServiceStartName: "LocalSystem", ServiceType: windows.SERVICE_WIN32_OWN_PROCESS}
	if !ownedRepairConfig(good, executable) {
		t.Fatal("owned service refused")
	}
	for _, change := range []func(*mgr.Config){
		func(c *mgr.Config) { c.BinaryPathName = `"D:\other.exe" --service` },
		func(c *mgr.Config) { c.BinaryPathName += ` --provision` },
		func(c *mgr.Config) { c.ServiceStartName = "LabUser" },
		func(c *mgr.Config) { c.ServiceType = windows.SERVICE_WIN32_SHARE_PROCESS },
	} {
		cfg := good
		change(&cfg)
		if ownedRepairConfig(cfg, executable) {
			t.Fatal("unowned service accepted")
		}
	}
}
