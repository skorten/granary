package exporter

// Document represents a meeting document from the Granola cache.
type Document struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	CreatedAt     string `json:"created_at"`
	NotesMarkdown string `json:"notes_markdown"`
	NotesPlain    string `json:"notes_plain"`
}

// TranscriptEntry represents a single transcript entry from the Granola cache.
type TranscriptEntry struct {
	ID             string `json:"id"`
	DocumentID     string `json:"document_id"`
	StartTimestamp string `json:"start_timestamp"`
	EndTimestamp   string `json:"end_timestamp"`
	Text           string `json:"text"`
	Source         string `json:"source"`
	IsFinal        bool   `json:"is_final"`
}

// CacheState holds the parsed state from the Granola cache.
type CacheState struct {
	Documents       map[string]Document          `json:"documents"`
	SharedDocuments map[string]Document          `json:"sharedDocuments"`
	Transcripts     map[string][]TranscriptEntry `json:"transcripts"`
}

// AllDocuments returns all documents (owned + shared) as a single map.
// Owned documents take precedence if a document ID appears in both.
func (s *CacheState) AllDocuments() map[string]Document {
	all := make(map[string]Document, len(s.Documents)+len(s.SharedDocuments))
	for id, doc := range s.SharedDocuments {
		all[id] = doc
	}
	for id, doc := range s.Documents {
		all[id] = doc
	}
	return all
}

// HasExportableContent returns true if the document has content worth exporting.
// A document is exportable if it has a transcript OR notes with more than 10 characters.
func (d *Document) HasExportableContent(transcripts map[string][]TranscriptEntry) bool {
	if _, hasTranscript := transcripts[d.ID]; hasTranscript {
		return true
	}
	if len(d.NotesMarkdown) > 10 {
		return true
	}
	if len(d.NotesPlain) > 10 {
		return true
	}
	return false
}

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

// GetNotes returns the best available notes content.
// Prefers notes_markdown, falls back to notes_plain.
func (d *Document) GetNotes() string {
	if d.NotesMarkdown != "" {
		return d.NotesMarkdown
	}
	return d.NotesPlain
}
