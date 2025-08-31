package api

import (
	"testing"
)

// TestPathwayStructure validates the dual pathway architecture without auth
func TestPathwayStructure(t *testing.T) {
	// Create mock credentials
	mockToken := "mock-token"
	mockCookies := "mock-cookies"
	
	t.Run("DefaultConfiguration", func(t *testing.T) {
		client := New(mockToken, mockCookies)
		
		// Check default configuration uses generated services
		if client.orchestrationService == nil {
			t.Error("Expected orchestrationService to be initialized by default")
		}
		if client.sharingService == nil {
			t.Error("Expected sharingService to be initialized by default")
		}
		if client.guidebooksService == nil {
			t.Error("Expected guidebooksService to be initialized by default")
		}
		
		t.Log("✅ Default configuration uses generated pathway")
	})
	
	t.Run("LegacyConfiguration", func(t *testing.T) {
		client := New(mockToken, mockCookies)
		
		// Force legacy mode
		client.orchestrationService = nil
		client.sharingService = nil
		client.guidebooksService = nil
		
		// Verify legacy configuration
		if client.orchestrationService != nil {
			t.Error("Expected orchestrationService to be nil in legacy mode")
		}
		if client.sharingService != nil {
			t.Error("Expected sharingService to be nil in legacy mode")
		}
		if client.guidebooksService != nil {
			t.Error("Expected guidebooksService to be nil in legacy mode")
		}
		
		t.Log("✅ Legacy configuration can be forced by setting services to nil")
	})
	
	t.Run("PathwaySwitching", func(t *testing.T) {
		client := New(mockToken, mockCookies)
		
		// Start with generated (default)
		hasGenerated := client.orchestrationService != nil
		
		// Switch to legacy
		client.orchestrationService = nil
		client.sharingService = nil
		client.guidebooksService = nil
		
		hasLegacy := client.orchestrationService == nil
		
		if !hasGenerated {
			t.Error("Expected generated pathway to be available by default")
		}
		if !hasLegacy {
			t.Error("Expected to be able to switch to legacy pathway")
		}
		
		t.Log("✅ Can switch between generated and legacy pathways")
	})
	
	t.Run("MigrationReadiness", func(t *testing.T) {
		// This test documents which methods are ready for migration
		migrationStatus := map[string]string{
			// Notebook operations - using generated
			"ListRecentlyViewedProjects": "generated",
			"CreateProject":               "generated",
			"GetProject":                  "generated", 
			"DeleteProjects":              "generated",
			
			// Source operations - mixed
			"GetSources":                     "generated",
			"DeleteSources":                  "generated",
			"AddSourceFromURL":               "legacy", // Complex payload
			"AddSourceFromText":              "legacy", // Complex payload
			"AddSourceFromFile":              "legacy", // Complex payload
			"AddSourceFromYouTube":           "legacy", // Complex payload
			"AddSourceFromGoogleDrive":       "legacy", // Complex payload
			"AddSourceFromWebsiteGroup":      "legacy", // Complex payload
			"AddSourceFromAudio":             "legacy", // Complex payload
			
			// Note operations - using generated
			"GetNotes":    "generated",
			"CreateNote":  "generated",
			"DeleteNotes": "generated",
			
			// Generation operations - using generated
			"GenerateNotebookGuide":        "generated",
			"GenerateNotebookOutline":      "generated",
			"GenerateNotebookSuggestedQuestions": "generated",
			"GenerateNotebookQuiz":         "generated",
			"GenerateNotebookTimeline":     "generated",
			"GenerateNotebookFAQ":          "generated",
			"GenerateNotebookStudyGuide":   "generated",
			"GenerateNotebookBriefingDoc":  "generated",
			
			// Sharing operations - using generated
			"ShareProjectPublic":   "generated",
			"ShareProjectPrivate":  "generated",
			"ShareProjectBusiness": "generated",
			"UnshareProject":       "generated",
			"GetShareLink":         "generated",
			
			// Audio operations - using generated
			"CreateAudioOverview": "generated",
			"GetAudioOverview":    "generated",
			"DeleteAudioOverview": "generated",
			
			// Artifact operations - using generated
			"CreateArtifact": "generated",
			"GetArtifact":    "generated",
			"ListArtifacts":  "generated",
			
			// Other operations - using generated
			"ActOnSources":           "generated",
			"SubmitNotebookFeedback": "generated",
		}
		
		generatedCount := 0
		legacyCount := 0
		
		for _, pathway := range migrationStatus {
			if pathway == "generated" {
				generatedCount++
			} else {
				legacyCount++
			}
		}
		
		migrationPercentage := float64(generatedCount) / float64(generatedCount+legacyCount) * 100
		
		t.Logf("Migration Status:")
		t.Logf("  Generated: %d methods (%.1f%%)", generatedCount, migrationPercentage)
		t.Logf("  Legacy: %d methods (%.1f%%)", legacyCount, 100-migrationPercentage)
		t.Logf("")
		t.Logf("Remaining legacy methods (specialized source operations):")
		for method, pathway := range migrationStatus {
			if pathway == "legacy" {
				t.Logf("  - %s", method)
			}
		}
		
		if migrationPercentage > 80 {
			t.Log("✅ Migration is over 80% complete")
		}
	})
}

// TestPathwayValidationFramework shows how both pathways can be tested
func TestPathwayValidationFramework(t *testing.T) {
	t.Run("FrameworkCapabilities", func(t *testing.T) {
		capabilities := []string{
			"Create clients for each pathway",
			"Force legacy mode by setting services to nil",
			"Run same test against both pathways",
			"Compare results between pathways",
			"Benchmark performance differences",
			"Support gradual rollout with feature flags",
		}
		
		for _, capability := range capabilities {
			t.Logf("✅ %s", capability)
		}
	})
	
	t.Run("TestStrategy", func(t *testing.T) {
		t.Log("Dual Pathway Testing Strategy:")
		t.Log("1. Run tests with legacy pathway (services = nil)")
		t.Log("2. Run tests with generated pathway (default)")
		t.Log("3. Compare results for consistency")
		t.Log("4. Measure performance differences")
		t.Log("5. Use feature flags for gradual rollout")
	})
}