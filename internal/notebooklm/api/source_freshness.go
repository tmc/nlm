package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// CheckFreshnessResult is the decoded response from yR9Yof.
// Wire shape (HAR-verified 2026-04-17):
//
//	[null, is_fresh_bool, [source_id]]
//
// The server returns a clean signal for Google-Drive sources. Non-Drive
// sources are rejected with "One or more arguments are invalid" against
// the correct [2] context value. The client gates refresh to Drive sources.
type CheckFreshnessResult struct {
	IsFresh  bool
	SourceID string
}

// RefreshSourceResult is the decoded response from FLmJqe.
// Wire shape (HAR-verified 2026-04-17 against a real Google Drive source;
// see internal/method/testdata/FLmJqe_refresh_source_response_drive.json
// for a synthetic fixture preserving the same shape):
//
//	[
//	  [source_id],
//	  title,
//	  [
//	    [drive_file_id, drive_rev_token, 93],
//	    316,
//	    [ts_sec, ts_nsec],      // revision timestamp
//	    [rev_uuid, [ts_sec, ts_nsec]],
//	    1, null, 1, null, 763,
//	    null, null, null, null, null,
//	    [ts_sec, ts_nsec],      // last-modified timestamp
//	  ],
//	  [null, final_state_enum],
//	]
//
// Only fields we currently surface are decoded; unknown positions are
// skipped. The final_state_enum value 2 observed in the sync-success
// capture; other enum values are not documented yet.
type RefreshSourceResult struct {
	SourceID         string
	Title            string
	DriveFileID      string
	DriveRevToken    string
	SourceRevisionID string
	RevisionTime     time.Time
	LastModified     time.Time
	FinalState       int
}

// parseCheckFreshnessResponse decodes a yR9Yof response payload.
func parseCheckFreshnessResponse(raw json.RawMessage) (*CheckFreshnessResult, error) {
	var outer []json.RawMessage
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, fmt.Errorf("check-freshness: outer decode: %w", err)
	}
	if len(outer) < 3 {
		return nil, fmt.Errorf("check-freshness: want >=3 positions, got %d", len(outer))
	}
	var isFresh bool
	if err := json.Unmarshal(outer[1], &isFresh); err != nil {
		return nil, fmt.Errorf("check-freshness: is_fresh decode: %w", err)
	}
	var idArr []string
	if err := json.Unmarshal(outer[2], &idArr); err != nil {
		return nil, fmt.Errorf("check-freshness: source_id decode: %w", err)
	}
	var sourceID string
	if len(idArr) > 0 {
		sourceID = idArr[0]
	}
	return &CheckFreshnessResult{IsFresh: isFresh, SourceID: sourceID}, nil
}

// parseRefreshSourceResponse decodes a FLmJqe response payload. Any
// unexpected internal shape falls back to best-effort decoding — the
// caller gets whatever fields we could extract rather than a hard
// failure, since the response is wide and fields added upstream should
// not break our parser.
func parseRefreshSourceResponse(raw json.RawMessage) (*RefreshSourceResult, error) {
	var outer []json.RawMessage
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, fmt.Errorf("refresh-source: outer decode: %w", err)
	}
	if len(outer) < 4 {
		return nil, fmt.Errorf("refresh-source: want >=4 positions, got %d", len(outer))
	}
	result := &RefreshSourceResult{}

	var idArr []string
	if err := json.Unmarshal(outer[0], &idArr); err == nil && len(idArr) > 0 {
		result.SourceID = idArr[0]
	}
	_ = json.Unmarshal(outer[1], &result.Title)

	var body []json.RawMessage
	if err := json.Unmarshal(outer[2], &body); err == nil {
		if len(body) > 0 {
			var driveTuple []json.RawMessage
			if err := json.Unmarshal(body[0], &driveTuple); err == nil {
				if len(driveTuple) > 0 {
					_ = json.Unmarshal(driveTuple[0], &result.DriveFileID)
				}
				if len(driveTuple) > 1 {
					_ = json.Unmarshal(driveTuple[1], &result.DriveRevToken)
				}
			}
		}
		if len(body) > 2 {
			result.RevisionTime = parseTimePair(body[2])
		}
		if len(body) > 3 {
			var revTuple []json.RawMessage
			if err := json.Unmarshal(body[3], &revTuple); err == nil {
				if len(revTuple) > 0 {
					_ = json.Unmarshal(revTuple[0], &result.SourceRevisionID)
				}
				if len(revTuple) > 1 {
					result.RevisionTime = parseTimePair(revTuple[1])
				}
			}
		}
		if len(body) > 15 {
			result.LastModified = parseTimePair(body[15])
		}
	}

	var trailer []json.RawMessage
	if err := json.Unmarshal(outer[3], &trailer); err == nil && len(trailer) > 1 {
		_ = json.Unmarshal(trailer[1], &result.FinalState)
	}
	return result, nil
}

// parseTimePair decodes a [seconds, nanoseconds] pair. Returns zero time
// on any decode error.
func parseTimePair(raw json.RawMessage) time.Time {
	var pair []int64
	if err := json.Unmarshal(raw, &pair); err != nil || len(pair) < 2 {
		return time.Time{}
	}
	return time.Unix(pair[0], pair[1]).UTC()
}
