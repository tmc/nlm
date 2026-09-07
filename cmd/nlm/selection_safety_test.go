package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestSelectionSourceIDs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		s       selection
		wantErr bool
	}{
		{"omitted", selection{}, false},
		{"explicit", selection{Explicit: true, IDs: []string{"s"}}, false},
		{"empty", selection{Explicit: true}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.s.sourceIDs()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if err := runInteractiveChat(nil, nil, selection{Explicit: true}, chatOptions{}); err == nil {
		t.Fatal("interactive adapter accepted empty scope")
	}
}

// Exercise every resolver call site with an explicit, empty stdin selection.
// The transport control below proves that an omitted selection reaches the API.
func TestSelectorConsumersRejectEmpty(t *testing.T) {
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	old := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = old }()
	opts := selectorOptions{SourceIDs: "-"}
	calls := map[string]func(*notebooklm.Client) error{
		"generate-chat": func(c *notebooklm.Client) error {
			return generateFreeFormChat(c, "nb", "prompt", generateChatOptions{Selectors: opts})
		},
		"create-report": func(c *notebooklm.Client) error {
			return createReport(c, "nb", "report", nil, createReportOptions{Selectors: opts})
		},
		"generate-report": func(c *notebooklm.Client) error {
			return generateReport(c, "nb", reportOptions{Selectors: selectorOptions{SourceIDs: ","}})
		},
		"one-shot": func(c *notebooklm.Client) error { return oneShotChat(c, "nb", "prompt", chatOptions{Selectors: opts}) },
		"one-shot-conversation": func(c *notebooklm.Client) error {
			return oneShotChatInConv(c, "nb", "conv", "prompt", chatOptions{Selectors: opts})
		},
		"interactive": func(c *notebooklm.Client) error { return interactiveChat(c, "nb", chatOptions{Selectors: opts}) },
		"interactive-conversation": func(c *notebooklm.Client) error {
			return interactiveChatWithConv(c, "nb", "conv", chatOptions{Selectors: opts})
		},
		"app": func(c *notebooklm.Client) error {
			return appCreateCall(appCreateArgs{NotebookID: "nb", Options: appCreateOptions{Type: "mindmap", Instructions: "map", Selectors: opts}})(context.Background(), c)
		},
	}
	for _, path := range []string{"deck create", "source-guide"} {
		parsed := parseCreateCommandForTest(t, path, []string{"--source-ids", "-", "nb"})
		var call commandCall
		if path == "deck create" {
			call, err = decodeSlidesCreate(parsed)
		} else {
			call, err = decodeSourceGuide(parsed)
		}
		if err != nil {
			t.Fatal(err)
		}
		calls[path] = func(c *notebooklm.Client) error { return call(context.Background(), c) }
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			tr := &chatRejectTransport{}
			c := notebooklm.New(notebooklm.Credentials{}, notebooklm.WithHTTPClient(&http.Client{Transport: tr}))
			err := call(c)
			if err == nil || !strings.Contains(err.Error(), "empty set") {
				t.Fatalf("error = %v", err)
			}
			if tr.calls.Load() != 0 {
				t.Fatalf("issued %d requests", tr.calls.Load())
			}
		})
	}
	t.Run("omitted scope reaches transport", func(t *testing.T) {
		tr := &chatRejectTransport{}
		c := notebooklm.New(notebooklm.Credentials{}, notebooklm.WithHTTPClient(&http.Client{Transport: tr}))
		err := appCreateCall(appCreateArgs{NotebookID: "nb", Options: appCreateOptions{Type: "mindmap", Instructions: "map"}})(context.Background(), c)
		if err == nil || tr.calls.Load() == 0 {
			t.Fatalf("error=%v requests=%d", err, tr.calls.Load())
		}
	})
}
