package api

import (
	"net/http"
	"os"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/httprr"
)

// TestListProjectsWithRecording tests the ListRecentlyViewedProjects method
// with request recording and replay using the enhanced httprr.
func TestListProjectsWithRecording(t *testing.T) {
	// Use the enhanced httprr's graceful skipping
	httprr.SkipIfNoNLMCredentialsOrRecording(t)

	// Create NLM test client with enhanced httprr
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Create API client
	client := New(
		os.Getenv("NLM_AUTH_TOKEN"),
		os.Getenv("NLM_COOKIES"),
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(true),
	)

	// Call the API method
	t.Log("Listing projects...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	// Check results
	t.Logf("Found %d projects", len(projects))
	for i, p := range projects {
		t.Logf("Project %d: %s (%s)", i, p.Title, p.ProjectId)
	}

	if len(projects) == 0 {
		t.Logf("Warning: No projects found")
	}
}

// TestCreateProjectWithRecording tests the CreateProject method with httprr recording
func TestCreateProjectWithRecording(t *testing.T) {
	// Use the enhanced httprr's graceful skipping
	httprr.SkipIfNoNLMCredentialsOrRecording(t)

	// Create NLM test client with enhanced httprr
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Create API client
	client := New(
		os.Getenv("NLM_AUTH_TOKEN"),
		os.Getenv("NLM_COOKIES"),
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(true),
	)

	// Call the API method
	t.Log("Creating test project...")
	project, err := client.CreateProject("Test Project - "+t.Name(), "📝")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	t.Logf("Created project: %s (%s)", project.Title, project.ProjectId)

	// Clean up by deleting the test project
	t.Cleanup(func() {
		if err := client.DeleteProjects([]string{project.ProjectId}); err != nil {
			t.Logf("Failed to clean up test project: %v", err)
		}
	})
}

// TestAddSourceFromTextWithRecording tests adding text sources with httprr recording
func TestAddSourceFromTextWithRecording(t *testing.T) {
	// Use the enhanced httprr's graceful skipping
	httprr.SkipIfNoNLMCredentialsOrRecording(t)

	// Create NLM test client with enhanced httprr
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	// Create API client
	client := New(
		os.Getenv("NLM_AUTH_TOKEN"),
		os.Getenv("NLM_COOKIES"),
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(true),
	)

	// First, we need a project to add sources to
	t.Log("Listing projects to find a project...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	// Use the first project
	projectID := projects[0].ProjectId
	t.Logf("Testing with project: %s", projectID)

	// Call the API method
	t.Log("Adding text source...")
	sourceID, err := client.AddSourceFromText(projectID, "This is a test source created by automated test", "Test Source - "+t.Name())
	if err != nil {
		t.Fatalf("Failed to add text source: %v", err)
	}

	t.Logf("Added source with ID: %s", sourceID)

	// Clean up by deleting the test source
	t.Cleanup(func() {
		if err := client.DeleteSources(projectID, []string{sourceID}); err != nil {
			t.Logf("Failed to clean up test source: %v", err)
		}
	})
}
