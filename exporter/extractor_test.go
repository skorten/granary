package exporter

import (
	"testing"
)

func TestExtractTranscriptFromMarkdown(t *testing.T) {
	t.Run("extracts transcript entries", func(t *testing.T) {
		content := `# Engineering Team Stand-Up
Date: 2026-01-21 20:30
Meeting ID: abc12345-1234-5678-9abc-def012345678

---

## AI-Generated Notes

Some notes here.

---

## Transcript

**Them:** Let's start with the first agenda item.

**Me:** Got it, thanks for the update.

**Them:** The meeting is scheduled for next week.

`
		result := ExtractTranscriptFromMarkdown(content)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if len(result) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(result))
		}
		if result[0].Text != "Let's start with the first agenda item." {
			t.Errorf("Unexpected text: %s", result[0].Text)
		}
		if result[0].Source != "system" {
			t.Errorf("Expected source 'system', got %s", result[0].Source)
		}
		if result[1].Text != "Got it, thanks for the update." {
			t.Errorf("Unexpected text: %s", result[1].Text)
		}
		if result[1].Source != "microphone" {
			t.Errorf("Expected source 'microphone', got %s", result[1].Source)
		}
	})

	t.Run("returns nil when no transcript section", func(t *testing.T) {
		content := `# Meeting Title
Date: 2026-01-21 14:30

## AI-Generated Notes

Just notes, no transcript.
`
		result := ExtractTranscriptFromMarkdown(content)

		if result != nil {
			t.Error("Expected nil when no transcript section")
		}
	})

	t.Run("returns nil for empty transcript section", func(t *testing.T) {
		content := `# Meeting Title

## Transcript

`
		result := ExtractTranscriptFromMarkdown(content)

		if result != nil {
			t.Error("Expected nil for empty transcript section")
		}
	})

	t.Run("roundtrip format then extract", func(t *testing.T) {
		doc := &Document{
			ID:        "test",
			Title:     "Test",
			CreatedAt: "2026-01-21T10:00:00Z",
		}
		originalTranscript := []TranscriptEntry{
			{Text: "Hello from me", Source: "microphone", IsFinal: true},
			{Text: "Hello from them", Source: "system", IsFinal: true},
		}

		formatted := FormatDocumentMarkdown(doc, originalTranscript)
		extracted := ExtractTranscriptFromMarkdown(formatted)

		if extracted == nil {
			t.Fatal("Expected non-nil result")
		}
		if len(extracted) != 2 {
			t.Fatalf("Expected 2 entries, got %d", len(extracted))
		}
		if extracted[0].Text != "Hello from me" {
			t.Errorf("Expected 'Hello from me', got %q", extracted[0].Text)
		}
		if extracted[0].Source != "microphone" {
			t.Errorf("Expected 'microphone', got %s", extracted[0].Source)
		}
		if extracted[1].Text != "Hello from them" {
			t.Errorf("Expected 'Hello from them', got %q", extracted[1].Text)
		}
		if extracted[1].Source != "system" {
			t.Errorf("Expected 'system', got %s", extracted[1].Source)
		}
	})

	t.Run("handles transcript with only one entry", func(t *testing.T) {
		content := `## Transcript

**Me:** Single entry.

`
		result := ExtractTranscriptFromMarkdown(content)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if len(result) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(result))
		}
	})

	t.Run("handles transcript with mixed speakers", func(t *testing.T) {
		content := `## Transcript

**Me:** First.

**Them:** Second.

**Me:** Third.

**Them:** Fourth.

`
		result := ExtractTranscriptFromMarkdown(content)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if len(result) != 4 {
			t.Errorf("Expected 4 entries, got %d", len(result))
		}
	})

	t.Run("handles transcript text with special characters", func(t *testing.T) {
		content := `## Transcript

**Me:** Hello! How are you? I'm doing great.

`
		result := ExtractTranscriptFromMarkdown(content)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result[0].Text != "Hello! How are you? I'm doing great." {
			t.Errorf("Unexpected text: %s", result[0].Text)
		}
	})

	t.Run("maps unknown speakers to lowercase source", func(t *testing.T) {
		content := `## Transcript

**Speaker1:** Hello from speaker1.

`
		result := ExtractTranscriptFromMarkdown(content)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result[0].Source != "speaker1" {
			t.Errorf("Expected 'speaker1', got %s", result[0].Source)
		}
	})
}

func TestSpeakerToSource(t *testing.T) {
	tests := []struct {
		speaker  string
		expected string
	}{
		{"Me", "microphone"},
		{"Them", "system"},
		{"Speaker1", "speaker1"},
		{"Unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.speaker, func(t *testing.T) {
			result := SpeakerToSource(tt.speaker)
			if result != tt.expected {
				t.Errorf("SpeakerToSource(%q) = %q, want %q", tt.speaker, result, tt.expected)
			}
		})
	}
}
