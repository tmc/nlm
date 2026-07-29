package notebooklm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
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
				Body:       io.NopCloser(strings.NewReader(deleteSourcesResponse(t, []interface{}{}))),
				Request:    req,
			}, nil
		}),
	}

	client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(httpClient))
	if err := client.DeleteSources(context.Background(), "project-123", []string{"source-1", "source-2"}); err != nil {
		t.Fatalf("DeleteSources: %v", err)
	}
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

func TestDeleteSourcesGeneratedEncoderMatchesLegacyShape(t *testing.T) {
	ids := []string{"source-1", "source-2"}
	req := &pb.DeleteSourcesRequest{Context: &pb.RequestContext{Version: proto.Int32(2)}}
	for _, id := range ids {
		req.SourceIds = append(req.SourceIds, &pb.SourceIdList{SourceId: id})
	}

	got := method.EncodeDeleteSourcesArgs(req)
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal generated args: %v", err)
	}
	const wantJSON = `[[["source-1"],["source-2"]],[2]]`
	if string(gotJSON) != wantJSON {
		t.Fatalf("generated delete-sources wire args = %s, want %s", gotJSON, wantJSON)
	}
}

func TestDeleteSourcesBatchesRequests(t *testing.T) {
	t.Parallel()

	var gotBodies []string
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll(req.Body): %v", err)
			}
			raw := string(body)
			gotBodies = append(gotBodies, raw)
			if n := countDeleteSourceIDs(t, raw); n > deleteSourcesBatchSize {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(deleteSourcesResponse(t, 3))),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(deleteSourcesResponse(t, []interface{}{}))),
				Request:    req,
			}, nil
		}),
	}

	var ids []string
	for i := 1; i <= 25; i++ {
		ids = append(ids, fmt.Sprintf("source-%02d", i))
	}
	client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(httpClient))
	if err := client.DeleteSources(context.Background(), "project-123", ids); err != nil {
		t.Fatalf("DeleteSources: %v", err)
	}
	if len(gotBodies) != 3 {
		t.Fatalf("requests = %d, want 3", len(gotBodies))
	}
	want := [][]string{ids[:10], ids[10:20], ids[20:]}
	for i, body := range gotBodies {
		payload := deleteSourcePayload(t, body)
		if n := strings.Count(payload, "source-"); n != len(want[i]) {
			t.Fatalf("request %d source count = %d, want %d in %q", i+1, n, len(want[i]), payload)
		}
		for _, id := range want[i] {
			if !strings.Contains(payload, id) {
				t.Fatalf("request %d missing %s in %q", i+1, id, payload)
			}
		}
	}
}

func TestDeleteSourcesBatchErrorIdentifiesFailedRange(t *testing.T) {
	t.Parallel()

	var calls int
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if calls == 2 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(deleteSourcesResponse(t, 3))),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(deleteSourcesResponse(t, []interface{}{}))),
				Request:    req,
			}, nil
		}),
	}

	var ids []string
	for i := 1; i <= 25; i++ {
		ids = append(ids, fmt.Sprintf("source-%02d", i))
	}
	client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(httpClient))
	err := client.DeleteSources(context.Background(), "project-123", ids)
	if err == nil {
		t.Fatal("DeleteSources succeeded, want batch error")
	}
	if !strings.Contains(err.Error(), "delete sources 11-20 of 25") {
		t.Fatalf("error = %v, want failed range", err)
	}
	if calls != 2 {
		t.Fatalf("requests = %d, want stop after second request", calls)
	}
}

func deleteSourcePayload(t *testing.T, body string) string {
	t.Helper()
	form, err := url.ParseQuery(body)
	if err != nil {
		t.Fatalf("ParseQuery(body): %v", err)
	}
	raw := form.Get("f.req")
	if raw == "" {
		t.Fatal("f.req missing")
	}
	return raw
}

func countDeleteSourceIDs(t *testing.T, body string) int {
	t.Helper()
	return strings.Count(deleteSourcePayload(t, body), "source-")
}

func deleteSourcesResponse(t *testing.T, data interface{}) string {
	t.Helper()
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(")]}'\n\n[[\"wrb.fr\",\"tGMBJ\",%s,null,null,null,\"generic\"]]", jsonData)
}
