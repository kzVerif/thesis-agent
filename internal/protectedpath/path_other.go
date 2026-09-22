//go:build !windows

package protectedpath

import "fmt"

func ValidateDirectory(string) error {
	return fmt.Errorf("protected Service key storage requires Windows")
}
func ValidateFile(string) error { return fmt.Errorf("protected Service key storage requires Windows") }
