package exporter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GranolaSupportDir returns the macOS Granola application support directory.
func GranolaSupportDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, "Library", "Application Support", "Granola"), nil
}

// AccessToken recovers the Granola API access token from the given support
// directory. It prefers the encrypted supabase.json.enc (decrypted with the
// macOS Keychain DEK) and falls back to plaintext supabase.json. The token
// lives in the JSON-encoded "workos_tokens" string under "access_token".
func AccessToken(supportDir string) (string, error) {
	encPath := filepath.Join(supportDir, "supabase.json.enc")
	plainPath := filepath.Join(supportDir, "supabase.json")

	var data []byte
	if _, err := os.Stat(encPath); err == nil {
		data, err = decryptCache(encPath)
		if err != nil {
			return "", fmt.Errorf("decrypting supabase.json.enc: %w", err)
		}
	} else if b, err := os.ReadFile(plainPath); err == nil {
		data = b
	} else {
		return "", fmt.Errorf("couldn't find your Granola login on this Mac. Make sure the Granola app is installed and you've signed in at least once, then try again")
	}

	var outer struct {
		WorkosTokens string `json:"workos_tokens"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return "", fmt.Errorf("parsing supabase config: %w", err)
	}
	if outer.WorkosTokens == "" {
		return "", fmt.Errorf("no workos_tokens in Granola supabase config")
	}

	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(outer.WorkosTokens), &tokens); err != nil {
		return "", fmt.Errorf("parsing workos_tokens: %w", err)
	}
	if tokens.AccessToken == "" {
		return "", fmt.Errorf("no access_token in Granola credentials — open the Granola app to sign in")
	}
	return tokens.AccessToken, nil
}

// DefaultAPIBaseURL is Granola's private API host.
const DefaultAPIBaseURL = "https://api.granola.ai"

// fallbackClientVersion is sent when the installed Granola app version cannot be
// determined. Granola rejects requests lacking a recognized client version.
const fallbackClientVersion = "7.319.1"

// defaultPageSize is the number of documents requested per get-documents call.
const defaultPageSize = 100

// GranolaClientVersion returns the installed Granola app version (read from the
// app bundle's Info.plist), or a known-good fallback if it cannot be read.
func GranolaClientVersion() string {
	out, err := exec.Command(
		"/usr/bin/defaults", "read", "/Applications/Granola.app/Contents/Info", "CFBundleShortVersionString",
	).Output()
	if v := strings.TrimSpace(string(out)); err == nil && v != "" {
		return v
	}
	return fallbackClientVersion
}

// ErrUnauthorized indicates the access token was rejected (HTTP 401/403). It is
// fatal: a token problem affects every request, so callers should stop rather
// than skip individual items.
var ErrUnauthorized = errors.New("granola api: unauthorized (token missing or expired) — open the Granola app to refresh it")

// APIClient fetches documents and transcripts from Granola's private API.
// See the verified contract in the project memory / issue #13.
type APIClient struct {
	BaseURL  string
	Token    string
	Version  string // app client version sent in User-Agent / X-Client-Version
	PageSize int

	// HTTPClient is optional; if nil a default client is created once per
	// FetchState so connections are reused across the many transcript requests.
	HTTPClient *http.Client
}

// apiDocument is the subset of a get-documents result we care about. Granola's
// AI notes (notes_markdown / last_viewed_panel) are intentionally ignored: the
// goal is transcripts on disk, not notes.
type apiDocument struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}

// FetchState pulls all documents (paginated) and their transcripts from the API
// and returns them as a CacheState, which the existing Exporter can consume.
//
// A failure fetching one document's transcript is logged and skipped so a single
// transient error doesn't discard the whole run. Authentication failures
// (ErrUnauthorized) abort, since they affect every request.
func (c *APIClient) FetchState() (*CacheState, error) {
	pageSize := c.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}

	state := &CacheState{
		Documents:       make(map[string]Document),
		SharedDocuments: make(map[string]Document),
		Transcripts:     make(map[string][]TranscriptEntry),
	}

	// Page through documents.
	for offset := 0; ; offset += pageSize {
		body := fmt.Sprintf(`{"limit":%d,"offset":%d}`, pageSize, offset)
		raw, err := c.post("/v2/get-documents", body)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Docs []apiDocument `json:"docs"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("parsing get-documents response: %w", err)
		}
		for _, d := range resp.Docs {
			if d.ID == "" {
				continue
			}
			state.Documents[d.ID] = Document{ID: d.ID, Title: d.Title, CreatedAt: d.CreatedAt}
		}
		if len(resp.Docs) < pageSize {
			break
		}
	}

	// Fetch each document's transcript in a stable order for reproducible output.
	ids := make([]string, 0, len(state.Documents))
	for id := range state.Documents {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var skipped int
	total := len(ids)
	for i, id := range ids {
		fmt.Fprintf(os.Stderr, "\rDownloading transcripts... %d/%d", i+1, total)
		segs, err := c.fetchTranscript(id)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				fmt.Fprintln(os.Stderr)
				return nil, err
			}
			fmt.Fprintf(os.Stderr, "\nwarning: skipping transcript for %s: %v\n", id, err)
			skipped++
			continue
		}
		if len(segs) > 0 {
			state.Transcripts[id] = segs
		}
	}
	if total > 0 {
		fmt.Fprintln(os.Stderr) // finish the progress line
	}
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d transcript(s) could not be downloaded and were skipped\n", skipped)
	}

	return state, nil
}

// fetchTranscript retrieves and decodes one document's transcript.
func (c *APIClient) fetchTranscript(documentID string) ([]TranscriptEntry, error) {
	reqBody, err := json.Marshal(map[string]string{"document_id": documentID})
	if err != nil {
		return nil, err
	}
	raw, err := c.post("/v1/get-document-transcript", string(reqBody))
	if err != nil {
		return nil, err
	}
	var segs []TranscriptEntry
	if err := json.Unmarshal(raw, &segs); err != nil {
		return nil, fmt.Errorf("parsing transcript for %s: %w", documentID, err)
	}
	return segs, nil
}

// post issues a POST with the required Granola headers and returns the body,
// translating non-2xx statuses into actionable errors.
func (c *APIClient) post(path, jsonBody string) ([]byte, error) {
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequest("POST", c.BaseURL+path, bytes.NewReader([]byte(jsonBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Granola/"+c.Version)
	req.Header.Set("X-Client-Version", c.Version)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach Granola's servers. Check your internet connection and try again (details: %w)", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("Granola API returned %d for %s: %w", resp.StatusCode, path, ErrUnauthorized)
	case resp.StatusCode >= 300:
		preview := strings.TrimSpace(string(body))
		if len(preview) > 200 {
			preview = preview[:200]
		}
		return nil, fmt.Errorf("Granola API returned %d for %s: %s", resp.StatusCode, path, preview)
	}
	return body, nil
}
