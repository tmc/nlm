package main

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestChatPartialRevision(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := beginGeneratedChatTurn("nb", "conv", nil, "question"); err != nil {
		t.Fatal(err)
	}
	p, err := newChatPartial("nb", "conv")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"hello world", "hello friend", "hi", "hi again"} {
		p.chunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Full: text, Text: text})
	}
	if err := p.close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p.path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	answer, revisions := "", 0
	for scan.Scan() {
		var e struct{ Phase, Text, Full string }
		if err := json.Unmarshal(scan.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		switch e.Phase {
		case "answer":
			answer += e.Text
		case "revised":
			answer = e.Full
			revisions++
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if answer != "hi again" || revisions != 2 {
		t.Fatalf("answer=%q revisions=%d", answer, revisions)
	}
}
