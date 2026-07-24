package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"unicode/utf16"
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

func TestDecodeWrbFRStreamEmbeddedNewline(t *testing.T) {
	row, err := json.Marshal([]any{"wrb.fr", nil, `[["draft"]]`})
	if err != nil {
		t.Fatal(err)
	}
	frame := "[" + string(row) + ",\n[\"noop\",null,\"ignored\"]]"
	final := streamTestFrame(t, `[["final"]]`, utf8.RuneCountInString)
	body := ")]}'\n\n" + streamTestFrame(t, frame, utf8.RuneCountInString) + final

	resp, frames, err := decodeWrbFRStream([]byte(body), "laWbsf")
	if err != nil {
		t.Fatal(err)
	}
	if frames != 2 || string(resp.Responses[0].Data) != `[["final"]]` {
		t.Fatalf("frames, response = %d, %+v", frames, resp)
	}
}

func TestDecodeWrbFRStreamRejectsBadLength(t *testing.T) {
	row, err := json.Marshal([]any{"wrb.fr", nil, `[["draft"]]`})
	if err != nil {
		t.Fatal(err)
	}
	frame := "[" + string(row) + ",\n[\"noop\",null,\"ignored\"]]"
	body := ")]}'\n\n" + fmt.Sprintf("%d\n%s\n", utf8.RuneCountInString(frame)+3, frame)
	if _, _, err := decodeWrbFRStream([]byte(body), "laWbsf"); err == nil {
		t.Fatal("decode succeeded with a malformed frame length")
	}
}

func TestDecodeWrbFRStreamUTF16Length(t *testing.T) {
	body := ")]}'\n\n" + streamTestFrame(t, `[["final 🧪"]]`, func(s string) int {
		return len(utf16.Encode([]rune(s)))
	})
	resp, frames, err := decodeWrbFRStream([]byte(body), "laWbsf")
	if err != nil {
		t.Fatal(err)
	}
	if frames != 1 || string(resp.Responses[0].Data) != `[["final 🧪"]]` {
		t.Fatalf("frames, response = %d, %+v", frames, resp)
	}
}

func streamTestFrame(t *testing.T, payload string, count func(string) int) string {
	t.Helper()
	row, err := json.Marshal([]any{[]any{"wrb.fr", nil, payload}})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d\n%s\n", count(string(row))+2, row)
}
