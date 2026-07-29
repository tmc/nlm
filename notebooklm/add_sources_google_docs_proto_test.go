package notebooklm

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestAddSourcesGoogleDocsScrubbedProjection(t *testing.T) {
	raw := loadFixture(t, "add_sources_google_docs_response_scrubbed.json")
	var response pb.AddSourcesResponse
	if err := beprotojson.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.GetSources()) != 1 || sourceID(response.GetSources()[0]) != "scrubbed" || sourceTitle(response.GetSources()[0]) != "scrubbed" {
		t.Fatal("scrubbed Google Docs response projection differs")
	}
}
