package api

import (
	"encoding/json"
	"testing"
)

func TestLegacyActOnSourcesArgs(t *testing.T) {
	got, err := json.Marshal(legacyActOnSourcesArgs("notebook-id", "summarize", []string{"source-a", "source-b"}))
	if err != nil {
		t.Fatal(err)
	}
	const want = `["notebook-id","summarize",["source-a","source-b"]]`
	if string(got) != want {
		t.Fatalf("legacy ActOnSources bytes = %s, want %s", got, want)
	}
}
