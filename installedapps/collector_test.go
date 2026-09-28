package installedapps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCollectViewsDedupSortAndOptionalFields(t *testing.T) {
	size := uint64(2048)
	views := []uint32{}
	reader := func(ctx context.Context, view uint32, yield func(Entry) error) error {
		views = append(views, view)
		for _, e := range []Entry{
			{Key: "blank", App: App{Name: "  "}},
			{Key: "same-product", App: App{Name: "Zoo", Version: "1", Publisher: "Company", InstallDate: "20260230", EstimatedSizeKB: &size}},
			{Key: "other-product", App: App{Name: "Zoo", Version: "1", Publisher: "Company"}},
			{Key: "alpha", App: App{Name: " alpha "}},
			{Key: fmt.Sprint(view), App: App{Name: "Zoo", Version: fmt.Sprint(view)}},
		} {
			if err := yield(e); err != nil {
				return err
			}
		}
		return nil
	}
	apps, err := CollectWith(context.Background(), reader)
	if err != nil || !reflect.DeepEqual(views, []uint32{64, 32}) || len(apps) != 5 || apps[0].Name != "alpha" {
		t.Fatalf("views=%v apps=%v error=%v", views, apps, err)
	}
	other, err := CollectWith(context.Background(), reader)
	if err != nil || !reflect.DeepEqual(apps, other) {
		t.Fatal("unstable order")
	}
	data, _ := json.Marshal(apps)
	if !strings.Contains(string(data), `"estimated_size_kb":2048`) || !strings.Contains(string(data), `"install_date":"20260230"`) {
		t.Fatal(string(data))
	}
	// Invalid dates stay raw for the UI's safe date formatter; sizes stay KB.
}
func TestBoundsMissingSizeAndCancellation(t *testing.T) {
	overflow := MaxSizeKB + 1
	apps, err := CollectWith(context.Background(), func(ctx context.Context, view uint32, y func(Entry) error) error {
		return y(Entry{Key: "x", App: App{Name: strings.Repeat("ก", 600), Version: strings.Repeat("v", 300), Publisher: strings.Repeat("p", 600), InstallDate: strings.Repeat("d", 40), EstimatedSizeKB: &overflow}})
	})
	if err != nil || len(apps) != 1 || len([]rune(apps[0].Name)) != MaxName || len(apps[0].Version) != MaxVersion || len(apps[0].Publisher) != MaxPublisher || len(apps[0].InstallDate) != MaxDate || apps[0].EstimatedSizeKB != nil {
		t.Fatal(apps, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = CollectWith(ctx, func(context.Context, uint32, func(Entry) error) error {
		t.Fatal("reader called after cancellation")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	_, err = CollectWith(ctx, func(ctx context.Context, view uint32, y func(Entry) error) error {
		cancel()
		return y(Entry{Key: "x", App: App{Name: "X"}})
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestApplicationAndEnumerationLimits(t *testing.T) {
	for _, empty := range []bool{false, true} {
		_, err := CollectWith(context.Background(), func(ctx context.Context, v uint32, y func(Entry) error) error {
			for i := 0; i <= MaxEntries; i++ {
				a := App{}
				if !empty {
					a.Name = "App"
				}
				if err := y(Entry{Key: fmt.Sprint(i), App: a}); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			t.Fatal("unbounded inventory")
		}
	}
	apps, err := CollectWith(context.Background(), func(context.Context, uint32, func(Entry) error) error { return nil })
	if err != nil || apps == nil || len(apps) != 0 {
		t.Fatal(apps, err)
	}
}
