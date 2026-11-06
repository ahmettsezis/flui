// Package secret encrypts data at rest with the flui master key (AES-256-GCM).
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const KeySize = 32

var ErrDecrypt = errors.New("secret: decryption failed")

type Keyring struct {
	aead cipher.AEAD
}

func New(key []byte) (*Keyring, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secret: master key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Keyring{aead: aead}, nil
}

// LoadOrCreate reads a base64-encoded master key from path, creating a new random key (mode 0600) if the file
// does not exist. insecure reports whether the file is accessible by group or others.
func LoadOrCreate(path string) (kr *Keyring, created, insecure bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from the operator
	if errors.Is(err, fs.ErrNotExist) {
		key := make([]byte, KeySize)
		if _, err := rand.Read(key); err != nil {
			return nil, false, false, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, false, false, err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path comes from the operator
		if err != nil {
			return nil, false, false, err
		}
		_, werr := f.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
		if err := errors.Join(werr, f.Close()); err != nil {
			return nil, false, false, err
		}
		kr, err := New(key)
		return kr, true, false, err
	}
	if err != nil {
		return nil, false, false, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, false, false, fmt.Errorf("secret: %s is not valid base64", path)
	}
	if st, err := os.Stat(path); err == nil && st.Mode().Perm()&0o077 != 0 {
		insecure = true
	}
	kr, err = New(key)
	return kr, false, insecure, err
}

// Seal encrypts plaintext. aad binds the ciphertext to its context (e.g. a setting name) and must match on Open.
func (k *Keyring) Seal(plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, k.aead.NonceSize(), k.aead.NonceSize()+len(plaintext)+k.aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return k.aead.Seal(nonce, nonce, plaintext, aad), nil
}

func (k *Keyring) Open(ciphertext, aad []byte) ([]byte, error) {
	ns := k.aead.NonceSize()
	if len(ciphertext) < ns+k.aead.Overhead() {
		return nil, ErrDecrypt
	}
	pt, err := k.aead.Open(nil, ciphertext[:ns], ciphertext[ns:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}
