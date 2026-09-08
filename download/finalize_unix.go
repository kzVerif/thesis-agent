//go:build !windows

package download

import "os"

func finalizeTemporary(source, destination string) error { return os.Rename(source, destination) }
