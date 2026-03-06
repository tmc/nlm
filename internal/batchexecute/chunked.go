package batchexecute

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// parseChunkedResponse parses a chunked response from the batchexecute API.
// The response format is:
// <chunk-length>
// <chunk-data>
// <chunk-length>
// <chunk-data>
// ...
func parseChunkedResponse(r io.Reader) ([]Response, error) {
	br := bufio.NewReader(r)

	// The response format starts with )]}'\n or )]}'\n\n
	// We need to consume the entire prefix including newlines
	prefix, err := br.Peek(6)
	if err != nil && err != io.EOF {
		prefix, err = br.Peek(4)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("peek response prefix: %w", err)
		}
	}

	// Check for and discard the )]}' prefix with newlines
	if len(prefix) >= 4 && string(prefix[:4]) == ")]}''" {
		if _, err := br.ReadString('\n'); err != nil && err != io.EOF {
			return nil, fmt.Errorf("read prefix line: %w", err)
		}
		// Consume additional empty line if present
		if nextByte, err := br.Peek(1); err == nil && len(nextByte) > 0 && nextByte[0] == '\n' {
			br.ReadByte()
		}
	}

	var (
		chunks     []string
		scanner    = bufio.NewScanner(br)
		chunkData  strings.Builder
		collecting bool
		chunkSize  int
		allLines   []string
	)

	// Increase scanner buffer size to handle large chunks (up to 10MB)
	const maxScanTokenSize = 10 * 1024 * 1024
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	for scanner.Scan() {
		line := scanner.Text()
		allLines = append(allLines, line)

		// Skip empty lines only if not collecting
		if !collecting && strings.TrimSpace(line) == "" {
			continue
		}

		// If we're not currently collecting a chunk, this line should be a chunk length
		if !collecting {
			size, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil {
				// Not a number — might be direct JSON data
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
					chunks = append(chunks, line)
				} else if strings.HasPrefix(trimmed, "wrb.fr") {
					chunks = append(chunks, "["+line+"]")
				} else {
					chunks = append(chunks, line)
				}
				continue
			}

			chunkSize = size
			collecting = true
			chunkData.Reset()
			continue
		}

		// Collecting a chunk — add this line
		if chunkData.Len() > 0 {
			chunkData.WriteString("\n")
		}
		chunkData.WriteString(line)

		if chunkData.Len() >= chunkSize {
			chunks = append(chunks, chunkData.String())
			collecting = false
		}
	}

	// Handle partial/remaining chunk data
	if collecting && chunkData.Len() > 0 {
		chunks = append(chunks, chunkData.String())
	} else if collecting && chunkData.Len() == 0 {
		if chunkSize < 1000 {
			// Small number might be an error code
			chunks = append(chunks, strconv.Itoa(chunkSize))
		} else if len(allLines) > 1 {
			// Large chunk size but no data collected — try using all lines
			chunks = append(chunks, strings.Join(allLines[1:], "\n"))
		}
	}

	// If no chunks parsed but we have lines, treat all lines as a single response
	if len(chunks) == 0 && len(allLines) > 0 {
		allData := strings.Join(allLines, "\n")
		if strings.TrimSpace(allData) != "" {
			chunks = append(chunks, allData)
		}
	}

	return processChunks(chunks)
}

// extractWRBResponse attempts to manually extract a response from a chunk that contains "wrb.fr"
// but can't be properly parsed as JSON
func extractWRBResponse(chunk string) *Response {
	var data []interface{}
	if err := json.Unmarshal([]byte(chunk), &data); err == nil {
		responses, err := extractResponses([][]interface{}{data})
		if err == nil && len(responses) > 0 {
			return &responses[0]
		}
	}

	// If JSON parsing fails, try manual extraction
	idMatch := strings.Index(chunk, "wrb.fr")
	if idMatch < 0 {
		return nil
	}

	// Skip past "wrb.fr" and find next quotes
	idStart := idMatch + 7
	for idStart < len(chunk) && (chunk[idStart] == ',' || chunk[idStart] == '"' || chunk[idStart] == ' ') {
		idStart++
	}

	idEnd := idStart
	for idEnd < len(chunk) && chunk[idEnd] != '"' && chunk[idEnd] != ',' && chunk[idEnd] != ' ' {
		idEnd++
	}

	if idStart >= idEnd || idStart >= len(chunk) {
		return nil
	}

	id := chunk[idStart:idEnd]

	// Look for JSON data after the ID
	dataStart := strings.Index(chunk[idEnd:], "{")
	var jsonData string
	if dataStart >= 0 {
		dataStart += idEnd
		dataEnd := findJSONEnd(chunk, dataStart, '{', '}')
		if dataEnd > dataStart {
			jsonData = chunk[dataStart:dataEnd]
		}
	} else {
		dataStart = strings.Index(chunk[idEnd:], "[")
		if dataStart >= 0 {
			dataStart += idEnd
			dataEnd := findJSONEnd(chunk, dataStart, '[', ']')
			if dataEnd > dataStart {
				jsonData = chunk[dataStart:dataEnd]
			}
		}
	}

	if jsonData != "" {
		return &Response{
			ID:   id,
			Data: json.RawMessage(jsonData),
		}
	}

	return &Response{
		ID:   id,
		Data: nil,
	}
}

