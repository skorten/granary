package exporter

import (
	"fmt"
	"strings"
	"time"
)

// partialMarker is written immediately after the speaker prefix for transcript
// entries that are not yet final. Its absence in a file means every entry is
// final, i.e. the transcript is complete. The skip logic in incremental.go
// keys off this marker.
const partialMarker = "<!--granary:partial-->"

// FormatDocumentMarkdown formats a document and its transcript as markdown.
func FormatDocumentMarkdown(doc *Document, transcript []TranscriptEntry) string {
	var lines []string

	title := doc.Title
	if title == "" {
		title = "Untitled"
	}

	dateStr := FormatDate(doc.CreatedAt)

	lines = append(lines, fmt.Sprintf("# %s", title))
	lines = append(lines, fmt.Sprintf("Date: %s", dateStr))
	lines = append(lines, fmt.Sprintf("Meeting ID: %s", doc.ID))
	lines = append(lines, "")
	lines = append(lines, "---")
	lines = append(lines, "")

	// Add AI-generated notes if they exist
	notes := doc.GetNotes()
	hasNotes := notes != "" && strings.TrimSpace(notes) != ""

	if hasNotes {
		lines = append(lines, "## AI-Generated Notes")
		lines = append(lines, "")
		lines = append(lines, notes)
		lines = append(lines, "")
	}

	// Add transcript if it exists
	hasTranscript := len(transcript) > 0
	if hasTranscript {
		if hasNotes {
			lines = append(lines, "---")
			lines = append(lines, "")
		}
		lines = append(lines, "## Transcript")
		lines = append(lines, "")

		for _, entry := range transcript {
			text := strings.TrimSpace(entry.Text)
			if text == "" {
				continue
			}

			speaker := SourceToSpeaker(entry.Source)
			if entry.IsFinal {
				lines = append(lines, fmt.Sprintf("**%s:** %s", speaker, text))
			} else {
				lines = append(lines, fmt.Sprintf("**%s:** %s %s", speaker, partialMarker, text))
			}
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}

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

// FormatDate parses an ISO8601 timestamp and formats it as "YYYY-MM-DD HH:MM".
// Returns "Unknown date" if parsing fails.
func FormatDate(timestamp string) string {
	if t, ok := parseTimestamp(timestamp); ok {
		return t.Format("2006-01-02 15:04")
	}
	return "Unknown date"
}

// FormatDateForFilename parses an ISO8601 timestamp and formats it as "YYYY-MM-DD".
// Returns "unknown-date" if parsing fails.
func FormatDateForFilename(timestamp string) string {
	if t, ok := parseTimestamp(timestamp); ok {
		return t.Format("2006-01-02")
	}
	return "unknown-date"
}

// SourceToSpeaker maps a transcript source to a speaker label.
func SourceToSpeaker(source string) string {
	switch source {
	case "microphone":
		return "Me"
	case "system":
		return "Them"
	default:
		if source == "" {
			return "Unknown"
		}
		// Capitalize first letter
		return strings.ToUpper(source[:1]) + source[1:]
	}
}

// NumberWithCommas formats a number with thousand separators.
func NumberWithCommas(n int) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	var result strings.Builder
	remainder := len(str) % 3
	if remainder > 0 {
		result.WriteString(str[:remainder])
		if len(str) > remainder {
			result.WriteString(",")
		}
	}

	for i := remainder; i < len(str); i += 3 {
		result.WriteString(str[i : i+3])
		if i+3 < len(str) {
			result.WriteString(",")
		}
	}

	return result.String()
}
