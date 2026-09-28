//go:build !windows

package installedapps

import (
	"context"
	"errors"
)

func Collect(context.Context) ([]App, error) {
	return nil, errors.New("installed applications requires Windows")
}
