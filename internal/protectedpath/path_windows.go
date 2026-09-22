//go:build windows

// Package protectedpath validates the existing Service ACL profile without
// changing permissions. Machine DPAPI relies on this filesystem boundary.
package protectedpath

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

func ValidateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("protected runtime directory is missing")
	}
	if err := noReparseAncestors(path); err != nil {
		return err
	}
	return validateACL(path, true)
}

func ValidateFile(path string) error {
	if err := ValidateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("protected identity/backup must be a regular file")
	}
	if err := noReparseAncestors(path); err != nil {
		return err
	}
	return validateACL(path, false)
}

func noReparseAncestors(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("protected path must be absolute")
	}
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		name, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return fmt.Errorf("invalid protected path")
		}
		attributes, err := windows.GetFileAttributes(name)
		if err != nil {
			return fmt.Errorf("cannot inspect protected path")
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("protected path must not contain reparse points")
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}

func validateACL(path string, directory bool) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("cannot inspect identity directory/file ACL")
	}
	return validateDescriptor(sd, directory)
}

func validateDescriptor(sd *windows.SECURITY_DESCRIPTOR, directory bool) error {
	const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
	invalid := fmt.Errorf("private-key storage requires SYSTEM and Administrators only, with inherited full control for runtime children; inspect Service ACLs")
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trustedSID(owner) {
		return invalid
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return invalid
	}
	system, admins := false, false
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return invalid
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !trustedSID(sid) {
			return invalid
		}
		if ace.Header.AceFlags&0x08 != 0 {
			continue
		} // INHERIT_ONLY
		if uint32(ace.Mask)&fileAllAccess != fileAllAccess {
			continue
		}
		if directory && ace.Header.AceFlags&0x03 != 0x03 {
			continue
		} // OI + CI
		if sid.IsWellKnown(windows.WinLocalSystemSid) {
			system = true
		}
		if sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			admins = true
		}
	}
	if !system || !admins {
		return invalid
	}
	return nil
}

func trustedSID(sid *windows.SID) bool {
	return sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}
