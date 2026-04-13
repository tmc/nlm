package method

import (
	"reflect"
	"testing"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestEncodeRenameArtifactArgs(t *testing.T) {
	t.Parallel()

	req := &notebooklmv1alpha1.RenameArtifactRequest{
		ArtifactId: "artifact-123",
		NewTitle:   "New Title",
	}

	got := EncodeRenameArtifactArgs(req)
	want := []interface{}{
		[]interface{}{"artifact-123", "New Title"},
		[]interface{}{[]interface{}{"title"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EncodeRenameArtifactArgs() = %#v, want %#v", got, want)
	}
}
