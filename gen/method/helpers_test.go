package method

import (
	"testing"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestEncodeSourceInput(t *testing.T) {
	tests := []struct {
		name     string
		input    *notebooklmv1alpha1.SourceInput
		expected []interface{}
	}{
		{
			name: "Google Docs source",
			input: &notebooklmv1alpha1.SourceInput{
				SourceType: notebooklmv1alpha1.SourceType_SOURCE_TYPE_GOOGLE_DOCS,
				Url:        "https://docs.google.com/document/d/123",
			},
			expected: []interface{}{
				nil,
				nil,
				[]string{"https://docs.google.com/document/d/123"},
			},
		},
		{
			name: "YouTube video source",
			input: &notebooklmv1alpha1.SourceInput{
				SourceType:     notebooklmv1alpha1.SourceType_SOURCE_TYPE_YOUTUBE_VIDEO,
				YoutubeVideoId: "abc123",
			},
			expected: []interface{}{
				nil,
				nil,
				"abc123",
				nil,
				int(notebooklmv1alpha1.SourceType_SOURCE_TYPE_YOUTUBE_VIDEO),
			},
		},
		{
			name: "Text source",
			input: &notebooklmv1alpha1.SourceInput{
				Title:   "Test Document",
				Content: "This is test content",
			},
			expected: []interface{}{
				nil,
				[]string{
					"Test Document",
					"This is test content",
				},
				nil,
				2, // text source type
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeSourceInput(tt.input)
			if !compareInterfaces(result, tt.expected) {
				t.Errorf("encodeSourceInput() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEncodeProjectUpdates(t *testing.T) {
	tests := []struct {
		name     string
		input    *notebooklmv1alpha1.Project
		expected interface{}
	}{
		{
			name: "Update title only",
			input: &notebooklmv1alpha1.Project{
				Title: "New Title",
			},
			expected: map[string]interface{}{
				"title": "New Title",
			},
		},
		{
			name: "Update emoji only",
			input: &notebooklmv1alpha1.Project{
				Emoji: "📚",
			},
			expected: map[string]interface{}{
				"emoji": "📚",
			},
		},
		{
			name: "Update both title and emoji",
			input: &notebooklmv1alpha1.Project{
				Title: "New Title",
				Emoji: "📚",
			},
			expected: map[string]interface{}{
				"title": "New Title",
				"emoji": "📚",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeProjectUpdates(tt.input)
			if !compareInterfaces(result, tt.expected) {
				t.Errorf("encodeProjectUpdates() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEncodeShareSettings(t *testing.T) {
	tests := []struct {
		name     string
		input    *notebooklmv1alpha1.ShareSettings
		expected interface{}
	}{
		{
			name:     "Nil settings",
			input:    nil,
			expected: nil,
		},
		{
			name: "Public share with comments",
			input: &notebooklmv1alpha1.ShareSettings{
				IsPublic:        true,
				AllowComments:   true,
				AllowDownloads:  false,
			},
			expected: map[string]interface{}{
				"is_public":       true,
				"allow_comments":  true,
				"allow_downloads": false,
			},
		},
		{
			name: "Private share with allowed emails",
			input: &notebooklmv1alpha1.ShareSettings{
				IsPublic:       false,
				AllowedEmails:  []string{"user1@example.com", "user2@example.com"},
				AllowComments:  false,
				AllowDownloads: true,
			},
			expected: map[string]interface{}{
				"is_public":       false,
				"allowed_emails":  []string{"user1@example.com", "user2@example.com"},
				"allow_comments":  false,
				"allow_downloads": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeShareSettings(tt.input)
			if !compareInterfaces(result, tt.expected) {
				t.Errorf("encodeShareSettings() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEncodeFieldMask(t *testing.T) {
	tests := []struct {
		name     string
		input    *fieldmaskpb.FieldMask
		expected interface{}
	}{
		{
			name:     "Nil field mask",
			input:    nil,
			expected: nil,
		},
		{
			name: "Single path",
			input: &fieldmaskpb.FieldMask{
				Paths: []string{"title"},
			},
			expected: []string{"title"},
		},
		{
			name: "Multiple paths",
			input: &fieldmaskpb.FieldMask{
				Paths: []string{"title", "emoji", "content"},
			},
			expected: []string{"title", "emoji", "content"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeFieldMask(tt.input)
			if !compareInterfaces(result, tt.expected) {
				t.Errorf("encodeFieldMask() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEncodeGenerateAnswerSettings(t *testing.T) {
	tests := []struct {
		name     string
		input    *notebooklmv1alpha1.GenerateAnswerSettings
		expected interface{}
	}{
		{
			name:     "Nil settings",
			input:    nil,
			expected: nil,
		},
		{
			name: "Full settings",
			input: &notebooklmv1alpha1.GenerateAnswerSettings{
				MaxLength:      500,
				Temperature:    0.7,
				IncludeSources: true,
			},
			expected: map[string]interface{}{
				"max_length":      int32(500),
				"temperature":     float32(0.7),
				"include_sources": true,
			},
		},
		{
			name: "Zero values omitted",
			input: &notebooklmv1alpha1.GenerateAnswerSettings{
				IncludeSources: false,
			},
			expected: map[string]interface{}{
				"include_sources": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeGenerateAnswerSettings(tt.input)
			if !compareInterfaces(result, tt.expected) {
				t.Errorf("encodeGenerateAnswerSettings() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Helper function to compare interfaces recursively
func compareInterfaces(a, b interface{}) bool {
	switch va := a.(type) {
	case []interface{}:
		vb, ok := b.([]interface{})
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if !compareInterfaces(va[i], vb[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		vb, ok := b.(map[string]interface{})
		if !ok || len(va) != len(vb) {
			return false
		}
		for k, v := range va {
			if !compareInterfaces(v, vb[k]) {
				return false
			}
		}
		return true
	case []string:
		vb, ok := b.([]string)
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if va[i] != vb[i] {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}