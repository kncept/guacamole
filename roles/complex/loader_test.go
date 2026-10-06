package complex

import "testing"

func TestCount(t *testing.T) {
	loaded := LoadRoles()
	if len(loaded) != 1 {
		t.Fatalf("Expected 1, found %d", len(loaded))
	}
}
