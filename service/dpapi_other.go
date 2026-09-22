//go:build !windows

package service

import "errors"

func protectPrivateKey([]byte) ([]byte, error) {
	return nil, errors.New("Windows DPAPI is only available on Windows")
}

func unprotectPrivateKey([]byte) ([]byte, error) {
	return nil, errors.New("Windows DPAPI is only available on Windows")
}

func protectPrivateKeyForMachine([]byte) ([]byte, error) {
	return nil, errors.New("Windows DPAPI is only available on Windows")
}
