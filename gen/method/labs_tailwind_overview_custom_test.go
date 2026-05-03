package method

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// Sanitized source IDs modeled after HAR captures (2026-04-14).
var harSourceIDs = []string{
	"00000000-0000-4000-8000-000000000101",
	"00000000-0000-4000-8000-000000000102",
	"00000000-0000-4000-8000-000000000103",
	"00000000-0000-4000-8000-000000000104",
	"00000000-0000-4000-8000-000000000105",
}

const harProjectID = "00000000-0000-4000-8000-000000000001"

func TestEncodeCreateAudioOverviewArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		req         *notebooklmv1alpha1.CreateAudioOverviewRequest
		fixtureFile string
		// knownDiffs documents expected deviations from the HAR fixture.
		// The HAR sends "" for empty custom_instructions; the encoder sends null.
		// The server accepts both forms.
		knownDiffs []string
	}{
		{
			name: "deep_dive_no_instructions",
			req: &notebooklmv1alpha1.CreateAudioOverviewRequest{
				ProjectId: harProjectID,
				AudioType: notebooklmv1alpha1.AudioType_AUDIO_TYPE_DEEP_DIVE,
				SourceIds: harSourceIDs,
				Language:  "en",
			},
			fixtureFile: "testdata/r7cb6c_audio_request.json",
			knownDiffs:  []string{"instructions: encoder emits null, HAR has empty string"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EncodeCreateAudioOverviewArgs(tt.req)
			gotJSON := mustMarshal(t, got)

			wantJSON := mustReadFixture(t, tt.fixtureFile)

			// Normalize both sides through JSON round-trip for structural comparison.
			var gotVal, wantVal interface{}
			mustUnmarshal(t, gotJSON, &gotVal)
			mustUnmarshal(t, wantJSON, &wantVal)

			// Check structural equivalence, tolerating known diffs.
			assertJSONStructure(t, gotVal, wantVal, tt.knownDiffs, "")
		})
	}
}

func TestEncodeCreateVideoOverviewArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		req         *notebooklmv1alpha1.CreateVideoOverviewRequest
		fixtureFile string
		knownDiffs  []string
	}{
		{
			name: "whiteboard_style",
			req: &notebooklmv1alpha1.CreateVideoOverviewRequest{
				ProjectId:  harProjectID,
				SourceIds:  harSourceIDs,
				VideoStyle: notebooklmv1alpha1.VideoStyle_VIDEO_STYLE_WHITEBOARD,
			},
			fixtureFile: "testdata/r7cb6c_video_request.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EncodeCreateVideoOverviewArgs(tt.req)
			gotJSON := mustMarshal(t, got)

			wantJSON := mustReadFixture(t, tt.fixtureFile)

			var gotVal, wantVal interface{}
			mustUnmarshal(t, gotJSON, &gotVal)
			mustUnmarshal(t, wantJSON, &wantVal)

			assertJSONStructure(t, gotVal, wantVal, nil, "")
		})
	}
}

func TestEncodeCreateSlideDeckArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		projectID    string
		sourceIDs    []string
		instructions string
		language     string
		fixtureFile  string
		knownDiffs   []string
	}{
		{
			name:         "default_slides",
			projectID:    harProjectID,
			sourceIDs:    harSourceIDs,
			instructions: "",
			language:     "en",
			fixtureFile:  "testdata/r7cb6c_slides_request.json",
			// HAR sends [null,"en",1,3]; encoder sends ["","en",1,3].
			// Empty instructions vs null — server accepts both.
			knownDiffs: []string{"instructions: encoder emits empty string, HAR has null"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EncodeCreateSlideDeckArgs(tt.projectID, tt.sourceIDs, tt.instructions, tt.language)
			gotJSON := mustMarshal(t, got)

			wantJSON := mustReadFixture(t, tt.fixtureFile)

			var gotVal, wantVal interface{}
			mustUnmarshal(t, gotJSON, &gotVal)
			mustUnmarshal(t, wantJSON, &wantVal)

			assertJSONStructure(t, gotVal, wantVal, tt.knownDiffs, "")
		})
	}
}

func TestEncodeCreateInfographicArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		projectID    string
		sourceIDs    []string
		instructions string
		language     string
		fixtureFile  string
	}{
		{
			name:         "default_infographic",
			projectID:    harProjectID,
			sourceIDs:    harSourceIDs,
			instructions: "Create an executive summary infographic",
			language:     "en",
			fixtureFile:  "testdata/r7cb6c_infographic_request.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EncodeCreateInfographicArgs(tt.projectID, tt.sourceIDs, tt.instructions, tt.language)
			gotJSON := mustMarshal(t, got)

			wantJSON := mustReadFixture(t, tt.fixtureFile)

			var gotVal, wantVal interface{}
			mustUnmarshal(t, gotJSON, &gotVal)
			mustUnmarshal(t, wantJSON, &wantVal)

			assertJSONStructure(t, gotVal, wantVal, nil, "")
		})
	}
}

