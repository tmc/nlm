package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPInitialize(t *testing.T) {
	// 1. Build the binary
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "nlm")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build nlm: %v\n%s", err, out)
	}

	// 2. Prepare MCP Initialize Request
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"roots": map[string]bool{
					"listChanged": true,
				},
				"sampling": map[string]interface{}{},
			},
			"clientInfo": map[string]string{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	reqBytes, err := json.Marshal(initReq)
	if err != nil {
		t.Fatalf("Failed to marshal init request: %v", err)
	}

	// 3. Run nlm mcp
	mcpCmd := exec.Command(binaryPath, "mcp")
	mcpCmd.Env = os.Environ() // Pass environment (HOME, etc)

	stdin, err := mcpCmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to get stdin pipe: %v", err)
	}
	stdout, err := mcpCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to get stdout pipe: %v", err)
	}
	// stderr to testing log
	mcpCmd.Stderr = os.Stderr

	if err := mcpCmd.Start(); err != nil {
		t.Fatalf("Failed to start nlm mcp: %v", err)
	}

	// 4. Send Requests (Initialize -> ListTools -> CallTool)
	go func() {
		defer stdin.Close()

		// Initialize
		stdin.Write(reqBytes)
		stdin.Write([]byte("\n"))

		// List Tools
		listToolsReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
			"params":  map[string]interface{}{},
		}
		listBytes, _ := json.Marshal(listToolsReq)
		stdin.Write(listBytes)
		stdin.Write([]byte("\n"))

		// Call Tool (list_notebooks)
		callToolReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      "list_notebooks",
				"arguments": map[string]interface{}{},
			},
		}
		callBytes, _ := json.Marshal(callToolReq)
		stdin.Write(callBytes)
		stdin.Write([]byte("\n"))
	}()

	// 5. Read Responses
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		seenInit := false
		seenList := false
		seenCall := false

		for scanner.Scan() {
			line := scanner.Text()
			t.Logf("MCP Output: %s", line)

			var resp map[string]interface{}
			if err := json.Unmarshal([]byte(line), &resp); err == nil {
				id, _ := resp["id"].(float64)

				if id == 1 && resp["result"] != nil {
					seenInit = true
				}
				if id == 2 && resp["result"] != nil {
					// Verify tools list
					result := resp["result"].(map[string]interface{})
					tools := result["tools"].([]interface{})
					foundTools := make(map[string]bool)
					for _, tool := range tools {
						t := tool.(map[string]interface{})
						name := t["name"].(string)
						foundTools[name] = true
					}

					expectedTools := []string{
						"list_notebooks",
						"list_sources",
						"list_notes",
						"create_note",
						"create_notebook",
						"delete_notebook",
						"delete_source",
						"add_source_url",
						"generate_chat",
						"generate_summarize",
						"generate_briefing_doc",
						"generate_faq",
						"generate_study_guide",
						"generate_rephrase",
						"generate_expand",
						"generate_critique",
						"generate_brainstorm",
						"generate_verify",
						"generate_explain",
						"generate_outline",
						"generate_mindmap",
						"generate_timeline",
						"generate_toc",
						"list_artifacts",
						"create_audio_overview",
						"get_audio_overview",
						"add_source_text",
						"delete_note",
						"rename_artifact",
						"share_audio",
					}

					for _, name := range expectedTools {
						if !foundTools[name] {
							t.Errorf("tool %q not found in list", name)
						}
					}
					seenList = true
				}
				if id == 3 {
					// It might be an error (auth required) or result, both confirm execution flow
					if resp["error"] != nil || resp["result"] != nil {
						seenCall = true
					}
				}
			}

			if seenInit && seenList && seenCall {
				break
			}
		}
		if !seenInit || !seenList || !seenCall {
			done <- scanner.Err()
		} else {
			done <- nil
		}
	}()

	// 6. Wait with timeout
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Scan failed or incomplete: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for MCP responses")
	}

	// Cleanup
	if err := mcpCmd.Process.Kill(); err != nil {
		t.Logf("Failed to kill process: %v", err)
	}
}
