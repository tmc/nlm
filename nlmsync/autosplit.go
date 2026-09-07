package nlmsync

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/tmc/nlm/internal/batchexecute"
	"golang.org/x/tools/txtar"
)

type uploadError struct {
	name  string
	bytes int
	err   error
}

func (e *uploadError) Error() string {
	return fmt.Sprintf("upload %q (%d bytes): %v", e.name, e.bytes, e.err)
}
func (e *uploadError) Unwrap() error { return e.err }

// Only server failures are candidates for splitting. Authentication, quota,
// cancellation, and local transport failures need a different remedy.
func canSplit(err error) bool {
	var upload *uploadError
	var api *batchexecute.APIError
	if !errors.As(err, &upload) || !errors.As(upload.err, &api) {
		return false
	}
	if api.HTTPStatus == 413 {
		return true
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
	return api.HTTPStatus >= 500
}

// runAutoSplit keeps a binary tree of part names on the server. Existing
// descendants preserve the split layout on the next run without a local
// manifest. Old parents are removed only after every leaf succeeds.
func runAutoSplit(ctx context.Context, c Client, notebookID, base string, names []string, chunks [][]byte, sources []Source, labels []string, opts Options, hc *hashCache, sc *sourceCache, out *outputWriter) error {
	byTitle := make(map[string]Source)
	for _, s := range sources {
		byTitle[s.Title] = s
	}
	active := make(map[string]bool)
	var mu sync.Mutex
	var visit func(string, []byte) error
	visit = func(name string, data []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		existing, exists := byTitle[name]
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		split := false
		for title := range byTitle {
			if splitDescendant(title, name) {
				split = true
				break
			}
		}
		if len(data) <= 4096 {
			split = false
		}
		if !split {
			if !opts.Force && exists && !hc.changed(name, hash) {
				if !opts.DryRun {
					for _, label := range labels {
						if err := c.(LabelPreserver).AttachLabelSource(ctx, notebookID, label, existing.ID); err != nil {
							return fmt.Errorf("attach label to %q: %w", name, err)
						}
					}
				}
				active[name] = true
				out.emit(event{Action: "skip", Name: name, Reason: "unchanged"})
				return nil
			}
			if opts.DryRun {
				action := "upload"
				if exists {
					action = "replace"
				}
				active[name] = true
				out.emit(event{Action: action, Name: name, Bytes: len(data), DryRun: true})
				return nil
			}
			err := uploadChunk(ctx, c, notebookID, name, data, hash, existing, exists, labels, hc, sc, out, &mu)
			if err == nil {
				active[name] = true
				return nil
			}
			if len(data) <= 4096 || !canSplit(err) {
				return err
			}
			out.emit(event{Action: "split", Name: name, Bytes: len(data), Reason: err.Error()})
		}
		parts, err := splitArchive(data)
		if err != nil {
			return fmt.Errorf("split %q: %w", name, err)
		}
		for i, part := range parts {
			if err := visit(splitChildName(name, i+1), part); err != nil {
				return err
			}
		}
		return nil
	}
	for i, name := range names {
		if err := visit(name, chunks[i]); err != nil {
			return err
		}
	}
	for _, source := range sources {
		if !isPartOf(source.Title, base) || active[source.Title] {
			continue
		}
		if !opts.DryRun {
			if err := c.DeleteSources(ctx, notebookID, []string{source.ID}); err != nil {
				return fmt.Errorf("delete obsolete part %q: %w", source.Title, err)
			}
			sc.remove(notebookID, source.ID)
		}
		out.emit(event{Action: "delete", Name: source.Title, OldID: source.ID, Reason: "orphan", DryRun: opts.DryRun})
	}
	return nil
}

// splitArchive splits members rather than slicing txtar syntax. A sole member
// is decoded, split, and quoted again so each child is independently readable.
func splitArchive(data []byte) ([][]byte, error) {
	ar := txtar.Parse(data)
	if len(ar.Files) == 0 {
		return nil, fmt.Errorf("archive has no files")
	}
	var files []txtar.File
	for i, f := range ar.Files {
		raw, err := restoreArchiveFile(ar, i)
		if err != nil {
			return nil, err
		}
		files = append(files, txtar.File{Name: f.Name, Data: raw})
	}
	var groups [][]txtar.File
	if len(files) > 1 {
		mid := len(files) / 2
		groups = [][]txtar.File{files[:mid], files[mid:]}
	} else {
		f := files[0]
		if len(f.Data) < 2 {
			return nil, fmt.Errorf("member cannot be split further")
		}
		cut := splitFileCut(f.Data, len(f.Data)/2)
		groups = [][]txtar.File{{{Name: f.Name + " (part 1/2)", Data: f.Data[:cut]}}, {{Name: f.Name + " (part 2/2)", Data: f.Data[cut:]}}}
	}
	var parts [][]byte
	for _, group := range groups {
		child := &txtar.Archive{}
		for i, f := range group {
			quoted := needsQuote(f.Data)
			if quoted {
				var err error
				f.Data, err = quote(f.Data)
				if err != nil {
					return nil, err
				}
			}
			child.Comment = appendFileDirectives(child.Comment, i, f.Name, f.Data, quoted)
			child.Files = append(child.Files, f)
		}
		part := txtar.Format(child)
		if len(part) >= len(data) {
			return nil, fmt.Errorf("split would not reduce upload size")
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func splitDescendant(title, parent string) bool {
	for {
		trimmed, ok := trimSplitSuffix(title)
		if !ok {
			return false
		}
		if trimmed == parent {
			return true
		}
		title = trimmed
	}
}
