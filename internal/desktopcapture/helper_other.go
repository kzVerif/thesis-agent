//go:build !windows

package desktopcapture

import "fmt"

func Run() error {
	return fmt.Errorf("desktop capture helper is only supported on Windows")
}
