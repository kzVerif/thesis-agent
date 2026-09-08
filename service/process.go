package service

import (
	"fmt"

	"github.com/shirou/gopsutil/v4/process"
)

type ProcessInfo struct {
	PID  int32  `json:"pid"`
	Name string `json:"name"`
}

func GetProcessList() ([]ProcessInfo, error) {

	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	var processList []ProcessInfo

	for _, p := range processes {

		name, err := p.Name()
		if err != nil {
			// บาง process อาจอ่านไม่ได้ เช่น permission ไม่พอ
			continue
		}

		info := ProcessInfo{
			PID:  p.Pid,
			Name: name,
		}

		processList = append(
			processList,
			info,
		)
	}

	return processList, nil
}

func KillProcess(pid int32) error {
	if pid <= 0 {
		return fmt.Errorf("หมายเลข PID ต้องมีค่ามากกว่าศูนย์")
	}

	target, err := process.NewProcess(pid)
	if err != nil {
		return fmt.Errorf("ไม่พบโปรเซส PID %d: %w", pid, err)
	}
	if err := target.Kill(); err != nil {
		return fmt.Errorf("ไม่สามารถปิดโปรเซส PID %d ได้: %w", pid, err)
	}
	return nil
}
