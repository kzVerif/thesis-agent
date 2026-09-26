//go:build windows

package protectedpath

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
	"ws-agent/internal/apppaths"

	"golang.org/x/sys/windows"
)

const fullControl = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff

// classifyDescriptor is read-only. Unknown ACE types (including deny/callback/
// object ACEs) are ambiguous, not permission to reset a DACL. A null/absent DACL
// grants unrestricted access and is never treated as a missing trusted ACE.
func classifyDescriptor(sd *windows.SECURITY_DESCRIPTOR, directory, root bool) assessment {
	bad := unsafeAssessment("security_descriptor_unreadable")
	if sd == nil || !sd.IsValid() {
		return bad
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return bad
	}
	bad.owner = owner.String()
	if !trustedSID(owner) {
		bad.reasons = []string{"untrusted_owner"}
		return bad
	}
	control, _, err := sd.Control()
	if err != nil {
		return bad
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil || control&windows.SE_DACL_PRESENT == 0 {
		return bad
	}
	a := assessment{class: canonical, owner: owner.String()}
	system, admins, flagsOK := false, false, true
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceSize < 16 {
			return bad
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsValid() || uint32(sid.Len())+8 > uint32(ace.Header.AceSize) {
			return bad
		}
		if !trustedSID(sid) {
			bad.reasons = []string{"untrusted_ace"}
			bad.principal = sid.String()
			bad.mask = uint32(ace.Mask)
			return bad // Conservatively reject even inherit-only/zero-mask foreign ACEs.
		}
		want := byte(0)
		if directory {
			want = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
		}
		flags := ace.Header.AceFlags &^ windows.INHERITED_ACE
		if flags != want || root && ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			flagsOK = false
		}
		if flags != want || ace.Mask != fullControl {
			continue
		}
		if sid.IsWellKnown(windows.WinLocalSystemSid) {
			system = true
		}
		if sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			admins = true
		}
	}
	if root && control&windows.SE_DACL_PROTECTED == 0 || !flagsOK {
		a.reasons = append(a.reasons, "inheritance_drift")
	}
	if !system {
		a.reasons = append(a.reasons, "missing_system_full_control")
	}
	if !admins {
		a.reasons = append(a.reasons, "missing_admin_full_control")
	}
	if dacl.AceCount != 2 {
		a.reasons = append(a.reasons, "noncanonical_trusted_aces")
	}
	if len(a.reasons) != 0 {
		a.class = repairable
	} else {
		a.reasons = []string{"acl_canonical"}
	}
	return a
}

// EnsureRuntimeSecurity only accepts the exact Known Folder machine paths.
// On success the caller owns the existing runtime byte lock until release.
// No identity/config/enrollment content is read or written by this package.
func EnsureRuntimeSecurity(ctx context.Context, paths apppaths.Paths, emit Reporter) (func(), error) {
	expected, err := apppaths.Machine()
	if err != nil {
		report(emit, paths.Root, unsafeAssessment("path_resolution_failed"), "fail_closed")
		return nil, err
	}
	if !samePaths(paths, expected) {
		report(emit, paths.Root, unsafeAssessment("path_escape"), "fail_closed")
		return nil, securityError(paths.Root, "path_escape")
	}
	return ensureStartup(ctx, paths, emit)
}

func samePaths(a, b apppaths.Paths) bool {
	left := []string{a.Install, a.Root, a.Identity, a.Enrollment, a.Config, a.Log, a.Downloads}
	right := []string{b.Install, b.Root, b.Identity, b.Enrollment, b.Config, b.Log, b.Downloads}
	for i := range left {
		if !strings.EqualFold(left[i], right[i]) {
			return false
		}
	}
	return true
}

type pinnedObject struct {
	file            *os.File
	path            string
	directory, root bool
	parent          *pinnedObject
	scope           string
}

func (o *pinnedObject) handle() windows.Handle { return windows.Handle(o.file.Fd()) }

