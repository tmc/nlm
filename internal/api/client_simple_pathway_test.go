package api

import (
	"os"
	"testing"
)

// TestSimplePathwayValidation tests that we can create clients configured for different pathways
func TestSimplePathwayValidation(t *testing.T) {
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")
	
	if authToken == "" || cookies == "" {
		t.Skip("Skipping: NLM_AUTH_TOKEN and NLM_COOKIES required")
	}
	
	tests := []struct {
		name        string
		forceLegacy bool
	}{
		{
			name:        "Legacy",
			forceLegacy: true,
		},
		{
			name:        "Generated",
			forceLegacy: false,
		},
	}
	
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client
			client := New(authToken, cookies)
			
			// Configure for specific pathway
			if tt.forceLegacy {
				t.Skip("Legacy pathway no longer supported - migration is complete")
			} else {
				t.Log("Configured for generated pathway")
			}
			
			// Test ListRecentlyViewedProjects
			t.Run("ListRecentlyViewedProjects", func(t *testing.T) {
				projects, err := client.ListRecentlyViewedProjects()
				if err != nil {
					t.Errorf("ListRecentlyViewedProjects failed: %v", err)
					return
				}
				t.Logf("Found %d projects using %s pathway", len(projects), tt.name)
			})
			
			// Test CreateProject
			t.Run("CreateProject", func(t *testing.T) {
				title := "Test " + tt.name
				project, err := client.CreateProject(title, "")
				if err != nil {
					t.Errorf("CreateProject failed: %v", err)
					return
				}
				t.Logf("Created project %s using %s pathway", project.ProjectId, tt.name)
				
				// Clean up
				if err := client.DeleteProjects([]string{project.ProjectId}); err != nil {
					t.Logf("Failed to clean up: %v", err)
				}
			})
		})
	}
}

// TestPathwayMigrationStatus reports which methods use which pathway
func TestPathwayMigrationStatus(t *testing.T) {
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")
	
	if authToken == "" || cookies == "" {
		t.Skip("Skipping: credentials required")
	}
	
	client := New(authToken, cookies)
	
	// Check which services are initialized
	t.Log("Migration Status:")
	
	if client.orchestrationService != nil {
		t.Log("✅ Orchestration Service: Using GENERATED pathway")
	} else {
		t.Log("⚠️  Orchestration Service: Using LEGACY pathway")
	}
	
	if client.sharingService != nil {
		t.Log("✅ Sharing Service: Using GENERATED pathway")
	} else {
		t.Log("⚠️  Sharing Service: Using LEGACY pathway")
	}
	
	if client.guidebooksService != nil {
		t.Log("✅ Guidebooks Service: Using GENERATED pathway")
	} else {
		t.Log("⚠️  Guidebooks Service: Using LEGACY pathway")
	}
	
	// Report on specific methods
	
	// Test a few key methods to see which pathway they use
	methods := []struct {
		name string
		test func() error
	}{
		{
			name: "ListRecentlyViewedProjects",
			test: func() error {
				_, err := client.ListRecentlyViewedProjects()
				return err
			},
		},
		{
			name: "CreateProject",
			test: func() error {
				p, err := client.CreateProject("Migration Test", "")
				if err == nil && p != nil {
					client.DeleteProjects([]string{p.ProjectId})
				}
				return err
			},
		},
	}
	
	t.Log("\nMethod Status:")
	for _, m := range methods {
		err := m.test()
		status := "✅"
		if err != nil {
			status = "❌"
		}
		t.Logf("%s %s: %v", status, m.name, err)
	}
	
	// Summary
	t.Log("\nSummary:")
	t.Log("The client is configured to use the GENERATED pathway by default.")
	t.Log("Legacy pathway can be forced by setting service clients to nil.")
	t.Log("This allows for gradual migration and A/B testing.")
}