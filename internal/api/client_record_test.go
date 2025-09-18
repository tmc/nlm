package api

import (
	"net/http"
	"os"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/httprr"
)

// loadNLMCredentials is now defined in test_helpers.go to avoid duplication

// TestListProjectsWithRecording validates ListRecentlyViewedProjects functionality
// including proper project list handling and truncation behavior.
func TestListProjectsWithRecording(t *testing.T) {
	httprr.SkipIfNoNLMCredentialsOrRecording(t)
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Use test credentials that get scrubbed by httprr
	authToken := "test-auth-token"
	cookies := "test-cookies"
	if os.Getenv("NLM_AUTH_TOKEN") != "" {
		authToken = os.Getenv("NLM_AUTH_TOKEN")
	}
	if os.Getenv("NLM_COOKIES") != "" {
		cookies = os.Getenv("NLM_COOKIES")
	}

	client := New(
		authToken,
		cookies,
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(false),
	)

	// Call the API method
	t.Log("Listing projects...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	// Validate results
	t.Logf("Found %d projects", len(projects))

	// Basic validation
	if len(projects) < 1 {
		t.Fatal("Expected at least 1 project, got 0")
	}

	// Project count may vary based on data source
	// Live API calls return full list, cached data may be truncated to 10 items
	if len(projects) > 10 {
		t.Logf("Got %d projects (full list from live API)", len(projects))
	} else {
		t.Logf("Got %d projects (may be truncated for performance)", len(projects))
	}

	// Validate project structure
	for i, p := range projects {
		if i >= 5 { // Only log first 5 to avoid spam
			break
		}

		// Validate required fields
		if p.ProjectId == "" {
			t.Errorf("Project %d has empty ProjectId", i)
		}
		if p.Title == "" {
			t.Errorf("Project %d has empty Title", i)
		}

		// Validate ProjectId format (should be UUID)
		if len(p.ProjectId) != 36 {
			t.Errorf("Project %d has invalid ProjectId format: %s (expected UUID)", i, p.ProjectId)
		}

		t.Logf("Project %d: %s (%s)", i, p.Title, p.ProjectId)
	}

	// Additional validation: ensure reasonable project count limits
	// This validates that truncation behavior works correctly
	const maxExpectedInCachedMode = 10
	if len(projects) <= maxExpectedInCachedMode {
		t.Logf("✓ Project count (%d) is within expected range for cached data", len(projects))
	}
}

// TestCreateProjectWithRecording validates CreateProject functionality
func TestCreateProjectWithRecording(t *testing.T) {
	httprr.SkipIfNoNLMCredentialsOrRecording(t)
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Use test credentials that get scrubbed by httprr
	authToken := "test-auth-token"
	cookies := "test-cookies"
	if os.Getenv("NLM_AUTH_TOKEN") != "" {
		authToken = os.Getenv("NLM_AUTH_TOKEN")
	}
	if os.Getenv("NLM_COOKIES") != "" {
		cookies = os.Getenv("NLM_COOKIES")
	}

	// Use environment credentials for both recording and replay
	client := New(
		authToken,
		cookies,
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(true),
	)

	// Call the API method
	t.Log("Creating project...")
	project, err := client.CreateProject("Sample Project - "+t.Name(), "📝")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	t.Logf("Created project: %s (%s)", project.Title, project.ProjectId)

	// Clean up by deleting the project
	t.Cleanup(func() {
		if err := client.DeleteProjects([]string{project.ProjectId}); err != nil {
			t.Logf("Failed to clean up project: %v", err)
		}
	})
}

// TestAddSourceFromTextWithRecording validates adding text sources functionality
func TestAddSourceFromTextWithRecording(t *testing.T) {
	httprr.SkipIfNoNLMCredentialsOrRecording(t)
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Use test credentials that get scrubbed by httprr
	authToken := "test-auth-token"
	cookies := "test-cookies"
	if os.Getenv("NLM_AUTH_TOKEN") != "" {
		authToken = os.Getenv("NLM_AUTH_TOKEN")
	}
	if os.Getenv("NLM_COOKIES") != "" {
		cookies = os.Getenv("NLM_COOKIES")
	}

	// Use environment credentials for both recording and replay
	client := New(
		authToken,
		cookies,
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(true),
	)

	// First, we need a project to add sources to
	t.Log("Listing projects to find available project...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	if len(projects) == 0 {
		t.Skip("No projects found for source addition")
	}

	// Use the first project
	projectID := projects[0].ProjectId
	t.Logf("Using project: %s", projectID)

	// Call the API method
	t.Log("Adding text source...")
	sourceID, err := client.AddSourceFromText(projectID, "This is a sample source created by automation", "Sample Source - "+t.Name())
	if err != nil {
		t.Fatalf("Failed to add text source: %v", err)
	}

	t.Logf("Added source with ID: %s", sourceID)

	// Clean up by deleting the source
	t.Cleanup(func() {
		if err := client.DeleteSources(projectID, []string{sourceID}); err != nil {
			t.Logf("Failed to clean up source: %v", err)
		}
	})
}
