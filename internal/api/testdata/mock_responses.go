package testdata

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MockResponse represents a mock API response
type MockResponse struct {
	Projects []MockProject `json:"projects"`
	Sources  []MockSource  `json:"sources"`
}

// MockProject represents a mock notebook/project
type MockProject struct {
	ProjectID    string    `json:"project_id"`
	Title        string    `json:"title"`
	Emoji        string    `json:"emoji"`
	SourceCount  int       `json:"source_count"`
	CreatedTime  time.Time `json:"created_time"`
	ModifiedTime time.Time `json:"modified_time"`
}

// MockSource represents a mock source
type MockSource struct {
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Content  string `json:"content,omitempty"`
	URL      string `json:"url,omitempty"`
}

// GenerateMockHTTPRRFiles creates httprr recording files for all tests
func GenerateMockHTTPRRFiles(testdataDir string) error {
	// Create mock projects
	projects := []MockProject{
		{
			ProjectID:    "mock-project-001",
			Title:        "Test Project 1",
			Emoji:        "📝",
			SourceCount:  3,
			CreatedTime:  time.Now().Add(-24 * time.Hour),
			ModifiedTime: time.Now(),
		},
		{
			ProjectID:    "mock-project-002",
			Title:        "Test Project 2",
			Emoji:        "📊",
			SourceCount:  0,
			CreatedTime:  time.Now().Add(-48 * time.Hour),
			ModifiedTime: time.Now().Add(-12 * time.Hour),
		},
	}

	// Create mock sources
	sources := []MockSource{
		{
			SourceID: "mock-source-001",
			Title:    "Test Document",
			Type:     "text",
			Content:  "This is test content for the mock source.",
		},
		{
			SourceID: "mock-source-002",
			Title:    "Example Website",
			Type:     "url",
			URL:      "https://example.com",
		},
	}

	// Generate httprr files for each test
	tests := []string{
		"TestListProjectsWithRecording",
		"TestCreateProjectWithRecording",
		"TestAddSourceFromTextWithRecording",
		"TestNotebookCommands_ListProjects",
		"TestNotebookCommands_CreateProject",
		"TestNotebookCommands_DeleteProject",
		"TestSourceCommands_ListSources",
		"TestSourceCommands_AddTextSource",
		"TestSourceCommands_AddURLSource",
		"TestSourceCommands_DeleteSource",
		"TestSourceCommands_RenameSource",
		"TestAudioCommands_CreateAudioOverview",
		"TestAudioCommands_GetAudioOverview",
		"TestGenerationCommands_GenerateNotebookGuide",
		"TestGenerationCommands_GenerateOutline",
		"TestMiscCommands_Heartbeat",
		"TestVideoCommands_CreateVideoOverview",
	}

	for _, test := range tests {
		if err := GenerateHTTPRRFile(testdataDir, test, projects, sources); err != nil {
			return fmt.Errorf("failed to generate httprr for %s: %w", test, err)
		}
	}

	return nil
}

// GenerateHTTPRRFile creates a single httprr recording file
func GenerateHTTPRRFile(dir, testName string, projects []MockProject, sources []MockSource) error {
	filePath := filepath.Join(dir, testName+".httprr")
	
	// Create mock HTTP request/response based on test type
	var request, response string
	
	switch testName {
	case "TestListProjectsWithRecording", "TestNotebookCommands_ListProjects":
		request = createListProjectsRequest()
		response = createListProjectsResponse(projects)
	case "TestCreateProjectWithRecording", "TestNotebookCommands_CreateProject":
		request = createCreateProjectRequest()
		response = createCreateProjectResponse(projects[0])
	case "TestSourceCommands_ListSources":
		request = createListSourcesRequest(projects[0].ProjectID)
		response = createListSourcesResponse(sources)
	case "TestSourceCommands_AddTextSource", "TestAddSourceFromTextWithRecording":
		request = createAddSourceRequest(projects[0].ProjectID)
		response = createAddSourceResponse(sources[0])
	default:
		// Generic success response for other tests
		request = createGenericRequest(testName)
		response = createGenericSuccessResponse()
	}

	// Write httprr format file
	content := fmt.Sprintf("httprr trace v1\n%d %d\n%s%s",
		len(request), len(response), request, response)
	
	return os.WriteFile(filePath, []byte(content), 0644)
}

// Helper functions to create mock requests and responses
func createListProjectsRequest() string {
	return `POST /_/NotebookLmUi/data/batchexecute HTTP/1.1\r\nHost: notebooklm.google.com\r\n\r\n[["wXbhsf","[]"]]`
}

func createListProjectsResponse(projects []MockProject) string {
	// Create batchexecute format response
	projectData := ""
	for _, p := range projects {
		projectData += fmt.Sprintf(`["%s","%s",[],%d,"%s"]`,
			p.ProjectID, p.Title, p.SourceCount, p.Emoji)
	}
	return fmt.Sprintf(`HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n[["wrb.fr","wXbhsf",[%s]]]`, projectData)
}

func createCreateProjectRequest() string {
	return `POST /_/NotebookLmUi/data/batchexecute HTTP/1.1\r\nHost: notebooklm.google.com\r\n\r\n[["CCqFvf",["Test Project","📝"]]]`
}

func createCreateProjectResponse(project MockProject) string {
	return fmt.Sprintf(`HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n[["wrb.fr","CCqFvf",["%s","%s","%s"]]]`,
		project.ProjectID, project.Title, project.Emoji)
}

func createListSourcesRequest(projectID string) string {
	return fmt.Sprintf(`POST /_/NotebookLmUi/data/batchexecute HTTP/1.1\r\nHost: notebooklm.google.com\r\n\r\n[["rLM1Ne",["%s"]]]`, projectID)
}

func createListSourcesResponse(sources []MockSource) string {
	sourceData := ""
	for _, s := range sources {
		sourceData += fmt.Sprintf(`["%s","%s","%s"]`, s.SourceID, s.Title, s.Type)
	}
	return fmt.Sprintf(`HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n[["wrb.fr","rLM1Ne",[%s]]]`, sourceData)
}

func createAddSourceRequest(projectID string) string {
	return fmt.Sprintf(`POST /_/NotebookLmUi/data/batchexecute HTTP/1.1\r\nHost: notebooklm.google.com\r\n\r\n[["izAoDd",["%s","test content","Test Source"]]]`, projectID)
}

func createAddSourceResponse(source MockSource) string {
	return fmt.Sprintf(`HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n[["wrb.fr","izAoDd",["%s","%s"]]]`,
		source.SourceID, source.Title)
}

func createGenericRequest(testName string) string {
	return fmt.Sprintf(`POST /_/NotebookLmUi/data/batchexecute HTTP/1.1\r\nHost: notebooklm.google.com\r\n\r\n[["%s",[]]]`, testName)
}

func createGenericSuccessResponse() string {
	return `HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n[["wrb.fr","generic",["success"]]]`
}