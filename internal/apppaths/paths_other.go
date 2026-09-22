//go:build !windows

package apppaths

import "fmt"

func Machine() (Paths, error) { return Paths{}, fmt.Errorf("machine Service paths require Windows") }
