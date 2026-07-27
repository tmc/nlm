package betool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// compactStr compacts JSON so pretty-printed output can be compared to a
// canonical single-line form.
func compactStr(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		t.Fatalf("compact %q: %v", b, err)
	}
	return buf.String()
}

func TestBeprotoUnmarshalOptionsUsesCommandDebugSettings(t *testing.T) {
	options := beprotoUnmarshalOptions(betoolOptions{
		debugParsing:      true,
		debugFieldMapping: true,
	})
	if !options.DiscardUnknown {
		t.Error("DiscardUnknown = false, want true")
	}
	if !options.DebugParsing {
		t.Error("DebugParsing = false, want true")
	}
	if !options.DebugFieldMapping {
		t.Error("DebugFieldMapping = false, want true")
	}
}

// runBetoolCapture runs a betool invocation, feeding stdinData on stdin and
// capturing stdout. It restores os.Stdin/os.Stdout before returning.
func runBetoolCapture(t *testing.T, args []string, stdinData string) (string, error) {
	t.Helper()

	origIn, origOut := os.Stdin, os.Stdout

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdin, os.Stdout = inR, outW
	defer func() { os.Stdin, os.Stdout = origIn, origOut }()

	go func() {
		inW.WriteString(stdinData)
		inW.Close()
	}()

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := outR.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()

	// A leading "--json" selects the JSON output path (the real CLI honors the
	// global --json flag); strip it and pass it through as the flag.
	jsonOutput := false
	if len(args) > 0 && args[0] == "--json" {
		jsonOutput = true
		args = args[1:]
	}
	runErr := Run(args, Options{JSONOutput: jsonOutput})
	outW.Close()
	out := <-done
	outR.Close()
	inR.Close()
	return out, runErr
}

