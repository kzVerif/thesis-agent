package installedapps

import (
	"context"
	"encoding/binary"
	"errors"
	"runtime"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

const uninstall = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

func Collect(ctx context.Context) ([]App, error) { return CollectWith(ctx, readView) }

func readView(ctx context.Context, view uint32, yield func(Entry) error) error {
	flag := uint32(registry.WOW64_64KEY)
	if view == 32 {
		flag = registry.WOW64_32KEY
	}
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstall, registry.ENUMERATE_SUB_KEYS|flag)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	// ReadSubKeyNames restarts at index zero on each call. Enumerate explicitly
	// with a bounded name buffer, cancellation between entries, and one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for index := uint32(0); ; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		var nameBuffer [256]uint16
		length := uint32(len(nameBuffer))
		err := syscall.RegEnumKeyEx(syscall.Handle(root), index, &nameBuffer[0], &length, nil, nil, nil, nil)
		if err == syscall.Errno(259) {
			return nil
		} // ERROR_NO_MORE_ITEMS
		if err != nil {
			return err
		}
		name := syscall.UTF16ToString(nameBuffer[:length])
		{
			if err := ctx.Err(); err != nil {
				return err
			}
			key, openErr := registry.OpenKey(root, name, registry.QUERY_VALUE|flag)
			if openErr != nil {
				// Count unreadable/deleted entries too, so enumeration stays bounded.
				if err := yield(Entry{Key: name}); err != nil {
					return err
				}
				continue
			}
			a := readApp(key)
			key.Close()
			if err := yield(Entry{Key: name, App: a}); err != nil {
				return err
			}
		}
	}
}

// Use a fixed buffer rather than GetStringValue, which allocates the value's
// claimed registry size. Expand strings are returned literally, never expanded.
func readText(key registry.Key, name string) string {
	buf := make([]byte, 8192)
	n, kind, err := key.GetValue(name, buf)
	if err != nil || (kind != registry.SZ && kind != registry.EXPAND_SZ) || n%2 != 0 {
		return ""
	}
	words := make([]uint16, n/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(buf[i*2:])
	}
	for len(words) > 0 && words[len(words)-1] == 0 {
		words = words[:len(words)-1]
	}
	return string(utf16.Decode(words))
}

func readApp(key registry.Key) App {
	a := App{Name: readText(key, "DisplayName"), Version: readText(key, "DisplayVersion"), Publisher: readText(key, "Publisher"), InstallDate: readText(key, "InstallDate")}
	if size, _, err := key.GetIntegerValue("EstimatedSize"); err == nil && size <= MaxSizeKB {
		a.EstimatedSizeKB = &size
	}
	return a
}
