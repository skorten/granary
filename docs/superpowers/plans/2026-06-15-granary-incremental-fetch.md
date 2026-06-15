# Granary Incremental Fetch + Daily Schedule Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop re-downloading every transcript on every run; fetch only new and still-partial transcripts, and move the background job from every-2-hours to once-daily at a randomized early-morning time.

**Architecture:** Two independent levers. (1) `service/` emits a `StartCalendarInterval` plist at a per-user random time in `[00:00, 03:00)`, overridable with `granary install --at HH:MM`. (2) `exporter/` records completeness in the markdown (non-final entries get a `<!--granary:partial-->` marker; absence means complete) and `FetchState` uses on-disk files + a recency window to decide which transcripts to fetch. Files on disk remain the source of truth; no new state file.

**Tech Stack:** Go 1.25 standard library + `spf13/cobra`. Tests use `testing` + `net/http/httptest`. macOS launchd via `launchctl` (shelled out, not unit-tested).

**Spec:** `docs/superpowers/specs/2026-06-15-granary-incremental-fetch-design.md`

---

## File Structure

- `exporter/formatter.go` (modify) — add `parseTimestamp`, the `partialMarker` const, and partial-entry markers in transcript output.
- `exporter/incremental.go` (create) — `recencyWindow` const and the skip-decision helpers (`shouldFetchTranscript`, `withinRecencyWindow`, `hasExportedFiles`).
- `exporter/incremental_test.go` (create) — tests for the skip helpers.
- `exporter/document.go` (modify) — `allDocumentSlice` helper on `CacheState`.
- `exporter/exporter.go` (modify) — build the filename map over the full document set.
- `exporter/api.go` (modify) — `OutputDir`/`ForceAll` fields; wire the skip decision into `FetchState`; report already-saved count.
- `exporter/formatter_test.go`, `exporter/extractor_test.go` (modify) — set `IsFinal: true` on fixtures that assert un-marked transcript output.
- `service/service.go` (modify) — `StartCalendarInterval` plist, `parseAtTime`, `pickRandomTime`, updated `Install` signature and message.
- `service/service_test.go` (create) — tests for plist generation and time parsing.
- `main.go` (modify) — `--at` on `install`, `--all` on `run`/bare, pass `OutputDir`, update `printStatus`.
- `README.md`, `CLAUDE.md` (modify) — document new behavior; add the Granola-MCP comparison.

A note on commits: the repo convention is plain descriptive commit messages with **no** co-authoring/credit lines. Keep it that way.

---

### Task 1: Extract a shared timestamp parser

**Files:**
- Modify: `exporter/formatter.go`
- Test: `exporter/formatter_test.go`

- [ ] **Step 1: Write the failing test**

Add to `exporter/formatter_test.go`:

```go
func TestParseTimestamp(t *testing.T) {
	if _, ok := parseTimestamp(""); ok {
		t.Error("empty string should not parse")
	}
	if _, ok := parseTimestamp("not-a-date"); ok {
		t.Error("garbage should not parse")
	}
	got, ok := parseTimestamp("2026-01-21T20:30:01.410Z")
	if !ok {
		t.Fatal("expected RFC3339-with-millis to parse")
	}
	if got.Year() != 2026 || got.Month() != 1 || got.Day() != 21 {
		t.Errorf("parsed wrong date: %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exporter -run TestParseTimestamp -v`
Expected: FAIL — `undefined: parseTimestamp`.

- [ ] **Step 3: Add `parseTimestamp` and refactor the two formatters to use it**

In `exporter/formatter.go`, add the helper:

```go
// parseTimestamp parses an ISO8601/RFC3339 timestamp using the formats Granola
// emits. The bool reports whether parsing succeeded.
func parseTimestamp(timestamp string) (time.Time, bool) {
	if timestamp == "" {
		return time.Time{}, false
	}
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, timestamp); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
```

