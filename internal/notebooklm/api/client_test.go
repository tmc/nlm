package api

import (
	"strings"
	"testing"
)

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
