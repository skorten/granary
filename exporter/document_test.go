package exporter

import (
	"testing"
)

func TestAllDocuments(t *testing.T) {
	t.Run("merges owned and shared documents", func(t *testing.T) {
		state := &CacheState{
			Documents: map[string]Document{
				"doc1": {ID: "doc1", Title: "Owned"},
			},
			SharedDocuments: map[string]Document{
				"doc2": {ID: "doc2", Title: "Shared"},
			},
		}

		all := state.AllDocuments()
		if len(all) != 2 {
			t.Errorf("Expected 2 documents, got %d", len(all))
		}
		if all["doc1"].Title != "Owned" {
			t.Error("Expected owned document in result")
		}
		if all["doc2"].Title != "Shared" {
			t.Error("Expected shared document in result")
		}
	})

	t.Run("owned documents take precedence over shared", func(t *testing.T) {
		state := &CacheState{
			Documents: map[string]Document{
				"doc1": {ID: "doc1", Title: "Owned Version"},
			},
			SharedDocuments: map[string]Document{
				"doc1": {ID: "doc1", Title: "Shared Version"},
			},
		}

		all := state.AllDocuments()
		if len(all) != 1 {
			t.Errorf("Expected 1 document (deduped), got %d", len(all))
		}
		if all["doc1"].Title != "Owned Version" {
			t.Errorf("Expected owned version to take precedence, got %q", all["doc1"].Title)
		}
	})

	t.Run("handles nil shared documents", func(t *testing.T) {
		state := &CacheState{
			Documents: map[string]Document{
				"doc1": {ID: "doc1", Title: "Owned"},
			},
		}

		all := state.AllDocuments()
		if len(all) != 1 {
			t.Errorf("Expected 1 document, got %d", len(all))
		}
	})
}
