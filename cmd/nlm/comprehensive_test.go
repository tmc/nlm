package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rsc.io/script"
	"rsc.io/script/scripttest"
)

// TestComprehensiveScripts runs all comprehensive scripttest files
// This provides additional test coverage for complex scenarios
func TestComprehensiveScripts(t *testing.T) {
	// Build test binary if needed
	if _, err := os.Stat("./nlm_test"); os.IsNotExist(err) {
		t.Skip("nlm_test binary not found - run TestMain first")
	}

	// Find all comprehensive test files
	testFiles, err := filepath.Glob("testdata/comprehensive_*.txt")
	if err != nil {
		t.Fatalf("Failed to find test files: %v", err)
	}

	if len(testFiles) == 0 {
		t.Skip("No comprehensive test files found")
	}

	// Create script engine
	engine := script.NewEngine()
	engine.Cmds["nlm_test"] = script.Program("./nlm_test", func(cmd *exec.Cmd) error {
		cmd.Env = []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + os.Getenv("HOME"),
			"TERM=" + os.Getenv("TERM"),
		}
		return nil
	}, time.Second*30)

	// Run each comprehensive test file
	for _, testFile := range testFiles {
		testFile := testFile // capture loop variable
		t.Run(filepath.Base(testFile), func(t *testing.T) {
			t.Parallel() // Run tests in parallel for speed

			// Create script state
			state, err := script.NewState(context.Background(), ".", []string{
				"PATH=" + os.Getenv("PATH"),
				"HOME=" + os.Getenv("HOME"),
				"TERM=" + os.Getenv("TERM"),
			})
			if err != nil {
				t.Fatalf("failed to create script state: %v", err)
			}
			defer state.CloseAndWait(os.Stderr)

			// Read test file
			content, err := os.ReadFile(testFile)
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			// Run the script test
			reader := bufio.NewReader(strings.NewReader(string(content)))
			scripttest.Run(t, engine, state, filepath.Base(testFile), reader)
		})
	}
}
