package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIntegration(t *testing.T) {
	// Skip if running in CI or without proper auth
	if os.Getenv("CI") != "" || os.Getenv("NLM_AUTH_TOKEN") == "" {
		t.Skip("Skipping integration test - requires proper authentication")
	}

	// Run the list command
	output, err := runNlmCommand("list")
	if err != nil {
		t.Fatalf("Error running list command: %v", err)
	}

	// Check if the output contains "Notebooks:"
	if !strings.Contains(output, "Notebooks:") {
		t.Fatalf("Output does not contain 'Notebooks:'\nOutput:\n%s", output)
	}
}

func runNlmCommand(command string) (string, error) {
	cmd := exec.Command("./nlm", strings.Split(command, " ")...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
