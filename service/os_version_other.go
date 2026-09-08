//go:build !windows

package service

import "runtime"

func getOSVersion() string {
	return runtime.GOOS
}
