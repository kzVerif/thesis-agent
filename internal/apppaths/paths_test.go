package apppaths

import (
	"path/filepath"
	"testing"
)

func TestMachinePathsDoNotDependOnCWD(t *testing.T) {
	data, programs := t.TempDir(), t.TempDir()
	expected, err := MachineFromRoots(data, programs)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	actual, err := MachineFromRoots(data, programs)
	if err != nil || actual != expected {
		t.Fatalf("paths changed: %v", err)
	}
	if expected.Identity != filepath.Join(data, Name, "agent_config.json") {
		t.Fatal("unexpected identity location")
	}
	if _, err := MachineFromRoots("relative", programs); err == nil {
		t.Fatal("relative machine root accepted")
	}
}

func TestConsoleCompatibility(t *testing.T) {
	cwd := t.TempDir()
	paths, err := Console(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Config != filepath.Join(cwd, ".env") || paths.Downloads != filepath.Join(cwd, "data", "downloads") {
		t.Fatal(paths)
	}
}
