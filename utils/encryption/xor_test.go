package encryption

import (
	"testing"
	"uuid"
)

func TestXorWithKey(t *testing.T) {
	key := uuid.New().String()
	data := uuid.New().String() + uuid.New().String()
	obfuscated := XorWithKey(data, key)
	recovered := XorWithKey(obfuscated, key)
	if data != recovered {
		t.Fatalf("Xor not cyclic")
	}
}
