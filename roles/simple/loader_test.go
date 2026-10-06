package simple

import "testing"

func TestCount(t *testing.T) {
	loaded := LoadRoles()
	if len(loaded) != 50 {
		t.Fatalf("Expected 50, found %d", len(loaded))
	}
}
