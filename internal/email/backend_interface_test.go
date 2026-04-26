package email

import (
	"reflect"
	"testing"
)

func TestBackendInterfaceExcludesDelete(t *testing.T) {
	ty := reflect.TypeOf((*Backend)(nil)).Elem()
	if m, ok := ty.MethodByName("Delete"); ok {
		t.Fatalf("Backend must not expose Delete; found %v", m)
	}
	for _, name := range []string{"Name", "Watch", "Fetch", "Reply"} {
		if _, ok := ty.MethodByName(name); !ok {
			t.Fatalf("Backend missing required method %q", name)
		}
	}
}
