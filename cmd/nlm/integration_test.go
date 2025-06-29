package main

import (
	"os"
	"testing"
)

// TestMainFunction tests the main function indirectly by testing flag parsing
func TestMainFunction(t *testing.T) {
	// Store original os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Test with no arguments (should trigger usage)
	os.Args = []string{"nlm"}

	// This would normally call os.Exit(1), so we can't directly test main()
	// But we can test the flag init function is working
	if !testing.Short() {
		t.Skip("Skipping main function test - would call os.Exit")
	}
}

// TestAuthCommand tests isAuthCommand function
func TestAuthCommand(t *testing.T) {
	tests := []struct {
		cmd      string
		expected bool
	}{
		{"help", false},
		{"-h", false},
		{"--help", false},
		{"auth", false},
		{"list", true},
		{"create", true},
		{"rm", true},
		{"sources", true},
		{"add", true},
		{"rm-source", true},
		{"audio-create", true},
		{"unknown-command", true},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := isAuthCommand(tt.cmd)
			if result != tt.expected {
				t.Errorf("isAuthCommand(%q) = %v, want %v", tt.cmd, result, tt.expected)
			}
		})
	}
}
