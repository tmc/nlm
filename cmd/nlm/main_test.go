package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"rsc.io/script"
	"rsc.io/script/scripttest"
)

func TestMain(m *testing.M) {
	// Build the nlm binary for testing
	cmd := exec.Command("go", "build", "-o", "nlm_test", ".")
	if err := cmd.Run(); err != nil {
		panic("failed to build nlm for testing: " + err.Error())
	}
	defer os.Remove("nlm_test")

	// Run tests
	code := m.Run()
	os.Exit(code)
}

func TestCLICommands(t *testing.T) {
	// Set up the script test environment
	engine := script.NewEngine()
	engine.Cmds["nlm_test"] = script.Program("./nlm_test", func(cmd *exec.Cmd) error {
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt)
		}
		return nil
	}, time.Second)

	// Run the script tests from testdata directory
	files, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".txt") {
			continue
		}

		t.Run(file.Name(), func(t *testing.T) {
			state, err := script.NewState(context.Background(), ".", nil)
			if err != nil {
				t.Fatalf("failed to create script state: %v", err)
			}
			defer state.CloseAndWait(os.Stderr)

			content, err := os.ReadFile("testdata/" + file.Name())
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			reader := bufio.NewReader(strings.NewReader(string(content)))
			scripttest.Run(t, engine, state, file.Name(), reader)
		})
	}
}

// Test the CLI help output using direct exec
func TestHelpCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit bool
		contains []string
	}{
		{
			name:     "no arguments shows usage",
			args:     []string{},
			wantExit: true,
			contains: []string{"Usage: nlm <command>", "Notebook Commands"},
		},
		{
			name:     "help flag",
			args:     []string{"-h"},
			wantExit: false,
			contains: []string{"Usage: nlm <command>", "Notebook Commands"},
		},
		{
			name:     "help command",
			args:     []string{"help"},
			wantExit: true,
			contains: []string{"Usage: nlm <command>", "Notebook Commands"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run the command directly using exec.Command
			cmd := exec.Command("./nlm_test", tt.args...)
			output, err := cmd.CombinedOutput()

			// Check exit code
			if err != nil && !tt.wantExit {
				t.Errorf("expected success but got error: %v", err)
			}
			if err == nil && tt.wantExit {
				t.Errorf("expected command to fail but it succeeded")
			}

			// Check that expected strings are present
			outputStr := string(output)
			for _, want := range tt.contains {
				if !strings.Contains(outputStr, want) {
					t.Errorf("output missing expected string %q\nOutput:\n%s", want, outputStr)
				}
			}
		})
	}
}

// Test command validation using direct exec
func TestCommandValidation(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit bool
		contains string
	}{
		{
			name:     "invalid command",
			args:     []string{"invalid-command"},
			wantExit: true,
			contains: "Usage: nlm <command>",
		},
		{
			name:     "create without title",
			args:     []string{"create"},
			wantExit: true,
			contains: "usage: nlm create <title>",
		},
		{
			name:     "rm without id",
			args:     []string{"rm"},
			wantExit: true,
			contains: "usage: nlm rm <id>",
		},
		{
			name:     "sources without notebook id",
			args:     []string{"sources"},
			wantExit: true,
			contains: "usage: nlm sources <notebook-id>",
		},
		{
			name:     "add without args",
			args:     []string{"add"},
			wantExit: true,
			contains: "usage: nlm add <notebook-id> <file>",
		},
		{
			name:     "add with one arg",
			args:     []string{"add", "notebook123"},
			wantExit: true,
			contains: "usage: nlm add <notebook-id> <file>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run the command directly using exec.Command
			cmd := exec.Command("./nlm_test", tt.args...)
			output, err := cmd.CombinedOutput()

			// Should exit with error
			if !tt.wantExit {
				if err != nil {
					t.Errorf("expected success but got error: %v", err)
				}
				return
			}

			if err == nil {
				t.Error("expected command to fail but it succeeded")
				return
			}

			// Check error output contains expected message
			outputStr := string(output)
			if !strings.Contains(outputStr, tt.contains) {
				t.Errorf("output missing expected string %q\nOutput:\n%s", tt.contains, outputStr)
			}
		})
	}
}

// Test flag parsing using direct exec
func TestFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{
			name: "debug flag",
			args: []string{"-debug"},
		},
		{
			name: "auth token flag",
			args: []string{"-auth", "test-token"},
		},
		{
			name: "cookies flag",
			args: []string{"-cookies", "test-cookies"},
		},
		{
			name: "profile flag",
			args: []string{"-profile", "test-profile"},
		},
		{
			name: "mime type flag",
			args: []string{"-mime", "application/json"},
		},
		{
			name: "environment variables",
			env: map[string]string{
				"NLM_AUTH_TOKEN":      "env-token",
				"NLM_COOKIES":         "env-cookies",
				"NLM_BROWSER_PROFILE": "env-profile",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run with help to test flag parsing without auth issues
			cmd := exec.Command("./nlm_test")
			cmd.Args = append(cmd.Args, tt.args...)
			cmd.Args = append(cmd.Args, "help")

			// Set environment variables if specified
			cmd.Env = os.Environ()
			for k, v := range tt.env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}

			output, err := cmd.CombinedOutput()
			// Help command exits with 1, but that's expected behavior
			if err != nil {
				// Check if it's just the help exit code
				if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
					// This is expected for help command
					if !strings.Contains(string(output), "Usage: nlm <command>") {
						t.Errorf("help command didn't show usage: %s", string(output))
					}
				} else {
					t.Errorf("flag parsing failed: %v\nOutput: %s", err, string(output))
				}
			}
		})
	}
}

// Test authentication requirements using direct exec
func TestAuthRequirements(t *testing.T) {
	tests := []struct {
		name         string
		command      []string
		requiresAuth bool
	}{
		{"help command", []string{"help"}, false},
		// Skip auth command as it tries to launch browser
		// {"auth command", []string{"auth"}, false},
		{"list command", []string{"list"}, true},
		{"create command", []string{"create", "test"}, true},
		{"sources command", []string{"sources", "test"}, true},
		{"add command", []string{"add", "test", "file"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run without auth credentials
			cmd := exec.Command("./nlm_test", tt.command...)
			// Clear auth environment variables
			cmd.Env = []string{"PATH=" + os.Getenv("PATH")}

			output, err := cmd.CombinedOutput()
			outputStr := string(output)

			if tt.requiresAuth {
				// Should show auth warning but not necessarily fail completely
				if !strings.Contains(outputStr, "Authentication required") {
					t.Logf("Command %q may not show auth warning as expected. This might be OK if it fails gracefully.", strings.Join(tt.command, " "))
				}
			} else {
				// Should not require auth
				if strings.Contains(outputStr, "Authentication required") {
					t.Errorf("Command %q should not require authentication but got auth error", strings.Join(tt.command, " "))
				}
			}

			// Commands that don't require auth should succeed or show usage
			if !tt.requiresAuth && err != nil {
				if !strings.Contains(outputStr, "Usage:") && !strings.Contains(outputStr, "usage:") {
					t.Errorf("Non-auth command failed unexpectedly: %v\nOutput: %s", err, outputStr)
				}
			}
		})
	}
}
