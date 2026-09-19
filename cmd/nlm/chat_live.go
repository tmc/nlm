package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

type chatLiveView struct {
	notebook string
	key      string
	opts     chatRenderOptions
}

func serveChatLive(ctx context.Context, notebookID, conversationID string, opts chatRenderOptions) error {
	if conversationID != "" {
		rows, err := chatViewRows(notebookID)
		if err != nil {
			return err
		}
		conversationID, err = resolveChatViewConversation(rows, conversationID)
		if err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return err
	}
	view := &chatLiveView{notebook: notebookID, key: hex.EncodeToString(secret[:]), opts: opts}
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()
	if opts.LiveDuration > 0 {
		var timeout context.CancelFunc
		ctx, timeout = context.WithTimeout(ctx, opts.LiveDuration)
		defer timeout()
	}
	server := &http.Server{Handler: view, ReadHeaderTimeout: 5 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			server.Close()
		case <-done:
		}
	}()
	defer close(done)
	address := "http://" + listener.Addr().String() + view.link("/", conversationID)
	fmt.Fprintf(os.Stderr, "nlm: serving %s (ctrl-c to stop)\n", address)
	if opts.Open {
		if err := openInBrowser(address); err != nil {
			return err
		}
	}
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (v *chatLiveView) link(path, conversation string) string {
	q := url.Values{"k": {v.key}}
	if conversation != "" {
		q.Set("c", conversation)
	}
	return path + "?" + q.Encode()
}

func (v *chatLiveView) rows() ([]chatViewRow, error) {
	rows, err := chatViewRows(v.notebook)
	for i := range rows {
		rows[i].URL = v.link("/", rows[i].ID)
	}
	return rows, err
}

func (v *chatLiveView) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("k")), []byte(v.key)) != 1 || v.key == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/conversation" && r.URL.Path != "/events" {
		http.NotFound(w, r)
		return
	}
	rows, err := v.rows()
	if err != nil {
		http.Error(w, "cannot read conversations", 500)
		return
	}
	id := r.URL.Query().Get("c")
	var selected *chatViewRow
	for i := range rows {
		if rows[i].ID == id {
			selected = &rows[i]
			break
		}
	}
	if id != "" && selected == nil {
		http.NotFound(w, r)
		return
	}
	switch r.URL.Path {
	case "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		chatViewTemplate.Execute(w, chatViewPage{Notebook: v.notebook, Rows: rows, Live: true, Conversation: id, BodyURL: v.link("/conversation", id), EventsURL: v.link("/events", id)})
	case "/conversation":
		if selected == nil {
			http.NotFound(w, r)
			return
		}
		var body bytes.Buffer
		if err := renderChatViewBody(&body, selected.Path, v.opts); err != nil {
			http.Error(w, "cannot render conversation", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(body.Bytes())
	case "/events":
		v.events(w, r, id)
	}
}

// readChatPartial returns only complete lines. The offset always points to the
// first unread byte; a writer's unfinished line is retried on the next poll.
func readChatPartial(path string, offset int64, emit func([]byte, int64) error) (int64, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return offset, nil
	}
	if err != nil {
		return offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return offset, nil
		}
		if err != nil {
			return offset, err
		}
		next := offset + int64(len(line))
		if !json.Valid(line) {
			return offset, fmt.Errorf("invalid partial event at byte %d", offset)
		}
		if err := emit(line, next); err != nil {
			return offset, err
		}
		offset = next
	}
}

func (v *chatLiveView) events(w http.ResponseWriter, r *http.Request, selected string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(event, id string, data []byte) error {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if id != "" {
			if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, strings.TrimSpace(string(data)))
		flusher.Flush()
		return err
	}
	var offset int64
	var partialInfo os.FileInfo
	generation := ""
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		g, o, ok := strings.Cut(raw, ":")
		if ok {
			if n, err := strconv.ParseInt(o, 10, 64); err == nil && n >= 0 {
				generation, offset = g, n
			}
		}
	}
	lastRows := ""
	lastChange := time.Now()
	interval := 250 * time.Millisecond
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		rows, err := v.rows()
		if err != nil {
			return
		}
		b, err := json.Marshal(rows)
		if err != nil {
			return
		}
		if string(b) != lastRows {
			if send("rows", "", b) != nil {
				return
			}
			lastRows = string(b)
			lastChange = time.Now()
		}
		for _, row := range rows {
			if row.ID != selected {
				continue
			}
			// StatusAt changes at both turn boundaries. A new running turn resets
			// replay, including reconnects whose saved offset belongs to an old turn.
			g := strconv.FormatInt(row.StatusAt.UnixNano(), 10)
			if g != generation {
				generation, offset = g, 0
				partialInfo = nil
				if send("partial", g+":0", []byte(`{"phase":"reset"}`)) != nil {
					return
				}
			}
			path := chatPartialPath(getChatSessionPathForConv(v.notebook, row.ID))
			if info, err := os.Stat(path); err == nil {
				if info.Size() < offset || partialInfo != nil && !os.SameFile(partialInfo, info) {
					offset = 0
					if send("partial", g+":0", []byte(`{"phase":"reset"}`)) != nil {
						return
					}
				}
				partialInfo = info
			}
			next, err := readChatPartial(path, offset, func(line []byte, end int64) error { return send("partial", g+":"+strconv.FormatInt(end, 10), line) })
			if err != nil {
				return
			}
			if next != offset {
				lastChange = time.Now()
				offset = next
			}
		}
		interval = 250 * time.Millisecond
		if time.Since(lastChange) > 5*time.Minute {
			interval = 2 * time.Second
		}
		timer.Reset(interval)
	}
}

func resolveChatViewConversation(rows []chatViewRow, id string) (string, error) {
	for _, row := range rows {
		if row.ID == id {
			return id, nil
		}
	}
	match := ""
	for _, row := range rows {
		if strings.HasPrefix(row.ID, id) {
			if match != "" {
				return "", fmt.Errorf("ambiguous conversation prefix %q", id)
			}
			match = row.ID
		}
	}
	if match == "" {
		return "", fmt.Errorf("no local conversation %q", id)
	}
	return match, nil
}
