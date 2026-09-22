// Package statefile publishes complete files on the same volume as their target.
package statefile

import (
	"os"
	"path/filepath"
)

// Write never exposes partially written JSON. exclusive refuses existing targets.
// The caller must establish protected directory ACLs before writing sensitive data.
func Write(path string, data []byte, exclusive bool) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if exclusive {
		return os.Link(tmp, path)
	}
	return replace(tmp, path)
}
