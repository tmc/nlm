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
	
	// Check for and discard the )]}' prefix
	if len(prefix) >= 4 && string(prefix[:4]) == ")]}''" {
		_, err = br.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read prefix line: %w", err)
		}
	}

	var (
		chunks    []string
		scanner   = bufio.NewScanner(br)
		chunkData strings.Builder
		collecting bool
		chunkSize int
	)

	// Process each line
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
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

// processChunks processes all chunks and extracts the RPC responses
func processChunks(chunks []string) ([]Response, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks found")
	}

	var allResponses []Response

	// Process each chunk
	for _, chunk := range chunks {
		// Try to parse as a JSON array
		var data [][]interface{}
		if err := json.Unmarshal([]byte(chunk), &data); err != nil {
			// Try to parse as a single RPC response
			var singleData []interface{}
			if err := json.Unmarshal([]byte(chunk), &singleData); err != nil {
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