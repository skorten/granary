package exporter

import (
	"crypto/rand"
	"encoding/json"
	"testing"
)

func TestAccessToken(t *testing.T) {
	t.Run("reads token from encrypted supabase.json.enc", func(t *testing.T) {
		password := "YWJjZGVmZ2hpamtsbW5vcA=="
		dek := make([]byte, 32)
		if _, err := rand.Read(dek); err != nil {
			t.Fatalf("rand: %v", err)
		}

		// workos_tokens is a JSON-encoded STRING inside the supabase JSON.
		inner, _ := json.Marshal(map[string]string{"access_token": "secret-token-123", "token_type": "Bearer"})
		supa, _ := json.Marshal(map[string]any{"workos_tokens": string(inner), "session_id": "s1"})

		dir := t.TempDir()
		writeFile(t, dir, dekFileName, wrapDEK(t, password, dek))
		writeFile(t, dir, "supabase.json.enc", sealGCM(t, dek, supa))

		restore := setKeychainSecret(func() (string, error) { return password, nil })
		defer restore()

		tok, err := AccessToken(dir)
		if err != nil {
			t.Fatalf("AccessToken: %v", err)
		}
		if tok != "secret-token-123" {
			t.Errorf("got %q, want %q", tok, "secret-token-123")
		}
	})

	t.Run("falls back to plaintext supabase.json when no .enc present", func(t *testing.T) {
		inner, _ := json.Marshal(map[string]string{"access_token": "plain-token-456"})
		supa, _ := json.Marshal(map[string]any{"workos_tokens": string(inner)})

		dir := t.TempDir()
		writeFile(t, dir, "supabase.json", supa)

		tok, err := AccessToken(dir)
		if err != nil {
			t.Fatalf("AccessToken: %v", err)
		}
		if tok != "plain-token-456" {
			t.Errorf("got %q, want %q", tok, "plain-token-456")
		}
	})

	t.Run("errors clearly when no supabase file exists", func(t *testing.T) {
		dir := t.TempDir()
		_, err := AccessToken(dir)
		if err == nil {
			t.Fatal("expected error when no supabase config is present")
		}
	})

	t.Run("errors when workos_tokens is absent", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "supabase.json", []byte(`{"session_id":"s1"}`))

		_, err := AccessToken(dir)
		if err == nil {
			t.Fatal("expected error when workos_tokens is missing")
		}
	})

	t.Run("errors when access_token is empty", func(t *testing.T) {
		inner, _ := json.Marshal(map[string]string{"token_type": "Bearer"}) // no access_token
		supa, _ := json.Marshal(map[string]any{"workos_tokens": string(inner)})

		dir := t.TempDir()
		writeFile(t, dir, "supabase.json", supa)

		_, err := AccessToken(dir)
		if err == nil {
			t.Fatal("expected error when access_token is empty")
		}
	})
}
