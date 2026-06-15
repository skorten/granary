package exporter

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Granola encrypts its cache using the Electron safeStorage scheme (Chromium
// OSCrypt on macOS). The data encryption key (DEK) is wrapped in storage.dek and
// protected by a passphrase held in the macOS Keychain. See:
// https://github.com/skorten/granary/issues/13
const (
	// keychainService and keychainAccount identify the Keychain entry that holds
	// the safeStorage passphrase.
	keychainService = "Granola Safe Storage"
	keychainAccount = "Granola Key"

	// dekFileName is the wrapped data encryption key, stored alongside the cache.
	dekFileName = "storage.dek"

	// safeStoragePrefix is the 3-byte version tag Electron prepends to storage.dek.
	safeStoragePrefix = "v10"

	// PBKDF2 parameters for deriving the AES-128-CBC key that unwraps the DEK.
	// These are the fixed Chromium OSCrypt values.
	pbkdf2Salt   = "saltysalt"
	pbkdf2Iters  = 1003
	pbkdf2KeyLen = 16
)

// keychainSecret returns the safeStorage passphrase from the macOS Keychain.
// It is a variable so tests can inject a known secret without touching the
// real Keychain (and so it can be exercised on non-macOS CI).
var keychainSecret = defaultKeychainSecret

// defaultKeychainSecret shells out to /usr/bin/security to read the passphrase.
func defaultKeychainSecret() (string, error) {
	out, err := exec.Command(
		"/usr/bin/security", "find-generic-password",
		"-s", keychainService, "-a", keychainAccount, "-w",
	).Output()
	if err != nil {
		return "", fmt.Errorf("reading Keychain entry %q (account %q): %w", keychainService, keychainAccount, err)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// decryptCache reads and decrypts a Granola encrypted cache file (cache-v*.json.enc),
// returning the plaintext JSON bytes (same shape as the legacy plaintext cache).
// The storage.dek file is expected in the same directory as encPath.
func decryptCache(encPath string) ([]byte, error) {
	password, err := keychainSecret()
	if err != nil {
		return nil, fmt.Errorf("couldn't read Granola's login from your Mac's Keychain. Make sure the Granola app is installed and you've signed in at least once, then try again (details: %w)", err)
	}

	dekPath := filepath.Join(filepath.Dir(encPath), dekFileName)
	dekBlob, err := os.ReadFile(dekPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dekFileName, err)
	}

	dek, err := unwrapDEK(password, dekBlob)
	if err != nil {
		return nil, fmt.Errorf("unwrapping data encryption key from %s: %w", dekFileName, err)
	}

	encData, err := os.ReadFile(encPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filepath.Base(encPath), err)
	}

	plaintext, err := aesGCMDecrypt(dek, encData)
	if err != nil {
		return nil, fmt.Errorf("decrypting %s: %w", filepath.Base(encPath), err)
	}
	return plaintext, nil
}

// unwrapDEK recovers the 32-byte data encryption key from a storage.dek blob.
// The blob is "v10" + AES-128-CBC(base64(dek)), with the CBC key derived from
// the keychain passphrase via PBKDF2 and a fixed all-spaces IV.
func unwrapDEK(keychainPassword string, dekBlob []byte) ([]byte, error) {
	if !bytes.HasPrefix(dekBlob, []byte(safeStoragePrefix)) {
		return nil, fmt.Errorf("missing %q version prefix", safeStoragePrefix)
	}
	ciphertext := dekBlob[len(safeStoragePrefix):]

	key, err := pbkdf2.Key(sha1.New, keychainPassword, []byte(pbkdf2Salt), pbkdf2Iters, pbkdf2KeyLen)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
	}

	iv := bytes.Repeat([]byte{' '}, aes.BlockSize)
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)

	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return nil, err
	}

	dek, err := base64.StdEncoding.DecodeString(string(plain))
	if err != nil {
		return nil, fmt.Errorf("decoding base64 DEK: %w", err)
	}
	return dek, nil
}

// aesGCMDecrypt decrypts a Granola .enc payload laid out as
// nonce(12) || ciphertext || tag(16), using the 32-byte DEK as an AES-256-GCM key.
func aesGCMDecrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize+gcm.Overhead() {
		return nil, fmt.Errorf("encrypted payload too short (%d bytes)", len(data))
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// pkcs7Unpad removes PKCS#7 padding.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padded data length %d", len(data))
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("invalid PKCS#7 padding")
	}
	for _, b := range data[len(data)-padLen:] {
		if int(b) != padLen {
			return nil, fmt.Errorf("invalid PKCS#7 padding")
		}
	}
	return data[:len(data)-padLen], nil
}
