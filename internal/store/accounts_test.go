package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tr4m0ryp/oido-google-calander/internal/auth/crypt"
)

func newBox(t *testing.T) *crypt.Box {
	t.Helper()
	box, err := crypt.New("test-encryption-secret-123")
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func TestAccountsLifecycle(t *testing.T) {
	dir := t.TempDir()
	box := newBox(t)
	s, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("Alice@Example.com", "1//alice-token"); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("bob@example.com", "1//bob-token"); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "alice-token") {
		t.Fatal("refresh token stored in plaintext")
	}
	info, _ := os.Stat(filepath.Join(dir, fileName))
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("store file mode = %o, want 600", perm)
	}

	reopened, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Len() != 2 || !reopened.Has("alice@example.com") {
		t.Fatalf("reopened store lost accounts: %v", reopened.List())
	}
	tok, err := reopened.RefreshToken("ALICE@example.com")
	if err != nil || tok != "1//alice-token" {
		t.Errorf("RefreshToken = %q, %v", tok, err)
	}

	if err := reopened.Remove("alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Remove("alice@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second remove err = %v, want ErrNotFound", err)
	}
	if _, err := reopened.RefreshToken("alice@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("token after remove err = %v", err)
	}
}

func TestWrongKeyCannotDecrypt(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, newBox(t))
	_ = s.Put("a@example.com", "secret")
	other, _ := crypt.New("a-completely-different-secret")
	s2, err := Open(dir, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.RefreshToken("a@example.com"); err == nil {
		t.Error("decrypt with wrong key succeeded")
	}
}
