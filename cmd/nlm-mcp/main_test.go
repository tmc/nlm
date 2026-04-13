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

func TestInitializeAndListTools(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "nlm-mcp")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build nlm-mcp: %v\n%s", err, out)
	}

	serverCmd := exec.Command(binaryPath)
	serverCmd.Env = append(os.Environ(),
		"NLM_AUTH_TOKEN=test-token",
		"NLM_COOKIES=test-cookies",
	)

	stdin, err := serverCmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := serverCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	serverCmd.Stderr = os.Stderr

	if err := serverCmd.Start(); err != nil {
		t.Fatalf("start nlm-mcp: %v", err)
	}
	defer func() {
		_ = serverCmd.Process.Kill()
	}()

	go func() {
		defer stdin.Close()

		writeJSONLine(t, stdin, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"roots": map[string]bool{
						"listChanged": true,
					},
				},
				"clientInfo": map[string]string{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
		})
		writeJSONLine(t, stdin, map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
			"params":  map[string]any{},
		})
		writeJSONLine(t, stdin, map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      "list_notebooks",
				"arguments": map[string]any{},
			},
		})
	}()

	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		seenInit := false
		seenList := false
		seenCall := false

		for scanner.Scan() {
			var response map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
				continue
			}
			id, _ := response["id"].(float64)

			switch id {
			case 1:
				seenInit = response["result"] != nil
			case 2:
				seenList = checkExpectedTools(t, response)
			case 3:
				seenCall = response["result"] != nil || response["error"] != nil
			}

			if seenInit && seenList && seenCall {
				done <- nil
				return
			}
		}
		done <- scanner.Err()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("scan responses: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for MCP responses")
	}
}

func writeJSONLine(t *testing.T, file interface{ Write([]byte) (int, error) }, msg map[string]any) {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		t.Fatalf("write message: %v", err)
	}
}

func checkExpectedTools(t *testing.T, response map[string]any) bool {
	t.Helper()

	result, ok := response["result"].(map[string]any)
	if !ok {
		return false
	}
	rawTools, ok := result["tools"].([]any)
	if !ok {
		return false
	}

	found := make(map[string]bool, len(rawTools))
	for _, rawTool := range rawTools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tool["name"].(string)
		found[name] = true
	}

	expected := []string{
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

	for _, name := range expected {
		if !found[name] {
			t.Errorf("tool %q not found in tools/list response", name)
			return false
		}
	}
	return true
}