Replace the body of `FormatDate` with:

```go
func FormatDate(timestamp string) string {
	if t, ok := parseTimestamp(timestamp); ok {
		return t.Format("2006-01-02 15:04")
	}
	return "Unknown date"
}
```

Replace the body of `FormatDateForFilename` with:

```go
func FormatDateForFilename(timestamp string) string {
	if t, ok := parseTimestamp(timestamp); ok {
		return t.Format("2006-01-02")
	}
	return "unknown-date"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./exporter -run 'TestParseTimestamp|TestFormatDate|TestFormatDateForFilename' -v`
Expected: PASS (the existing `TestFormatDate`/`TestFormatDateForFilename` still pass against the refactor).

- [ ] **Step 5: Commit**

```bash
git add exporter/formatter.go exporter/formatter_test.go
git commit -m "Extract shared timestamp parser in exporter"
```

---

### Task 2: Mark non-final transcript entries in the markdown

**Files:**
- Modify: `exporter/formatter.go`
- Test: `exporter/formatter_test.go`, `exporter/extractor_test.go`

- [ ] **Step 1: Write the failing test**

Add to `exporter/formatter_test.go`:

```go
func TestFormatDocumentMarkdownPartialMarker(t *testing.T) {
	doc := &Document{ID: "x", Title: "T", CreatedAt: "2026-01-21T10:00:00Z"}
	transcript := []TranscriptEntry{
		{Text: "done speaking", Source: "microphone", IsFinal: true},
		{Text: "still typing", Source: "system", IsFinal: false},
	}

	result := FormatDocumentMarkdown(doc, transcript)

	if !strings.Contains(result, "**Me:** done speaking") {
		t.Error("final entry should have no marker")
	}
	if strings.Contains(result, "**Me:** "+partialMarker) {
		t.Error("final entry must not be marked partial")
	}
	if !strings.Contains(result, "**Them:** "+partialMarker+" still typing") {
		t.Errorf("non-final entry should carry the partial marker, got:\n%s", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exporter -run TestFormatDocumentMarkdownPartialMarker -v`
Expected: FAIL — `undefined: partialMarker`.

- [ ] **Step 3: Add the marker const and emit it for non-final entries**

In `exporter/formatter.go`, add near the top (after the imports):

```go
// partialMarker is written immediately after the speaker prefix for transcript
// entries that are not yet final. Its absence in a file means every entry is
// final, i.e. the transcript is complete. The skip logic in incremental.go
// keys off this marker.
const partialMarker = "<!--granary:partial-->"
```

In `FormatDocumentMarkdown`, replace the transcript loop body line that appends the entry:

```go
			speaker := SourceToSpeaker(entry.Source)
			lines = append(lines, fmt.Sprintf("**%s:** %s", speaker, text))
			lines = append(lines, "")
```

with:

```go
			speaker := SourceToSpeaker(entry.Source)
			if entry.IsFinal {
				lines = append(lines, fmt.Sprintf("**%s:** %s", speaker, text))
			} else {
				lines = append(lines, fmt.Sprintf("**%s:** %s %s", speaker, partialMarker, text))
			}
			lines = append(lines, "")
```

- [ ] **Step 4: Update existing fixtures that assert un-marked output**

These fixtures construct entries without `IsFinal`, which now renders as partial and breaks their `Contains` assertions. Set `IsFinal: true` (they represent finalized transcripts).

In `exporter/formatter_test.go`, subtest **"document with transcript only"**:

```go
		transcript := []TranscriptEntry{
			{Text: "Hello from system", Source: "system", IsFinal: true},
			{Text: "Hello from mic", Source: "microphone", IsFinal: true},
		}
```

Subtest **"source to speaker mapping"**:

```go
		transcript := []TranscriptEntry{
			{Text: "From microphone", Source: "microphone", IsFinal: true},
			{Text: "From system", Source: "system", IsFinal: true},
		}
```

