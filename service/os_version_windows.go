//go:build windows

package service

import (
	"fmt"
	"syscall"
	"unsafe"
)

type osVersionInfo struct {
	size        uint32
	major       uint32
	minor       uint32
	build       uint32
	platformID  uint32
	servicePack [128]uint16
}

func getOSVersion() string {
	info := osVersionInfo{size: uint32(unsafe.Sizeof(osVersionInfo{}))}
	proc := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion")
	status, _, _ := proc.Call(uintptr(unsafe.Pointer(&info)))
	if status != 0 {
		return "unknown"
	}
	if info.major == 10 && info.build >= 22000 {
		return "11"
	}
	return fmt.Sprintf("%d.%d", info.major, info.minor)
}