func (o *pinnedObject) inspect() assessment {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(o.handle(), &info) != nil {
		return unsafeAssessment("security_descriptor_unreadable")
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return unsafeAssessment("reparse_point_detected")
	}
	if (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != o.directory {
		return unsafeAssessment("unexpected_object_type")
	}
	if !o.directory && info.NumberOfLinks != 1 {
		return unsafeAssessment("hard_link_detected")
	}
	name, err := finalPath(o.handle())
	if err != nil || !strings.EqualFold(name, o.path) {
		return unsafeAssessment("path_escape")
	}
	sd, err := windows.GetSecurityInfo(o.handle(), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return unsafeAssessment("security_descriptor_unreadable")
	}
	return classifyDescriptor(sd, o.directory, o.root)
}

func finalPath(h windows.Handle) (string, error) {
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || n >= uint32(len(buf)) {
		return "", fmt.Errorf("cannot resolve pinned path")
	}
	return strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`), nil
}

func openPinned(path string, access uint32, disposition uint32, sa *windows.SecurityAttributes) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, sa, disposition,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

// Pin ancestors top-down without DELETE sharing before opening descendants.
// They are never repaired. Their reparse/type/final-path checks prevent traversal
// through a redirected ancestor; runtime objects enforce the trusted-only ACL.
func pinAncestors(root string) ([]*os.File, error) {
	if !filepath.IsAbs(root) || strings.HasPrefix(root, `\\`) {
		return nil, securityError(root, "path_escape")
	}
	var paths []string
	for p := filepath.Dir(root); ; p = filepath.Dir(p) {
		paths = append(paths, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	var pins []*os.File
	fail := func(path, reason string) ([]*os.File, error) {
		for _, f := range pins {
			f.Close()
		}
		return nil, securityError(path, reason)
	}
	for i := len(paths) - 1; i >= 0; i-- {
		p := paths[i]
		f, err := openPinned(p, windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING, nil)
		if err != nil {
			return fail(p, "security_descriptor_unreadable")
		}
		pins = append(pins, f)
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info) != nil {
			return fail(p, "security_descriptor_unreadable")
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fail(p, "reparse_point_detected")
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return fail(p, "unexpected_object_type")
		}
		resolved, err := finalPath(windows.Handle(f.Fd()))
		if err != nil || !strings.EqualFold(resolved, p) {
			return fail(p, "path_escape")
		}
	}
	return pins, nil
}

func canonicalDACL(directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	flags := ""
	if directory {
		flags = "OICI"
	}
	return windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)")
}

func (o *pinnedObject) repair() error {
	sd, err := canonicalDACL(o.directory)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	// MAXIMUM_ALLOWED handles suppress automatic propagation into children.
	// Only selected startup objects are inspected/repaired through their handles;
	// historical dynamic children must remain untouched.
	// https://learn.microsoft.com/windows/win32/api/aclapi/nf-aclapi-setsecurityinfo
	err = windows.SetSecurityInfo(o.handle(), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	runtime.KeepAlive(sd)
	return err
}

func ensureStartup(ctx context.Context, paths apppaths.Paths, emit Reporter) (release func(), result error) {
	root := paths.Root
	baseEmit := emit
	emit = func(d Diagnostic) {
		if d.Scope == "" {
			d.Scope = "critical"
		}
		if baseEmit != nil {
			baseEmit(d)
		}
	}
	ancestors, err := pinAncestors(root)
	if err != nil {
		// The returned error contains the exact ancestor/reason, never file contents.
		var security *SecurityError
		if errors.As(err, &security) {
			report(emit, security.Path, unsafeAssessment(security.Reason), "fail_closed")
		}
		report(emit, root, unsafeAssessment("unsafe_acl_fail_closed"), "fail_closed")
		return nil, err
	}
	var objects []*pinnedObject
	var lock *pinnedObject
	locked := false
	cleanup := func() {
		if locked {
			_ = windows.UnlockFileEx(lock.handle(), 0, 1, 0, &windows.Overlapped{})
		}
		for i := len(objects) - 1; i >= 0; i-- {
			_ = objects[i].file.Close()
		}
		for i := len(ancestors) - 1; i >= 0; i-- {
			_ = ancestors[i].Close()
		}
	}
	defer func() {
		if result != nil {
			report(emit, root, unsafeAssessment("unsafe_acl_fail_closed"), "fail_closed")
			cleanup()
		}
	}()
	fail := func(path string, a assessment) error {
		report(emit, path, a, "fail_closed")
		return securityError(path, a.reasons[0])
	}
	// Open only the named critical objects and container directories. No ReadDir,
	// Walk, descendant count, or recursive repair is part of this startup gate.
	byPath := make(map[string]*pinnedObject)
	if err := inspectStartupScope(ctx, paths, func(target startupTarget) error {
		path := target.path
		failTarget := func(reason string) error {
			report(scopedReporter(emit, target.scope), path, unsafeAssessment(reason), "fail_closed")
			return securityError(path, reason)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, `..\`) || filepath.IsAbs(rel) {
			return failTarget("path_escape")
		}
		f, err := openPinned(path, windows.MAXIMUM_ALLOWED, windows.OPEN_EXISTING, nil)
		if err != nil {
			if !target.required && (errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)) {
				return nil
			}
			return failTarget("security_descriptor_unreadable")
		}
		o := &pinnedObject{file: f, path: path, directory: target.directory, root: target.required, scope: target.scope}
		if !o.root {
			o.parent = byPath[strings.ToLower(filepath.Dir(path))]
		}
		objects = append(objects, o)
		a := o.inspect()
		if a.class == unsafeState {
			report(scopedReporter(emit, target.scope), path, a, "fail_closed")
			return securityError(path, a.reasons[0])
		}
		report(scopedReporter(emit, target.scope), path, a, "inspect")
		byPath[strings.ToLower(path)] = o
		if strings.EqualFold(path, paths.RuntimeLock()) {
			lock = o
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if lock == nil {
		// The only new object permitted is the existing runtime coordination file.
		// Explicit ACL at CREATE_NEW avoids inheriting a drifted root DACL.
		sd, err := windows.SecurityDescriptorFromString("O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)")
		if err != nil {
			return nil, err
		}
		sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
		f, err := openPinned(paths.RuntimeLock(), windows.MAXIMUM_ALLOWED, windows.CREATE_NEW, sa)
		runtime.KeepAlive(sd)
		if err != nil {
			return nil, fail(root, unsafeAssessment("runtime_lock_failed"))
		}
		lock = &pinnedObject{file: f, path: paths.RuntimeLock(), parent: objects[0], scope: "critical"}
		objects = append(objects, lock)
	}
	if a := lock.inspect(); a.class == unsafeState {
		return nil, fail(lock.path, a)
	}
	if err := windows.LockFileEx(lock.handle(), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}); err != nil {
		return nil, fail(lock.path, unsafeAssessment("runtime_lock_failed"))
	}
	locked = true
	if err := ensureObjects(ctx, objects, emit); err != nil {
		return nil, err
	}
	report(emit, root, objects[0].inspect(), "verified")
	// Retain only the lock after startup security. Normal atomic state writes and
	// log rotation need to rename files. Privileged writers remain a trust boundary.
	for _, o := range objects {
		if o != lock {
			o.file.Close()
		}
	}
	objects = []*pinnedObject{lock}
	return cleanup, nil
}

// Separate engine permits fault injection without touching real machine state.
type securityObject interface {
	inspect() assessment
	repair() error
}

func ensureObject(ctx context.Context, path string, o securityObject, emit Reporter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a := o.inspect() // immediately before mutation, on the same pinned handle
	if a.class == unsafeState {
		report(emit, path, a, "fail_closed")
		return securityError(path, a.reasons[0])
	}
	if a.class == canonical {
		return nil
	}
	report(emit, path, a, "self_repair")
	report(emit, path, assessment{class: repairable, reasons: []string{"repair_started"}}, "self_repair")
	if err := o.repair(); err != nil {
		report(emit, path, unsafeAssessment("repair_failed"), "fail_closed")
		return securityError(path, "repair_failed")
	}
	a = o.inspect()
	if a.class != canonical {
		report(emit, path, a, "fail_closed")
		report(emit, path, unsafeAssessment("post_repair_validation_failed"), "fail_closed")
		return securityError(path, "post_repair_validation_failed")
	}
	report(emit, path, assessment{class: canonical, owner: a.owner, reasons: []string{"repair_success"}}, "verified")
	return nil
}

func ensureObjects(ctx context.Context, objects []*pinnedObject, emit Reporter) error {
	// Repeat preflight under the lock before any mutation.
	for _, o := range objects {
		if a := o.inspect(); a.class == unsafeState {
			report(scopedReporter(emit, o.scope), o.path, a, "fail_closed")
			return securityError(o.path, a.reasons[0])
		}
	}
	for _, o := range objects {
		if o.parent != nil {
			if a := o.parent.inspect(); a.class != canonical {
				report(scopedReporter(emit, o.parent.scope), o.parent.path, a, "fail_closed")
				report(scopedReporter(emit, o.parent.scope), o.parent.path, unsafeAssessment("parent_validation_failed"), "fail_closed")
				return securityError(o.parent.path, "parent_validation_failed")
			}
		}
		if err := ensureObject(ctx, o.path, o, scopedReporter(emit, o.scope)); err != nil {
			return err
		}
	}
	for _, o := range objects {
		if a := o.inspect(); a.class != canonical {
			report(scopedReporter(emit, o.scope), o.path, a, "fail_closed")
			report(scopedReporter(emit, o.scope), o.path, unsafeAssessment("post_repair_validation_failed"), "fail_closed")
			return securityError(o.path, "post_repair_validation_failed")
		}
	}
	return nil
}
