package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// runPreProcess pipes r through `sh -c cmd` and returns the command's stdout.
// A non-zero exit propagates as an error, including stderr for diagnosis. The
// input is fully consumed before the command runs so we can report clean
// errors for the (common) case of a small source; streaming support is a
// deliberate non-goal for now.
func runPreProcess(cmd string, label string, r io.Reader) (io.Reader, error) {
	c := exec.Command("sh", "-c", cmd)
	c.Stdin = r
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, fmt.Errorf("pre-process %q for %s: %w", cmd, label, err)
		}
		return nil, fmt.Errorf("pre-process %q for %s: %w: %s", cmd, label, err, msg)
	}
	return bytes.NewReader(stdout.Bytes()), nil
}

// splitIntoChunks divides content into parts of at most chunkSize bytes each.
// The last part may be smaller. chunkSize must be > 0.
func splitIntoChunks(content []byte, chunkSize int) [][]byte {
	if len(content) == 0 {
		return nil
	}
	n := (len(content) + chunkSize - 1) / chunkSize
	parts := make([][]byte, 0, n)
	for i := 0; i < len(content); i += chunkSize {
		end := min(i+chunkSize, len(content))
		parts = append(parts, content[i:end])
	}
	return parts
}

// chunkedSourceNames returns names for n chunks: the first is base, the rest
// are "base (pt2)", "base (pt3)", ... Matches the naming scheme used by
// `nlm sync` so chunked sources from either path are visually consistent in
// the notebook.
func chunkedSourceNames(base string, n int) []string {
	names := make([]string, n)
	for i := range names {
		if i == 0 {
			names[i] = base
		} else {
			names[i] = fmt.Sprintf("%s (pt%d)", base, i+1)
		}
	}
	return names
}

// addSourceChunked uploads content as text sources in chunkSize-byte parts.
// Used when the caller passed --chunk N and the source is not a URL. Returns
// one source ID per uploaded part, in order. The first error aborts remaining
// uploads.
func addSourceChunked(c *api.Client, notebookID string, content []byte, baseName string, chunkSize int) ([]string, error) {
	parts := splitIntoChunks(content, chunkSize)
	if len(parts) == 0 {
		return nil, fmt.Errorf("nothing to upload: source is empty")
	}
	names := chunkedSourceNames(baseName, len(parts))
	ids := make([]string, 0, len(parts))
	for i, part := range parts {
		id, err := c.AddSourceFromText(notebookID, string(part), names[i])
		if err != nil {
			return ids, fmt.Errorf("upload %s (part %d/%d): %w", names[i], i+1, len(parts), err)
		}
		fmt.Fprintf(os.Stderr, "  uploaded %s (%d bytes) -> %s\n", names[i], len(part), id)
		ids = append(ids, id)
	}
	return ids, nil
}

// addSourceInputs is the identity function today: positional source args
// pass through unchanged to addSources, which dispatches per entry. The
// single-arg `-` case (stream all of stdin as one source) is handled in
// addSource itself via AddSourceFromReader, preserving the pre-bulk
// semantics where piped input became one blob rather than many lines.
//
// Users who want to add a list of sources from stdin should compose with
// xargs: `cat urls.txt | xargs nlm add <notebook-id>`.
func addSourceInputs(args []string) ([]string, error) {
	return args, nil
}

// validateSourceInputs performs fail-fast checks on every input before any
// RPC call fires. Partial success on batches would be lying — the all-or-
// nothing shape preserves shell-pipe semantics (one non-zero exit = retry
// the whole batch). Rules:
//   - "-" is only valid as the sole argument; it means "stream stdin as
//     one source." Mixing it with other args is rejected.
//   - Inputs looking like URLs (http/https) are accepted as-is; DNS/HTTP
//     failures surface from the RPC.
//   - Inputs with a path separator or that os.Stat can open are treated as
//     files and must exist + be readable.
//   - Anything else is accepted as literal text. Matches the single-arg
//     rule today: `nlm add <nb> hello` = text body "hello"; `nlm add <nb>
//     ./hello` = file (documented in command usage).
func validateSourceInputs(inputs []string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no sources provided")
	}
	if len(inputs) == 1 && inputs[0] == "-" {
		if isTerminal(os.Stdin) {
			return fmt.Errorf("refusing to read source from an interactive stdin; pipe input or pass arguments instead")
		}
		return nil
	}
	for _, in := range inputs {
		if in == "" {
			return fmt.Errorf("empty source argument")
		}
		if in == "-" {
			return fmt.Errorf("stdin ('-') may only appear as the sole source argument")
		}
		if strings.HasPrefix(in, "http://") || strings.HasPrefix(in, "https://") {
			continue
		}
		// Treat anything with a path separator as a file and require it to
		// exist. Bare tokens fall through to the text-content path.
		if strings.ContainsAny(in, "/\\") || strings.HasSuffix(in, ".txt") || strings.HasSuffix(in, ".md") || strings.HasSuffix(in, ".pdf") {
			if _, err := os.Stat(in); err != nil {
				return fmt.Errorf("source %q: %w", in, err)
			}
		}
	}
	return nil
}