Subtest **"handles unknown source"**:

```go
		transcript := []TranscriptEntry{
			{Text: "Hello", Source: "speaker1", IsFinal: true},
		}
```

In `exporter/extractor_test.go`, subtest **"roundtrip format then extract"**:

```go
		originalTranscript := []TranscriptEntry{
			{Text: "Hello from me", Source: "microphone", IsFinal: true},
			{Text: "Hello from them", Source: "system", IsFinal: true},
		}
```

- [ ] **Step 5: Run the formatter and extractor tests**

Run: `go test ./exporter -run 'TestFormatDocumentMarkdown|TestExtractTranscriptFromMarkdown' -v`
Expected: PASS (including the new partial-marker test).

- [ ] **Step 6: Commit**

```bash
git add exporter/formatter.go exporter/formatter_test.go exporter/extractor_test.go
git commit -m "Mark non-final transcript entries with a partial marker"
```

---

### Task 3: Skip-decision helpers

**Files:**
- Create: `exporter/incremental.go`
- Test: `exporter/incremental_test.go`

- [ ] **Step 1: Write the failing test**

Create `exporter/incremental_test.go`:

```go
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
		os.WriteFile(path, []byte("## Transcript\n\n**Me:** all done\n"), 0600)
		if shouldFetchTranscript(path, old, now) {
			t.Error("file with no partial marker should be skipped")
		}
	})

	t.Run("partial file -> fetch", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "doc.md")
		os.WriteFile(path, []byte("## Transcript\n\n**Me:** "+partialMarker+" mid-sentence\n"), 0600)
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
		os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0600)
		if !hasExportedFiles(dir) {
			t.Error("dir with a .md should report exported files")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exporter -run 'TestShouldFetchTranscript|TestHasExportedFiles' -v`
Expected: FAIL — `undefined: shouldFetchTranscript`, `undefined: hasExportedFiles`.

- [ ] **Step 3: Implement the helpers**

Create `exporter/incremental.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./exporter -run 'TestShouldFetchTranscript|TestHasExportedFiles' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add exporter/incremental.go exporter/incremental_test.go
git commit -m "Add transcript skip-decision helpers"
```

---

### Task 4: Build the filename map over the full document set

**Files:**
- Modify: `exporter/document.go`, `exporter/exporter.go`
- Test: `exporter/exporter_test.go`

**Why:** the skip check (Task 5) computes a document's expected filename to look for its file. The writer must compute the *same* filename. Today `Export` maps only over the exportable subset, so a new same-title/same-day meeting could be assigned a different name than the skip check expects. Computing over all documents makes both agree and guarantees a new colliding meeting still gets a unique, fetchable name.

- [ ] **Step 1: Write the failing test**

Add to `exporter/exporter_test.go`:

```go
func TestExportFilenameMapUsesAllDocuments(t *testing.T) {
	// Two documents share title+date; only one is exportable (has a transcript).
	// Mapping over all documents must still give the exportable one an
	// ID-suffixed (collision) name, so it lines up with the skip check.
	tmpDir := t.TempDir()
	exp := NewExporter(tmpDir)

	state := &CacheState{
		Documents: map[string]Document{
			"aaaaaaaa-0000": {ID: "aaaaaaaa-0000", Title: "Sync", CreatedAt: "2026-01-21T10:00:00Z"},
			"bbbbbbbb-1111": {ID: "bbbbbbbb-1111", Title: "Sync", CreatedAt: "2026-01-21T15:00:00Z"},
		},
		Transcripts: map[string][]TranscriptEntry{
			"aaaaaaaa-0000": {{Text: "hi", Source: "microphone", IsFinal: true}},
		},
	}

	if _, err := exp.Export(state, false); err != nil {
		t.Fatalf("Export: %v", err)
	}

	want := "2026-01-21_Sync (aaaaaaaa).md"
	if _, err := os.Stat(filepath.Join(tmpDir, want)); err != nil {
		entries, _ := os.ReadDir(tmpDir)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected collision-suffixed file %q; got %v", want, names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exporter -run TestExportFilenameMapUsesAllDocuments -v`
