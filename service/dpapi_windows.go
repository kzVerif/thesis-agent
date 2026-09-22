//go:build windows

package service

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cryptProtectUIForbidden  = 0x1
	cryptProtectLocalMachine = 0x4
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32            = windows.NewLazySystemDLL("crypt32.dll")
	cryptProtectData   = crypt32.NewProc("CryptProtectData")
	cryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	localFree          = kernel32.NewProc("LocalFree")
)

func protectPrivateKey(data []byte) ([]byte, error) {
	return dpapiTransform(cryptProtectData, cryptProtectUIForbidden, data)
}

func protectPrivateKeyForMachine(data []byte) ([]byte, error) {
	return dpapiTransform(cryptProtectData, cryptProtectUIForbidden|cryptProtectLocalMachine, data)
}

func unprotectPrivateKey(data []byte) ([]byte, error) {
	// Scope is selected when protecting, not through unprotect flags.
	return dpapiTransform(cryptUnprotectData, cryptProtectUIForbidden, data)
}

func dpapiTransform(proc *windows.LazyProc, flags uint32, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot protect empty data")
	}
	in := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var out dataBlob
	ret, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0,
		uintptr(flags),
		uintptr(unsafe.Pointer(&out)),
	)
	runtime.KeepAlive(data)
	if ret == 0 {
		return nil, fmt.Errorf("DPAPI call failed: %w", callErr)
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	defer clear(unsafe.Slice(out.pbData, out.cbData))
	result := make([]byte, out.cbData)
	copy(result, unsafe.Slice(out.pbData, out.cbData))
	return result, nil
}