// addSources processes a batch of inputs by calling the existing single-
// source dispatch per entry and emitting one ID per line to stdout. Errors
// are fail-fast: the first failure aborts remaining inputs.
//
// NOTE: the underlying wire path is per-type RPCs (text/base64/upload-url),
// not the izAoDd AddSources bulk envelope — see api.Client.AddSources for
// why. When HAR evidence lands for the bulk wire, this function becomes the
// single switch point.
func addSources(c *api.Client, notebookID string, inputs []string, opts sourceAddOptions) error {
	if err := validateSourceInputs(inputs); err != nil {
		return err
	}
	knownSourceIDs, _ := sourceIDSet(c, notebookID) // Best-effort cleanup guard.
	for _, in := range inputs {
		ids, err := addSourceEntry(c, notebookID, in, opts)
		if err != nil {
			if cleanupErr := cleanupFailedAdd(c, notebookID, knownSourceIDs); cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", cleanupErr)
			}
			return err
		}
		for _, id := range ids {
			if knownSourceIDs != nil {
				knownSourceIDs[id] = struct{}{}
			}
			fmt.Println(id)
		}
		if opts.ReplaceSourceID != "" && len(inputs) == 1 && len(ids) == 1 {
			fmt.Fprintf(os.Stderr, "Replacing source %s...\n", opts.ReplaceSourceID)
			if delErr := c.DeleteSources(notebookID, []string{opts.ReplaceSourceID}); delErr != nil {
				fmt.Fprintf(os.Stderr, "warning: uploaded new source but failed to delete old: %v\n", delErr)
			}
		}
	}
	return nil
}

// addSourceEntry dispatches a single positional input to the appropriate
// upload path and returns one or more source IDs. Non-chunked inputs return
// a single-element slice; chunked inputs return one ID per part.
func addSourceEntry(c *api.Client, notebookID, input string, opts sourceAddOptions) ([]string, error) {
	if opts.Chunk > 0 && !isURL(input) {
		content, name, err := collectChunkedInput(input, opts)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "Chunking %q into parts of %d bytes (total %d bytes)\n", name, opts.Chunk, len(content))
		return addSourceChunked(c, notebookID, content, name, opts.Chunk)
	}
	id, err := addSource(c, notebookID, input, opts)
	if err != nil {
		return nil, err
	}
	return []string{id}, nil
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// collectChunkedInput resolves the input (stdin/file/text) into a byte slice
// and a base name, applying --pre-process when set. URL inputs are excluded
// by the caller.
func collectChunkedInput(input string, opts sourceAddOptions) ([]byte, string, error) {
	var (
		content []byte
		name    string
	)
	switch input {
	case "-":
		fmt.Fprintln(os.Stderr, "Reading from stdin...")
		name = "Pasted Text"
		if opts.Name != "" {
			name = opts.Name
		}
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, "", fmt.Errorf("read stdin: %w", err)
		}
		content = b
	default:
		if _, err := os.Stat(input); err == nil {
			name = filepath.Base(input)
			if opts.Name != "" {
				name = opts.Name
			}
			b, err := os.ReadFile(input)
			if err != nil {
				return nil, "", fmt.Errorf("read %s: %w", input, err)
			}
			content = b
		} else {
			name = "Text Source"
			if opts.Name != "" {
				name = opts.Name
			}
			content = []byte(input)
		}
	}
	if opts.PreProcess != "" {
		fmt.Fprintf(os.Stderr, "Pre-processing through: %s\n", opts.PreProcess)
		piped, err := runPreProcess(opts.PreProcess, name, bytes.NewReader(content))
		if err != nil {
			return nil, "", err
		}
		content, err = io.ReadAll(piped)
		if err != nil {
			return nil, "", fmt.Errorf("read pre-process output: %w", err)
		}
	}
	return content, name, nil
}

func sourceIDSet(c *api.Client, notebookID string) (map[string]struct{}, error) {
	project, err := c.GetProject(notebookID)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]struct{}, len(project.GetSources()))
	for _, src := range project.GetSources() {
		id := src.GetSourceId().GetSourceId()
		if id == "" {
			continue
		}
		ids[id] = struct{}{}
	}
	return ids, nil
}

func cleanupFailedAdd(c *api.Client, notebookID string, knownSourceIDs map[string]struct{}) error {
	if knownSourceIDs == nil {
		return nil
	}
	project, err := c.GetProject(notebookID)
	if err != nil {
		return fmt.Errorf("could not inspect sources after failed add: %w", err)
	}
	stale := staleFailedAddSourceIDs(knownSourceIDs, project)
	if len(stale) == 0 {
		return nil
	}
	if err := c.DeleteSources(notebookID, stale); err != nil {
		return fmt.Errorf("failed add left stale error source IDs %s; remove them with `nlm rm-source %s %s`: %w",
			strings.Join(stale, ","),
			notebookID,
			strings.Join(stale, ","),
			err,
		)
	}
	if len(stale) == 1 {
		fmt.Fprintf(os.Stderr, "Removed stale failed source record: %s\n", stale[0])
		return nil
	}
	fmt.Fprintf(os.Stderr, "Removed %d stale failed source records\n", len(stale))
	return nil
}

func staleFailedAddSourceIDs(knownSourceIDs map[string]struct{}, project *pb.Project) []string {
	var stale []string
	for _, src := range project.GetSources() {
		id := src.GetSourceId().GetSourceId()
		if id == "" {
			continue
		}
		if _, ok := knownSourceIDs[id]; ok {
			continue
		}
		if !sourceHasErrorStatus(src) {
			continue
		}
		stale = append(stale, id)
	}
	return stale
}

func sourceHasErrorStatus(src *pb.Source) bool {
	return src.GetSettings().GetStatus() == pb.SourceSettings_SOURCE_STATUS_ERROR ||
		src.GetMetadata().GetStatus() == pb.SourceSettings_SOURCE_STATUS_ERROR
}
