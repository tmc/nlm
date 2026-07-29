package notebooklm

import (
	"bytes"
	"reflect"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestPollDeepResearchProtoScrubbedFixture(t *testing.T) {
	raw := loadFixture(t, "e3bVqc_poll_response_scrubbed.json")
	legacy, err := parseDeepResearchSessions(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	var generated pb.PollDeepResearchResponse
	if err := beprotojson.Unmarshal(raw, &generated); err != nil {
		t.Fatal(err)
	}
	got := deepResearchSessionsFromProto(generated.GetSessions())
	assertDeepResearchSessionsEqual(t, "scrubbed fixture", 0, got, legacy)
	if len(got) == 0 || got[0].Report == "" || len(got[0].Sources) == 0 {
		t.Fatal("scrubbed fixture does not exercise the completed report projection")
	}
}

func assertDeepResearchSessionsEqual(t *testing.T, path string, record int, got, want []deepResearchSession) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s:%d: session count generated=%d legacy=%d", path, record, len(got), len(want))
	}
	for i := range want {
		expected := want[i]
		if len(expected.MainBlob) != 0 {
			expected.Report, expected.Sources = decodeDeepResearchContent(expected.MainBlob)
			if expected.Mode != 5 {
				expected.Report, expected.Sources = decodeFastMainBlob(expected.MainBlob)
			}
		}
		if got[i].ConversationID != expected.ConversationID || got[i].ProjectID != expected.ProjectID || got[i].Query != expected.Query || got[i].Mode != expected.Mode || got[i].State != expected.State || got[i].ResearchID != expected.ResearchID {
			t.Fatalf("%s:%d: session %d generated identity differs", path, record, i)
		}
		if !bytes.Equal(got[i].Plan, expected.Plan) {
			t.Fatalf("%s:%d: session %d generated plan differs", path, record, i)
		}
		if got[i].Report != expected.Report {
			t.Fatalf("%s:%d: session %d generated report differs", path, record, i)
		}
		if !reflect.DeepEqual(got[i].Sources, expected.Sources) {
			t.Fatalf("%s:%d: session %d generated sources differ (%d != %d)", path, record, i, len(got[i].Sources), len(expected.Sources))
		}
	}
}
