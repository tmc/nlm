package argbuilder

import (
	"encoding/json"
	"testing"

	notebooklm "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// TestEncodeRPCArgsMessage covers positional encoding of nested messages.
func TestEncodeRPCArgsMessage(t *testing.T) {
	zero := int32(0)
	req := &notebooklm.MutateNoteRequest{
		ProjectId: "abc-123",
		NoteId:    "note-xyz",
		Updates: &notebooklm.NoteUpdates{Update: &notebooklm.NoteUpdateGroup{Update: &notebooklm.NoteUpdate{
			Content:    "body text",
			Title:      "the title",
			Tags:       &notebooklm.NoteTags{},
			UpdateMode: &zero,
			StateCode:  &zero,
		}}},
	}
	format := `[%project_id%, %note_id%, %updates%]`
	want := `["abc-123","note-xyz",[[["body text","the title",[],0,null,0]]]]`

	got, err := EncodeRPCArgs(req, format)
	if err != nil {
		t.Fatalf("EncodeRPCArgs() error = %v", err)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	if string(b) != want {
		t.Errorf("MutateNote args:\n got: %s\nwant: %s", b, want)
	}
}

func TestEncodeRPCArgs(t *testing.T) {
	tests := []struct {
		name      string
		msg       proto.Message
		argFormat string
		want      []interface{}
		wantErr   bool
	}{
		{
			name:      "empty format",
			msg:       &notebooklm.CreateProjectRequest{},
			argFormat: "[]",
			want:      []interface{}{},
		},
		{
			name: "simple fields",
			msg: &notebooklm.CreateProjectRequest{
				Title: "Test Project",
				Emoji: "📚",
			},
			argFormat: "[%title%, %emoji%]",
			want:      []interface{}{"Test Project", "📚"},
		},
		{
			name:      "with null",
			msg:       &notebooklm.ListRecentlyViewedProjectsRequest{},
			argFormat: "[null, 1, null, [2]]",
			want:      []interface{}{nil, 1, nil, []interface{}{2}},
		},
		{
			name: "single field",
			msg: &notebooklm.GetProjectRequest{
				ProjectId: "project123",
			},
			argFormat: "[%project_id%]",
			want:      []interface{}{"project123"},
		},
		{
			name: "nested array with field",
			msg: &notebooklm.DeleteSourcesRequest{
				SourceIds: []*notebooklm.SourceIdList{
					{SourceId: "src1"},
					{SourceId: "src2"},
					{SourceId: "src3"},
				},
			},
			argFormat: "[%source_ids%]",
			want: []interface{}{[]interface{}{
				[]interface{}{"src1"},
				[]interface{}{"src2"},
				[]interface{}{"src3"},
			}},
		},
		{
			name: "multiple fields",
			msg: &notebooklm.ActOnSourcesRequest{
				ProjectId: "proj456",
				Action:    "delete",
				SourceIds: []string{"s1", "s2"},
			},
			argFormat: "[%project_id%, %action%, %source_ids%]",
			want:      []interface{}{"proj456", "delete", []string{"s1", "s2"}},
		},
		{
			name: "chat command - GenerateFreeFormStreamed",
			msg: &notebooklm.GenerateFreeFormStreamedRequest{
				ProjectId: "notebook123",
				Prompt:    "test prompt",
			},
			argFormat: "[%project_id%, %prompt%]",
			want:      []interface{}{"notebook123", "test prompt"},
		},
		{
			// A scalar field wrapped in a one-element array. This must not be
			// misparsed as a bare field (which would drop the brackets).
			name:      "scalar field in nested array",
			msg:       &notebooklm.SourceIdList{SourceId: "sid"},
			argFormat: "[%source_id%]",
			want:      []interface{}{"sid"},
		},
		{
			// The full source-freshness wire shape: [null, [source_id], [2]].
			name:      "source freshness shape",
			msg:       &notebooklm.SourceIdList{SourceId: "sid"},
			argFormat: "[null, [%source_id%], [2]]",
			want:      []interface{}{nil, []interface{}{"sid"}, []interface{}{2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeRPCArgs(tt.msg, tt.argFormat)
			if (err != nil) != tt.wantErr {
				t.Errorf("EncodeRPCArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !equalSlices(got, tt.want) {
				t.Errorf("EncodeRPCArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func equalSlices(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		// Handle nested slices
		if sa, ok := a[i].([]interface{}); ok {
			if sb, ok := b[i].([]interface{}); ok {
				if !equalSlices(sa, sb) {
					return false
				}
				continue
			}
			return false
		}
		// Handle string slices
		if sa, ok := a[i].([]string); ok {
			if sb, ok := b[i].([]string); ok {
				if !equalStringSlices(sa, sb) {
					return false
				}
				continue
			}
			return false
		}
		// Simple comparison
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
