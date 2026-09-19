package service

import (
	"os"
	"runtime"

	"ws-agent/model"
)

func GetSystemInfo() model.SystemInfo {
	info := model.SystemInfo{
		OSInfo: getOSInfo(),
	}

	if id, err := GetOrCreateAgentID(); err == nil {
		info.ID = id
	}
	if hostname, err := os.Hostname(); err == nil {
		info.Hostname = hostname
	}
	info.IPAddress, info.MACAddress = getPrimaryNetwork()

	return info
}

func getOSInfo() model.OSInfo {
	return model.OSInfo{
		Name:    runtime.GOOS,
		Edition: runtime.GOARCH,
		Version: getOSVersion(),
	}
}
