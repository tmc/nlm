package batchexecute

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// WrbFRFrames splits Google's length-prefixed wrb.fr response into JSON
// frames. Google has used byte, code point, UTF-16, and continuation counts
// in observed streams.
func WrbFRFrames(body []byte) ([][]byte, int, error) {
	body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte(")]}'")))

	var frames [][]byte
	chunks := 0
	for offset := 0; offset < len(body); {
		for offset < len(body) && (body[offset] == '\n' || body[offset] == '\r') {
			offset++
		}
		if offset == len(body) {
			break
		}
		lineEnd := bytes.IndexByte(body[offset:], '\n')
		if lineEnd < 0 {
			return nil, chunks, fmt.Errorf("stream chunk %d: missing length delimiter", chunks+1)
		}
		lineEnd += offset
		line := strings.TrimSpace(string(body[offset:lineEnd]))
		if line == "" {
			offset = lineEnd + 1
			continue
		}
		declared, err := strconv.Atoi(line)
		if err != nil {
			return nil, chunks, fmt.Errorf("stream chunk %d: parse length %q: %w", chunks+1, line, err)
		}
		chunks++
		frame, next, err := wrbFRFrame(body, lineEnd+1, declared)
		if err != nil {
			return nil, chunks, fmt.Errorf("stream chunk %d: %w", chunks, err)
		}
		frames = append(frames, frame)
		offset = next
	}
	return frames, chunks, nil
}

func wrbFRFrame(body []byte, start, declared int) ([]byte, int, error) {
	target := declared - 2
	if target < 0 {
		return nil, start, fmt.Errorf("length %d is too small", declared)
	}
	var ends []int
	if end := start + target; end <= len(body) {
		ends = append(ends, end)
	}
	if end, ok := wrbFRFrameEndRunes(body, start, target, false); ok {
		ends = append(ends, end)
	}
	if end, ok := wrbFRFrameEndRunes(body, start, target, true); ok {
		ends = append(ends, end)
	}
	for _, end := range ends {
		next, ok := wrbFRFrameDelimiter(body, end)
		if !ok || !json.Valid(body[start:end]) {
			continue
		}
		return body[start:end], next, nil
	}
	if frame, next, ok := wrbFRContinuationFrame(body, start, target); ok {
		return frame, next, nil
	}
	return nil, start, fmt.Errorf("length %d does not delimit a valid frame", declared)
}

func wrbFRContinuationFrame(body []byte, start, target int) ([]byte, int, bool) {
	limit := start + target
	if limit > len(body) {
		limit = len(body)
	}
	for search := limit; search > start; {
		relativeEnd := bytes.LastIndexByte(body[start:search], '\n')
		if relativeEnd < 0 {
			break
		}
		end := start + relativeEnd
		search = end
		next, ok := wrbFRFrameDelimiter(body, end)
		if !ok || !wrbFRNextLength(body, next) || !json.Valid(body[start:end]) {
			continue
		}
		return body[start:end], next, true
	}
	return nil, start, false
}

func wrbFRNextLength(body []byte, start int) bool {
	end := bytes.IndexByte(body[start:], '\n')
	if end <= 0 {
		return false
	}
	_, err := strconv.Atoi(string(body[start : start+end]))
	return err == nil
}

func wrbFRFrameDelimiter(body []byte, end int) (int, bool) {
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

func wrbFRFrameEndRunes(body []byte, start, count int, utf16Units bool) (int, bool) {
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
