//go:build !windows

package antivirus

import (
	"context"
	"fmt"
)

func Scan(ctx context.Context, c Command) (Report, error) {
	return Report{ExitCode: -1}, fmt.Errorf("virus scans require Microsoft Defender on Windows")
}
