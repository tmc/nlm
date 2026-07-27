package api

import (
	"encoding/json"
	"testing"
)

// canonicalJSON normalizes fixture whitespace for exact wire-shape tests.
func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("canonicalize fixture: %v", err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("canonicalize fixture marshal: %v", err)
	}
	return string(b)
}
