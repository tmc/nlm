package api

import (
	"path/filepath"
	"testing"

	"github.com/tmc/nlm/internal/api/testdata"
)

// TestGenerateMockHTTPRRFiles generates all the httprr recording files
// Run with: go test -run TestGenerateMockHTTPRRFiles ./internal/api
func TestGenerateMockHTTPRRFiles(t *testing.T) {
	testdataDir := filepath.Join(".", "testdata")

	err := testdata.GenerateMockHTTPRRFiles(testdataDir)
	if err != nil {
		t.Fatalf("Failed to generate mock httprr files: %v", err)
	}

	t.Log("Successfully generated all mock httprr files")
}