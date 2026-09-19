package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tmc/nlm/notebooklm"
)

func TestReadChatPartialBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.jsonl")
	first := `{"phase":"answer","text":"one"}` + "\n"
	if err := os.WriteFile(path, []byte(first+`{"phase":"answer"`), 0600); err != nil {
		t.Fatal(err)
	}
	count := 0
	offset, err := readChatPartial(path, 0, func(b []byte, n int64) error { count++; return nil })
	if err != nil || offset != int64(len(first)) || count != 1 {
		t.Fatalf("offset=%d count=%d err=%v", offset, count, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`,"text":"two"}` + "\n")
	f.Close()
	_, err = readChatPartial(path, offset, func(b []byte, n int64) error { count++; return nil })
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestChatLiveAccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	writeTestSession(t, testSession("nb", "conv", 2))
	v := &chatLiveView{notebook: "nb", key: "secret"}
	for _, tt := range []struct {
		path string
		want int
	}{
		{"/", 403}, {"/events", 403}, {"/conversation?k=wrong&c=conv", 403},
		{"/?k=secret", 200}, {"/conversation?k=secret&c=conv", 200},
		{"/conversation?k=secret&c=../../other", 404}, {"/unknown?k=secret", 404},
	} {
		w := httptest.NewRecorder()
		v.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
		if w.Code != tt.want {
			t.Errorf("%s: got %d want %d", tt.path, w.Code, tt.want)
		}
	}
}

func TestChatLiveStream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := beginGeneratedChatTurn("nb", "conv", nil, "question"); err != nil {
		t.Fatal(err)
	}
	partial, err := newChatPartial("nb", "conv")
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			partial.close()
		}
	}()
	v := &chatLiveView{notebook: "nb", key: "secret"}
	server := httptest.NewServer(v)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+v.link("/events", "conv"), nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	next := func(phase string) string {
		t.Helper()
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				var event struct{ Phase, Text, Full string }
				json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event)
				if event.Phase == phase {
					if phase == "revised" {
						return event.Full
					}
					return event.Text
				}
			}
		}
		t.Fatalf("no %s event: %v", phase, scanner.Err())
		return ""
	}
	partial.chunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Full: "old answer", Text: "old answer"})
	if got := next("answer"); got != "old answer" {
		t.Fatal(got)
	}
	partial.chunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Full: "new answer", Text: "new answer"})
	if got := next("revised"); got != "new answer" {
		t.Fatal(got)
	}
	// Keep the file open long enough to observe done; production readers also
	// detect completion through the session when removal beats their next poll.
	if err := saveGeneratedChatTurn("nb", "conv", nil, "question", chatResult{Answer: "new answer"}); err != nil {
		t.Fatal(err)
	}
	partial.event(map[string]any{"phase": "done", "answer": "new answer"})
	next("done")
	if err := partial.close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	body, err := http.Get(server.URL + v.link("/conversation", "conv"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body.Body)
	body.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := renderChatViewBody(&want, getChatSessionPathForConv("nb", "conv"), chatRenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatal("live final body differs from static renderer")
	}
}

func TestChatLiveFlags(t *testing.T) {
	for _, tt := range []struct {
		args    []string
		wantErr bool
	}{
		{[]string{"--live", "nb"}, false},
		{[]string{"--live=1s", "nb", "conv"}, false},
		{[]string{"--live=0s", "nb"}, true},
		{[]string{"--live", "--out=x", "nb"}, true},
		{[]string{"--live", "--inline", "nb"}, true},
		{[]string{"--live", "--format=text", "nb"}, true},
	} {
		parsed, err := tryParseChatCommandForTest(t, "chat show", tt.args, globalOptions{})
		var args chatShowArgs
		if err == nil {
			args, err = decodeChatShowArgs(parsed)
		}
		if (err != nil) != tt.wantErr {
			t.Errorf("%v: %v", tt.args, err)
		}
		if err == nil && !args.Options.Live {
			t.Errorf("%v: live=false", tt.args)
		}
	}
}

func TestChatIndexEscapesAndCaches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	session := testSession("nb", "conv", 2)
	session.Messages[0].Content = "<script>alert(1)</script>"
	session.Messages[1] = storedMessage{Role: "assistant", Content: "saved answer", Thinking: "private reasoning"}
	writeTestSession(t, session)
	out := filepath.Join(t.TempDir(), "index.html")
	opts := chatRenderOptions{Format: "html", OutFile: out}
	if err := chatShowIndex("nb", opts); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(index, []byte("<script>alert(1)</script>")) {
		t.Fatal("unescaped title")
	}
	bodyPath := filepath.Join(filepath.Dir(out), "conv.html")
	first, err := os.Stat(bodyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := chatShowIndex("nb", opts); err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(bodyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !first.ModTime().Equal(second.ModTime()) {
		t.Fatal("unchanged body was rewritten")
	}
	opts.ShowThinking = true
	if err := chatShowIndex("nb", opts); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("private reasoning")) {
		t.Fatal("changed render option did not invalidate cached body")
	}
}

func TestChatLiveReconnectOffset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := beginGeneratedChatTurn("nb", "conv", nil, "question"); err != nil {
		t.Fatal(err)
	}
	p, err := newChatPartial("nb", "conv")
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	p.chunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Text: "first"})
	info, err := os.Stat(p.path)
	if err != nil {
		t.Fatal(err)
	}
	session, err := loadChatSessionForConv("nb", "conv")
	if err != nil {
		t.Fatal(err)
	}
	p.chunk(notebooklm.ChatChunk{Phase: notebooklm.ChatChunkAnswer, Text: " second"})
	v := &chatLiveView{notebook: "nb", key: "secret"}
	server := httptest.NewServer(v)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+v.link("/events", "conv"), nil)
	req.Header.Set("Last-Event-ID", fmt.Sprintf("%d:%d", session.StatusAt.UnixNano(), info.Size()))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scan := bufio.NewScanner(response.Body)
	for scan.Scan() {
		line := scan.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct{ Phase, Text string }
		json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event)
		if event.Phase == "answer" {
			if event.Text != " second" {
				t.Fatalf("replayed old bytes: %q", event.Text)
			}
			return
		}
		if event.Phase == "reset" {
			t.Fatal("same-turn reconnect unexpectedly reset")
		}
	}
	t.Fatalf("no resumed answer: %v", scan.Err())
}

func TestResolveChatViewConversation(t *testing.T) {
	rows := []chatViewRow{{ID: "conv-one"}, {ID: "conv-two"}}
	for _, tt := range []struct {
		id, want string
		wantErr  bool
	}{
		{"conv-o", "conv-one", false}, {"conv-two", "conv-two", false},
		{"conv", "", true}, {"missing", "", true},
	} {
		got, err := resolveChatViewConversation(rows, tt.id)
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("%q: %q, %v", tt.id, got, err)
		}
	}
}
