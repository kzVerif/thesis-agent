//go:build !windows && !unix

package runlock

import "fmt"

func Acquire(string) (func(), error) {
	return nil, fmt.Errorf("runtime locking is unsupported on this platform")
}
