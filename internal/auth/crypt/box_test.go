package crypt

import (
	"strings"
	"testing"
)

func TestSealOpen(t *testing.T) {
	box, err := New("0123456789abcdef-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := box.Seal("1//refresh-token", "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "refresh-token") {
		t.Fatal("ciphertext leaks plaintext")
	}
	again, _ := box.Seal("1//refresh-token", "a@example.com")
	if again == sealed {
		t.Error("two seals of the same value must differ (fresh nonce)")
	}

	tests := []struct {
		name    string
		value   string
		assoc   string
		wantErr bool
	}{
		{"round trip", sealed, "a@example.com", false},
		{"wrong associated data", sealed, "b@example.com", true},
		{"tampered", sealed[:len(sealed)-2] + "AA", "a@example.com", true},
		{"bad prefix", "v0:" + strings.TrimPrefix(sealed, "v1:"), "a@example.com", true},
		{"garbage", "not-sealed", "a@example.com", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := box.Open(tc.value, tc.assoc)
			if tc.wantErr != (err != nil) {
				t.Fatalf("Open err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != "1//refresh-token" {
				t.Errorf("Open = %q", got)
			}
		})
	}

	other, _ := New("another-secret-of-enough-length")
	if _, err := other.Open(sealed, "a@example.com"); err == nil {
		t.Error("a different key must not decrypt")
	}
}

func TestNewRejectsShortSecret(t *testing.T) {
	if _, err := New("short"); err == nil {
		t.Error("want error for short secret")
	}
}
