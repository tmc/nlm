package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestExtractTextFromRawGRPCBytes is a regression test for the --format plain raw JSON bug.
// When DecodeBodyData fails to parse the chunked gRPC response (e.g. due to a streaming
// response with mismatched chunk sizes), GenerateFreeFormStreamed used to fall back to
// string(respBytes) which printed the raw HTTP body including )]}\' and wrb.fr JSON.
// The fix must extract readable text from raw bytes without returning raw framing.
func TestExtractTextFromRawGRPCBytes(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		wantContain string
		wantNot     []string
	}{
		{
			name: "single chunk standard format",
			// )]}\' prefix, chunk-size line, wrb.fr JSON with data string at position [2].
			// Data string is a JSON-encoded array: ["text", false] — field 1 = chunk text.
			body: []byte(")]}'\n\n145\n[[\"wrb.fr\",\"GenerateFreeFormStreamed\",\"[\\\"Dopamine is a neurotransmitter.\\\",false]\",null,null,null,\"generic\"]]\n25\n[[\"e\",4,null,null,237]]\n"),
			wantContain: "Dopamine",
			wantNot:     []string{"wrb.fr", ")]}"},
		},
		{
			name: "multiple streaming chunks concatenated",
			// Streaming endpoint sends multiple wrb.fr frames; old code returned all raw.
			body: []byte(")]}'\n\n80\n[[\"wrb.fr\",\"rpc\",\"[\\\"Hello \\\",false]\",null,null,null,\"generic\"]]\n78\n[[\"wrb.fr\",\"rpc\",\"[\\\"world.\\\",true]\",null,null,null,\"generic\"]]\n"),
			wantContain: "Hello",
			wantNot:     []string{"wrb.fr", ")]}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextFromRawGRPCBytes(tt.body)
			for _, bad := range tt.wantNot {
				if strings.Contains(got, bad) {
					t.Errorf("extractTextFromRawGRPCBytes contains unwanted %q in output: %q", bad, got[:minInt(len(got), 300)])
				}
			}
			if got == "" {
				t.Errorf("extractTextFromRawGRPCBytes returned empty string, want text content")
			}
			if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
				t.Errorf("extractTextFromRawGRPCBytes = %q, want string containing %q", got, tt.wantContain)
			}
		})
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestExtractTextFromJSON is a regression test for the generate-chat 400 fix.
// When GenerateFreeFormStreamed falls back to extracting text from a raw gRPC
// response payload, extractTextFromJSON must return the longest string found
// in the nested JSON structure (i.e., the actual chat text).
func TestExtractTextFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin int    // minimum expected length of extracted text
		wantStr string // if non-empty, exact expected string
	}{
		{
			name:    "simple string array matches beprotojson Chunk position",
			input:   `["Dopamine is a neurotransmitter.", true]`,
			wantStr: "Dopamine is a neurotransmitter.",
		},
		{
			name:    "nested array extracts longest string",
			input:   `[[["Dopamine regulates reward and motivation.",null],null],null,null]`,
			wantStr: "Dopamine regulates reward and motivation.",
		},
		{
			name:    "plain string",
			input:   `"Hello world"`,
			wantStr: "Hello world",
		},
		{
			name:    "wrb.fr style response data",
			input:   `[["This is the answer to your question about dopamine and reward pathways.",null,null],null]`,
			wantMin: 50,
		},
		{
			name:    "empty array returns empty string",
			input:   `[]`,
			wantStr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextFromJSON(json.RawMessage(tt.input))
			if tt.wantStr != "" && got != tt.wantStr {
				t.Errorf("extractTextFromJSON(%q) = %q, want %q", tt.input, got, tt.wantStr)
			}
			if tt.wantMin > 0 && len(got) < tt.wantMin {
				t.Errorf("extractTextFromJSON(%q) = %q (len %d), want at least %d chars", tt.input, got, len(got), tt.wantMin)
			}
		})
	}
}

func TestDetectMIMEType(t *testing.T) {
	tests := []struct {
		name         string
		content      []byte
		filename     string
		providedType string
		want         string
	}{
		{
			name:     "XML file with .xml extension",
			content:  []byte(`<?xml version="1.0"?><root><item>test</item></root>`),
			filename: "test.xml",
			want:     "text/xml",
		},
		{
			name:     "XML file without extension",
			content:  []byte(`<?xml version="1.0"?><root><item>test</item></root>`),
			filename: "test",
			want:     "text/xml",
		},
		{
			name:         "XML file with provided type",
			content:      []byte(`<?xml version="1.0"?><root><item>test</item></root>`),
			filename:     "test.xml",
			providedType: "application/xml",
			want:         "application/xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMIMEType(tt.content, tt.filename, tt.providedType)
			// Strip charset for comparison
			gotType := strings.Split(got, ";")[0]
			if gotType != tt.want {
				t.Errorf("detectMIMEType() = %v, want %v", gotType, tt.want)
			}
		})
	}
}
