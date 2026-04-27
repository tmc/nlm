package main

// JSON record types for list-style commands. Each mirrors the columns of
// the corresponding tab-separated table, with snake_case keys and
// RFC3339 timestamps. Empty optional fields are omitted so consumers can
// distinguish "field absent" from "field empty string".

type notebookListRecord struct {
	NotebookID  string `json:"notebook_id"`
	Title       string `json:"title"`
	Emoji       string `json:"emoji,omitempty"`
	SourceCount int    `json:"source_count"`
	LastUpdated string `json:"last_updated,omitempty"`
}

type sourceListRecord struct {
	SourceID    string `json:"source_id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	LastUpdated string `json:"last_updated,omitempty"`
}

type noteListRecord struct {
	NoteID         string `json:"note_id"`
	Title          string `json:"title"`
	ContentPreview string `json:"content_preview,omitempty"`
}

type featuredProjectRecord struct {
	ProjectID   string `json:"project_id"`
	Title       string `json:"title"`
	Emoji       string `json:"emoji,omitempty"`
	Description string `json:"description,omitempty"`
	SourceCount int    `json:"source_count"`
}

type artifactListRecord struct {
	ArtifactID  string `json:"artifact_id"`
	Type        string `json:"type"`
	State       string `json:"state"`
	SourceCount int    `json:"source_count"`
}

type audioOverviewRecord struct {
	AudioID string `json:"audio_id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
}

type videoOverviewRecord struct {
	VideoID string `json:"video_id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
}

type guidebookListRecord struct {
	GuidebookID string `json:"guidebook_id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
}

type chatConversationRecord struct {
	ConversationID string `json:"conversation_id"`
	MessageCount   int    `json:"message_count,omitempty"`
	Status         string `json:"status"`
	LastUpdated    string `json:"last_updated,omitempty"`
}

type labelListRecord struct {
	LabelID     string   `json:"label_id"`
	Name        string   `json:"name"`
	SourceCount int      `json:"source_count"`
	SourceIDs   []string `json:"source_ids,omitempty"`
}
