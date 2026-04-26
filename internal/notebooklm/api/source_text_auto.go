package api

import (
	"errors"
	"fmt"

	"github.com/tmc/nlm/internal/batchexecute"
)

// AutoChunkSchedule is the descending sequence of byte sizes the auto-chunked
// text upload tries when the server rejects a part. The first element is the
// initial part size; on rejection, a failing part is re-split using the next
// smaller size, and so on until a part either succeeds or the floor is
// reached. The floor is the last element — if a part still fails at that
// size, the upload aborts and surfaces the error.
//
// Empirically the server tolerates much less than the documented 13 MB
// ceiling for some payloads (HAR-captured JS bundles in the 1–3 MB range
// have triggered "Internal errors that shouldn't be exposed to clients"
// even though equally large content from other sources uploads cleanly).
// The schedule cuts roughly in half each step so a content-localized
// failure converges quickly without too many round-trips.
var AutoChunkSchedule = []int{
	10 * 1024 * 1024, // 10 MiB
	5 * 1024 * 1024,  // 5 MiB
	4 * 1024 * 1024,  // 4 MiB
	2 * 1024 * 1024,  // 2 MiB
	1 * 1024 * 1024,  // 1 MiB
	512 * 1024,
	256 * 1024,
	128 * 1024,
	64 * 1024,
	32 * 1024,
	16 * 1024,
	8 * 1024,
	4 * 1024,
}

// AutoChunkProgress reports a single auto-chunk upload event. ProgressFunc
// receives one of these per attempt so callers can render status. SizeBytes is
// the size attempted, ByteOffset is the position within the original content
// the chunk starts at, and Index/Total describe the part within the leaf-flat
// numbering visible to the user as `name`, `name (pt2)`, `name (pt3)`, etc.
// PartName is the title that will be sent (or was sent) to the server.
type AutoChunkProgress struct {
	PartName   string
	Index      int    // 0-based leaf index assigned at the moment of upload
	Total      int    // total leaves uploaded so far including this one (grows on splits)
	SizeBytes  int    // bytes attempted in this call
	ByteOffset int    // byte offset within the original content
	Attempt    int    // 1-based: 1 on first try, >1 after a re-split descent
	SourceID   string // populated only on success
	Err        error  // populated only on failure (terminal or about to descend)
	Descending bool   // true when Err is non-nil and we will retry smaller; false on terminal
}

// ProgressFunc is invoked once per attempt for AutoChunkAddSourceFromText.
// Implementations should not retain the AutoChunkProgress beyond the call.
type ProgressFunc func(AutoChunkProgress)

// AddSourceFromTextAuto uploads content as one or more text sources, splitting
// on the descending AutoChunkSchedule whenever the server rejects a part. It
// is the recommended entry point for arbitrary-sized text content — small
// payloads upload as a single source under baseName, while oversize payloads
// arrive as `baseName`, `baseName (pt2)`, … in the notebook.
//
// Errors that look like permanent client problems (auth, source-cap, oversize
// past MaxTextSourceBytes, not-found, permission-denied, rate-limit) are
// returned immediately without descent — descending only helps for errors
// that appear payload-related. If a part still fails at the schedule's floor,
// the underlying error is wrapped with the failing part's name and offset so
// the caller can pinpoint the offending byte range.
//
// progress may be nil. When non-nil it is called per attempt; callers
// typically use it to print "uploaded part-N (X bytes) -> ID" or to drive
// progress bars.
func (c *Client) AddSourceFromTextAuto(projectID string, content []byte, baseName string, progress ProgressFunc) ([]string, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("nothing to upload: content is empty")
	}
	state := &autoChunkState{
		client:    c,
		projectID: projectID,
		baseName:  baseName,
		progress:  progress,
	}
	// Don't fail upfront on MaxTextSourceBytes — the schedule's first level
	// (10 MiB) caps a single request to within the safe band, so a 50 MiB
	// input lands as five 10 MiB parts and any rejected part descends per
	// the schedule. AddSourceFromText still enforces the per-call cap so a
	// schedule entry above the limit would fail fast at uploadOne.
	if err := state.upload(content, 0, 0); err != nil {
		return state.ids, err
	}
	return state.ids, nil
}

type autoChunkState struct {
	client    *Client
	projectID string
	baseName  string
	progress  ProgressFunc
	ids       []string
}

