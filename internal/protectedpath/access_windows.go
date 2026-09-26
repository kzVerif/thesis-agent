//go:build windows

package protectedpath

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
)

func (b *Boundary) reject(path, reason string) error {
	report(scopedReporter(b.Report, "on_access"), path, unsafeAssessment(reason), "fail_closed")
	return securityError(path, reason)
}

func (b *Boundary) inside(path string) error {
	if !filepath.IsAbs(b.Root) || !filepath.IsAbs(path) {
		return b.reject(path, "path_escape")
	}
	rel, err := filepath.Rel(b.Root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, `..\`) || filepath.IsAbs(rel) || strings.Contains(rel, ":") {
		return b.reject(path, "path_escape")
	}
	for _, part := range strings.Split(rel, `\`) {
		if part != "." && (strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ")) {
			return b.reject(path, "path_escape")
		}
	}
	return nil
}

func (b *Boundary) validate(o *pinnedObject) error {
	a := o.inspect()
	if a.class != canonical {
		report(scopedReporter(b.Report, "on_access"), o.path, a, "fail_closed")
		return securityError(o.path, a.reasons[0])
	}
	return nil
}

func closePins(pins []*os.File) {
	for i := len(pins) - 1; i >= 0; i-- {
		_ = pins[i].Close()
	}
}

// pinDirectories checks only the ancestry of the requested object, never siblings
// or descendants. Missing children may be created under an already checked parent.
func (b *Boundary) pinDirectories(directory string, create bool) (pins []*os.File, result error) {
	if err := b.inside(directory); err != nil {
		return nil, err
	}
	ancestors, err := pinAncestors(b.Root)
	if err != nil {
		var e *SecurityError
		if errors.As(err, &e) {
			return nil, b.reject(e.Path, e.Reason)
		}
		return nil, err
	}
	pins = ancestors
	defer func() {
		if result != nil {
			closePins(pins)
			pins = nil
		}
	}()
	rel, _ := filepath.Rel(b.Root, directory)
	paths := []string{b.Root}
	if rel != "." {
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			paths = append(paths, filepath.Join(paths[len(paths)-1], part))
		}
	}
	for i, path := range paths {
		f, err := openPinned(path, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING, nil)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			if create && i > 0 {
				if err = os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
					return pins, b.reject(path, "security_descriptor_unreadable")
				}
				f, err = openPinned(path, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING, nil)
			}
		}
		if err != nil {
			return pins, b.reject(path, "security_descriptor_unreadable")
		}
		pins = append(pins, f)
		if err = b.validate(&pinnedObject{file: f, path: path, directory: true, root: i == 0}); err != nil {
			return pins, err
		}
	}
	return pins, nil
}

func (b *Boundary) EnsureDirectory(path string) error {
	pins, err := b.pinDirectories(path, true)
	if err != nil {
		return err
	}
	defer closePins(pins)
	return nil
}

// OpenFile never truncates an existing object before validating its handle.
// OPEN_REPARSE_POINT and no DELETE sharing bind validation and use to one object.
func (b *Boundary) OpenFile(path string, flag int, mode os.FileMode) (*os.File, error) {
	if err := b.inside(path); err != nil {
		return nil, err
	}
	if flag & ^(os.O_RDONLY|os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_EXCL) != 0 {
		return nil, b.reject(path, "unsupported_open_mode")
	}
	pins, err := b.pinDirectories(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer closePins(pins)
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)
	if flag&os.O_WRONLY == 0 {
		access |= windows.GENERIC_READ
	}
	if flag&os.O_APPEND != 0 {
		access |= windows.FILE_APPEND_DATA
	} else if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		access |= windows.GENERIC_WRITE
	}
	disposition := uint32(windows.OPEN_EXISTING)
	if flag&os.O_CREATE != 0 {
		disposition = windows.OPEN_ALWAYS
		if flag&os.O_EXCL != 0 {
			disposition = windows.CREATE_NEW
		}
	}
	f, err := openPinned(path, access, disposition, nil)
	if err != nil {
		return nil, b.reject(path, "security_descriptor_unreadable")
	}
	if err = b.validate(&pinnedObject{file: f, path: path}); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (b *Boundary) CreateTemp(directory, pattern string) (*os.File, error) {
	pins, err := b.pinDirectories(directory, false)
	if err != nil {
		return nil, err
	}
	defer closePins(pins)
	// os.CreateTemp uses exclusive creation. It cannot open a historical .part or
	// follow an existing link; the checked parent stays pinned during creation.
	f, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return nil, err
	}
	if err = b.validate(&pinnedObject{file: f, path: f.Name()}); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// WithFiles validates exactly the existing objects about to be renamed/deleted,
// allowing missing targets. DELETE sharing permits the caller's metadata operation;
// checked containers remain pinned. Privileged writers must honor runtime locking.
func (b *Boundary) WithFiles(paths []string, action func() error) error {
	var pins []*os.File
	defer func() { closePins(pins) }()
	for _, path := range paths {
		if err := b.inside(path); err != nil {
			return err
		}
		parents, err := b.pinDirectories(filepath.Dir(path), false)
		if err != nil {
			return err
		}
		pins = append(pins, parents...)
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return b.reject(path, "path_escape")
		}
		h, err := windows.CreateFile(name, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			continue
		}
		if err != nil {
			return b.reject(path, "security_descriptor_unreadable")
		}
		f := os.NewFile(uintptr(h), path)
		pins = append(pins, f)
		if err = b.validate(&pinnedObject{file: f, path: path}); err != nil {
			return err
		}
	}
	return action()
}
