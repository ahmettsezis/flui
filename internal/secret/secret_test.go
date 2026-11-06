package secret

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreatePersistsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "master.key")
	kr1, created, insecure, err := LoadOrCreate(path)
	if err != nil || !created || insecure {
		t.Fatalf("create: created=%v insecure=%v err=%v", created, insecure, err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, err = %v", st.Mode().Perm(), err)
	}
	ct, err := kr1.Seal([]byte("hello"), []byte("ctx"))
	if err != nil {
		t.Fatal(err)
	}

	kr2, created, _, err := LoadOrCreate(path)
	if err != nil || created {
		t.Fatalf("load: created=%v err=%v", created, err)
	}
	pt, err := kr2.Open(ct, []byte("ctx"))
	if err != nil || !bytes.Equal(pt, []byte("hello")) {
		t.Fatalf("Open = %q, %v", pt, err)
	}
}

func TestInsecurePermissionsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "master.key")
	if _, _, _, err := LoadOrCreate(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, insecure, err := LoadOrCreate(path); err != nil || !insecure {
		t.Fatalf("insecure=%v err=%v", insecure, err)
	}
}

func TestOpenRejectsTamperingAndWrongContext(t *testing.T) {
	kr, err := New(bytes.Repeat([]byte{1}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := kr.Seal([]byte("secret"), []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Open(ct, []byte("b")); !errors.Is(err, ErrDecrypt) {
		t.Errorf("wrong aad: err = %v", err)
	}
	ct[len(ct)-1] ^= 1
	if _, err := kr.Open(ct, []byte("a")); !errors.Is(err, ErrDecrypt) {
		t.Errorf("tampered: err = %v", err)
	}
	if _, err := kr.Open([]byte("short"), nil); !errors.Is(err, ErrDecrypt) {
		t.Errorf("short: err = %v", err)
	}
}

func TestInvalidKey(t *testing.T) {
	if _, err := New([]byte("short")); err == nil {
		t.Fatal("expected error")
	}
	path := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(path, []byte("not base64!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := LoadOrCreate(path); err == nil {
		t.Fatal("expected error for invalid key file")
	}
}
