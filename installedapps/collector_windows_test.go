package installedapps

import (
	"context"
	"os"
	"testing"
	"time"
)

// Opt-in, read-only adapter smoke test. Never writes registry keys or logs names.
func TestWindowsRegistryReadOnlySmoke(t *testing.T) {
	if os.Getenv("INSTALLED_APPS_READONLY_SMOKE") != "1" {
		t.Skip("set INSTALLED_APPS_READONLY_SMOKE=1 for read-only Windows registry smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	apps, err := Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) > MaxApps {
		t.Fatal("application limit exceeded")
	}
	t.Logf("Read-only machine inventory: count=%d elapsed=%s", len(apps), time.Since(start))
}
