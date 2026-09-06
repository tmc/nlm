package nlmsync

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IncompleteError reports a sync that did not finish. Some remote operations
// may have succeeded; the receipt records the operations observed by this run.
// Unwrap preserves the underlying upload, authentication, or transport error.
type IncompleteError struct {
	Name    string
	Receipt string
	Err     error
}

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("sync family %q incomplete; sources may contain mixed revisions (receipt %s): %v", e.Name, e.Receipt, e.Err)
}

func (e *IncompleteError) Unwrap() error { return e.Err }

type receiptPart struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// A receipt describes one attempt, not the authoritative state of a notebook.
// Each attempt has its own file so concurrent runs cannot overwrite evidence.
type syncReceipt struct {
	Version         int           `json:"version"`
	Action          string        `json:"action"`
	NotebookID      string        `json:"notebook_id"`
	Name            string        `json:"name"`
	Started         time.Time     `json:"started"`
	Status          string        `json:"status"`
	Readiness       string        `json:"readiness"`
	CleanupComplete bool          `json:"cleanup_complete"`
	MaxBytes        int           `json:"max_bytes"`
	AutoSplit       bool          `json:"auto_split"`
	DryRun          bool          `json:"dry_run"`
	Parts           []receiptPart `json:"parts"`
	Operations      []event       `json:"operations"`
	Error           string        `json:"error,omitempty"`
	Path            string        `json:"receipt,omitempty"`
}

func newSyncReceipt(notebookID, name string, names, hashes []string, chunks [][]byte, opts Options) (*syncReceipt, error) {
	r := &syncReceipt{
		Version: 1, Action: "result", NotebookID: notebookID, Name: name,
		Started: time.Now().UTC(), Status: "in-progress", Readiness: "unchecked",
		MaxBytes: opts.maxBytes(), AutoSplit: opts.AutoSplit, DryRun: opts.DryRun,
		Operations: []event{},
	}
	for i, name := range names {
		r.Parts = append(r.Parts, receiptPart{name, hashes[i], len(chunks[i])})
	}
	if opts.DryRun {
		return r, nil
	}
	key := sha256.Sum256([]byte(notebookID + "\x00" + name))
	dir := filepath.Join(cacheRoot(), "sync-attempts", fmt.Sprintf("%x", key))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, "attempt-*.json")
	if err != nil {
		return nil, err
	}
	r.Path = f.Name()
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := r.save(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *syncReceipt) save() error {
	if r.Path == "" {
		return nil
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(r.Path), ".receipt-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), r.Path)
}

func (o *outputWriter) finish(runErr error) error {
	r := o.receipt
	r.CleanupComplete = runErr == nil && !r.DryRun
	err := errors.Join(runErr, o.err)
	r.Status = "complete"
	if r.DryRun {
		r.Status = "planned"
	}
	if err != nil {
		r.Status = "incomplete"
		r.Error = err.Error()
	}
	if saveErr := r.save(); saveErr != nil {
		err = errors.Join(err, fmt.Errorf("finish sync receipt: %w", saveErr))
		r.Status = "incomplete"
		r.Error = err.Error()
	}
	if o.json {
		if outputErr := json.NewEncoder(o.w).Encode(r); outputErr != nil {
			err = errors.Join(err, fmt.Errorf("write sync result: %w", outputErr))
			r.Status = "incomplete"
			r.Error = err.Error()
			err = errors.Join(err, r.save())
		}
	} else {
		fmt.Fprintf(os.Stderr, "  family: %s: %s (indexing readiness unchecked; receipt %s)\n", r.Name, r.Status, r.Path)
	}
	if err != nil {
		return &IncompleteError{Name: r.Name, Receipt: r.Path, Err: err}
	}
	return nil
}

func checkSourceFamily(sources []Source, name string) error {
	seen := make(map[string]string)
	var errs []error
	for _, source := range sources {
		if !isPartOf(source.Title, name) {
			continue
		}
		if id, ok := seen[source.Title]; ok {
			errs = append(errs, fmt.Errorf("duplicate part %q: sources %s and %s", source.Title, id, source.ID))
		}
		seen[source.Title] = source.ID
		if source.Status == "error" {
			errs = append(errs, fmt.Errorf("part %q: source %s has server status error", source.Title, source.ID))
		}
	}
	return errors.Join(errs...)
}
