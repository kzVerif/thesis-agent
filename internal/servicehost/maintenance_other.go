//go:build !windows

package servicehost

import "fmt"

func RequireStoppedForKeyMaintenance() error {
	return fmt.Errorf("private-key maintenance requires Windows")
}
