package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestDualPathwayConsistency tests that both legacy and generated pathways
// behave consistently for all migrated operations
func TestDualPathwayConsistency(t *testing.T) {
	// Create a temporary home directory for test isolation
	tmpHome, err := os.MkdirTemp("", "nlm-test-home-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	tests := []struct {
		name    string
		command []string
		wantErr bool
		errMsg  string // Expected error message pattern
	}{
		// Project operations
		{"list projects", []string{"list"}, true, "Authentication required"},
		{"create project validation", []string{"create"}, true, "usage: nlm create <title>"},
		{"delete project validation", []string{"rm"}, true, "usage: nlm rm <id>"},
		
		// Source operations  
		{"list sources validation", []string{"sources"}, true, "usage: nlm sources <notebook-id>"},
		{"add source validation", []string{"add"}, true, "usage: nlm add <notebook-id> <file>"},
		{"add source partial args", []string{"add", "notebook123"}, true, "usage: nlm add <notebook-id> <file>"},
		
		// Note operations
		{"list notes validation", []string{"notes"}, true, "usage: nlm notes <notebook-id>"},
		{"create note validation", []string{"new-note"}, true, "usage: nlm new-note <notebook-id> <title>"},
		{"create note partial args", []string{"new-note", "notebook123"}, true, "usage: nlm new-note <notebook-id> <title>"},
		
		// Audio operations
		{"create audio validation", []string{"audio-create"}, true, "usage: nlm audio-create <notebook-id> <instructions>"},
		{"create audio partial args", []string{"audio-create", "notebook123"}, true, "usage: nlm audio-create <notebook-id> <instructions>"},
		{"get audio validation", []string{"audio-get"}, true, "usage: nlm audio-get <notebook-id>"},
		{"delete audio validation", []string{"audio-rm"}, true, "usage: nlm audio-rm <notebook-id>"},
		
		// Help commands should work regardless of pathway
		{"help command", []string{"help"}, false, "Usage: nlm <command>"},
		{"help flag", []string{"-h"}, false, "Usage: nlm <command>"},
	}

	pathways := []struct {
		name   string
		useGen bool
		envVar string
	}{
		{"Legacy", false, "false"},
		{"Generated", true, "true"},
	}

	for _, pathway := range pathways {
		t.Run(pathway.name+"_pathway", func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					cmd := exec.Command("./nlm_test", tt.command...)
					
					// Set up environment
					env := []string{
						"PATH=" + os.Getenv("PATH"),
						"HOME=" + tmpHome,
						"NLM_USE_GENERATED_CLIENT=" + pathway.envVar,
						// Clear auth for consistent testing
						"NLM_AUTH_TOKEN=",
						"NLM_COOKIES=",
					}
					cmd.Env = env
					
					output, err := cmd.CombinedOutput()
					outputStr := string(output)
					
					// Check error expectation
					if tt.wantErr && err == nil {
						t.Errorf("expected command to fail but it succeeded. Output: %s", outputStr)
						return
					}
					if !tt.wantErr && err != nil {
						t.Errorf("expected command to succeed but it failed: %v. Output: %s", err, outputStr)
						return
					}
					
					// Check error message pattern
					if tt.errMsg != "" && !strings.Contains(outputStr, tt.errMsg) {
						t.Errorf("expected output to contain %q, but got: %s", tt.errMsg, outputStr)
					}
				})
			}
		})
	}
}

// TestPathwayBehaviorWithAuth tests behavior when auth is provided
func TestPathwayBehaviorWithAuth(t *testing.T) {
	// Create a temporary home directory for test isolation
	tmpHome, err := os.MkdirTemp("", "nlm-test-home-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	testCommands := []struct {
		name    string
		command []string
	}{
		{"list projects", []string{"list"}},
		{"list sources", []string{"sources", "test-notebook"}},
		{"list notes", []string{"notes", "test-notebook"}},
		{"get audio", []string{"audio-get", "test-notebook"}},
	}

	pathways := []struct {
		name   string
		useGen bool
		envVar string
	}{
		{"Legacy", false, "false"},
		{"Generated", true, "true"},
	}

	for _, pathway := range pathways {
		t.Run(pathway.name+"_pathway_with_auth", func(t *testing.T) {
			for _, tc := range testCommands {
				t.Run(tc.name, func(t *testing.T) {
					cmd := exec.Command("./nlm_test", tc.command...)
					
					// Set up environment with auth
					env := []string{
						"PATH=" + os.Getenv("PATH"),
						"HOME=" + tmpHome,
						"NLM_USE_GENERATED_CLIENT=" + pathway.envVar,
						"NLM_AUTH_TOKEN=test-token",
						"NLM_COOKIES=test-cookies",
					}
					cmd.Env = env
					
					output, err := cmd.CombinedOutput()
					outputStr := string(output)
					
					// With auth, commands should fail gracefully (not due to missing auth)
					// They will fail due to network/server issues but that's expected in tests
					if strings.Contains(outputStr, "Authentication required") {
						t.Errorf("command should not require auth when credentials are provided. Output: %s", outputStr)
					}
					
					// Both pathways should behave similarly when failing
					// We expect some kind of network/connection error, not auth error
					t.Logf("Pathway %s for %s: %v (output: %s)", pathway.name, tc.name, err, outputStr)
				})
			}
		})
	}
}

// TestDebugModeConsistency tests that debug mode works with both pathways
func TestDebugModeConsistency(t *testing.T) {
	// Create a temporary home directory for test isolation
	tmpHome, err := os.MkdirTemp("", "nlm-test-home-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	pathways := []struct {
		name   string
		useGen bool
		envVar string
	}{
		{"Legacy", false, "false"},
		{"Generated", true, "true"},
	}

	for _, pathway := range pathways {
		t.Run(pathway.name+"_debug_mode", func(t *testing.T) {
			cmd := exec.Command("./nlm_test", "list")
			
			env := []string{
				"PATH=" + os.Getenv("PATH"),
				"HOME=" + tmpHome,
				"NLM_USE_GENERATED_CLIENT=" + pathway.envVar,
				"NLM_DEBUG=true",
				"NLM_AUTH_TOKEN=test-token",
				"NLM_COOKIES=test-cookies",
			}
			cmd.Env = env
			
			output, err := cmd.CombinedOutput()
			outputStr := string(output)
			
			// Debug mode should produce some kind of debug output
			// The exact content may vary between pathways but both should handle debug mode
			t.Logf("Debug mode output for %s pathway: %s", pathway.name, outputStr)
			
			// Verify command runs (may fail due to network but not due to debug issues)
			if err != nil && strings.Contains(outputStr, "Authentication required") {
				t.Errorf("debug mode should not affect authentication: %s", outputStr)
			}
		})
	}
}