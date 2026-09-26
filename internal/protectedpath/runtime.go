package protectedpath

import (
	"context"
	"fmt"
	"path/filepath"
	"ws-agent/internal/apppaths"
)

// Diagnostic contains filesystem metadata only, never runtime file contents.
type Diagnostic struct {
	Path, Classification, Reason, Action, Owner, Principal string
	Mask                                                   uint32
	Scope                                                  string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("security: runtime ACL scope=%s path=%q classification=%s reason=%s action=%s owner=%q principal=%q mask=0x%08x",
		d.Scope, d.Path, d.Classification, d.Reason, d.Action, d.Owner, d.Principal, d.Mask)
}

type Reporter func(Diagnostic)

type startupTarget struct {
	path                string
	directory, required bool
	scope               string
}

// This fixed plan intentionally has no directory-enumeration dependency.
// Missing containers are created and validated at their use boundary.
func inspectStartupScope(ctx context.Context, paths apppaths.Paths, visit func(startupTarget) error) error {
	targets := []startupTarget{
		{paths.Root, true, true, "critical"},
		{paths.Identity, false, false, "critical"},
		{paths.Enrollment, false, false, "critical"},
		{paths.Config, false, false, "critical"},
		{paths.RuntimeLock(), false, false, "critical"},
		{paths.IdentityBackup(), false, false, "critical"},
		{filepath.Dir(paths.Log), true, false, "container"},
		{filepath.Dir(paths.Downloads), true, false, "container"},
		{paths.Downloads, true, false, "container"},
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(target); err != nil {
			return err
		}
	}
	return nil
}

func scopedReporter(emit Reporter, scope string) Reporter {
	return func(d Diagnostic) {
		if emit != nil {
			d.Scope = scope
			emit(d)
		}
	}
}

type assessment struct {
	class, owner, principal string
	mask                    uint32
	reasons                 []string
}

const (
	canonical   = "canonical"
	repairable  = "repairable"
	unsafeState = "unsafe"
)

func unsafeAssessment(reason string) assessment {
	return assessment{class: unsafeState, reasons: []string{reason}}
}

func report(emit Reporter, path string, a assessment, action string) {
	if emit == nil {
		return
	}
	for _, reason := range a.reasons {
		emit(Diagnostic{Path: path, Classification: a.class, Reason: reason, Action: action,
			Owner: a.owner, Principal: a.principal, Mask: a.mask})
	}
}

func securityError(path, reason string) error {
	return &SecurityError{Path: path, Reason: reason}
}

type SecurityError struct{ Path, Reason string }

func (e *SecurityError) Error() string {
	return fmt.Sprintf("runtime ACL reason=%s path=%q; manual security review/recovery required", e.Reason, e.Path)
}
