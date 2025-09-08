package argbuilder

import (
	"testing"

	notebooklm "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

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
			name: "with null",
			msg:  &notebooklm.ListRecentlyViewedProjectsRequest{},
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
				SourceIds: []string{"src1", "src2", "src3"},
			},
			argFormat: "[[%source_ids%]]",
			want:      []interface{}{[]string{"src1", "src2", "src3"}},
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