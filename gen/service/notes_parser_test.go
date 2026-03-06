package service

import (
	"testing"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestParseNoteEntry(t *testing.T) {
	tests := []struct {
		name        string
		entry       interface{}
		wantNil     bool
		wantID      string
		wantTitle   string
		wantContent string
	}{
		{
			name:    "nil entry returns nil",
			entry:   nil,
			wantNil: true,
		},
		{
			name:    "non-array entry returns nil",
			entry:   "not an array",
			wantNil: true,
		},
		{
			name:    "empty array returns nil",
			entry:   []interface{}{},
			wantNil: true,
		},
		{
			name:    "single element array returns nil",
			entry:   []interface{}{"note-id"},
			wantNil: true,
		},
		{
			name: "valid entry with all fields",
			entry: []interface{}{
				"note-uuid-123",
				[]interface{}{
					"note-uuid-123",        // details[0]: id (duplicate)
					"This is note content", // details[1]: content
					[]interface{}{ // details[2]: metadata
						1,
						"meta-id",
						[]interface{}{float64(1733900000), float64(0)}, // timestamp
					},
					nil,          // details[3]: reserved
					"Note Title", // details[4]: title
				},
			},
			wantNil:     false,
			wantID:      "note-uuid-123",
			wantTitle:   "Note Title",
			wantContent: "This is note content",
		},
		{
			name: "entry with missing content",
			entry: []interface{}{
				"note-uuid-456",
				[]interface{}{
					"note-uuid-456",
					nil, // content is nil
					[]interface{}{1, "meta-id", []interface{}{float64(1733900000), float64(0)}},
					nil,
					"Title Only",
				},
			},
			wantNil:     false,
			wantID:      "note-uuid-456",
			wantTitle:   "Title Only",
			wantContent: "", // no content
		},
		{
			name: "entry with nil title",
			entry: []interface{}{
				"note-uuid-789",
				[]interface{}{
					"note-uuid-789",
					"Content here",
					[]interface{}{1, "meta-id", []interface{}{float64(1733900000), float64(0)}},
					nil,
					nil, // title is nil
				},
			},
			wantNil:     false,
			wantID:      "note-uuid-789",
			wantTitle:   "",
			wantContent: "Content here",
		},
		{
			name: "entry with short details array",
			entry: []interface{}{
				"note-uuid-short",
				[]interface{}{"id", "content"}, // only 2 elements, not 5
			},
			wantNil:   false, // returns Source with just ID populated
			wantID:    "note-uuid-short",
			wantTitle: "", // can't extract title from short array
		},
		{
			name: "entry with malformed metadata (nil)",
			entry: []interface{}{
				"note-uuid-nil-meta",
				[]interface{}{
					"note-uuid-nil-meta",
					"Some content",
					nil, // metadata is nil
					nil,
					"A Title",
				},
			},
			wantNil:     false,
			wantID:      "note-uuid-nil-meta",
			wantTitle:   "A Title",
			wantContent: "Some content",
		},
		{
			name: "entry with non-string source ID",
			entry: []interface{}{
				12345, // not a string
				[]interface{}{
					"id",
					"content",
					[]interface{}{1, "meta-id", []interface{}{float64(1733900000), float64(0)}},
					nil,
					"Title",
				},
			},
			wantNil:     false,
			wantID:      "", // can't extract non-string ID
			wantTitle:   "Title",
			wantContent: "content", // content is still parseable
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNoteEntry(tt.entry)

			if tt.wantNil {
				if got != nil {
					t.Errorf("parseNoteEntry() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Errorf("parseNoteEntry() = nil, want non-nil")
				return
			}

			gotID := ""
			if got.SourceId != nil {
				gotID = got.SourceId.SourceId
			}
			if gotID != tt.wantID {
				t.Errorf("parseNoteEntry().SourceId = %v, want %v", gotID, tt.wantID)
			}

			if got.Title != tt.wantTitle {
				t.Errorf("parseNoteEntry().Title = %v, want %v", got.Title, tt.wantTitle)
			}

			if got.Content != tt.wantContent {
				t.Errorf("parseNoteEntry().Content = %v, want %v", got.Content, tt.wantContent)
			}
		})
	}
}

func TestParseNoteEntry_NilGuards(t *testing.T) {
	// These tests specifically verify that malformed data doesn't cause panics

	testCases := []interface{}{
		nil,
		"string",
		123,
		[]interface{}{},
		[]interface{}{nil},
		[]interface{}{nil, nil},
		[]interface{}{"id", nil},
		[]interface{}{"id", "not-an-array"},
		[]interface{}{"id", []interface{}{}},
		[]interface{}{"id", []interface{}{nil, nil, nil, nil, nil}},
	}

	for i, tc := range testCases {
		t.Run(testName(i), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("parseNoteEntry() panicked with input %v: %v", tc, r)
				}
			}()

			// Should not panic
			_ = parseNoteEntry(tc)
		})
	}
}

func testName(i int) string {
	return "malformed_input_" + string(rune('A'+i))
}

// Verify Source has Content field (compile-time check)
var _ = notebooklmv1alpha1.Source{Content: "test"}
