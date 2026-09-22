//go:build windows

package protectedpath

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestProtectedDescriptor(t *testing.T) {
	for _, tc := range []struct {
		name, sddl       string
		directory, valid bool
	}{
		{"runtime", "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", true, true},
		{"file", "O:SYG:SYD:P(A;;FA;;;SY)(A;;FA;;;BA)", false, true},
		{"users-read", "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GR;;;BU)", true, false},
		{"no-child-inheritance", "O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)", true, false},
		{"admins-read-only", "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;GR;;;BA)", true, false},
		{"untrusted-owner", "O:BUG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", true, false},
		{"empty-dacl", "O:BAG:BAD:P", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tc.sddl)
			if err != nil {
				t.Fatal(err)
			}
			if got := validateDescriptor(sd, tc.directory); (got == nil) != tc.valid {
				t.Fatalf("ACL decision=%v", got)
			}
		})
	}
}
