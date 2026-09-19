package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tmc/nlm/internal/richrender"
	"github.com/tmc/nlm/notebooklm"
)

// chatPartial serializes renderer events and heartbeats to one append-only file.
// The final event is written only after the durable session has been replaced.
type chatPartial struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	err      error
	last     time.Time
	stop     chan struct{}
	stopped  chan struct{}
	renderer *richrender.StreamRenderer
}

func newChatPartial(notebookID, conversationID string) (*chatPartial, error) {
	path := chatPartialPath(getChatSessionPathForConv(notebookID, conversationID))
	f, err := os.CreateTemp(filepath.Dir(path), ".partial-*.tmp")
	if err != nil {
		return nil, err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	p := &chatPartial{path: path, file: f, stop: make(chan struct{}), stopped: make(chan struct{})}
	p.renderer = newChatStreamRenderer(p, io.Discard, chatStreamOptions{JSONL: true, JSONLIncludeThinking: true})
	p.event(map[string]any{"phase": "alive", "t": time.Now()})
	go func() {
		defer close(p.stopped)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-p.stop:
				return
			case now := <-ticker.C:
				p.mu.Lock()
				idle := now.Sub(p.last) >= 15*time.Second
				p.mu.Unlock()
				if idle {
					p.event(map[string]any{"phase": "alive", "t": now})
				}
			}
		}
	}()
	return p, nil
}

func (p *chatPartial) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return 0, p.err
	}
	n, err := p.file.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	p.err = err
	p.last = time.Now()
	return n, err
}

func (p *chatPartial) event(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	p.Write(append(b, '\n'))
}

func (p *chatPartial) chunk(chunk notebooklm.ChatChunk) {
	if p != nil {
		p.renderer.WriteChunk(chunk)
	}
}

func (p *chatPartial) close() error {
	close(p.stop)
	<-p.stopped
	p.mu.Lock()
	defer p.mu.Unlock()
	err := p.file.Close()
	if p.err != nil {
		return p.err
	}
	return err
}

func (p *chatPartial) finish(answer string, keep bool) error {
	p.event(map[string]any{"phase": "done", "answer": strings.TrimSpace(answer)})
	if err := p.close(); err != nil {
		return err
	}
	path := p.path
	if keep {
		return os.Rename(path, strings.TrimSuffix(path, ".partial.jsonl")+fmt.Sprintf(".%d.jsonl", time.Now().UnixNano()))
	}
	return os.Remove(path)
}

// appendChatPartial adds a render-only, explicitly labeled answer. It never
// modifies the durable session or promotes an abandoned answer into history.
func appendChatPartial(doc *chatDocument, session *chatSession) error {
	path := getChatSessionPathForConv(session.NotebookID, session.ConversationID)
	status := resolveChatStatus(session.Status, session.StatusAt, session.WriterPID, path, time.Now())
	if status == "" {
		return nil
	}
	var answer string
	_, err := readChatPartial(chatPartialPath(path), 0, func(line []byte, _ int64) error {
		var event struct{ Phase, Text, Full, Answer string }
		if err := json.Unmarshal(line, &event); err != nil {
			return err
		}
		switch event.Phase {
		case "answer":
			answer += event.Text
		case "revised":
			answer = event.Full
		case "done":
			answer = event.Answer
		}
		return nil
	})
	if err != nil {
		return err
	}
	if answer != "" {
		doc.Messages = append(doc.Messages, chatDocMessage{Role: "assistant", Content: "Partial answer (" + status + ")\n\n" + answer})
	}
	return nil
}
