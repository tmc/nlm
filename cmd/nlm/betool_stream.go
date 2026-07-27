package main

import (
	"encoding/json"
	"fmt"
	"net/url"

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
	rawFrames, _, err := batchexecute.WrbFRFrames(body)
	if err != nil {
		return nil, 0, err
	}
	var final json.RawMessage
	frames := 0
	for _, frame := range rawFrames {
		var rows [][]json.RawMessage
		if err := json.Unmarshal(frame, &rows); err != nil {
			return nil, frames, fmt.Errorf("parse stream frame: %w", err)
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
				return nil, frames, fmt.Errorf("wrb.fr payload is not valid JSON")
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
