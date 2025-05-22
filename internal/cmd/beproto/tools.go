package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tmc/nlm/internal/api"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/httprr"
)

// recordAndReplayListProjects records the list projects API call and replays it
func recordAndReplayListProjects() error {
	// Check for credentials
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")
	
	if authToken == "" || cookies == "" {
		return fmt.Errorf("missing credentials. Set NLM_AUTH_TOKEN and NLM_COOKIES environment variables")
	}
	
	recordingsDir := filepath.Join("testdata", "recordings")
	os.MkdirAll(recordingsDir, 0755)
	
	// Record mode
	fmt.Println("Recording mode:")
	recordingClient := httprr.NewRecordingClient(httprr.ModeRecord, recordingsDir, nil)
	
	client := api.New(
		authToken, 
		cookies,
		batchexecute.WithHTTPClient(recordingClient),
		batchexecute.WithDebug(true),
	)
	
	fmt.Println("Listing projects (recording)...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	
	fmt.Printf("Found %d projects in recording mode\n", len(projects))
	for i, p := range projects {
		fmt.Printf("  Project %d: %s (%s)\n", i, p.Title, p.ProjectId)
	}
	
	// Replay mode
	fmt.Println("\nReplay mode:")
	replayClient := httprr.NewRecordingClient(httprr.ModeReplay, recordingsDir, &http.Client{
		// Configure a failing transport to verify we're actually using recordings
		Transport: http.RoundTripper(failingTransport{}),
	})
	
	replayAPIClient := api.New(
		"fake-token", // Use fake credentials to verify we're using recordings
		"fake-cookie",
		batchexecute.WithHTTPClient(replayClient),
		batchexecute.WithDebug(true),
	)
	
	fmt.Println("Listing projects (replaying)...")
	replayProjects, err := replayAPIClient.ListRecentlyViewedProjects()
	if err != nil {
		return fmt.Errorf("list projects (replay): %w", err)
	}
	
	fmt.Printf("Found %d projects in replay mode\n", len(replayProjects))
	for i, p := range replayProjects {
		fmt.Printf("  Project %d: %s (%s)\n", i, p.Title, p.ProjectId)
	}
	
	return nil
}

// failingTransport is an http.RoundTripper that always fails
type failingTransport struct{}

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("this transport intentionally fails - if you see this, replay isn't working")
}