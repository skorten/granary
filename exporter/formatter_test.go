package exporter

import (
	"strings"
	"testing"
)

func TestFormatDocumentMarkdown(t *testing.T) {
	t.Run("document with notes only", func(t *testing.T) {
		doc := &Document{
			ID:            "abc12345-1234-5678-9abc-def012345678",
			Title:         "Engineering Team Stand-Up",
			CreatedAt:     "2026-01-21T20:30:01.410Z",
			NotesMarkdown: "# Action Items\n\n- Follow up on project timeline",
		}

		result := FormatDocumentMarkdown(doc, nil)

		if !strings.Contains(result, "# Engineering Team Stand-Up") {
			t.Error("Expected title to be in output")
		}
		if !strings.Contains(result, "Date: 2026-01-21 20:30") {
			t.Error("Expected formatted date in output")
		}
		if !strings.Contains(result, "Meeting ID: abc12345-1234-5678-9abc-def012345678") {
			t.Error("Expected meeting ID in output")
		}
		if !strings.Contains(result, "## AI-Generated Notes") {
			t.Error("Expected AI-Generated Notes section")
		}
		if !strings.Contains(result, "Follow up on project timeline") {
			t.Error("Expected notes content in output")
		}
		if strings.Contains(result, "## Transcript") {
			t.Error("Should not have transcript section when no transcript")
		}
	})

	t.Run("document with transcript only", func(t *testing.T) {
		doc := &Document{
			ID:        "test-id",
			Title:     "Test Meeting",
			CreatedAt: "2026-01-21T10:00:00Z",
		}
		transcript := []TranscriptEntry{
			{Text: "Hello from system", Source: "system"},
			{Text: "Hello from mic", Source: "microphone"},
		}

		result := FormatDocumentMarkdown(doc, transcript)

		if !strings.Contains(result, "## Transcript") {
			t.Error("Expected Transcript section")
		}
		if !strings.Contains(result, "**Them:** Hello from system") {
			t.Error("Expected system entry as 'Them'")
		}
		if !strings.Contains(result, "**Me:** Hello from mic") {
			t.Error("Expected microphone entry as 'Me'")
		}
		if strings.Contains(result, "## AI-Generated Notes") {
			t.Error("Should not have notes section when no notes")
		}
	})

	t.Run("document with both notes and transcript", func(t *testing.T) {
		doc := &Document{
			ID:            "test-id",
			Title:         "Test Meeting",
			CreatedAt:     "2026-01-21T10:00:00Z",
			NotesMarkdown: "Some notes",
		}
		transcript := []TranscriptEntry{
			{Text: "Hello", Source: "microphone"},
		}

		result := FormatDocumentMarkdown(doc, transcript)

		if !strings.Contains(result, "## AI-Generated Notes") {
			t.Error("Expected AI-Generated Notes section")
		}
		if !strings.Contains(result, "## Transcript") {
			t.Error("Expected Transcript section")
		}
		// Check that separator exists between sections
		notesIdx := strings.Index(result, "## AI-Generated Notes")
		transcriptIdx := strings.Index(result, "## Transcript")
		separatorBetween := result[notesIdx:transcriptIdx]
		if !strings.Contains(separatorBetween, "---") {
			t.Error("Expected separator between notes and transcript")
		}
	})

	t.Run("source to speaker mapping", func(t *testing.T) {
		doc := &Document{
			ID:        "test",
			Title:     "Test",
			CreatedAt: "2026-01-21T10:00:00Z",
		}
		transcript := []TranscriptEntry{
			{Text: "From microphone", Source: "microphone"},
			{Text: "From system", Source: "system"},
		}

		result := FormatDocumentMarkdown(doc, transcript)

		if !strings.Contains(result, "**Me:** From microphone") {
			t.Error("Expected microphone to map to 'Me'")
		}
		if !strings.Contains(result, "**Them:** From system") {
			t.Error("Expected system to map to 'Them'")
		}
	})

	t.Run("skips empty transcript entries", func(t *testing.T) {
		doc := &Document{
			ID:        "test",
			Title:     "Test",
			CreatedAt: "2026-01-21T10:00:00Z",
		}
		transcript := []TranscriptEntry{
			{Text: "Valid entry", Source: "microphone"},
			{Text: "", Source: "system"},
			{Text: "   ", Source: "microphone"},
		}

		result := FormatDocumentMarkdown(doc, transcript)

		count := strings.Count(result, "**Me:**") + strings.Count(result, "**Them:**")
		if count != 1 {
			t.Errorf("Expected 1 transcript entry, got %d", count)
		}
	})

	t.Run("prefers notes_markdown over notes_plain", func(t *testing.T) {
		doc := &Document{
			ID:            "test",
			Title:         "Test",
			CreatedAt:     "2026-01-21T10:00:00Z",
			NotesMarkdown: "# Markdown Header",
			NotesPlain:    "Plain text version",
		}

		result := FormatDocumentMarkdown(doc, nil)

		if !strings.Contains(result, "# Markdown Header") {
			t.Error("Expected markdown notes to be used")
		}
		if strings.Contains(result, "Plain text version") {
			t.Error("Should not contain plain text when markdown exists")
		}
	})

	t.Run("falls back to notes_plain", func(t *testing.T) {
		doc := &Document{
			ID:         "test",
			Title:      "Test",
			CreatedAt:  "2026-01-21T10:00:00Z",
			NotesPlain: "Plain text fallback",
		}

		result := FormatDocumentMarkdown(doc, nil)

		if !strings.Contains(result, "Plain text fallback") {
			t.Error("Expected plain text fallback")
		}
	})

	t.Run("handles unknown source", func(t *testing.T) {
		doc := &Document{
			ID:        "test",
			Title:     "Test",
			CreatedAt: "2026-01-21T10:00:00Z",
		}
		transcript := []TranscriptEntry{
			{Text: "Hello", Source: "speaker1"},
		}

		result := FormatDocumentMarkdown(doc, transcript)

		if !strings.Contains(result, "**Speaker1:** Hello") {
			t.Error("Expected unknown source to be capitalized")
		}
	})

	t.Run("handles missing created_at", func(t *testing.T) {
		doc := &Document{
			ID:    "test",
			Title: "Test",
		}

		result := FormatDocumentMarkdown(doc, nil)

		if !strings.Contains(result, "Date: Unknown date") {
			t.Error("Expected 'Unknown date' for missing created_at")
		}
	})

	t.Run("handles empty title", func(t *testing.T) {
		doc := &Document{
			ID:        "test",
			CreatedAt: "2026-01-21T10:00:00Z",
		}

		result := FormatDocumentMarkdown(doc, nil)

		if !strings.Contains(result, "# Untitled") {
			t.Error("Expected 'Untitled' for empty title")
		}
	})
}

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

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		expected  string
	}{
		{
			name:      "ISO8601 with Z suffix",
			timestamp: "2026-01-21T20:30:01.410Z",
			expected:  "2026-01-21 20:30",
		},
		{
			name:      "ISO8601 without milliseconds",
			timestamp: "2026-01-21T20:30:01Z",
			expected:  "2026-01-21 20:30",
		},
		{
			name:      "empty string",
			timestamp: "",
			expected:  "Unknown date",
		},
		{
			name:      "malformed date",
			timestamp: "not-a-date",
			expected:  "Unknown date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDate(tt.timestamp)
			if result != tt.expected {
				t.Errorf("FormatDate(%q) = %q, want %q", tt.timestamp, result, tt.expected)
			}
		})
	}
}