Expected: FAIL — the file is written as `2026-01-21_Sync.md` (no suffix), because the map is built over the single exportable doc.

- [ ] **Step 3: Add the helper and switch `Export` to the full set**

In `exporter/document.go`, add:

```go
// allDocumentSlice returns every document (owned + shared) as a slice. Used to
// compute filenames over the full set so the writer and the fetch skip-check
// agree on each document's filename.
func (s *CacheState) allDocumentSlice() []Document {
	all := s.AllDocuments()
	docs := make([]Document, 0, len(all))
	for _, d := range all {
		docs = append(docs, d)
	}
	return docs
}
```

In `exporter/exporter.go`, inside `Export`, replace:

```go
	// Build filename map: assign unique filenames using document ID for collisions
	filenameMap := buildFilenameMap(exportable)
```

with:

```go
	// Build filename map over ALL documents (not just exportable ones) so the
	// names match what FetchState's skip-check computes.
	filenameMap := buildFilenameMap(state.allDocumentSlice())
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./exporter -run TestExport -v`
Expected: PASS (new test plus all existing `TestExport*` subtests).

- [ ] **Step 5: Commit**

```bash
git add exporter/document.go exporter/exporter.go exporter/exporter_test.go
git commit -m "Compute export filenames over the full document set"
```

---

### Task 5: Wire the skip decision into FetchState

**Files:**
- Modify: `exporter/api.go`
- Test: `exporter/api_test.go`

- [ ] **Step 1: Write the failing test**

Add to `exporter/api_test.go`:

```go
// recordingGranola is like newMockGranola but records which document_ids had
// their transcript requested.
func recordingGranola(t *testing.T, docs []map[string]any, transcripts map[string][]map[string]any) (*httptest.Server, *[]string) {
	t.Helper()
	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/v2/get-documents":
			var req struct {
				Offset int `json:"offset"`
			}
			json.Unmarshal(body, &req)
			if req.Offset > 0 {
				json.NewEncoder(w).Encode(map[string]any{"docs": []map[string]any{}})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"docs": docs})
		case "/v1/get-document-transcript":
			var req struct {
				DocumentID string `json:"document_id"`
			}
			json.Unmarshal(body, &req)
			requested = append(requested, req.DocumentID)
			segs := transcripts[req.DocumentID]
			if segs == nil {
				segs = []map[string]any{}
			}
			json.NewEncoder(w).Encode(segs)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &requested
}

func TestFetchStateSkipsExisting(t *testing.T) {
	// d1 already saved (complete file on disk), d2 brand new today.
	today := time.Now().UTC().Format(time.RFC3339)
	docs := []map[string]any{
		{"id": "d1", "title": "Old", "created_at": "2026-01-01T10:00:00Z"},
		{"id": "d2", "title": "New", "created_at": today},
	}
	transcripts := map[string][]map[string]any{
		"d1": {{"text": "hi", "source": "microphone", "is_final": true}},
		"d2": {{"text": "yo", "source": "microphone", "is_final": true}},
	}
	srv, requested := recordingGranola(t, docs, transcripts)

	outDir := t.TempDir()
	// Pre-create d1's complete file (no partial marker) under its mapped name.
	os.WriteFile(filepath.Join(outDir, "2026-01-01_Old.md"),
		[]byte("## Transcript\n\n**Me:** hi\n"), 0600)

	client := &APIClient{
		BaseURL:    srv.URL,
		Token:      "t",
		Version:    "7",
		HTTPClient: srv.Client(),
		PageSize:   100,
		OutputDir:  outDir,
	}

	if _, err := client.FetchState(); err != nil {
		t.Fatalf("FetchState: %v", err)
	}

	if len(*requested) != 1 || (*requested)[0] != "d2" {
		t.Errorf("expected only d2 to be fetched, got %v", *requested)
	}
}

func TestFetchStateForceAllFetchesEverything(t *testing.T) {
	docs := []map[string]any{
		{"id": "d1", "title": "Old", "created_at": "2026-01-01T10:00:00Z"},
	}
	transcripts := map[string][]map[string]any{
		"d1": {{"text": "hi", "source": "microphone", "is_final": true}},
	}
	srv, requested := recordingGranola(t, docs, transcripts)

	outDir := t.TempDir()
	os.WriteFile(filepath.Join(outDir, "2026-01-01_Old.md"),
		[]byte("## Transcript\n\n**Me:** hi\n"), 0600)

	client := &APIClient{
		BaseURL: srv.URL, Token: "t", Version: "7", HTTPClient: srv.Client(),
		PageSize: 100, OutputDir: outDir, ForceAll: true,
	}
	if _, err := client.FetchState(); err != nil {
		t.Fatalf("FetchState: %v", err)
	}
	if len(*requested) != 1 {
		t.Errorf("ForceAll should fetch d1 despite its file, got %v", *requested)
	}
}
```

