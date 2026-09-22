//go:build !windows

package statefile

import "os"

func replace(source, target string) error { return os.Rename(source, target) }
