//go:build !windows

package protectedpath

import (
	"fmt"
	"os"
)

func (b *Boundary) EnsureDirectory(string) error {
	return fmt.Errorf("protected dynamic I/O requires Windows")
}
func (b *Boundary) OpenFile(string, int, os.FileMode) (*os.File, error) {
	return nil, fmt.Errorf("protected dynamic I/O requires Windows")
}
func (b *Boundary) CreateTemp(string, string) (*os.File, error) {
	return nil, fmt.Errorf("protected dynamic I/O requires Windows")
}
func (b *Boundary) WithFiles([]string, func() error) error {
	return fmt.Errorf("protected dynamic I/O requires Windows")
}