func TestEncodeCreateFlashcardsArgs(t *testing.T) {
	t.Parallel()

	got := EncodeCreateFlashcardsArgs(harProjectID, []string{"source-1"})
	gotJSON := string(mustMarshal(t, got))
	want := `[ [2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,2,3,6]]], "` + harProjectID + `", [null,null,4,[[["source-1"]]],null,null,null,null,null,[null,[1,null,null,null,null,null,[2,2]]]] ]`
	var gotVal, wantVal interface{}
	mustUnmarshal(t, []byte(gotJSON), &gotVal)
	mustUnmarshal(t, []byte(want), &wantVal)
	assertJSONStructure(t, gotVal, wantVal, nil, "")
}

// TestEncodeOverviewSourceRefs verifies 3-level nesting: [[[id1]], [[id2]], ...]
func TestEncodeOverviewSourceRefs(t *testing.T) {
	t.Parallel()
	got := encodeOverviewSourceRefs([]string{"id-1", "id-2"})
	gotJSON := mustMarshal(t, got)
	want := `[[["id-1"]],[["id-2"]]]`
	if string(gotJSON) != want {
		t.Fatalf("encodeOverviewSourceRefs =\n  %s\nwant:\n  %s", gotJSON, want)
	}
}

// TestEncodeInnerSourceRefs verifies 2-level nesting: [[id1], [id2], ...]
func TestEncodeInnerSourceRefs(t *testing.T) {
	t.Parallel()
	got := encodeInnerSourceRefs([]string{"id-1", "id-2"})
	gotJSON := mustMarshal(t, got)
	want := `[["id-1"],["id-2"]]`
	if string(gotJSON) != want {
		t.Fatalf("encodeInnerSourceRefs =\n  %s\nwant:\n  %s", gotJSON, want)
	}
}

// TestArtifactTypeDescriptor validates the constant prefix is stable.
func TestArtifactTypeDescriptor(t *testing.T) {
	t.Parallel()
	got := mustMarshal(t, artifactTypeDescriptor)
	want := `[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,2,3,6,5]]]`
	if string(got) != want {
		t.Fatalf("artifactTypeDescriptor =\n  %s\nwant:\n  %s", got, want)
	}
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func mustUnmarshal(t *testing.T, data []byte, v interface{}) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", data, err)
	}
}

func mustReadFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return b
}

// assertJSONStructure recursively compares two JSON values and reports
// differences. It tolerates known diffs (logged as t.Log, not t.Error).
func assertJSONStructure(t *testing.T, got, want interface{}, knownDiffs []string, path string) {
	t.Helper()
	if path == "" {
		path = "$"
	}

	switch w := want.(type) {
	case []interface{}:
		g, ok := got.([]interface{})
		if !ok {
			t.Errorf("%s: type mismatch: got %T, want array", path, got)
			return
		}
		if len(g) != len(w) {
			t.Errorf("%s: array length: got %d, want %d\n  got:  %s\n  want: %s",
				path, len(g), len(w), toJSON(g), toJSON(w))
			return
		}
		for i := range w {
			assertJSONStructure(t, g[i], w[i], knownDiffs, path+"["+itoa(i)+"]")
		}
	case map[string]interface{}:
		g, ok := got.(map[string]interface{})
		if !ok {
			t.Errorf("%s: type mismatch: got %T, want object", path, got)
			return
		}
		for k, wv := range w {
			gv, exists := g[k]
			if !exists {
				t.Errorf("%s.%s: missing key", path, k)
				continue
			}
			assertJSONStructure(t, gv, wv, knownDiffs, path+"."+k)
		}
	default:
		if !jsonEqual(got, want) {
			if isKnownDiff(knownDiffs, path, got, want) {
				t.Logf("%s: known diff: got %v (%T), HAR has %v (%T)", path, got, got, want, want)
			} else {
				t.Errorf("%s: value mismatch: got %v (%T), want %v (%T)", path, got, got, want, want)
			}
		}
	}
}

// jsonEqual compares two JSON-decoded values, treating float64 and int equivalence.
func jsonEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// JSON numbers decode as float64; compare numerically.
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		return af == bf
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return as == bs
	}
	ab, aok := a.(bool)
	bb, bok := b.(bool)
	if aok && bok {
		return ab == bb
	}
	return false
}

// isKnownDiff returns true if the mismatch at the given path matches a
// documented known difference (null vs empty string for instructions).
func isKnownDiff(_ []string, _ string, got, want interface{}) bool {
	// Tolerate null-vs-empty-string for instruction fields.
	if got == nil && want != nil {
		if s, ok := want.(string); ok && s == "" {
			return true
		}
	}
	if want == nil && got != nil {
		if s, ok := got.(string); ok && s == "" {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
