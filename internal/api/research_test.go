package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestResearchResultsStruct tests that ResearchResults struct is properly defined
func TestResearchResultsStruct(t *testing.T) {
	// Test that we can create a ResearchResults struct
	results := ResearchResults{
		TaskID: "task-123",
		Status: "complete",
		Sources: []ResearchSource{
			{
				ID:    "source-1",
				Title: "Test Source",
				URL:   "https://example.com",
				Type:  1, // web
			},
		},
	}

	if results.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %s", results.TaskID)
	}

	if results.Status != "complete" {
		t.Errorf("expected Status 'complete', got %s", results.Status)
	}

	if len(results.Sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(results.Sources))
	}

	if results.Sources[0].ID != "source-1" {
		t.Errorf("expected source ID 'source-1', got %s", results.Sources[0].ID)
	}
}

// TestResearchSourceStruct tests that ResearchSource struct is properly defined
func TestResearchSourceStruct(t *testing.T) {
	source := ResearchSource{
		ID:    "src-abc",
		Title: "Sample Document",
		URL:   "https://example.com/doc",
		Type:  2, // doc type
	}

	if source.ID != "src-abc" {
		t.Errorf("expected ID 'src-abc', got %s", source.ID)
	}

	if source.Title != "Sample Document" {
		t.Errorf("expected Title 'Sample Document', got %s", source.Title)
	}

	if source.URL != "https://example.com/doc" {
		t.Errorf("expected URL 'https://example.com/doc', got %s", source.URL)
	}

	if source.Type != 2 {
		t.Errorf("expected Type 2, got %d", source.Type)
	}
}

// TestStartResearchMethodExists tests that the StartResearch method exists on Client
func TestStartResearchMethodExists(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a mock response with task ID
		response := `)]}'
[[["wrb.fr","Ljjv0c","[\"task-fast-123\"]",null,null,null,"generic"]]]`
		w.Write([]byte(response))
	}))
	defer server.Close()

	// Create client - we just need to verify the method exists
	client := New("test-token", "test-cookies")

	// Call StartResearch with deep=false (fast research)
	_, err := client.StartResearch("project-123", "test query", false, 1)
	// We expect an error since we're not hitting a real server, but the method should exist
	if err == nil {
		t.Log("StartResearch executed without error")
	}

	// Call StartResearch with deep=true (deep research)
	_, err = client.StartResearch("project-123", "test query", true, 1)
	if err == nil {
		t.Log("StartResearch (deep) executed without error")
	}
}

// TestPollResearchResultsMethodExists tests that the PollResearchResults method exists on Client
func TestPollResearchResultsMethodExists(t *testing.T) {
	// Create client
	client := New("test-token", "test-cookies")

	// Call PollResearchResults
	_, err := client.PollResearchResults("project-123")
	// We expect an error since we're not hitting a real server, but the method should exist
	if err == nil {
		t.Log("PollResearchResults executed without error")
	}
}

// TestImportResearchSourcesMethodExists tests that the ImportResearchSources method exists on Client
func TestImportResearchSourcesMethodExists(t *testing.T) {
	// Create client
	client := New("test-token", "test-cookies")

	// Call ImportResearchSources
	sources := []ResearchSource{
		{URL: "https://example.com/1", Title: "Source 1", Type: 1},
		{URL: "https://example.com/2", Title: "Source 2", Type: 2},
	}
	err := client.ImportResearchSources("project-123", "task-456", sources, 1)
	// We expect an error since we're not hitting a real server, but the method should exist
	if err == nil {
		t.Log("ImportResearchSources executed without error")
	}
}

// TestParseResearchResults tests parsing of research results from RPC response
func TestParseResearchResults(t *testing.T) {
	// Actual nested response format from PollResearchResults:
	// [[[task_id, [project_id, [query, mode], 1, [[[url, title, desc, type]...], summary], status], [ts], [ts]]]]
	responseJSON := `[[[
		"task-123",
		["proj-1", ["test query", 1], 1,
			[[["https://example.com/1", "Source One", "Description one", 1],
			  ["https://example.com/2", "Source Two", "Description two", 2]],
			 "Research summary"],
			2
		],
		[1234567890],
		[1234567891]
	]]]`

	var data []interface{}
	err := json.Unmarshal([]byte(responseJSON), &data)
	if err != nil {
		t.Fatalf("failed to unmarshal test data: %v", err)
	}

	results := parseResearchResultsFromData(data)
	if results == nil {
		t.Fatal("parseResearchResultsFromData returned nil")
	}

	if results.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %s", results.TaskID)
	}

	if results.Status != "complete" {
		t.Errorf("expected Status 'complete', got %s", results.Status)
	}

	if len(results.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(results.Sources))
	}

	if results.Sources[0].URL != "https://example.com/1" {
		t.Errorf("expected first source URL 'https://example.com/1', got %s", results.Sources[0].URL)
	}

	if results.Sources[0].Title != "Source One" {
		t.Errorf("expected first source title 'Source One', got %s", results.Sources[0].Title)
	}

	if results.Summary != "Research summary" {
		t.Errorf("expected summary 'Research summary', got %s", results.Summary)
	}
}
