package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/tmc/nlm/internal/batchexecute"
)

// decodeWrbFRRequest unwraps the form-encoded request used by Google's
// streaming endpoint. Its f.req value is [null,"<positional-json>",...].
func decodeWrbFRRequest(body []byte) (json.RawMessage, error) {
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse stream request form: %w", err)
	}
	value := form.Get("f.req")
	if value == "" {
		return nil, fmt.Errorf("stream request has no f.req")
	}
	var envelope []json.RawMessage
	if err := json.Unmarshal([]byte(value), &envelope); err != nil {
		return nil, fmt.Errorf("parse stream request envelope: %w", err)
	}
	if len(envelope) < 2 {
		return nil, fmt.Errorf("stream request envelope has %d fields, want at least 2", len(envelope))
	}
	var payload string
	if err := json.Unmarshal(envelope[1], &payload); err != nil {
		return nil, fmt.Errorf("parse stream request payload: %w", err)
	}
	if !json.Valid([]byte(payload)) {
		return nil, fmt.Errorf("stream request payload is not valid JSON")
	}
	return json.RawMessage(payload), nil
}

// decodeWrbFRStream decodes Google's length-prefixed streaming response
// envelope. The wrb.fr payloads are cumulative snapshots, so the returned
// response contains only the final snapshot.
func decodeWrbFRStream(body []byte, rpcID string) (*batchexecute.WireResponse, int, error) {
	body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte(")]}'")))

	var final json.RawMessage
	chunks := 0
	frames := 0
	for offset := 0; offset < len(body); {
		for offset < len(body) && (body[offset] == '\n' || body[offset] == '\r') {
			offset++
		}
		if offset == len(body) {
			break
		}
		lineEnd := bytes.IndexByte(body[offset:], '\n')
		if lineEnd < 0 {
			return nil, frames, fmt.Errorf("stream chunk %d: missing length delimiter", chunks+1)
		}
		lineEnd += offset
		line := strings.TrimSpace(string(body[offset:lineEnd]))
		if line == "" {
			offset = lineEnd + 1
			continue
		}
		declared, err := strconv.Atoi(line)
		if err != nil {
			return nil, frames, fmt.Errorf("stream chunk %d: parse length %q: %w", chunks+1, line, err)
		}
		chunks++
		frame, next, err := streamFrame(body, lineEnd+1, declared)
		if err != nil {
			return nil, frames, fmt.Errorf("stream chunk %d: %w", chunks, err)
		}
		offset = next
		var rows [][]json.RawMessage
		if err := json.Unmarshal(frame, &rows); err != nil {
			return nil, frames, fmt.Errorf("stream chunk %d: parse frame: %w", chunks, err)
		}
		for _, row := range rows {
			if len(row) < 3 {
				continue
			}
			var kind string
			if json.Unmarshal(row[0], &kind) != nil || kind != "wrb.fr" {
				continue
			}
			var payload string
			if err := json.Unmarshal(row[2], &payload); err != nil || payload == "" {
				continue
			}
			if !json.Valid([]byte(payload)) {
				return nil, frames, fmt.Errorf("stream chunk %d: wrb.fr payload is not valid JSON", chunks)
			}
			final = append(final[:0], payload...)
			frames++
		}
	}
	if frames == 0 {
		return nil, 0, fmt.Errorf("stream contains no wrb.fr payloads")
	}
	return &batchexecute.WireResponse{Responses: []batchexecute.WireRPCResponse{{
		ID: rpcID, Data: final,
	}}}, frames, nil
}

// streamFrame returns one length-prefixed frame. The declared length includes
// the newline after the length and the newline after the frame. Google uses
// bytes, code points, or UTF-16 code units for that count in observed streams.
func streamFrame(body []byte, start, declared int) ([]byte, int, error) {
	target := declared - 2
	if target < 0 {
		return nil, start, fmt.Errorf("length %d is too small", declared)
	}
	// Most frames count the two framing newlines. A captured continuation
	// stream adds ten more bytes after its first answer frame. Keep that
	// observed variant narrow: it must still end at a delimiter and form JSON.
	targets := []int{target}
	if continuationTarget := declared - 12; continuationTarget >= 0 {
		targets = append(targets, continuationTarget)
	}
	for _, target := range targets {
		var ends []int
		if end := start + target; end <= len(body) {
			ends = append(ends, end)
		}
		if end, ok := streamFrameEndRunes(body, start, target, false); ok {
			ends = append(ends, end)
		}
		if end, ok := streamFrameEndRunes(body, start, target, true); ok {
			ends = append(ends, end)
		}
		for _, end := range ends {
			next, ok := streamFrameDelimiter(body, end)
			if !ok || !json.Valid(body[start:end]) {
				continue
			}
			return body[start:end], next, nil
		}
	}
	return nil, start, fmt.Errorf("length %d does not delimit a valid frame", declared)
}

func streamFrameDelimiter(body []byte, end int) (int, bool) {
	if end == len(body) {
		return end, true
	}
	if end > len(body) {
		return 0, false
	}
	if body[end] == '\n' {
		return end + 1, true
	}
	if body[end] == '\r' && end+1 < len(body) && body[end+1] == '\n' {
		return end + 2, true
	}
	return 0, false
}

func streamFrameEndRunes(body []byte, start, count int, utf16Units bool) (int, bool) {
	end := start
	for units := 0; units < count; {
		if end >= len(body) {
			return 0, false
		}
		r, size := utf8.DecodeRune(body[end:])
		if r == utf8.RuneError && size == 1 {
			return 0, false
		}
		step := 1
		if utf16Units {
			step = len(utf16.Encode([]rune{r}))
		}
		if units+step > count {
			return 0, false
		}
		units += step
		end += size
	}
	return end, true
}
