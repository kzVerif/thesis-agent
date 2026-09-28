// Package installedapps provides a bounded, read-only machine application inventory.
package installedapps

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

const (
	MaxApps      = 2000
	MaxEntries   = 10000
	MaxPayload   = 4 << 20
	MaxName      = 512
	MaxVersion   = 256
	MaxPublisher = 512
	MaxDate      = 32
	MaxSizeKB    = uint64(1<<32 - 1)
)

type App struct {
	Name            string  `json:"name"`
	Version         string  `json:"version,omitempty"`
	Publisher       string  `json:"publisher,omitempty"`
	InstallDate     string  `json:"install_date,omitempty"`
	EstimatedSizeKB *uint64 `json:"estimated_size_kb,omitempty"`
}

// Entry identity is the uninstall subkey, not DisplayName. Matching identity AND
// all fields deduplicate mirrored views; distinct products/versions survive.
type Entry struct {
	Key string
	App App
}
type ViewReader func(context.Context, uint32, func(Entry) error) error

func bounded(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func CollectWith(ctx context.Context, read ViewReader) ([]App, error) {
	apps := make([]App, 0)
	seen := make(map[string]struct{})
	entries := 0
	for _, view := range []uint32{64, 32} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err := read(ctx, view, func(e Entry) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries++
			if entries > MaxEntries {
				return errors.New("registry entry limit exceeded")
			}
			a := e.App
			a.Name = bounded(a.Name, MaxName)
			if a.Name == "" {
				return nil
			}
			a.Version = bounded(a.Version, MaxVersion)
			a.Publisher = bounded(a.Publisher, MaxPublisher)
			a.InstallDate = bounded(a.InstallDate, MaxDate)
			if a.EstimatedSizeKB != nil && *a.EstimatedSizeKB > MaxSizeKB {
				a.EstimatedSizeKB = nil
			}
			encoded, _ := json.Marshal(a)
			key := strings.ToLower(e.Key) + "\x00" + string(encoded)
			if _, exists := seen[key]; exists {
				return nil
			}
			seen[key] = struct{}{}
			if len(apps) >= MaxApps {
				return errors.New("application limit exceeded")
			}
			apps = append(apps, a)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(apps, func(i, j int) bool {
		a, b := strings.ToLower(apps[i].Name), strings.ToLower(apps[j].Name)
		if a != b {
			return a < b
		}
		x, _ := json.Marshal(apps[i])
		y, _ := json.Marshal(apps[j])
		return string(x) < string(y)
	})
	return apps, ctx.Err()
}