func TestFormatDateForFilename(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		expected  string
	}{
		{
			name:      "valid timestamp",
			timestamp: "2026-01-21T20:30:01.410Z",
			expected:  "2026-01-21",
		},
		{
			name:      "empty string",
			timestamp: "",
			expected:  "unknown-date",
		},
		{
			name:      "malformed",
			timestamp: "invalid",
			expected:  "unknown-date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDateForFilename(tt.timestamp)
			if result != tt.expected {
				t.Errorf("FormatDateForFilename(%q) = %q, want %q", tt.timestamp, result, tt.expected)
			}
		})
	}
}

func TestSourceToSpeaker(t *testing.T) {
	tests := []struct {
		source   string
		expected string
	}{
		{"microphone", "Me"},
		{"system", "Them"},
		{"speaker1", "Speaker1"},
		{"", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			result := SourceToSpeaker(tt.source)
			if result != tt.expected {
				t.Errorf("SourceToSpeaker(%q) = %q, want %q", tt.source, result, tt.expected)
			}
		})
	}
}

func TestNumberWithCommas(t *testing.T) {
	tests := []struct {
		n        int
		expected string
	}{
		{0, "0"},
		{100, "100"},
		{1000, "1,000"},
		{10000, "10,000"},
		{100000, "100,000"},
		{1000000, "1,000,000"},
		{1234567, "1,234,567"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := NumberWithCommas(tt.n)
			if result != tt.expected {
				t.Errorf("NumberWithCommas(%d) = %q, want %q", tt.n, result, tt.expected)
			}
		})
	}
}
