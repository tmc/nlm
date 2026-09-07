package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
)

type countingTransport struct{ calls int }

func (t *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, errors.New("test transport")
}

func TestUnverifiedMethodsDoNotSend(t *testing.T) {
	for _, name := range []string{"CancelDiscoverSourcesJob", "AddFileSource", "UpdateFeaturedNotebookStatus"} {
		t.Run(name, func(t *testing.T) {
			transport := new(countingTransport)
			c := NewLabsTailwindOrchestrationServiceClient("", "", batchexecute.WithHTTPClient(&http.Client{Transport: transport}))
			var err error
			switch name {
			case "CancelDiscoverSourcesJob":
				_, err = c.CancelDiscoverSourcesJob(context.Background(), &pb.CancelDiscoverSourcesJobRequest{})
			case "AddFileSource":
				_, err = c.AddFileSource(context.Background(), &pb.AddFileSourceRequest{})
			case "UpdateFeaturedNotebookStatus":
				_, err = c.UpdateFeaturedNotebookStatus(context.Background(), &pb.UpdateFeaturedNotebookStatusRequest{})
			}
			if err == nil || !strings.Contains(err.Error(), "argument format not defined") {
				t.Errorf("error = %v, want undefined argument format", err)
			}
			if transport.calls != 0 {
				t.Fatalf("sent %d requests for unverified method", transport.calls)
			}
			// The same client must still send methods with a known encoder.
			_, _ = c.MutateSource(context.Background(), &pb.MutateSourceRequest{})
			if transport.calls == 0 {
				t.Fatal("known method did not reach transport")
			}
		})
	}
}
