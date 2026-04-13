package rpc

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func TestNotebookIDFromMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  proto.Message
		want string
	}{
		{
			name: "project request",
			msg: &pb.GetProjectRequest{
				ProjectId: "project-123",
			},
			want: "project-123",
		},
		{
			name: "nested context",
			msg: &pb.CreateArtifactRequest{
				Context: &pb.Context{ProjectId: "project-456"},
			},
			want: "project-456",
		},
		{
			name: "no notebook context",
			msg: &pb.GetArtifactRequest{
				ArtifactId: "artifact-123",
			},
			want: "",
		},
		{
			name: "nil",
			msg:  nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NotebookIDFromMessage(tt.msg); got != tt.want {
				t.Fatalf("NotebookIDFromMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}
