package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestDeleteSourcesUsesNotebookContext(t *testing.T) {
	t.Parallel()

	var gotSourcePath string
	var gotBody string

	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotSourcePath = req.URL.Query().Get("source-path")
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll(req.Body): %v", err)
			}
			gotBody = string(body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(GenerateMockResponse("tGMBJ", []interface{}{}))),
				Request:    req,
			}, nil
		}),
	}

	client := New("auth", "cookie", batchexecute.WithHTTPClient(httpClient))
	_ = client.DeleteSources("project-123", []string{"source-1", "source-2"})
	if gotSourcePath != "/notebook/project-123" {
		t.Fatalf("source-path = %q, want %q", gotSourcePath, "/notebook/project-123")
	}
	form, err := url.ParseQuery(gotBody)
	if err != nil {
		t.Fatalf("ParseQuery(body): %v", err)
	}
	raw := form.Get("f.req")
	if raw == "" {
		t.Fatal("f.req missing")
	}
	var envelope []interface{}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("Unmarshal(f.req): %v", err)
	}
	if len(envelope) == 0 {
		t.Fatal("empty envelope")
	}
	payload := raw
	if !strings.Contains(payload, "source-1") || !strings.Contains(payload, "source-2") {
		t.Fatalf("payload %q missing source ids", payload)
	}
}
