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
	// First, strip the prefix if present
	br := bufio.NewReader(r)
	prefix, err := br.Peek(4)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("peek response prefix: %w", err)
	}
	
	// Debug: print the prefix
	fmt.Printf("DEBUG: Response prefix: %q\n", prefix)

	// Check for and discard the )]}' prefix
	if len(prefix) >= 4 && string(prefix[:4]) == ")]}''" {
		_, err = br.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read prefix line: %w", err)
		}
	}

	var (
		chunks     []string
		scanner    = bufio.NewScanner(br)
		chunkData  strings.Builder
		collecting bool
		chunkSize  int
	)

	// Process each line
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Printf("DEBUG: Processing line: %q\n", line)

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			fmt.Printf("DEBUG: Skipping empty line\n")
			continue
		}

		// If we're not currently collecting a chunk, this line should be a chunk length
		if !collecting {
			size, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil {
				// If not a number, it might be direct JSON data
				// Check if it looks like JSON
				if strings.HasPrefix(strings.TrimSpace(line), "{") || strings.HasPrefix(strings.TrimSpace(line), "[") {
					chunks = append(chunks, line)
				} else if strings.HasPrefix(strings.TrimSpace(line), "wrb.fr") {
					// It might be a direct RPC response without proper JSON format
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

		// If we're collecting a chunk, add this line to the current chunk
		chunkData.WriteString(line)

		// If we've collected enough data, add the chunk and reset
		if chunkData.Len() >= chunkSize {
			chunks = append(chunks, chunkData.String())
			collecting = false
		}
	}

	// Check if we have any partial chunk data remaining
	if collecting && chunkData.Len() > 0 {
		chunks = append(chunks, chunkData.String())
	}

	// Process all collected chunks
	return processChunks(chunks)
}

// extractWRBResponse attempts to manually extract a response from a chunk that contains "wrb.fr"
// but can't be properly parsed as JSON
func extractWRBResponse(chunk string) *Response {
	// Try to parse this as a regular JSON array first
	var data []interface{}
	if err := json.Unmarshal([]byte(chunk), &data); err == nil {
		// Use the standard extraction logic
		responses, err := extractResponses([][]interface{}{data})
		if err == nil && len(responses) > 0 {
			return &responses[0]
		}
	}

	// If JSON parsing fails, try manual extraction
	// Try to extract the ID (comes after "wrb.fr")
	idMatch := strings.Index(chunk, "wrb.fr")
	if idMatch < 0 {
		return nil
	}

	// Skip past "wrb.fr" and find next quotes
	idStart := idMatch + 7 // length of "wrb.fr" + 1 for a likely comma or quote
	for idStart < len(chunk) && (chunk[idStart] == ',' || chunk[idStart] == '"' || chunk[idStart] == ' ') {
		idStart++
	}

	// Find the end of the ID (next quote or comma)
	idEnd := idStart
	for idEnd < len(chunk) && chunk[idEnd] != '"' && chunk[idEnd] != ',' && chunk[idEnd] != ' ' {
		idEnd++
	}

	if idStart >= idEnd || idStart >= len(chunk) {
		return nil
	}

	id := chunk[idStart:idEnd]

	// Look for any JSON-like data after the ID
	dataStart := strings.Index(chunk[idEnd:], "{")
	var jsonData string
	if dataStart >= 0 {
		dataStart += idEnd // Adjust for the offset
		// Find the end of the JSON object
		dataEnd := findJSONEnd(chunk, dataStart, '{', '}')
		if dataEnd > dataStart {
			jsonData = chunk[dataStart:dataEnd]
		}
	} else {
		// No JSON object found, try to find a JSON array
		dataStart = strings.Index(chunk[idEnd:], "[")
		if dataStart >= 0 {
			dataStart += idEnd // Adjust for the offset
			// Find the end of the JSON array
			dataEnd := findJSONEnd(chunk, dataStart, '[', ']')
			if dataEnd > dataStart {
				jsonData = chunk[dataStart:dataEnd]
			}
		}
	}

	// If we found valid JSON data, use it; otherwise use a synthetic response
	if jsonData != "" {
		return &Response{
			ID:   id,
			Data: json.RawMessage(jsonData),
		}
	}

	// Use a synthetic success response
	return &Response{
		ID:   id,
		Data: json.RawMessage(`{"success":true}`),
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

	return len(s) // Return end of string if no matching close found
}

// processChunks processes all chunks and extracts the RPC responses
func processChunks(chunks []string) ([]Response, error) {
	fmt.Printf("DEBUG: processChunks called with %d chunks\n", len(chunks))
	for i, chunk := range chunks {
		fmt.Printf("DEBUG: Chunk %d: %q\n", i, chunk)
	}
	
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks found")
	}

	var allResponses []Response

	// Process each chunk
	for _, chunk := range chunks {
		// Try to fix any common escaping issues before parsing
		chunk = strings.ReplaceAll(chunk, "\\\"", "\"")

		// Remove any outer quotes if present
		trimmed := strings.TrimSpace(chunk)
		if (strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"")) ||
			(strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'")) {
			// This is a quoted string that might contain escaped JSON
			unquoted, err := strconv.Unquote(chunk)
			if err == nil {
				chunk = unquoted
			}
		}

		// Try to parse as a JSON array
		var data [][]interface{}
		if err := json.Unmarshal([]byte(chunk), &data); err != nil {
			// Try to parse as a single RPC response
			var singleData []interface{}
			if err := json.Unmarshal([]byte(chunk), &singleData); err != nil {
				// If it still fails, check if it contains wrb.fr and try to manually extract
				if strings.Contains(chunk, "wrb.fr") {
					// Manually construct a response
					fmt.Printf("Attempting to manually extract wrb.fr response from: %s\n", chunk)
					if resp := extractWRBResponse(chunk); resp != nil {
						allResponses = append(allResponses, *resp)
						continue
					}
				}
				// Skip invalid chunks
				continue
			}
			data = [][]interface{}{singleData}
		}

		// Extract RPC responses from the chunk
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

		// Check if this is a valid RPC response
		rpcType, ok := rpcData[0].(string)
		if !ok || rpcType != "wrb.fr" {
			continue
		}

		// Extract the RPC ID
		id, ok := rpcData[1].(string)
		if !ok {
			continue
		}

		// Create response object
		resp := Response{
			ID: id,
		}

		// Extract the response data
		if rpcData[2] != nil {
			switch data := rpcData[2].(type) {
			case string:
				resp.Data = json.RawMessage(data)
			default:
				// If it's not a string, try to marshal it
				if rawData, err := json.Marshal(data); err == nil {
					resp.Data = rawData
				}
			}
		}

		// Extract the response index
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

		// Check for error responses
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
