//go:build !windows

package servicehost

import "fmt"

func RequireStoppedOwnedService() error { return fmt.Errorf("ACL repair requires Windows") }
