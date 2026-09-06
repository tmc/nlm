package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tmc/nlm/notebooklm"
)

type chatRejectTransport struct{ calls atomic.Int32 }

func (tr *chatRejectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusUnauthorized,
		Status:     "401 Unauthorized",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("Unauthorized")),
		Request:    req,
	}, nil
}

func TestChatPromptFileDoesNotReadStdin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(path, []byte("Summarize the sources."), 0600); err != nil {
		t.Fatal(err)
	}
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	old := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = old }()
	parsed := parseChatCommandForTest(t, "chat", []string{"--citations", "json", "--prompt-file", path, "nb"}, globalOptions{})
	call, err := decodeChat(parsed)
	if err != nil {
		t.Fatal(err)
	}
	transport := &chatRejectTransport{}
	client := notebooklm.New(notebooklm.Credentials{}, notebooklm.WithHTTPClient(&http.Client{Transport: transport}))
	done := make(chan error, 1)
	go func() { done <- call(context.Background(), client) }()
	select {
	case err := <-done:
		if err == nil || transport.calls.Load() == 0 {
			t.Fatalf("error=%v requests=%d, want request failure", err, transport.calls.Load())
		}
	case <-time.After(5 * time.Second):
		writer.Close()
		<-done
		t.Fatal("prompt-file command waited for stdin")
	}
}

func TestInteractiveChatEndsOnEOF(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	writer.Close()
	old := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = old }()
	if err := runInteractiveChat(nil, &chatSession{NotebookID: "nb", ConversationID: "conv"}, nil, chatOptions{}); err != nil {
		t.Fatal(err)
	}
}
