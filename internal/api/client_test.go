package api

import (
	"encoding/json"
	"strings"
	"testing"
)

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
