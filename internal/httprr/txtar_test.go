package httprr

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
)

func TestExportToTxtar(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	httprFile := filepath.Join(tmpDir, "test.httprr")
	txtarFile := filepath.Join(tmpDir, "test.txtar")

	// Create a simple httprr file for testing
	requestPart := "POST /test HTTP/1.1\r\nHost: example.com\r\nContent-Length: 9\r\nCookie: secret=token\r\n\r\ntest body"
	responsePart := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 7\r\n\r\nsuccess"

	httprContent := "httprr trace v1\n" +
		fmt.Sprintf("%d %d\n", len(requestPart), len(responsePart)) +
		requestPart + responsePart

	if err := os.WriteFile(httprFile, []byte(httprContent), 0644); err != nil {
		t.Fatalf("failed to write test httprr file: %v", err)
	}

	// Create RecordReplay instance
	rr := &RecordReplay{
		file: httprFile,
	}

	// Test export without secrets
	t.Run("export_without_secrets", func(t *testing.T) {
		if err := rr.ExportToTxtar(txtarFile, false); err != nil {
			t.Fatalf("ExportToTxtar failed: %v", err)
		}

		// Read and parse the txtar file
		archive, err := txtar.ParseFile(txtarFile)
		if err != nil {
			t.Fatalf("failed to parse txtar file: %v", err)
		}

		// Verify structure
		if len(archive.Files) != 2 {
			t.Errorf("expected 2 files in archive, got %d", len(archive.Files))
		}

		// Verify secrets are redacted
		requestData := string(archive.Files[0].Data)
		if strings.Contains(requestData, "secret=token") {
			t.Error("secrets should be redacted but found in request")
		}
		if !strings.Contains(requestData, "[REDACTED]") {
			t.Error("expected [REDACTED] placeholder for secrets")
		}
	})

	// Test export with secrets
	t.Run("export_with_secrets", func(t *testing.T) {
		txtarFileWithSecrets := filepath.Join(tmpDir, "test_with_secrets.txtar")
		if err := rr.ExportToTxtar(txtarFileWithSecrets, true); err != nil {
			t.Fatalf("ExportToTxtar with secrets failed: %v", err)
		}

		// Read and parse the txtar file
		archive, err := txtar.ParseFile(txtarFileWithSecrets)
		if err != nil {
			t.Fatalf("failed to parse txtar file: %v", err)
		}

		// Verify secrets are preserved
		requestData := string(archive.Files[0].Data)
		if !strings.Contains(requestData, "secret=token") {
			t.Error("secrets should be preserved but not found in request")
		}
	})
}

func TestExtractRPCID(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "notebooklm_rpc_format",
			body:     `f.req=[["AHyHrd",["arg1","arg2"]]]`,
			expected: "AHyHrd",
		},
		{
			name:     "empty_body",
			body:     "",
			expected: "",
		},
		{
			name:     "no_rpc_id",
			body:     "some other content",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := newTestRequest(tt.body)
			if err != nil {
				t.Fatalf("failed to create test request: %v", err)
			}

			rpcID := extractRPCID(req)
			if rpcID != tt.expected {
				t.Errorf("extractRPCID() = %q, want %q", rpcID, tt.expected)
			}
		})
	}
}

// Helper to create a test HTTP request with a body
func newTestRequest(body string) (*http.Request, error) {
	req, err := http.NewRequest("POST", "https://example.com/test", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}
