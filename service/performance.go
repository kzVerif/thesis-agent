package service

import (
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

type PerformanceInfo struct {
	CPUUsage float64 `json:"cpu_usage"`

	RAMTotalGB float64 `json:"ram_total_gb"`
	RAMUsedGB  float64 `json:"ram_used_gb"`
	RAMUsage   float64 `json:"ram_usage"`

	DiskTotalGB float64 `json:"disk_total_gb"`
	DiskUsedGB  float64 `json:"disk_used_gb"`
	DiskFreeGB  float64 `json:"disk_free_gb"`
	DiskUsage   float64 `json:"disk_usage"`
}

func GetPerformanceInfo() (PerformanceInfo, error) {
	var info PerformanceInfo

	// =========================
	// CPU
	// =========================

	cpuPercent, err := cpu.Percent(
		1*time.Second,
		false,
	)

	if err != nil {
		return info, err
	}

	if len(cpuPercent) > 0 {
		info.CPUUsage = cpuPercent[0]
	}

	// =========================
	// RAM
	// =========================

	ram, err := mem.VirtualMemory()
	if err != nil {
		return info, err
	}

	info.RAMTotalGB = bytesToGB(ram.Total)
	info.RAMUsedGB = bytesToGB(ram.Used)
	info.RAMUsage = ram.UsedPercent

	// =========================
	// DISK
	// =========================

	diskPath := "/"

	if runtime.GOOS == "windows" {
		diskPath = "C:\\"
	}

	diskInfo, err := disk.Usage(diskPath)
	if err != nil {
		return info, err
	}

	info.DiskTotalGB = bytesToGB(diskInfo.Total)
	info.DiskUsedGB = bytesToGB(diskInfo.Used)
	info.DiskFreeGB = bytesToGB(diskInfo.Free)
	info.DiskUsage = diskInfo.UsedPercent

	return info, nil
}

func bytesToGB(bytes uint64) float64 {
	return float64(bytes) / 1024 / 1024 / 1024
}
