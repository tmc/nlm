package notebooklm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResolveSourceIDsReportsLookupFailure covers the case where the source
// lookup that precedes a chat fails, typically on an expired auth token. The
// resolver used to swallow the error and return no sources, so the chat went
// out with an empty selection and the model answered — fluently, and with
// nothing to ground it — that the notebook's sources were "not selected".
func TestResolveSourceIDsReportsLookupFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(")]}'\n\n[[\"wrb.fr\",\"rLM1Ne\",null,null,null,[16,\"Unauthenticated\"]]]"))
	}))
	defer server.Close()

	client := New(
		Credentials{AuthToken: "test-token", Cookies: "test-cookies"},
		WithHTTPClient(&http.Client{Transport: &testTransport{baseURL: server.URL}}),
	)

	got, err := client.resolveSourceIDs(context.Background(), "project1", nil)
	if err == nil {
		t.Fatalf("resolveSourceIDs(...) = %v, nil; want an error", got)
	}
	if got != nil {
		t.Errorf("resolveSourceIDs(...) source IDs = %v; want nil", got)
	}
	if !strings.Contains(err.Error(), "list notebook sources for chat") {
		t.Errorf("resolveSourceIDs(...) error = %v; want it to mention the source lookup", err)
	}
}

// TestResolveSourceIDsKeepsCallerSelection checks the paths that must not hit
// the network: an explicit selection and SkipSources both return as-is, so a
// failing lookup cannot affect them.
func TestResolveSourceIDsKeepsCallerSelection(t *testing.T) {
	client := New(Credentials{AuthToken: "test-token", Cookies: "test-cookies"},
		WithHTTPClient(&http.Client{Transport: errorTransport{}}))

	got, err := client.resolveSourceIDs(context.Background(), "project1", []string{"src1", "src2"})
	if err != nil {
		t.Fatalf("resolveSourceIDs(explicit) error = %v; want nil", err)
	}
	if want := []string{"src1", "src2"}; !equalStrings(got, want) {
		t.Errorf("resolveSourceIDs(explicit) = %v; want %v", got, want)
	}

	client.config.SkipSources = true
	got, err = client.resolveSourceIDs(context.Background(), "project1", nil)
	if err != nil {
		t.Fatalf("resolveSourceIDs(skip) error = %v; want nil", err)
	}
	if got != nil {
		t.Errorf("resolveSourceIDs(skip) = %v; want nil", got)
	}
}

// errorTransport fails every request, so a test using it proves the code under
// test made no request at all.
type errorTransport struct{}

func (errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, http.ErrServerClosed
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
