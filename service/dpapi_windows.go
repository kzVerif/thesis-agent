//go:build windows

package service

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const cryptProtectUIForbidden = 0x1

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
	return dpapiTransform(cryptProtectData, data)
}

func unprotectPrivateKey(data []byte) ([]byte, error) {
	return dpapiTransform(cryptUnprotectData, data)
}

func dpapiTransform(proc *windows.LazyProc, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot protect empty data")
	}
	in := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var out dataBlob
	ret, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("DPAPI call failed: %w", callErr)
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	result := make([]byte, out.cbData)
	copy(result, unsafe.Slice(out.pbData, out.cbData))
	return result, nil
}
