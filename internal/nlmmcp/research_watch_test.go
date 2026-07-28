package nlmmcp

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

type researchPollResult struct {
	result *api.DeepResearchResult
	err    error
}

type fakeResearchPoller struct {
	results []researchPollResult
	calls   int
}

func (p *fakeResearchPoller) PollDeepResearch(context.Context, string, string) (*api.DeepResearchResult, error) {
	i := p.calls
	p.calls++
	if i >= len(p.results) {
		return nil, errors.New("unexpected poll")
	}
	return p.results[i].result, p.results[i].err
}

func TestWatchDeepResearch(t *testing.T) {
	t.Parallel()

	want := &api.DeepResearchResult{
		ResearchID: "research-1",
		Done:       true,
		Report:     "# Complete",
	}
	poller := &fakeResearchPoller{results: []researchPollResult{
		{result: &api.DeepResearchResult{ResearchID: "research-1"}, err: api.ErrResearchPolling},
		{result: &api.DeepResearchResult{ResearchID: "research-1"}, err: api.ErrResearchPolling},
		{result: want},
	}}
	var progress []researchWatchProgress
	got, err := watchDeepResearch(context.Background(), poller, watchDeepResearchInput{
		NotebookID:     "notebook-1",
		ResearchID:     "research-1",
		PollIntervalMS: 1,
	}, func(p researchWatchProgress) error {
		progress = append(progress, p)
		return nil
	})
	if err != nil {
		t.Fatalf("watchDeepResearch() error = %v", err)
	}
	if got != want {
		t.Fatalf("watchDeepResearch() result = %#v, want %#v", got, want)
	}
	if poller.calls != 3 {
		t.Fatalf("poll calls = %d, want 3", poller.calls)
	}
	if len(progress) != 3 {
		t.Fatalf("progress count = %d, want 3", len(progress))
	}
	for i, p := range progress {
		if p.Value != float64(i+1) {
			t.Errorf("progress[%d].Value = %v, want %d", i, p.Value, i+1)
		}
	}
	if progress[2].Message != "deep research complete" {
		t.Fatalf("final progress message = %q", progress[2].Message)
	}
}

func TestWatchDeepResearchStopsOnContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	poller := &fakeResearchPoller{results: []researchPollResult{{
		result: &api.DeepResearchResult{ResearchID: "research-1"},
		err:    api.ErrResearchPolling,
	}}}
	_, err := watchDeepResearch(ctx, poller, watchDeepResearchInput{
		NotebookID: "notebook-1",
		ResearchID: "research-1",
	}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watchDeepResearch() error = %v, want context.Canceled", err)
	}
}

func TestWatchDeepResearchValidatesInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input watchDeepResearchInput
	}{
		{"notebook", watchDeepResearchInput{ResearchID: "research-1"}},
		{"research", watchDeepResearchInput{NotebookID: "notebook-1"}},
		{"interval", watchDeepResearchInput{NotebookID: "notebook-1", ResearchID: "research-1", PollIntervalMS: -1}},
		{"wait", watchDeepResearchInput{NotebookID: "notebook-1", ResearchID: "research-1", MaxWaitSeconds: -1}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := watchDeepResearch(context.Background(), &fakeResearchPoller{}, tt.input, nil); err == nil {
				t.Fatal("watchDeepResearch() succeeded, want validation error")
			}
		})
	}
}

func TestWatchDeepResearchReturnsNotificationError(t *testing.T) {
	t.Parallel()

	notifyErr := errors.New("client disconnected")
	poller := &fakeResearchPoller{results: []researchPollResult{{
		result: &api.DeepResearchResult{ResearchID: "research-1"},
		err:    api.ErrResearchPolling,
	}}}
	_, err := watchDeepResearch(context.Background(), poller, watchDeepResearchInput{
		NotebookID: "notebook-1",
		ResearchID: "research-1",
	}, func(researchWatchProgress) error {
		return notifyErr
	})
	if !errors.Is(err, notifyErr) {
		t.Fatalf("watchDeepResearch() error = %v, want notification error", err)
	}
}

func TestServerListsWatchDeepResearch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := New(nil, nil).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "nlm-test",
		Version: "devel",
	}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { clientSession.Close() })

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(result.Tools) != 25 {
		t.Fatalf("tool count = %d, want 25", len(result.Tools))
	}
	removed := map[string]bool{
		"generate_summarize":    true,
		"generate_briefing_doc": true,
		"generate_faq":          true,
		"generate_study_guide":  true,
		"generate_rephrase":     true,
		"generate_expand":       true,
		"generate_critique":     true,
		"generate_brainstorm":   true,
		"generate_verify":       true,
		"generate_explain":      true,
		"generate_outline":      true,
		"generate_mindmap":      true,
		"generate_timeline":     true,
		"generate_toc":          true,
	}
	foundWatch := false
	for _, tool := range result.Tools {
		if removed[tool.Name] {
			t.Fatalf("unproven tool %q is still listed", tool.Name)
		}
		if tool.Name == "watch_deep_research" {
			foundWatch = true
		}
	}
	if !foundWatch {
		t.Fatal("watch_deep_research not listed")
	}
}
