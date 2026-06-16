package exporter

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldFetchTranscript(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	old := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)

	t.Run("missing file, recent meeting -> fetch", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nope.md")
		if !shouldFetchTranscript(path, recent, now) {
			t.Error("recent meeting with no file should be fetched")
		}
	})

	t.Run("missing file, old meeting -> skip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nope.md")
		if shouldFetchTranscript(path, old, now) {
			t.Error("old meeting with no file should be skipped")
		}
	})

	t.Run("missing file, unparseable date -> fetch", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nope.md")
		if !shouldFetchTranscript(path, "garbage", now) {
			t.Error("unparseable date should fall back to fetch")
		}
	})

	t.Run("complete file -> skip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "doc.md")
		if err := os.WriteFile(path, []byte("## Transcript\n\n**Me:** all done\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if shouldFetchTranscript(path, old, now) {
			t.Error("file with no partial marker should be skipped")
		}
	})

	t.Run("partial file -> fetch", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "doc.md")
		if err := os.WriteFile(path, []byte("## Transcript\n\n**Me:** "+partialMarker+" mid-sentence\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if !shouldFetchTranscript(path, old, now) {
			t.Error("file with a partial marker should be fetched")
		}
	})
}

func TestHasExportedFiles(t *testing.T) {
	t.Run("missing dir -> false", func(t *testing.T) {
		if hasExportedFiles(filepath.Join(t.TempDir(), "absent")) {
			t.Error("absent dir should report no exported files")
		}
	})
	t.Run("empty dir -> false", func(t *testing.T) {
		if hasExportedFiles(t.TempDir()) {
			t.Error("empty dir should report no exported files")
		}
	})
	t.Run("dir with a .md -> true", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if !hasExportedFiles(dir) {
			t.Error("dir with a .md should report exported files")
		}
	})
}