Add `"os"`, `"path/filepath"`, and `"time"` to the `import` block of `exporter/api_test.go` if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exporter -run 'TestFetchStateSkipsExisting|TestFetchStateForceAllFetchesEverything' -v`
Expected: FAIL — `unknown field 'OutputDir'` / `unknown field 'ForceAll'`.

- [ ] **Step 3: Add fields and wire the decision into FetchState**

In `exporter/api.go`, add fields to `APIClient`:

```go
	// OutputDir, when set, enables incremental fetch: transcripts already saved
	// (complete) on disk are not re-downloaded. Empty disables the skip logic
	// (fetch everything) — used by tests.
	OutputDir string
	// ForceAll bypasses the skip logic and fetches every transcript.
	ForceAll bool
```

In `FetchState`, replace the block that builds `ids` and runs the fetch loop (from `// Fetch each document's transcript in a stable order...` through the end of that loop, i.e. the current `api.go:163-193`) with:

```go
	// Decide which documents actually need a transcript fetch. This keeps the
	// daily run bounded to new and still-partial transcripts instead of
	// re-downloading the whole corpus every time.
	filenameMap := buildFilenameMap(state.allDocumentSlice())
	backfill := c.ForceAll || c.OutputDir == "" || !hasExportedFiles(c.OutputDir)
	now := time.Now()

	ids := make([]string, 0, len(state.Documents))
	for id := range state.Documents {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var toFetch []string
	var alreadyComplete int
	for _, id := range ids {
		if backfill {
			toFetch = append(toFetch, id)
			continue
		}
		filePath := filepath.Join(c.OutputDir, filenameMap[id])
		if shouldFetchTranscript(filePath, state.Documents[id].CreatedAt, now) {
			toFetch = append(toFetch, id)
		} else {
			alreadyComplete++
		}
	}

	var skipped int
	total := len(toFetch)
	for i, id := range toFetch {
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
	if alreadyComplete > 0 {
		fmt.Fprintf(os.Stderr, "%d transcript(s) already saved (skipped download)\n", alreadyComplete)
	}
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d transcript(s) could not be downloaded and were skipped\n", skipped)
	}

	return state, nil
```

(`filepath` and `time` are already imported in `api.go`.)

- [ ] **Step 4: Run the full exporter suite**

Run: `go test ./exporter -v`
Expected: PASS — new skip tests plus all existing tests (which use empty `OutputDir`, so they still fetch everything).

- [ ] **Step 5: Commit**

```bash
git add exporter/api.go exporter/api_test.go
git commit -m "Skip already-saved transcripts in FetchState"
```

---

### Task 6: Daily schedule in the LaunchAgent plist

