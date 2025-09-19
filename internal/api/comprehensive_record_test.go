//go:build integration
// +build integration

package api

import (
	"net/http"
	"os"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/httprr"
)

// TestNotebookCommands_ListProjects tests the list projects command
func TestNotebookCommands_ListProjects(t *testing.T) {
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

	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	t.Logf("Found %d projects", len(projects))
	for i, p := range projects {
		t.Logf("Project %d: %s (%s)", i, p.Title, p.ProjectId)
	}
}

// TestNotebookCommands_CreateProject records the create project command
func TestNotebookCommands_CreateProject(t *testing.T) {
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

	project, err := client.CreateProject("Test Project for Recording", "📝")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	t.Logf("Created project: %s (%s)", project.Title, project.ProjectId)

	// Store project ID for cleanup
	t.Cleanup(func() {
		if err := client.DeleteProjects([]string{project.ProjectId}); err != nil {
			t.Logf("Failed to clean up test project: %v", err)
		}
	})
}

// TestNotebookCommands_DeleteProject records the delete project command
func TestNotebookCommands_DeleteProject(t *testing.T) {
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

	// First create a project to delete
	project, err := client.CreateProject("Test Project for Delete Recording", "🗑️")
	if err != nil {
		t.Fatalf("Failed to create project for deletion test: %v", err)
	}
	t.Logf("Created project to delete: %s (%s)", project.Title, project.ProjectId)

	// Now delete it
	err = client.DeleteProjects([]string{project.ProjectId})
	if err != nil {
		t.Fatalf("Failed to delete project: %v", err)
	}
	t.Logf("Successfully deleted project: %s", project.ProjectId)
}

// TestSourceCommands_ListSources records the list sources command
func TestSourceCommands_ListSources(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId
	project, err := client.GetProject(projectID)
	if err != nil {
		t.Fatalf("Failed to get project: %v", err)
	}

	t.Logf("Found %d sources in project %s", len(project.Sources), project.Title)
}

// TestSourceCommands_AddTextSource records adding a text source
func TestSourceCommands_AddTextSource(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId
	sourceID, err := client.AddSourceFromText(projectID, "This is a test source for httprr recording. It contains sample text to demonstrate the API functionality.", "Test Source for Recording")
	if err != nil {
		t.Fatalf("Failed to add text source: %v", err)
	}

	t.Logf("Added text source: %s", sourceID)

	// Cleanup
	t.Cleanup(func() {
		if err := client.DeleteSources(projectID, []string{sourceID}); err != nil {
			t.Logf("Failed to clean up test source: %v", err)
		}
	})
}

// TestSourceCommands_AddURLSource records adding a URL source
func TestSourceCommands_AddURLSource(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId
	sourceID, err := client.AddSourceFromURL(projectID, "https://example.com")
	if err != nil {
		t.Fatalf("Failed to add URL source: %v", err)
	}

	t.Logf("Added URL source: %s", sourceID)

	// Cleanup
	t.Cleanup(func() {
		if err := client.DeleteSources(projectID, []string{sourceID}); err != nil {
			t.Logf("Failed to clean up test source: %v", err)
		}
	})
}

// TestSourceCommands_DeleteSource records the delete source command
func TestSourceCommands_DeleteSource(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId

	// First add a source to delete
	sourceID, err := client.AddSourceFromText(projectID, "This is a test source that will be deleted for httprr recording.", "Test Source for Delete Recording")
	if err != nil {
		t.Fatalf("Failed to add source for deletion test: %v", err)
	}
	t.Logf("Created source to delete: %s", sourceID)

	// Now delete it
	err = client.DeleteSources(projectID, []string{sourceID})
	if err != nil {
		t.Fatalf("Failed to delete source: %v", err)
	}
	t.Logf("Successfully deleted source: %s", sourceID)
}

