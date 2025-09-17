package main

import (
	"testing"
)

// TestMainFunction is deprecated - use scripttest framework instead
// The scripttest files in testdata/ provide better coverage of the CLI
// For example: testdata/comprehensive_auth.txt tests command parsing
func TestMainFunction(t *testing.T) {
	t.Skip("Deprecated - use scripttest framework (see TestCLICommands and TestComprehensiveScripts)")
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
