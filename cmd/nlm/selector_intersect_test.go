package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/notebooklm"
)

func TestSelectorIntersection(t *testing.T) {
	sources := []sourceSummary{{ID: "a", Title: "match"}, {ID: "b", Title: "match"}, {ID: "c", Title: "other"}}
	labels := []notebooklm.Label{{LabelID: "l", Name: "tag", SourceIDs: []string{"b", "c"}}}
	for _, tt := range []struct {
		name, mode, match string
		want              []string
		wantErr           bool
	}{
		{"intersection", "intersect", "match", []string{"b"}, false},
		{"union control", "union", "match", []string{"a", "b", "c"}, false},
		{"disjoint", "intersect", "^a$", nil, true},
		{"disjoint union", "union", "^a$", []string{"a", "b", "c"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSelectorIDs(selectorOptions{Mode: tt.mode, SourceMatch: tt.match, LabelMatch: "tag"}, nil, nil, nil, sources, labels, io.Discard)
			if (err != nil) != tt.wantErr || !reflect.DeepEqual(got.IDs, tt.want) {
				t.Fatalf("got %+v, %v; want %v", got, err, tt.want)
			}
		})
	}
	selected, err := resolveSelectorIDs(selectorOptions{SourceExclude: "^b$"}, nil, nil, nil, sources, nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	got, err := selected.withSuggestions([]string{"b", "c", "stale"})
	if err != nil || !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Fatalf("suggestions restored exclusions: %v, %v", got, err)
	}
}

type selectorTransport struct {
	mu        sync.Mutex
	t         *testing.T
	responses map[string]string
	calls     []string
	bodies    []string
}

func (tr *selectorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	id := req.URL.Query().Get("rpcids")
	tr.calls = append(tr.calls, id)
	body, _ := io.ReadAll(req.Body)
	tr.bodies = append(tr.bodies, string(body))
	payload, ok := tr.responses[id]
	if !ok {
		tr.t.Errorf("unexpected RPC %q", id)
		payload = "[]"
	}
	data, _ := json.Marshal([]any{[]any{"wrb.fr", id, payload, nil, nil, nil, "generic"}})
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(")]}'\n\n" + string(data))), Request: req}, nil
}
func selectorTestClient(t *testing.T, responses map[string]string) (*notebooklm.Client, *selectorTransport) {
	t.Helper()
	tr := &selectorTransport{t: t, responses: responses}
	return notebooklm.New(notebooklm.Credentials{}, notebooklm.WithHTTPClient(&http.Client{Transport: tr})), tr
}
func selectorProjectJSON(t *testing.T) string {
	t.Helper()
	raw, err := beprotojson.Marshal(&pb.Project{Sources: []*pb.Source{
		{SourceId: &pb.SourceId{SourceId: "a"}, Title: "a"},
		{SourceId: &pb.SourceId{SourceId: "b"}, Title: "b"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestEmptyLabelSelectionStopsGenerativeConsumers(t *testing.T) {
	for _, name := range []string{"create-report", "generate-report", "generate-chat"} {
		t.Run(name, func(t *testing.T) {
			c, tr := selectorTestClient(t, map[string]string{"rLM1Ne": selectorProjectJSON(t), "I3xc3c": `[[["tag",[["a"],["b"]],"l",""]]]`})
			opts := selectorOptions{LabelNone: true}
			var err error
			switch name {
			case "create-report":
				err = createReport(c, "nb", "report", nil, createReportOptions{Selectors: opts})
			case "generate-report":
				err = generateReport(c, "nb", reportOptions{Selectors: opts, Instructions: "must not be set"})
			case "generate-chat":
				err = generateFreeFormChat(c, "nb", "prompt", generateChatOptions{Selectors: opts})
			}
			if err == nil || !strings.Contains(err.Error(), "empty set") {
				t.Fatalf("error=%v", err)
			}
			slices.Sort(tr.calls) // Source and label lookups may complete in either order.
			if !reflect.DeepEqual(tr.calls, []string{"I3xc3c", "rLM1Ne"}) {
				t.Fatalf("calls=%v", tr.calls)
			}
		})
	}
}

func TestCreateReportExclusionsSurviveSuggestions(t *testing.T) {
	c, tr := selectorTestClient(t, map[string]string{
		"rLM1Ne": selectorProjectJSON(t),
		"ciyUvf": `[[["report","description",null,[["a"],["b"]],"prompt",2]]]`,
		"R7cb6c": `["artifact"]`,
	})
	if err := createReport(c, "nb", "report", nil, createReportOptions{Selectors: selectorOptions{SourceExclude: "^b$"}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tr.calls, []string{"rLM1Ne", "rLM1Ne", "ciyUvf", "R7cb6c"}) {
		t.Fatalf("calls=%v", tr.calls)
	}
	values, err := url.ParseQuery(tr.bodies[len(tr.bodies)-1])
	if err != nil {
		t.Fatal(err)
	}
	var batch [][][]any
	if err := json.Unmarshal([]byte(values.Get("f.req")), &batch); err != nil {
		t.Fatal(err)
	}
	args := batch[0][0][1].(string)
	if strings.Contains(args, `"b"`) || !strings.Contains(args, `"a"`) {
		t.Fatalf("report arguments=%s", args)
	}
}
