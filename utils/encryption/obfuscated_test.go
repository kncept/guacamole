package encryption

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNewObfuscatedValueRoundTrip(t *testing.T) {
	data := "sk-super-secret-key"
	key := "some key"
	v := NewObfuscatedValue(data, key)
	if v.String() != data {
		t.Errorf("String() = %q, want %q", v.String(), data)
	}
	// The stored form must be the xored value, not the original.
	if v.data == data {
		t.Errorf("stored data = %q, want the obfuscated form", v.data)
	}
}

func TestNewObfuscatedValueEmptyKey(t *testing.T) {
	v := NewObfuscatedValue("plain", "")
	if v.String() != "plain" {
		t.Errorf("String() = %q, want %q", v.String(), "plain")
	}
}

func TestObfuscatedValueIsEmpty(t *testing.T) {
	if !(ObfuscatedValue{}).IsEmpty() {
		t.Error("zero value: IsEmpty() = false, want true")
	}
	if !(NewObfuscatedValue("", "k")).IsEmpty() {
		t.Error("empty data: IsEmpty() = false, want true")
	}
	if NewObfuscatedValue("x", "k").IsEmpty() {
		t.Error("non-empty data: IsEmpty() = true, want false")
	}
}

func TestObfuscatedValueReobfuscated(t *testing.T) {
	v := NewObfuscatedValue("secret", "key1")
	if got := v.Reobfuscated("key1").String(); got != "secret" {
		t.Errorf("Reobfuscated(same key).String() = %q, want %q", got, "secret")
	}
	// A legacy plain value (empty key) gets wrapped.
	legacy := ObfuscatedValue{data: "plain"}
	if got := legacy.Reobfuscated("key2").String(); got != "plain" {
		t.Errorf("Reobfuscated(legacy).String() = %q, want %q", got, "plain")
	}
	// An empty value stays empty.
	if !(ObfuscatedValue{}).Reobfuscated("key2").IsEmpty() {
		t.Error("Reobfuscated(empty): IsEmpty() = false, want true")
	}
}

func TestObfuscatedValueJSONRoundTrip(t *testing.T) {
	original := NewObfuscatedValue("sk-super-secret-key", "key")
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if bytes.Contains(b, []byte("sk-super-secret-key")) {
		t.Errorf("marshaled JSON %s contains the plain value, want it obfuscated", b)
	}

	var back ObfuscatedValue
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.String() != "sk-super-secret-key" {
		t.Errorf("round-tripped String() = %q, want the original", back.String())
	}
}

func TestObfuscatedValueJSONNull(t *testing.T) {
	var v ObfuscatedValue
	if err := json.Unmarshal([]byte("null"), &v); err != nil {
		t.Fatalf("Unmarshal(null): %v", err)
	}
	if !v.IsEmpty() {
		t.Error("null: IsEmpty() = false, want true")
	}
}

func TestObfuscatedValueJSONLegacyString(t *testing.T) {
	var v ObfuscatedValue
	if err := json.Unmarshal([]byte(`"plain-key"`), &v); err != nil {
		t.Fatalf("Unmarshal(legacy string): %v", err)
	}
	if v.String() != "plain-key" {
		t.Errorf("legacy String() = %q, want %q", v.String(), "plain-key")
	}
	if got := v.Reobfuscated("key").String(); got != "plain-key" {
		t.Errorf("upgraded String() = %q, want %q", got, "plain-key")
	}
}

func TestObfuscatedValueJSONBadBase64(t *testing.T) {
	var v ObfuscatedValue
	if err := json.Unmarshal([]byte(`{"data":"!!not-base64!!","key":"eA=="}`), &v); err == nil {
		t.Fatal("Unmarshal(bad base64): expected an error, got nil")
	}
}
