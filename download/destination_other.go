//go:build !windows

package download

import "os"

func isDestinationLink(info os.FileInfo) bool { return info.Mode()&os.ModeSymlink != 0 }
