//go:build !windows

package protectedpath

import (
	"context"
	"fmt"
	"ws-agent/internal/apppaths"
)

func EnsureRuntimeSecurity(context.Context, apppaths.Paths, Reporter) (func(), error) {
	return nil, fmt.Errorf("runtime ACL security requires Windows")
}