// TestSourceCommands_RenameSource records the rename source command
func TestSourceCommands_RenameSource(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId

	// First add a source to rename
	sourceID, err := client.AddSourceFromText(projectID, "This is a test source that will be renamed for httprr recording.", "Original Source Name")
	if err != nil {
		t.Fatalf("Failed to add source for rename test: %v", err)
	}
	t.Logf("Created source to rename: %s", sourceID)

	// Now rename it
	newTitle := "Renamed Source for Recording"
	_, err = client.MutateSource(sourceID, &pb.Source{Title: newTitle})
	if err != nil {
		t.Fatalf("Failed to rename source: %v", err)
	}
	t.Logf("Successfully renamed source to: %s", newTitle)

	// Cleanup
	t.Cleanup(func() {
		if err := client.DeleteSources(projectID, []string{sourceID}); err != nil {
			t.Logf("Failed to clean up test source: %v", err)
		}
	})
}

// TestAudioCommands_CreateAudioOverview records the create audio overview command
func TestAudioCommands_CreateAudioOverview(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId
	instructions := "Create a brief overview suitable for recording API tests"

	result, err := client.CreateAudioOverview(projectID, instructions)
	if err != nil {
		t.Fatalf("Failed to create audio overview: %v", err)
	}

	t.Logf("Created audio overview: %s (Ready: %v)", result.AudioID, result.IsReady)
}

// TestAudioCommands_GetAudioOverview records the get audio overview command
func TestAudioCommands_GetAudioOverview(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId

	result, err := client.GetAudioOverview(projectID)
	if err != nil {
		// This might fail if no audio overview exists, which is expected
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no audio") {
			t.Logf("No audio overview found for project %s (expected)", projectID)
			return
		}
		t.Fatalf("Failed to get audio overview: %v", err)
	}

	t.Logf("Got audio overview: %s (Ready: %v)", result.AudioID, result.IsReady)
}

// TestGenerationCommands_GenerateNotebookGuide records the generate guide command
func TestGenerationCommands_GenerateNotebookGuide(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId

	guide, err := client.GenerateNotebookGuide(projectID)
	if err != nil {
		t.Fatalf("Failed to generate notebook guide: %v", err)
	}

	t.Logf("Generated guide with %d characters", len(guide.Content))
}

// TestGenerationCommands_GenerateOutline records the generate outline command
func TestGenerationCommands_GenerateOutline(t *testing.T) {
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

	// Get a project to test with
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("No projects found to test with")
	}

	projectID := projects[0].ProjectId

	outline, err := client.GenerateOutline(projectID)
	if err != nil {
		t.Fatalf("Failed to generate outline: %v", err)
	}

	t.Logf("Generated outline with %d characters", len(outline.Content))
}

// TestMiscCommands_Heartbeat records the heartbeat command
func TestMiscCommands_Heartbeat(t *testing.T) {
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

	_ = New(
		authToken,
		cookies,
		batchexecute.WithHTTPClient(httpClient),
		batchexecute.WithDebug(false),
	)

	// The heartbeat method might not exist or might be a no-op
	// This is just to record any potential network activity
	t.Logf("Heartbeat test completed (no-op)")
}

func TestVideoCommands_CreateVideoOverview(t *testing.T) {
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

	// First, we need a project to create video for
	t.Log("Listing projects to find available project...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		t.Fatalf("Failed to list projects: %v", err)
	}

	if len(projects) == 0 {
		t.Skip("No projects found for video overview creation")
	}

	projectID := projects[0].ProjectId
	t.Logf("Using project: %s", projectID)

	t.Log("Creating video overview...")
	result, err := client.CreateVideoOverview(projectID, "Create a comprehensive video overview of this notebook")
	if err != nil {
		// Video creation might not be available yet, or might require special permissions
		// Log the error but don't fail the test if it's a service availability issue
		if strings.Contains(err.Error(), "Service unavailable") || strings.Contains(err.Error(), "API error 3") {
			t.Logf("Video overview creation not available: %v", err)
			t.Skip("Video overview creation service not available")
		}
		t.Fatalf("Failed to create video overview: %v", err)
	}

	t.Logf("Video overview creation result:")
	t.Logf("  Project ID: %s", result.ProjectID)
	t.Logf("  Video ID: %s", result.VideoID)
	t.Logf("  Title: %s", result.Title)
	t.Logf("  Is Ready: %v", result.IsReady)

	if result.VideoData != "" {
		t.Logf("  Video Data: %s", result.VideoData[:min(100, len(result.VideoData))]+"...")
	}
}