**Files:**
- Modify: `service/service.go`
- Test: `service/service_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `service/service_test.go`:

```go
package service

import (
	"strings"
	"testing"
)

func TestParseAtTime(t *testing.T) {
	ok := []struct {
		in   string
		h, m int
	}{
		{"00:00", 0, 0}, {"2:30", 2, 30}, {"23:59", 23, 59}, {"09:05", 9, 5},
	}
	for _, c := range ok {
		h, m, err := parseAtTime(c.in)
		if err != nil || h != c.h || m != c.m {
			t.Errorf("parseAtTime(%q) = %d,%d,%v; want %d,%d,nil", c.in, h, m, err, c.h, c.m)
		}
	}
	bad := []string{"", "2", "24:00", "12:60", "-1:00", "aa:bb", "1:2:3"}
	for _, in := range bad {
		if _, _, err := parseAtTime(in); err == nil {
			t.Errorf("parseAtTime(%q) should have errored", in)
		}
	}
}

func TestPickRandomTime(t *testing.T) {
	for i := 0; i < 200; i++ {
		h, m := pickRandomTime()
		if h < 0 || h > 2 || m < 0 || m > 59 {
			t.Fatalf("pickRandomTime() = %d:%d, out of [00:00,03:00)", h, m)
		}
	}
}

