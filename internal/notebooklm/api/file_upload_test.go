package api

import "testing"

func TestBuildSourceUploadMetadataUsesRawJSON(t *testing.T) {
	t.Parallel()

	got, err := buildSourceUploadMetadata("notebook-123", "source.mp3", "source-456")
	if err != nil {
		t.Fatalf("buildSourceUploadMetadata() error = %v", err)
	}

	want := `{"PROJECT_ID":"notebook-123","SOURCE_NAME":"source.mp3","SOURCE_ID":"source-456"}`
	if string(got) != want {
		t.Fatalf("buildSourceUploadMetadata() = %s, want %s", got, want)
	}
}
