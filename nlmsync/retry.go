package nlmsync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tmc/nlm/internal/batchexecute"
)

// NotebookLM answers a transient upload failure with a generic internal
// error that carries no size signal, so a first failure says nothing about
// whether the payload was too large. Retrying the same bytes distinguishes
// the two: a fault that clears on a retry was never about size, and
// splitting it produces a deep tree of parts for no reason.
// uploadAttempts bounds re-attempts when retrying is the only remedy left.
// A chunk that can still be split gets uploadSplitAttempts instead: splitting
// also clears a transient fault, so spending three attempts at every level
// of a deep tree buys little and costs a lot.
const (
	uploadAttempts      = 3
	uploadSplitAttempts = 2
)

// uploadRetryDelay is the base backoff between attempts; tests set it to zero.
var uploadRetryDelay = 2 * time.Second

// retryableUpload reports whether err is a server-side fault worth retrying
// with the same payload. Authentication, quota, and "too large" failures
// need a different remedy, and a payload-too-large response is a real size
// signal that should go straight to a split.
func retryableUpload(err error) bool {
	var upload *uploadError
	var api *batchexecute.APIError
	if !errors.As(err, &upload) || !errors.As(upload.err, &api) {
		return false
	}
	if api.HTTPStatus == 413 {
		return false
	}
	if api.HTTPStatus == 401 || api.HTTPStatus == 403 || api.HTTPStatus == 404 || api.HTTPStatus == 429 {
		return false
	}
	if api.ErrorCode != nil {
		switch api.ErrorCode.Type {
		case batchexecute.ErrorTypeAuthentication, batchexecute.ErrorTypeAuthorization,
			batchexecute.ErrorTypePermissionDenied, batchexecute.ErrorTypeNotFound,
			batchexecute.ErrorTypeRateLimit, batchexecute.ErrorTypeResourceExhausted:
			return false
		}
		return api.ErrorCode.Retryable
	}
	return api.HTTPStatus == 0 || api.HTTPStatus >= 500
}

// uploadChunkWithRetry uploads a chunk, re-attempting a transient server
// failure with backoff before reporting it. The chunk keeps its name across
// attempts, so a retry replaces whatever a failed attempt left behind.
func uploadChunkWithRetry(ctx context.Context, c Client, notebookID, chunkName string, data []byte, hash string, existing Source, exists bool, labelIDs []string, hc *hashCache, sc *sourceCache, out *outputWriter, mu *sync.Mutex, attempts int) error {
	var err error
	for attempt := 1; ; attempt++ {
		err = uploadChunk(ctx, c, notebookID, chunkName, data, hash, existing, exists, labelIDs, hc, sc, out, mu)
		if err == nil || attempt >= attempts || !retryableUpload(err) {
			return err
		}
		// Clear whatever the rejected attempt left behind before sending the
		// same bytes again, so retries do not stack duplicate dead sources.
		if cleanupErr := discardFailedPart(ctx, c, notebookID, chunkName, existing.ID, sc, out, mu); cleanupErr != nil {
			return errors.Join(err, cleanupErr)
		}
		mu.Lock()
		out.emit(event{Action: "retry", Name: chunkName, Bytes: len(data), Reason: err.Error()})
		mu.Unlock()
		select {
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		case <-time.After(time.Duration(attempt) * uploadRetryDelay):
		}
	}
}

// discardFailedPart removes sources the server marks failed under an exact
// part title. A rejected upload sometimes still creates the source, which
// then settles into an error state: without this sweep a retry or a split
// leaves one dead source behind per attempt, and they accumulate into
// dozens of identically named red rows in the notebook. keepID names a
// source the caller still needs (the original a replacement failed over),
// which is never removed.
func discardFailedPart(ctx context.Context, c Client, notebookID, title, keepID string, sc *sourceCache, out *outputWriter, mu *sync.Mutex) error {
	sources, err := c.ListSources(ctx, notebookID)
	if err != nil {
		return fmt.Errorf("list sources to discard failed %q: %w", title, err)
	}
	var ids []string
	for _, source := range sources {
		if source.Title == title && source.Status == "error" && source.ID != keepID {
			ids = append(ids, source.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if err := c.DeleteSources(ctx, notebookID, ids); err != nil {
		return fmt.Errorf("discard failed %q: %w", title, err)
	}
	mu.Lock()
	for _, id := range ids {
		sc.remove(notebookID, id)
		out.emit(event{Action: "delete", Name: title, OldID: id, Reason: "failed upload"})
	}
	mu.Unlock()
	return nil
}
