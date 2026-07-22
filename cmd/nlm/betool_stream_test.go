package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"unicode/utf8"
)

func TestDecodeWrbFRRequest(t *testing.T) {
	wire := `[[["source-id"]],"prompt"]`
	envelope, err := json.Marshal([]any{nil, wire, nil})
	if err != nil {
		t.Fatal(err)
	}
	body := "f.req=" + url.QueryEscape(string(envelope)) + "&at=TOKEN"
	got, err := decodeWrbFRRequest([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != wire {
		t.Fatalf("request = %s, want %s", got, wire)
	}
}

func TestDecodeWrbFRStream(t *testing.T) {
	frame := func(payload string) string {
		row, err := json.Marshal([]any{[]any{"wrb.fr", nil, payload}})
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%d\n%s\n", utf8.RuneCount(row)+2, row)
	}
	body := ")]}'\n\n" + frame(`[["draft 🧪"]]`) + frame(`[["final 🧪"]]`)
	resp, frames, err := decodeWrbFRStream([]byte(body), "laWbsf")
	if err != nil {
		t.Fatal(err)
	}
	if frames != 2 {
		t.Fatalf("frames = %d, want 2", frames)
	}
	if len(resp.Responses) != 1 || resp.Responses[0].ID != "laWbsf" || string(resp.Responses[0].Data) != `[["final 🧪"]]` {
		t.Fatalf("response = %+v", resp)
	}
}
