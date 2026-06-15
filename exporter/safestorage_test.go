package exporter

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setKeychainSecret swaps the keychain lookup and returns a function that
// restores the original. Test-only helper.
func setKeychainSecret(fn func() (string, error)) (restore func()) {
	prev := keychainSecret
	keychainSecret = fn
	return func() { keychainSecret = prev }
}

// writeFile writes data to dir/name and returns the full path. Test helper only.
func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// sealGCM builds a Granola-format encrypted blob: nonce(12) || ciphertext || tag(16).
// This is the inverse of aesGCMDecrypt and is used only by tests.
func sealGCM(t *testing.T, key, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("NewGCM: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return append(nonce, gcm.Seal(nil, nonce, plaintext, nil)...)
}

func TestAESGCMDecrypt(t *testing.T) {
	t.Run("recovers plaintext from nonce||ciphertext||tag layout", func(t *testing.T) {
		key := make([]byte, 32) // AES-256 DEK
		if _, err := rand.Read(key); err != nil {
			t.Fatalf("rand: %v", err)
		}
		plaintext := []byte(`{"cache":{"state":{"documents":{}}}}`)

		blob := sealGCM(t, key, plaintext)

		got, err := aesGCMDecrypt(key, blob)
		if err != nil {
			t.Fatalf("aesGCMDecrypt: %v", err)
		}
		if string(got) != string(plaintext) {
			t.Errorf("got %q, want %q", got, plaintext)
		}
	})

	t.Run("returns error for blob shorter than nonce+tag", func(t *testing.T) {
		key := make([]byte, 32)
		if _, err := aesGCMDecrypt(key, []byte("too short")); err == nil {
			t.Error("expected error for undersized blob")
		}
	})

	t.Run("returns error on tampered ciphertext", func(t *testing.T) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Fatalf("rand: %v", err)
		}
		blob := sealGCM(t, key, []byte("hello world payload"))
		blob[len(blob)-1] ^= 0xff // corrupt the tag

		if _, err := aesGCMDecrypt(key, blob); err == nil {
			t.Error("expected authentication error for tampered blob")
		}
	})
}

// wrapDEK builds a Granola-format storage.dek: "v10" || AES-128-CBC(base64(dek)).
// The CBC key is derived from the keychain password exactly as production does.
// Inverse of unwrapDEK; used only by tests.
func wrapDEK(t *testing.T, keychainPassword string, dek []byte) []byte {
	t.Helper()
	key, err := pbkdf2.Key(sha1.New, keychainPassword, []byte(pbkdf2Salt), pbkdf2Iters, pbkdf2KeyLen)
	if err != nil {
		t.Fatalf("pbkdf2: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	// Inner plaintext is the base64 encoding of the raw DEK.
	plain := []byte(base64.StdEncoding.EncodeToString(dek))
	// PKCS7 pad to the AES block size.
	padLen := aes.BlockSize - len(plain)%aes.BlockSize
	for i := 0; i < padLen; i++ {
		plain = append(plain, byte(padLen))
	}
	iv := make([]byte, aes.BlockSize)
	for i := range iv {
		iv[i] = ' '
	}
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, plain)
	return append([]byte(safeStoragePrefix), ct...)
}

func TestUnwrapDEK(t *testing.T) {
	t.Run("recovers the 32-byte DEK from a v10 CBC blob", func(t *testing.T) {
		password := "YWJjZGVmZ2hpamtsbW5vcA==" // arbitrary base64 keychain secret
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			t.Fatalf("rand: %v", err)
		}

		blob := wrapDEK(t, password, dek)

		got, err := unwrapDEK(password, blob)
		if err != nil {
			t.Fatalf("unwrapDEK: %v", err)
		}
		if string(got) != string(dek) {
			t.Errorf("DEK mismatch: got %x, want %x", got, dek)
		}
	})

	t.Run("returns error when v10 prefix is missing", func(t *testing.T) {
		password := "YWJjZGVmZ2hpamtsbW5vcA=="
		blob := wrapDEK(t, password, make([]byte, 32))
		blob = blob[3:] // strip the v10 prefix

		if _, err := unwrapDEK(password, blob); err == nil {
			t.Error("expected error for missing v10 prefix")
		}
	})
}

func TestDecryptCache(t *testing.T) {
	t.Run("end-to-end: keychain -> DEK -> recovers original plaintext", func(t *testing.T) {
		password := "YWJjZGVmZ2hpamtsbW5vcA=="
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			t.Fatalf("rand: %v", err)
		}
		payload := []byte(`{"workos_tokens":"{\"access_token\":\"secret-token-123\"}"}`)

		dir := t.TempDir()
		writeFile(t, dir, dekFileName, wrapDEK(t, password, dek))
		encPath := writeFile(t, dir, "supabase.json.enc", sealGCM(t, dek, payload))

		// Inject the keychain secret so the test never touches the real Keychain.
		restore := setKeychainSecret(func() (string, error) { return password, nil })
		defer restore()

		plaintext, err := decryptCache(encPath)
		if err != nil {
			t.Fatalf("decryptCache: %v", err)
		}
		if string(plaintext) != string(payload) {
			t.Errorf("decrypted payload mismatch:\n got %s\nwant %s", plaintext, payload)
		}
	})

	t.Run("returns an actionable, plain-language error when keychain fails", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, dekFileName, []byte("v10garbage"))
		encPath := writeFile(t, dir, "cache-v6.json.enc", []byte("ignored"))

		restore := setKeychainSecret(func() (string, error) {
			return "", &keychainError{}
		})
		defer restore()

		_, err := decryptCache(encPath)
		if err == nil {
			t.Fatal("expected error when keychain lookup fails")
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "granola") || !strings.Contains(msg, "sign") {
			t.Errorf("error should tell the user to install/sign in to Granola, got: %v", err)
		}
	})
}

// keychainError is a sentinel error type used only in tests.
type keychainError struct{}

func (e *keychainError) Error() string { return "keychain unavailable (test)" }