func TestGeneratePlistCalendarInterval(t *testing.T) {
	p := generatePlist("/usr/local/bin/granary", 2, 30)
	if strings.Contains(p, "StartInterval") {
		t.Error("plist should no longer use StartInterval")
	}
	if !strings.Contains(p, "StartCalendarInterval") {
		t.Error("plist should use StartCalendarInterval")
	}
	for _, want := range []string{
		"<key>Hour</key>", "<integer>2</integer>",
		"<key>Minute</key>", "<integer>30</integer>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q:\n%s", want, p)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./service -v`
Expected: FAIL — `undefined: parseAtTime`, `undefined: pickRandomTime`, and `generatePlist` arity mismatch.

- [ ] **Step 3: Implement the schedule changes**

In `service/service.go`, update imports to include `"math/rand/v2"` and `"strconv"`.

Replace `generatePlist` with:

```go
func generatePlist(binaryPath string, hour, minute int) string {
	logDir := LogDir()
	stdoutLog := filepath.Join(logDir, "stdout.log")
	stderrLog := filepath.Join(logDir, "stderr.log")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>run</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key>
        <integer>%d</integer>
        <key>Minute</key>
        <integer>%d</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
    </dict>
</dict>
</plist>`, Label, binaryPath, hour, minute, stdoutLog, stderrLog)
}

// pickRandomTime returns a per-user random daily run time in [00:00, 03:00).
// Randomizing across installs avoids a synchronized spike against Granola's API.
func pickRandomTime() (int, int) {
	return rand.IntN(3), rand.IntN(60)
}

// parseAtTime parses an "HH:MM" 24-hour time.
func parseAtTime(at string) (int, int, error) {
	parts := strings.Split(at, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q: use HH:MM (24-hour), e.g. 02:30", at)
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid time %q: use HH:MM (24-hour), e.g. 02:30", at)
	}
	return hour, minute, nil
}
```

Change `Install`'s signature and the plist-generation/message section. Replace:

```go
func Install(force bool) error {
```

with:

```go
func Install(force bool, at string) error {
```

Immediately after the `binaryPath` is resolved (after the `os.Executable()` block), add:

```go
	var hour, minute int
	if at == "" {
		hour, minute = pickRandomTime()
	} else {
		var err error
		hour, minute, err = parseAtTime(at)
		if err != nil {
			return err
		}
	}
```

Replace `content := generatePlist(binaryPath)` with:

```go
	content := generatePlist(binaryPath, hour, minute)
```

Replace the success message block:

```go
	fmt.Println("Done. Granary will now back up your Granola transcripts automatically")
	fmt.Println("every 2 hours, in the background, while your Mac is on.")
```

with:

```go
	fmt.Printf("Done. Granary will back up your Granola transcripts automatically\n")
	fmt.Printf("once a day at %02d:%02d, in the background, while your Mac is on.\n", hour, minute)
	fmt.Println("(If your Mac is asleep at that time, it runs at the next wake.)")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./service -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add service/service.go service/service_test.go
git commit -m "Schedule the LaunchAgent once daily at a randomized time"
```

---

### Task 7: CLI wiring in main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add the `--all` flag and pass it through `runExport`**

Change `runExport`'s signature:

```go
func runExport(outputDir string, openAfter bool, forceAll bool) error {
```

Inside `runExport`, set the new client fields:

```go
	client := &exporter.APIClient{
		BaseURL:   exporter.DefaultAPIBaseURL,
		Token:     token,
		Version:   exporter.GranolaClientVersion(),
		OutputDir: outputDir,
		ForceAll:  forceAll,
	}
```

- [ ] **Step 2: Declare the flag var and bind it; update both call sites**

In `main`, add near `var outputDir string`:

```go
	var forceAll bool
```

Update the bare root `RunE`:

```go
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(resolveOutputDir(outputDir), openAfter, forceAll)
		},
```

After the `rootCmd.PersistentFlags()` lines, add:

```go
	rootCmd.Flags().BoolVar(&forceAll, "all", false, "Re-download every transcript, ignoring what's already saved")
```

Update the `run` command's `RunE` and add its flag:

```go
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Download and export your transcripts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(resolveOutputDir(outputDir), openAfter, forceAll)
		},
	}
	runCmd.Flags().BoolVar(&forceAll, "all", false, "Re-download every transcript, ignoring what's already saved")
	rootCmd.AddCommand(runCmd)
```

- [ ] **Step 3: Add the `--at` flag to `install` and pass it to `service.Install`**

Replace the `install` command block with:

```go
	// install
	var force bool
	var atTime string
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Set up automatic daily exports (macOS background task)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.Install(force, atTime)
		},
	}
	installCmd.Flags().BoolVar(&force, "force", false, "Replace an existing background task")
	installCmd.Flags().StringVar(&atTime, "at", "", "Daily run time as HH:MM, 24-hour (default: a random time between 00:00 and 03:00)")
	rootCmd.AddCommand(installCmd)
```

- [ ] **Step 4: Update `printStatus` wording**

Replace the `printStatus` body's two installed cases:

```go
	case installed && running:
		fmt.Println("Automatic exports: ON — Granary backs up your transcripts once a day.")
	case installed:
		fmt.Println("Automatic exports: set up but not currently running (it runs once a day).")
```

- [ ] **Step 5: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: no output (success).

- [ ] **Step 6: Cross-compile for the Linux CI target**

Run: `GOOS=linux GOARCH=amd64 go build ./...`
Expected: success (the macOS-only code must still compile on Linux).

- [ ] **Step 7: Commit**

```bash
git add main.go
git commit -m "Add --all (force re-fetch) and install --at flags"
```

---

### Task 8: Documentation

**Files:**
- Modify: `README.md`, `CLAUDE.md`

- [ ] **Step 1: Update the README schedule/usage sections**

In `README.md`, find the text describing automatic exports "every 2 hours" and update it to describe the once-daily behavior. Ensure the README covers:

- Automatic exports now run **once a day** at a per-user random time between 00:00 and 03:00 local; if the Mac is asleep then, the run happens at the next wake.
- Setting a specific time: `granary install --at 02:30` (24-hour HH:MM).
- Pulling immediately instead of waiting: just run `granary` by hand. The next scheduled run will complete anything that was still in progress.
- Forcing a full re-download of every transcript: `granary run --all`.
- Why incremental: granary only downloads new and still-in-progress transcripts, so it stays light on Granola's API and on your machine.

- [ ] **Step 2: Add the "Why granary instead of the Granola MCP?" section**

Add this section to `README.md` (place it near the top, after the intro/overview):

```markdown
## Why granary instead of the Granola MCP?

Granola offers an MCP server that lets an AI assistant query your meetings and
transcripts on demand. That is great for asking questions in the moment, but it
is a different tool for a different job:

| | Granola MCP | granary |
|---|---|---|
| Where your transcripts live | In Granola's cloud; fetched per request | Plain `.md` files on your own disk |
| Reading them | Through an AI assistant, online | Any tool — grep, an editor, your own scripts, any LLM |
| Cost to read | Spends AI tokens/round-trips each time | Free; they're just local files |
| Works offline | No | Yes, once exported |
| If you leave Granola | Access goes away | You keep the archive |

Use the MCP when you want to *ask Granola questions live*. Use granary when you
want to *own a durable, local, plain-text copy* of your transcripts that any
tool can read without ongoing AI cost. The two are complementary — granary is
your backup and your data, on your terms.
```

- [ ] **Step 3: Update CLAUDE.md**

In `CLAUDE.md`, update the architecture/service notes:
- Replace the `service/` description "Generates a plist (`com.skorten.granary`) with `StartInterval 7200` (2 hours)." with a description of the once-daily `StartCalendarInterval` at a randomized `[00:00, 03:00)` time, overridable via `install --at HH:MM`.
- In the API client section, note that `FetchState` now skips transcripts already saved on disk (incremental fetch) unless `ForceAll` / `run --all` is set or it's a backfill (empty output dir), bounded for the missing-file case by `recencyWindow` (7 days).
- In the markdown export section, note the `<!--granary:partial-->` marker for non-final entries (absence = complete) and that it drives the skip decision.

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "Document daily schedule, incremental fetch, and MCP comparison"
```

---

### Task 9: Final verification

**Files:** none (verification only)

- [ ] **Step 1: Run the entire test suite**

Run: `go test ./...`
Expected: PASS for all packages.

- [ ] **Step 2: Build and cross-compile**

Run: `go build ./... && GOOS=linux GOARCH=amd64 go build ./...`
Expected: success on both.

- [ ] **Step 3: Real end-to-end check (no data mutation)**

Run: `go run .`
Expected: against your real Granola account, the run reports a large "already saved (skipped download)" count and downloads only new/partial transcripts; your existing files are untouched. This is the proof the skip logic works; do not edit the real corpus to test.

---

## Self-Review

**Spec coverage:**
- Daily randomized schedule + `--at` override → Task 6, Task 7. ✓
- launchd wake-coalescing note → Task 6 message, Task 8 README. ✓
- Completeness marker (absence = complete) → Task 2. ✓
- `OutputDir`/`ForceAll` fields + skip decision table → Task 3, Task 5. ✓
- Full-set filename consistency → Task 4. ✓
- Recency window for missing-file case → Task 3. ✓
- Backfill on empty/missing dir + `run --all` escape hatch → Task 5, Task 7. ✓
- Reporting "already saved (skipped)" + progress counts only fetched → Task 5. ✓
- README (schedule, --at, manual run, --all, MCP comparison) + CLAUDE.md → Task 8. ✓
- Version bump deferred to release time → out of scope (noted in spec). ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code. README Step 1 is a guided edit (existing README text not quoted) because its current wording is not reproduced here — the required content points are explicit.

**Type consistency:** `partialMarker` (Task 2) consumed by Task 3/5; `parseTimestamp` (Task 1) consumed by Task 3; `shouldFetchTranscript`/`hasExportedFiles` (Task 3) consumed by Task 5; `allDocumentSlice` (Task 4) consumed by Task 5; `generatePlist(binaryPath, hour, minute)` / `Install(force, at)` (Task 6) consumed by Task 7. Signatures match across tasks. ✓
