//go:build windows

package apppaths

import "golang.org/x/sys/windows"

func Machine() (Paths, error) {
	data, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return Paths{}, err
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return Paths{}, err
	}
	return MachineFromRoots(data, programs)
}
