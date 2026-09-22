package statefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicPublicationAndExclusiveRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	original := []byte("{\"test\":\"original\"}")
	next := []byte("{\"test\":\"next\"}")
	if err := Write(path, original, true); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, next, true); err == nil {
		t.Fatal("exclusive write replaced existing state")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatal("exclusive failure changed original")
	}
	if err := Write(path, next, false); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil || string(got) != string(next) {
		t.Fatal("replacement incomplete")
	}
	tmp, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".state-*.tmp"))
	if err != nil || len(tmp) != 0 {
		t.Fatal("temporary state file leaked")
	}
}
