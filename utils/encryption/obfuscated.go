package encryption

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// ObfuscatedValue is a string stored XOR-obfuscated with a key. It is not
// encryption: the key travels with the value, so this only keeps the value
// from being readable straight out of the config file.
type ObfuscatedValue struct {
	// data is the stored value: XorWithKey(original, key) — or the plain
	// value when key is empty.
	data string
	// key is the XOR key data was obfuscated with; empty when data is plain.
	key string
}

// NewObfuscatedValue returns data XOR-obfuscated with key (via
// XorWithKey). An empty key leaves the value unobfuscated.
func NewObfuscatedValue(data, key string) ObfuscatedValue {
	if key == "" {
		return ObfuscatedValue{data: data}
	}
	return ObfuscatedValue{data: XorWithKey(data, key), key: key}
}

// String un-obfuscates the value (via XorWithKey) and returns the original
// data. An empty value returns an empty string.
func (v ObfuscatedValue) String() string {
	if v.key == "" {
		return v.data
	}
	return XorWithKey(v.data, v.key)
}

// IsEmpty reports whether v holds no value.
func (v ObfuscatedValue) IsEmpty() bool {
	return v.data == ""
}

// Reobfuscated returns v's value obfuscated with key. It is a no-op for an
// empty value and idempotent for a value already obfuscated with key, which
// makes it safe to run on every load to upgrade legacy plain values.
func (v ObfuscatedValue) Reobfuscated(key string) ObfuscatedValue {
	if v.IsEmpty() {
		return v
	}
	return NewObfuscatedValue(v.String(), key)
}

// jsonObfuscatedValue is the on-disk shape of a non-empty ObfuscatedValue.
// Both halves are base64-encoded so arbitrary bytes survive the JSON round
// trip.
type jsonObfuscatedValue struct {
	Data string `json:"data"`
	Key  string `json:"key"`
}

// MarshalJSON implements json.Marshaler. An empty value marshals as null.
func (v ObfuscatedValue) MarshalJSON() ([]byte, error) {
	if v.IsEmpty() {
		return []byte("null"), nil
	}
	return json.Marshal(jsonObfuscatedValue{
		Data: base64.StdEncoding.EncodeToString([]byte(v.data)),
		Key:  base64.StdEncoding.EncodeToString([]byte(v.key)),
	})
}

// UnmarshalJSON implements json.Unmarshaler. It accepts the object form
// written by MarshalJSON and JSON null; a plain JSON string is a legacy
// (unobfuscated) value and is kept as-is so the caller can upgrade it.
func (v *ObfuscatedValue) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*v = ObfuscatedValue{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*v = ObfuscatedValue{data: s}
		return nil
	}
	var j jsonObfuscatedValue
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(j.Data)
	if err != nil {
		return fmt.Errorf("encryption: bad obfuscated data: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(j.Key)
	if err != nil {
		return fmt.Errorf("encryption: bad obfuscation key: %w", err)
	}
	*v = ObfuscatedValue{data: string(data), key: string(key)}
	return nil
}
