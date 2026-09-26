package apppaths

import (
	"fmt"
	"path/filepath"
)

const Name = "ThesisAgentDev"

// LegacyPrivateKeyBackupSuffix is shared by migration and startup ACL checks.
const LegacyPrivateKeyBackupSuffix = ".dpapi-user-v1.bak"

func (p Paths) IdentityBackup() string { return p.Identity + LegacyPrivateKeyBackupSuffix }
func (p Paths) RuntimeLock() string    { return filepath.Join(p.Root, ".runtime.lock") }

type Paths struct {
	Install    string `json:"install"`
	Root       string `json:"root"`
	Identity   string `json:"identity"`
	Enrollment string `json:"enrollment"`
	Config     string `json:"config"`
	Log        string `json:"log"`
	Downloads  string `json:"downloads"`
}

func fromRoot(root, install string) Paths {
	return Paths{Install: install, Root: root, Identity: filepath.Join(root, "agent_config.json"),
		Enrollment: filepath.Join(root, "enrollment_state.json"), Config: filepath.Join(root, ".env"),
		Log: filepath.Join(root, "logs", "agent.log"), Downloads: filepath.Join(root, "data", "downloads")}
}

func Console(cwd string) (Paths, error) {
	if !filepath.IsAbs(cwd) {
		return Paths{}, fmt.Errorf("console working directory must be absolute")
	}
	return fromRoot(filepath.Clean(cwd), ""), nil
}

// MachineFromRoots is pure: service paths never depend on the working directory.
func MachineFromRoots(programData, programFiles string) (Paths, error) {
	if !filepath.IsAbs(programData) || !filepath.IsAbs(programFiles) {
		return Paths{}, fmt.Errorf("machine directory roots must be absolute")
	}
	return fromRoot(filepath.Join(programData, Name), filepath.Join(programFiles, Name)), nil
}
