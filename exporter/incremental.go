package exporter

import (
	"os"
	"strings"
	"time"
)

// recencyWindow bounds the "first look" for a document we have no file for. A
// meeting created within this window may still gain a transcript, so we fetch
// it; an older document with no file is assumed transcript-less and left alone
// rather than re-polled every run. Use `granary run --all` to force a re-fetch.
const recencyWindow = 7 * 24 * time.Hour

// shouldFetchTranscript reports whether FetchState should download the
// transcript for a document, given its expected output path and created_at.
//
//   - file exists, no partial marker -> complete, skip
//   - file exists, has partial marker -> still partial, fetch
//   - no file, meeting within recencyWindow -> new meeting, fetch
//   - no file, meeting older than recencyWindow -> assumed empty, skip
func shouldFetchTranscript(filePath, createdAt string, now time.Time) bool {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return withinRecencyWindow(createdAt, now)
	}
	return strings.Contains(string(content), partialMarker)
}

// withinRecencyWindow reports whether createdAt is within recencyWindow of now.
// An unparseable timestamp returns true so a document is never silently dropped.
func withinRecencyWindow(createdAt string, now time.Time) bool {
	t, ok := parseTimestamp(createdAt)
	if !ok {
		return true
	}
	return now.Sub(t) < recencyWindow
}

// hasExportedFiles reports whether dir contains at least one .md file. Used to
// detect a first/backfill run (empty or missing output dir).
func hasExportedFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			return true
		}
	}
	return false
}
