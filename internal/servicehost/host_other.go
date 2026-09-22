//go:build !windows

package servicehost

import "fmt"

func Configure() error {
	return fmt.Errorf("Windows Service provisioning is unavailable on this platform")
}

func IsService() (bool, error)    { return false, nil }
func RequireAdministrator() error { return fmt.Errorf("Service provisioning requires Windows") }
func Run(Runtime) error           { return fmt.Errorf("Windows Service hosting is unavailable on this platform") }
