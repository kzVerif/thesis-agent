package protectedpath

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"ws-agent/internal/apppaths"
)

func TestLargeDownloadsDirectoryDoesNotPreventStartupSecurity(t *testing.T) {
	paths, _ := apppaths.Console(t.TempDir())
	// Virtual metadata inventory: more than the old 8,192 limit, with deliberately
	// unsafe historical files. Production uses this same fixed startup planner;
	// its visitor has no enumeration method, and must never request these entries.
	for _, count := range []int{0, 20000, 50000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			metadata := map[string]assessment{}
			strict := []string{paths.Root, paths.Identity, paths.Enrollment, paths.Config, paths.RuntimeLock(), paths.IdentityBackup(), filepath.Dir(paths.Log), filepath.Dir(paths.Downloads), paths.Downloads}
			for _, p := range strict {
				metadata[p] = assessment{class: canonical, reasons: []string{"acl_canonical"}}
			}
			for i := 0; i < count; i++ {
				metadata[filepath.Join(paths.Downloads, fmt.Sprintf("%06d.bin", i))] = unsafeAssessment("untrusted_ace")
			}
			metadata[paths.Log+".1"] = unsafeAssessment("untrusted_owner")
			metadata[filepath.Join(filepath.Dir(paths.Log), "old", "nested", "link")] = unsafeAssessment("reparse_point_detected")
			metadata[filepath.Join(paths.Downloads, "nested", "stale.part")] = unsafeAssessment("security_descriptor_unreadable")
			metadata[filepath.Join(paths.Root, "arbitrary", "unknown")] = unsafeAssessment("untrusted_ace")
			visited := map[string]int{}
			err := inspectStartupScope(context.Background(), paths, func(target startupTarget) error {
				visited[target.path]++
				a, ok := metadata[target.path]
				if !ok {
					return fmt.Errorf("unexpected startup path %s", target.path)
				}
				if a.class != canonical {
					return securityError(target.path, a.reasons[0])
				}
				return nil
			})
			if err != nil {
				t.Fatalf("historical child count %d blocked startup preflight: %v", count, err)
			}
			if len(visited) != 9 {
				t.Fatalf("startup accessed %d objects instead of fixed 9", len(visited))
			}
			for _, p := range strict {
				if visited[p] != 1 {
					t.Fatalf("critical/container skipped or enumerated: %s", p)
				}
			}
			if visited[paths.Log] != 0 || visited[paths.Log+".1"] != 0 {
				t.Fatal("startup scanned log files")
			}
		})
	}
}

func TestStartupPlanPreservesCriticalAndContainerChecks(t *testing.T) {
	paths, _ := apppaths.Console(t.TempDir())
	for _, path := range []string{paths.Root, paths.Identity, paths.Enrollment, paths.Config, paths.RuntimeLock(), paths.IdentityBackup(), filepath.Dir(paths.Log), filepath.Dir(paths.Downloads), paths.Downloads} {
		err := inspectStartupScope(context.Background(), paths, func(target startupTarget) error {
			if target.path != path {
				return nil
			}
			if target.scope != "critical" && target.scope != "container" {
				t.Fatal("missing diagnostic scope")
			}
			return securityError(path, "untrusted_ace")
		})
		if err == nil {
			t.Fatalf("unsafe startup object bypassed: %s", path)
		}
	}
}