// findJSONEnd finds the end of a JSON object or array starting from the given position
func findJSONEnd(s string, start int, openChar, closeChar rune) int {
	count := 0
	inQuotes := false
	escaped := false

	for i := start; i < len(s); i++ {
		c := rune(s[i])

		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inQuotes {
			escaped = true
			continue
		}

		if c == '"' {
			inQuotes = !inQuotes
			continue
		}

		if !inQuotes {
			if c == openChar {
				count++
			} else if c == closeChar {
				count--
				if count == 0 {
					return i + 1
				}
			}
		}
	}

	return len(s)
}

// isNumeric checks if a string contains only numeric characters
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// processChunks processes all chunks and extracts the RPC responses
func processChunks(chunks []string) ([]Response, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks found")
	}

	// Check for numeric responses (potential error codes)
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" || trimmed == ")]}'" {
			continue
		}
		if len(trimmed) <= 10 && isNumeric(trimmed) && !strings.Contains(trimmed, "wrb.fr") {
			return []Response{
				{
					ID:   "numeric",
					Data: json.RawMessage(trimmed),
				},
			}, nil
		}
	}

	var allResponses []Response

	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if (strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"")) ||
			(strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'")) {
			unquoted, err := strconv.Unquote(chunk)
			if err == nil {
				chunk = unquoted
			}
		}

		// Chunks may have trailing "\nNNN" (next chunk's size prefix) appended.
		// Trim any trailing non-JSON data after the last ']'.
		if lastBracket := strings.LastIndex(chunk, "]"); lastBracket >= 0 && lastBracket < len(chunk)-1 {
			chunk = chunk[:lastBracket+1]
		}

		var data [][]interface{}
		if err := json.Unmarshal([]byte(chunk), &data); err != nil {
			var singleData []interface{}
			if err := json.Unmarshal([]byte(chunk), &singleData); err != nil {
				if strings.Contains(chunk, "wrb.fr") {
					if resp := extractWRBResponse(chunk); resp != nil {
						allResponses = append(allResponses, *resp)
						continue
					}
				}
				continue
			}
			data = [][]interface{}{singleData}
		}

		responses, err := extractResponses(data)
		if err != nil {
			continue
		}

		allResponses = append(allResponses, responses...)
	}

	if len(allResponses) == 0 {
		return nil, fmt.Errorf("no valid responses found")
	}

	return allResponses, nil
}

// extractResponses extracts Response objects from RPC data
func extractResponses(data [][]interface{}) ([]Response, error) {
	var responses []Response

	for _, rpcData := range data {
		if len(rpcData) < 3 {
			continue
		}

		rpcType, ok := rpcData[0].(string)
		if !ok || rpcType != "wrb.fr" {
			continue
		}

		id, ok := rpcData[1].(string)
		if !ok {
			continue
		}

		resp := Response{
			ID: id,
		}

		if rpcData[2] != nil {
			switch data := rpcData[2].(type) {
			case string:
				resp.Data = json.RawMessage(data)
			default:
				if rawData, err := json.Marshal(data); err == nil {
					resp.Data = rawData
				}
			}
		}

		if len(rpcData) > 6 {
			if rpcData[6] == "generic" {
				resp.Index = 0
			} else if indexStr, ok := rpcData[6].(string); ok {
				index, err := strconv.Atoi(indexStr)
				if err == nil {
					resp.Index = index
				}
			}
		}

		if resp.ID == "error" && resp.Data != nil {
			var errorData struct {
				Error string `json:"error"`
				Code  int    `json:"code"`
			}
			if err := json.Unmarshal(resp.Data, &errorData); err == nil {
				resp.Error = errorData.Error
			}
		}

		responses = append(responses, resp)
	}

	return responses, nil
}