// upload tries to upload data as one source at scheduleIdx; on a payload-
// shaped failure it splits at the next size and recurses. byteOffset is the
// position of data[0] within the original content, used purely for progress
// reporting so a caller can localize a failing range.
func (s *autoChunkState) upload(data []byte, byteOffset, scheduleIdx int) error {
	size := AutoChunkSchedule[scheduleIdx]
	// At each level the part we receive may already be ≤size (the bottom of
	// a recursion path). Only re-split when the data still exceeds the
	// current size; otherwise upload as-is.
	if len(data) <= size {
		return s.uploadOne(data, byteOffset, scheduleIdx)
	}
	// Slice data into size-sized parts and try each in turn at this level.
	for i := 0; i < len(data); i += size {
		end := i + size
		if end > len(data) {
			end = len(data)
		}
		if err := s.uploadOne(data[i:end], byteOffset+i, scheduleIdx); err != nil {
			return err
		}
	}
	return nil
}

// uploadOne uploads a single part. On a content-shaped failure it descends
// to the next schedule level (re-splitting *this* part) — that level's call
// to upload may produce many sub-parts that themselves descend further.
func (s *autoChunkState) uploadOne(part []byte, byteOffset, scheduleIdx int) error {
	leafIdx := len(s.ids)
	name := autoChunkPartName(s.baseName, leafIdx)
	id, err := s.client.AddSourceFromText(s.projectID, string(part), name)
	attempt := scheduleIdx + 1
	if err == nil {
		s.ids = append(s.ids, id)
		if s.progress != nil {
			s.progress(AutoChunkProgress{
				PartName:   name,
				Index:      leafIdx,
				Total:      len(s.ids),
				SizeBytes:  len(part),
				ByteOffset: byteOffset,
				Attempt:    attempt,
				SourceID:   id,
			})
		}
		return nil
	}
	if !shouldDescendOnAutoChunkError(err) {
		if s.progress != nil {
			s.progress(AutoChunkProgress{
				PartName:   name,
				Index:      leafIdx,
				Total:      len(s.ids) + 1,
				SizeBytes:  len(part),
				ByteOffset: byteOffset,
				Attempt:    attempt,
				Err:        err,
			})
		}
		return fmt.Errorf("upload %s (%d bytes at offset %d): %w", name, len(part), byteOffset, err)
	}
	// Descend if there's room in the schedule, otherwise surface as terminal.
	next := scheduleIdx + 1
	if next >= len(AutoChunkSchedule) {
		if s.progress != nil {
			s.progress(AutoChunkProgress{
				PartName:   name,
				Index:      leafIdx,
				Total:      len(s.ids) + 1,
				SizeBytes:  len(part),
				ByteOffset: byteOffset,
				Attempt:    attempt,
				Err:        err,
			})
		}
		return fmt.Errorf("upload %s (%d bytes at offset %d) failed at schedule floor: %w", name, len(part), byteOffset, err)
	}
	if s.progress != nil {
		s.progress(AutoChunkProgress{
			PartName:   name,
			Index:      leafIdx,
			Total:      len(s.ids) + 1,
			SizeBytes:  len(part),
			ByteOffset: byteOffset,
			Attempt:    attempt,
			Err:        err,
			Descending: true,
		})
	}
	return s.upload(part, byteOffset, next)
}

// autoChunkPartName returns the leaf-flat name for a 0-based index. Index 0
// is the bare base; subsequent indexes are "<base> (pt2)", "<base> (pt3)",
// matching the scheme `nlm sync` and `nlm source add --chunk` use, so all
// chunked-source names look the same in the notebook UI regardless of which
// path produced them.
func autoChunkPartName(base string, idx int) string {
	if idx == 0 {
		return base
	}
	return fmt.Sprintf("%s (pt%d)", base, idx+1)
}

// shouldDescendOnAutoChunkError reports whether err looks like a server-side
// rejection that might be cured by uploading less content per request. We
// descend on anything that isn't clearly a permanent client problem, so any
// new server error mode we don't yet recognize will at least get a chance to
// resolve via splitting before surfacing.
func shouldDescendOnAutoChunkError(err error) bool {
	if err == nil {
		return false
	}
	// Don't descend on our own client-side typed errors — they describe
	// states (auth, cap reached, oversize) that splitting can't fix.
	if errors.Is(err, ErrSourceCapReached) || errors.Is(err, ErrSourceTooLarge) {
		return false
	}
	var apiErr *batchexecute.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatus {
		case 401, 403, 404, 429:
			return false
		}
		if apiErr.ErrorCode != nil {
			switch apiErr.ErrorCode.Type {
			case batchexecute.ErrorTypeAuthentication,
				batchexecute.ErrorTypeAuthorization,
				batchexecute.ErrorTypePermissionDenied,
				batchexecute.ErrorTypeNotFound,
				batchexecute.ErrorTypeRateLimit:
				return false
			}
		}
	}
	return true
}
