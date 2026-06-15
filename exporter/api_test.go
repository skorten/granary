package exporter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newMockGranola returns an httptest server that emulates the two Granola API
// endpoints granary uses, plus a record of the auth header it last saw.
func newMockGranola(t *testing.T, pages [][]map[string]any, transcripts map[string][]map[string]any) (*httptest.Server, *string) {
	t.Helper()
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/v2/get-documents":
			var req struct {
				Offset int `json:"offset"`
				Limit  int `json:"limit"`
			}
			json.Unmarshal(body, &req)
			page := req.Offset / max(req.Limit, 1)
			var docs []map[string]any
			if page < len(pages) {
				docs = pages[page]
			}
			json.NewEncoder(w).Encode(map[string]any{"docs": docs})
		case "/v1/get-document-transcript":
			var req struct {
				DocumentID string `json:"document_id"`
			}
			json.Unmarshal(body, &req)
			segs := transcripts[req.DocumentID]
			if segs == nil {
				segs = []map[string]any{}
			}
			json.NewEncoder(w).Encode(segs)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &lastAuth
}

func TestAPIClientFetchState(t *testing.T) {
	t.Run("paginates documents and fetches each transcript into a CacheState", func(t *testing.T) {
		pages := [][]map[string]any{
			{
				{"id": "d1", "title": "Standup", "created_at": "2026-05-20T10:00:00Z", "notes_markdown": ""},
				{"id": "d2", "title": "Planning", "created_at": "2026-05-21T11:00:00Z"},
			},
			{
				{"id": "d3", "title": "Retro", "created_at": "2026-05-22T12:00:00Z"},
			},
		}
		transcripts := map[string][]map[string]any{
			"d1": {
				{"text": "Hello team", "source": "microphone", "is_final": true},
				{"text": "Hi there", "source": "system", "is_final": true},
			},
			"d3": {
				{"text": "What went well", "source": "microphone", "is_final": true},
			},
			// d2 has no transcript.
		}
		srv, lastAuth := newMockGranola(t, pages, transcripts)

		client := &APIClient{
			BaseURL:    srv.URL,
			Token:      "tok-abc",
			Version:    "7.319.1",
			HTTPClient: srv.Client(),
			PageSize:   2,
		}

		state, err := client.FetchState()
		if err != nil {
			t.Fatalf("FetchState: %v", err)
		}

		if len(state.Documents) != 3 {
			t.Fatalf("expected 3 documents, got %d", len(state.Documents))
		}
		if state.Documents["d1"].Title != "Standup" {
			t.Errorf("d1 title = %q", state.Documents["d1"].Title)
		}
		if state.Documents["d2"].CreatedAt != "2026-05-21T11:00:00Z" {
			t.Errorf("d2 created_at = %q", state.Documents["d2"].CreatedAt)
		}
		if len(state.Transcripts["d1"]) != 2 {
			t.Errorf("expected 2 transcript entries for d1, got %d", len(state.Transcripts["d1"]))
		}
		if state.Transcripts["d1"][0].Text != "Hello team" || state.Transcripts["d1"][0].Source != "microphone" {
			t.Errorf("d1 first segment = %+v", state.Transcripts["d1"][0])
		}
		if len(state.Transcripts["d3"]) != 1 {
			t.Errorf("expected 1 transcript entry for d3, got %d", len(state.Transcripts["d3"]))
		}
		if _, ok := state.Transcripts["d2"]; ok && len(state.Transcripts["d2"]) != 0 {
			t.Errorf("d2 should have no transcript entries, got %d", len(state.Transcripts["d2"]))
		}

		if *lastAuth != "Bearer tok-abc" {
			t.Errorf("auth header = %q, want %q", *lastAuth, "Bearer tok-abc")
		}
	})

	t.Run("surfaces an actionable error on 401 (expired token)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)

		client := &APIClient{BaseURL: srv.URL, Token: "expired", Version: "7.319.1", HTTPClient: srv.Client(), PageSize: 10}
		_, err := client.FetchState()
		if err == nil {
			t.Fatal("expected error on 401")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "granola") {
			t.Errorf("error should mention Granola/open the app, got: %v", err)
		}
	})
}

func TestAPIClientPartialTranscriptFailure(t *testing.T) {
	t.Run("skips a document whose transcript fails and keeps the rest", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/get-documents":
				json.NewEncoder(w).Encode(map[string]any{"docs": []map[string]any{
					{"id": "d1", "title": "A", "created_at": "2026-05-20T10:00:00Z"},
					{"id": "d2", "title": "B", "created_at": "2026-05-21T10:00:00Z"},
					{"id": "d3", "title": "C", "created_at": "2026-05-22T10:00:00Z"},
				}})
			case "/v1/get-document-transcript":
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), "d2") {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
				json.NewEncoder(w).Encode([]map[string]any{{"text": "hi", "source": "system"}})
			}
		}))
		t.Cleanup(srv.Close)

		client := &APIClient{BaseURL: srv.URL, Token: "t", Version: "7", HTTPClient: srv.Client(), PageSize: 10}
		state, err := client.FetchState()
		if err != nil {
			t.Fatalf("FetchState should not fail on a single transcript error: %v", err)
		}
		if len(state.Documents) != 3 {
			t.Errorf("expected 3 documents, got %d", len(state.Documents))
		}
		if len(state.Transcripts["d1"]) != 1 || len(state.Transcripts["d3"]) != 1 {
			t.Errorf("expected d1 and d3 transcripts present")
		}
		if _, ok := state.Transcripts["d2"]; ok {
			t.Errorf("d2 transcript should have been skipped")
		}
	})

	t.Run("aborts when a transcript fetch returns 401 (token problem affects everything)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/get-documents":
				json.NewEncoder(w).Encode(map[string]any{"docs": []map[string]any{
					{"id": "d1", "title": "A", "created_at": "2026-05-20T10:00:00Z"},
				}})
			case "/v1/get-document-transcript":
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			}
		}))
		t.Cleanup(srv.Close)

		client := &APIClient{BaseURL: srv.URL, Token: "expired", Version: "7", HTTPClient: srv.Client(), PageSize: 10}
		_, err := client.FetchState()
		if err == nil {
			t.Fatal("expected FetchState to abort on a 401 during transcript fetch")
		}
		if !errors.Is(err, ErrUnauthorized) {
			t.Errorf("expected ErrUnauthorized, got: %v", err)
		}
	})
}
