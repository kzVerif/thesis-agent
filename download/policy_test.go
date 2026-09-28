package download

import (
	"path/filepath"
	"testing"
)

func TestDownloadRootPolicy(t *testing.T) {
	base := t.TempDir()
	runtimeRoot := filepath.Join(base, "runtime")
	installRoot := filepath.Join(base, "installed")
	first, second := filepath.Join(base, "downloads"), filepath.Join(base, "lessons")
	roots, err := ValidateRoots(first, []string{first, second}, runtimeRoot, installRoot)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: Config{Directory: first, AllowedRoots: roots}}
	for _, path := range []string{"", first, filepath.Join(second, "Room101")} {
		if _, err := m.destinationDirectory(path); err != nil {
			t.Fatalf("allowed %q: %v", path, err)
		}
	}
	for _, path := range []string{first + "-sibling", runtimeRoot, filepath.Join(second, "..", "elsewhere"), "relative"} {
		if _, err := m.destinationDirectory(path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	for _, root := range []string{base, runtimeRoot, filepath.Join(runtimeRoot, "logs"), filepath.Join(runtimeRoot, "data"), installRoot, filepath.Join(installRoot, "child"), "relative", ""} {
		if _, err := ValidateRoots(root, []string{root}, runtimeRoot, installRoot); err == nil {
			t.Fatalf("unsafe root %q", root)
		}
	}
	if _, err := ValidateRoots(first, []string{second}, runtimeRoot, installRoot); err == nil {
		t.Fatal("default outside allowlist")
	}
	if _, err := ValidateRoots(first, nil, runtimeRoot, installRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRoots(filepath.Join(runtimeRoot, "data", "downloads"), nil, runtimeRoot, installRoot); err != nil {
		t.Fatal(err)
	}
}