func TestBetoolRoundTripRequest(t *testing.T) {
	// JSON -> wire -> JSON reproduces the request, and the intermediate wire
	// body is a well-formed batchexecute form body.
	spec := `{"rpcs":[{"id":"wXbhsf","args":["My Notebook","😀"]}],"at":"TOK"}`

	body, err := runBetoolCapture(t, []string{"encode-request"}, spec)
	if err != nil {
		t.Fatalf("encode-request: %v", err)
	}
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "f.req=") || !strings.HasSuffix(body, "&") {
		t.Fatalf("bad wire body: %q", body)
	}
	// Space must be %20 (browser encodeURIComponent), never +.
	if strings.Contains(body, "My+Notebook") {
		t.Errorf("space encoded as + instead of %%20: %q", body)
	}

	jsonOut, err := runBetoolCapture(t, []string{"--json", "decode-request"}, body)
	if err != nil {
		t.Fatalf("decode-request: %v", err)
	}
	var got struct {
		RPCs []struct {
			ID   string          `json:"id"`
			Args json.RawMessage `json:"args"`
		} `json:"rpcs"`
		At string `json:"at"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil {
		t.Fatalf("decode-request output not JSON: %v\n%s", err, jsonOut)
	}
	if len(got.RPCs) != 1 || got.RPCs[0].ID != "wXbhsf" || got.At != "TOK" {
		t.Fatalf("round-trip mismatch: %+v at=%q", got.RPCs, got.At)
	}
	if want := `["My Notebook","😀"]`; compactStr(t, got.RPCs[0].Args) != want {
		t.Errorf("args = %s, want %s", got.RPCs[0].Args, want)
	}
}

func TestBetoolRoundTripResponse(t *testing.T) {
	// A raw response body (with escaped inner payload) decodes to unescaped
	// JSON, and re-encoding then decoding reproduces the same JSON.
	raw := ")]}'\n\n" + `[["wrb.fr","CCqFvf","[[[\"nb-123\",\"My Notebook\"]]]",null,null,null,"generic"]]`

	jsonOut, err := runBetoolCapture(t, []string{"--json", "decode-response"}, raw)
	if err != nil {
		t.Fatalf("decode-response: %v", err)
	}
	if !strings.Contains(jsonOut, `"nb-123"`) || !strings.Contains(jsonOut, `"CCqFvf"`) {
		t.Fatalf("unexpected decode-response output:\n%s", jsonOut)
	}

	wire, err := runBetoolCapture(t, []string{"encode-response"}, jsonOut)
	if err != nil {
		t.Fatalf("encode-response: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(wire), ")]}'") {
		t.Fatalf("missing anti-hijack prefix: %q", wire)
	}

	jsonOut2, err := runBetoolCapture(t, []string{"--json", "decode-response"}, wire)
	if err != nil {
		t.Fatalf("decode-response (2): %v", err)
	}
	if strings.Join(strings.Fields(jsonOut), "") != strings.Join(strings.Fields(jsonOut2), "") {
		t.Errorf("response round-trip not stable:\n a=%s\n b=%s", jsonOut, jsonOut2)
	}
}

func TestBetoolFileInput(t *testing.T) {
	// betool reads its payload from a file argument as well as stdin.
	dir := t.TempDir()
	path := filepath.Join(dir, "req.json")
	if err := os.WriteFile(path, []byte(`{"rpcs":[{"id":"X","args":[1,2]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runBetoolCapture(t, []string{"encode-request", path}, "")
	if err != nil {
		t.Fatalf("encode-request from file: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "f.req=") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestBetoolErrors(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"no mode", nil, "", "missing mode"},
		{"unknown mode", []string{"nope"}, "", "unknown mode"},
		{"too many files", []string{"decode-request", "a", "b"}, "", "at most one input file"},
		{"bad request json", []string{"encode-request"}, "not json", "parse request JSON"},
		{"empty request body", []string{"decode-request"}, "", "decode request"},
		{"bad response json", []string{"encode-response"}, "not json", "parse response JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runBetoolCapture(t, tt.args, tt.stdin)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestBetoolProtoRequest(t *testing.T) {
	// A request whose args are fully modeled decodes into a typed proto and
	// verifies as lossless.
	spec := `{"rpcs":[{"id":"CCqFvf","args":["My Notebook","📓"]}]}`
	wire, err := runBetoolCapture(t, []string{"encode-request"}, spec)
	if err != nil {
		t.Fatalf("encode-request: %v", err)
	}
	out, err := runBetoolCapture(t, []string{"--json", "decode-request", "--verify"}, wire)
	if err != nil {
		t.Fatalf("decode-request --verify: %v", err)
	}
	var envs []struct {
		RPCID    string          `json:"rpc_id"`
		Method   string          `json:"method"`
		Type     string          `json:"type"`
		Message  json.RawMessage `json:"message"`
		Lossless *bool           `json:"roundtrip_lossless"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(envs))
	}
	e := envs[0]
	if e.RPCID != "CCqFvf" || e.Type != "notebooklm.v1alpha1.CreateProjectRequest" {
		t.Errorf("rpc_id/type = %q/%q", e.RPCID, e.Type)
	}
	if compactStr(t, e.Message) != `{"title":"My Notebook","emoji":"📓"}` {
		t.Errorf("message = %s", e.Message)
	}
	if e.Lossless == nil || !*e.Lossless {
		t.Errorf("expected lossless=true, got %v", e.Lossless)
	}
}

func TestBetoolProtoResponseFixture(t *testing.T) {
	// The list_notebooks fixture decodes into a typed response with named
	// fields. Every source field is now modeled (SourceMetadata field #4 is
	// RevisionData), so the fixture round-trips losslessly.
	raw, err := os.ReadFile("../batchexecute/testdata/list_notebooks.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t, []string{"--json", "decode-response", "--verify", "--rpc-id=wXbhsf"}, string(raw))
	if err != nil {
		t.Fatalf("decode-response --verify: %v", err)
	}
	if !strings.Contains(out, `"title": "nbname2"`) {
		t.Errorf("expected named field title=nbname2 in output:\n%s", out[:min(len(out), 400)])
	}
	if !strings.Contains(out, `"source_type": "SOURCE_TYPE_WEB_PAGE"`) {
		t.Errorf("expected enum SOURCE_TYPE_WEB_PAGE resolved")
	}
	if !strings.Contains(out, `"roundtrip_lossless": true`) {
		t.Errorf("expected lossless=true (every source field is now modeled)")
	}

	var envs []struct {
		MissingCount  int `json:"missing_field_count"`
		MissingGroups []struct {
			Path   string `json:"path"`
			Kind   string `json:"kind"`
			Name   string `json:"name"`
			Count  int    `json:"count"`
			Shapes int    `json:"shapes"`
		} `json:"missing_field_groups"`
		Missing []json.RawMessage `json:"missing_fields"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	e := envs[0]
	if e.MissingCount != 0 {
		t.Errorf("missing_field_count = %d, want 0 (fully modeled)", e.MissingCount)
	}
	if len(e.MissingGroups) != 0 {
		t.Errorf("got %d finding groups, want 0: %+v", len(e.MissingGroups), e.MissingGroups)
	}
}

// TestBetoolProtoConversationHistoryDecodes exercises a real
// GetConversationHistory capture (sanitized). ChatMessage matches the wire turn
// shape (message_id, timestamp, role, text, rich_content), so all six turns
// decode into typed messages rather than dropping as whole elements. An
// assistant turn's rich_content is fully modeled down to its rich-text document
// tree, so the segment text decodes and no element drops as "does not fit".
func TestBetoolProtoConversationHistoryDecodes(t *testing.T) {
	raw, err := os.ReadFile("../batchexecute/testdata/conversation_history.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t, []string{"--json", "decode-response", "--proto", "--verify", "--rpc-id=khqZz"}, string(raw))
	if err != nil {
		t.Fatalf("decode-response --verify: %v", err)
	}
	var envs []struct {
		Type    string `json:"type"`
		Message struct {
			Messages []struct {
				MessageID   string `json:"message_id"`
				Timestamp   string `json:"timestamp"`
				Role        int    `json:"role"`
				Text        string `json:"text"`
				RichContent struct {
					Segment struct {
						Text string `json:"text"`
					} `json:"segment"`
				} `json:"rich_content"`
			} `json:"messages"`
		} `json:"message"`
		MissingGroups []struct {
			Path string `json:"path"`
			Name string `json:"name"`
		} `json:"missing_field_groups"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	e := envs[0]
	if e.Type != "notebooklm.v1alpha1.GetConversationHistoryResponse" {
		t.Errorf("type = %q", e.Type)
	}

	// All six turns decode into typed messages (previously all were dropped).
	if got := len(e.Message.Messages); got != 6 {
		t.Fatalf("decoded %d messages, want 6", got)
	}
	// A user turn (role 1) carries text; an assistant turn (role 2) carries
	// rich_content whose segment holds the rendered text. Verify both shapes
	// decoded with a real timestamp.
	var sawUser, sawAssistant bool
	for _, m := range e.Message.Messages {
		if m.MessageID == "" || m.Timestamp == "" {
			t.Errorf("turn missing message_id/timestamp: %+v", m)
		}
		switch m.Role {
		case 1:
			sawUser = true
			if m.Text == "" {
				t.Errorf("user turn has no text: %+v", m)
			}
		case 2:
			sawAssistant = true
			if m.RichContent.Segment.Text == "" {
				t.Errorf("assistant turn has no rich_content segment text: %+v", m)
			}
		}
	}
	if !sawUser || !sawAssistant {
		t.Errorf("expected both user and assistant turns, sawUser=%v sawAssistant=%v", sawUser, sawAssistant)
	}

	// No turn drops as a whole element: ChatMessage fits the wire shape.
	for _, g := range e.MissingGroups {
		if strings.Contains(g.Name, "does not fit ChatMessage") {
			t.Errorf("ChatMessage should fit the wire, but got whole-element finding: %+v", g)
		}
	}
}

// TestBetoolProtoArtifactSuggestionCategories guards the ArtifactSuggestion
// categories field (#3). The web UI polls GenerateArtifactSuggestions (otmP3b)
// repeatedly while suggestions generate; early poll snapshots return
// [title, description] tuples, but once suggestions are ready each tuple gains
// a trailing repeated-string category tag ([title, description, ["explanatory"]]).
// That third element is populated only in the later poll, so the static
// list_notebooks fixture never exercised it — the growing poll response did.
// A regression that drops the categories field would push this fixture below
// lossless and fail here.
func TestBetoolProtoArtifactSuggestionCategories(t *testing.T) {
	raw, err := os.ReadFile("../batchexecute/testdata/responses/otmP3b.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t,
		[]string{"--json", "decode-response", "--proto", "--verify", "--rpc-id=otmP3b"},
		string(raw))
	if err != nil {
		t.Fatalf("decode-response --verify: %v", err)
	}
	var envs []struct {
		Type    string `json:"type"`
		Message struct {
			Suggestions []struct {
				Title       string   `json:"title"`
				Description string   `json:"description"`
				Categories  []string `json:"categories"`
			} `json:"suggestions"`
		} `json:"message"`
		Lossless *bool `json:"roundtrip_lossless"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(envs))
	}
	e := envs[0]
	if e.Type != "notebooklm.v1alpha1.GenerateArtifactSuggestionsResponse" {
		t.Errorf("type = %q", e.Type)
	}
	if len(e.Message.Suggestions) != 3 {
		t.Fatalf("decoded %d suggestions, want 3", len(e.Message.Suggestions))
	}
	// Every suggestion carries its category tag decoded into the named field.
	for i, s := range e.Message.Suggestions {
		if s.Title == "" || s.Description == "" {
			t.Errorf("suggestion %d missing title/description: %+v", i, s)
		}
		if len(s.Categories) != 1 || s.Categories[0] != "explanatory" {
			t.Errorf("suggestion %d categories = %v, want [explanatory]", i, s.Categories)
		}
	}
	if e.Lossless == nil || !*e.Lossless {
		t.Errorf("expected roundtrip_lossless=true, got %v", e.Lossless)
	}
}

// TestBetoolProtoCopyProjectResponse guards the te3DCe CopyProject reply. The
// rpc previously declared a Project return type (an unverified guess); a HAR
// showed the wire is a single status int [3], so the type is now
// CopyProjectResponse. CopyProject has no live caller, so the return-type
// change is decode-only.
func TestBetoolProtoCopyProjectResponse(t *testing.T) {
	raw, err := os.ReadFile("../batchexecute/testdata/responses/te3DCe.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t,
		[]string{"--json", "decode-response", "--proto", "--verify", "--rpc-id=te3DCe"},
		string(raw))
	if err != nil {
		t.Fatalf("decode-response --verify: %v", err)
	}
	var envs []struct {
		Type    string `json:"type"`
		Message struct {
			Status string `json:"status"`
		} `json:"message"`
		Lossless *bool `json:"roundtrip_lossless"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(envs))
	}
	e := envs[0]
	if e.Type != "notebooklm.v1alpha1.CopyProjectResponse" {
		t.Errorf("type = %q, want CopyProjectResponse", e.Type)
	}
	if e.Message.Status != "3" {
		t.Errorf("status = %q, want 3", e.Message.Status)
	}
	if e.Lossless == nil || !*e.Lossless {
		t.Errorf("expected roundtrip_lossless=true, got %v", e.Lossless)
	}
}

// TestBetoolProtoProjectDetailsResponse guards the JFMDGd (GetProjectDetails)
// reply. The message was remodeled to the HAR-verified wire shape
// ([collaborators, flags, limit, ...]); the earlier speculative field layout
// landed shape/value mismatches at nearly every position. The live path
// hand-parses the raw JSON (not proto) and only reads OwnerName/IsPublic by Go
// field name, so the renumber is decode-only. Fixture is PII-scrubbed.
func TestBetoolProtoProjectDetailsResponse(t *testing.T) {
	raw, err := os.ReadFile("../batchexecute/testdata/responses/JFMDGd.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t,
		[]string{"--json", "decode-response", "--proto", "--verify", "--rpc-id=JFMDGd"},
		string(raw))
	if err != nil {
		t.Fatalf("decode-response --verify: %v", err)
	}
	var envs []struct {
		Type    string `json:"type"`
		Message struct {
			Collaborators []struct {
				Email   string `json:"email"`
				Role    int    `json:"role"`
				Profile struct {
					DisplayName string `json:"display_name"`
				} `json:"profile"`
			} `json:"collaborators"`
			Limit string `json:"limit"`
		} `json:"message"`
		Lossless *bool `json:"roundtrip_lossless"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(envs))
	}
	e := envs[0]
	if e.Type != "notebooklm.v1alpha1.ProjectDetails" {
		t.Errorf("type = %q, want ProjectDetails", e.Type)
	}
	if len(e.Message.Collaborators) != 1 {
		t.Fatalf("got %d collaborators, want 1", len(e.Message.Collaborators))
	}
	c := e.Message.Collaborators[0]
	if c.Role != 1 || c.Profile.DisplayName == "" || c.Email == "" {
		t.Errorf("owner entry decoded wrong: %+v", c)
	}
	if e.Lossless == nil || !*e.Lossless {
		t.Errorf("expected roundtrip_lossless=true, got %v", e.Lossless)
	}
}

func TestBetoolProtoErrors(t *testing.T) {
	// Build a raw request body carrying an unknown rpc_id, and a raw response
	// body carrying the ambiguous R7cb6c id.
	unknownReq, err := runBetoolCapture(t, []string{"encode-request"}, `{"rpcs":[{"id":"zzz","args":[]}]}`)
	if err != nil {
		t.Fatalf("encode-request: %v", err)
	}
	ambResp, err := runBetoolCapture(t, []string{"encode-response"}, `{"responses":[{"id":"R7cb6c","data":[]}]}`)
	if err != nil {
		t.Fatalf("encode-response: %v", err)
	}

	tests := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"unknown rpc_id", []string{"decode-request", "--proto"}, unknownReq, "no proto method bound"},
		{"ambiguous rpc_id", []string{"decode-response", "--proto"}, ambResp, "multiple methods"},
		{"proto on encode", []string{"encode-request", "--proto"}, `{"rpcs":[]}`, "apply only to decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runBetoolCapture(t, tt.args, tt.stdin)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

// TestBetoolProtoRequestFixtures guards the request-side lossless claims. Each
// fixture is a sanitized real f.req argument payload (UUIDs replaced with stable
// placeholders) captured from the web UI. A regression that breaks any modeled
// *Request layout, its shared RequestContext preamble, or a present-but-empty
// list wrapper would drop these below lossless and fail here.
func TestBetoolProtoRequestFixtures(t *testing.T) {
	cases := []struct {
		file     string // fixture basename
		selector string // --rpc-id value (rpc_id, or method name to disambiguate)
		typ      string
	}{
		{"wXbhsf", "wXbhsf", "notebooklm.v1alpha1.ListRecentlyViewedProjectsRequest"},
		{"LQhfEb", "LQhfEb", "notebooklm.v1alpha1.UpdateProjectUserStateRequest"},
		{"le8sX", "le8sX", "notebooklm.v1alpha1.MutateLabelRequest"},
		// gArtLc's rpc_id is shared by ListArtifacts/QueryArtifacts, so select
		// the method explicitly.
		{"gArtLc", "ListArtifacts", "notebooklm.v1alpha1.ListArtifactsRequest"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(
				"../batchexecute/testdata/requests", tc.file+".txt"))
			if err != nil {
				t.Skipf("fixture unavailable: %v", err)
			}
			out, err := runBetoolCapture(t,
				[]string{"--json", "decode-request", "--proto", "--verify", "--rpc-id=" + tc.selector},
				string(raw))
			if err != nil {
				t.Fatalf("decode-request --verify: %v", err)
			}
			var envs []struct {
				RPCID    string `json:"rpc_id"`
				Type     string `json:"type"`
				Lossless *bool  `json:"roundtrip_lossless"`
			}
			if err := json.Unmarshal([]byte(out), &envs); err != nil {
				t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
			}
			if len(envs) != 1 {
				t.Fatalf("got %d envelopes, want 1", len(envs))
			}
			e := envs[0]
			if e.Type != tc.typ {
				t.Errorf("type = %q, want %q", e.Type, tc.typ)
			}
			if e.Lossless == nil || !*e.Lossless {
				t.Errorf("expected roundtrip_lossless=true, got %v", e.Lossless)
			}
		})
	}
}
