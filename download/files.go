package download

import (
	"context"
	"crypto/sha256"
	"io"
	"os"
)

func (m *Manager) removePart(path string) error {
	remove := func() error { return os.Remove(path) }
	if m.cfg.Boundary != nil {
		return m.cfg.Boundary.WithFiles([]string{path}, remove)
	}
	return remove()
}

func (m *Manager) publish(part, dest string) error {
	publish := func() error { return finalizeTemporary(part, dest) }
	if m.cfg.Boundary != nil {
		return m.cfg.Boundary.WithFiles([]string{part, dest}, publish)
	}
	return publish()
}

func (m *Manager) fileSHA256(ctx context.Context, path string) ([]byte, error) {
	if m.cfg.Boundary == nil {
		return fileSHA256Context(ctx, path)
	}
	f, err := m.cfg.Boundary.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, contextReader{ctx, f}); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
