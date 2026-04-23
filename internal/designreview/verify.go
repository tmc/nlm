// Package designreview verifies `file:line` citations in a design-review
// report against a local checkout.
//
// The model generates prose with citations like "control_socket.go:255". This
// package extracts those citations, resolves each filename against the
// reviewed repo, checks whether the line number is in range, and returns a
// verdict per citation so the caller can build a cleaned report.
package designreview

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Status is the verification outcome for a single citation.
type Status string

const (
	StatusOK         Status = "ok"          // file exists and line is in range.
	StatusFileMiss   Status = "file_miss"   // file not found in repo.
	StatusLineMiss   Status = "line_miss"   // file found but cited line is out of range.
	StatusAmbiguous  Status = "ambiguous"   // basename matches more than one file.
	StatusHeaderSpan Status = "header_span" // citation overlaps a txtar header, not a file body.
	StatusOffsetMiss Status = "offset_miss" // citation points outside the indexed source text.
)

// Citation is a single `file:line` reference extracted from the report.
type Citation struct {
	Raw        string  `json:"raw"`                  // original matched text, e.g. "control_socket.go:255".
	File       string  `json:"file"`                 // filename as written in the report.
	Line       int     `json:"line"`                 // 1-based line number from the report.
	Column     int     `json:"column,omitempty"`     // 1-based column when known; defaults to 1 for report citations.
	EndLine    int     `json:"end_line,omitempty"`   // inclusive end position when known.
	EndColumn  int     `json:"end_column,omitempty"` // inclusive end position when known.
	Match      string  `json:"match"`                // resolved path relative to Repo.Root, "" if none.
	Status     Status  `json:"status"`               // verification outcome.
	Reason     string  `json:"reason,omitempty"`
	Context    string  `json:"context"`           // the line in the report that contained the citation.
	Snippet    string  `json:"snippet,omitempty"` // quoted source text for native citations.
	SourceID   string  `json:"source_id,omitempty"`
	StartChar  int     `json:"start_char,omitempty"`
	EndChar    int     `json:"end_char,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Repo is the set of files a Verifier checks citations against.
// Populate it with Scan before calling Verify.
type Repo struct {
	Root   string              // absolute path to the repo root.
	byPath map[string]int      // rel-path → line count.
	byBase map[string][]string // basename → list of rel paths.
}

// Scan walks root and records every regular file's line count and basename
// index. Binary files and files larger than maxBytes are recorded with an
// unknown line count (0), which causes Verify to treat their line checks as
// passing — we only reject lines for files we could read.
func (r *Repo) Scan(root string, skipDir func(string) bool) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	r.Root = abs
	r.byPath = map[string]int{}
	r.byBase = map[string][]string{}
	return filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDir != nil && skipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return nil
		}
		lines := countLines(path)
		r.byPath[rel] = lines
		base := filepath.Base(rel)
		r.byBase[base] = append(r.byBase[base], rel)
		return nil
	})
}

// countLines returns the number of lines in path. Returns -1 on read error;
// callers treat negative counts as "unknown, don't reject".
func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	const bufSize = 64 * 1024
	buf := make([]byte, bufSize)
	count := 0
	trailingNewline := false
	sawBytes := false
	for {
		n, err := f.Read(buf)
		if n > 0 {
			sawBytes = true
		}
		for i := 0; i < n; i++ {
			if buf[i] == '\n' {
				count++
				trailingNewline = true
			} else {
				trailingNewline = false
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return -1
		}
	}
	// A file with content but no trailing newline still has one more line.
	if sawBytes && !trailingNewline {
		count++
	}
	return count
}

// citationRE matches `file.ext:NNN` anywhere in prose. The extension must be
// a word of 1-5 chars so URLs (http://host:80) and times (12:34) don't
// match. Backtick-fencing is stripped by the caller.
var citationRE = regexp.MustCompile(`((?:\./)?[A-Za-z0-9_][A-Za-z0-9_./\-]*\.[A-Za-z0-9]{1,5}):(\d+)`)

// Extract scans r line by line and returns every citation it finds.
// Duplicate citations (same file+line with identical context) are kept —
// the caller decides whether to dedupe.
func Extract(r io.Reader) []Citation {
	var out []Citation
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		// Trim backticks around the citation so `file:10` still matches.
		stripped := strings.ReplaceAll(line, "`", "")
		for _, m := range citationRE.FindAllStringSubmatchIndex(stripped, -1) {
			raw := stripped[m[0]:m[1]]
			file := stripped[m[2]:m[3]]
			lineStr := stripped[m[4]:m[5]]
			n, err := strconv.Atoi(lineStr)
			if err != nil || n <= 0 {
				continue
			}
			out = append(out, Citation{
				Raw:     raw,
				File:    file,
				Line:    n,
				Context: strings.TrimSpace(line),
			})
		}
	}
	return out
}

// Verify resolves each citation against repo and fills in Match/Status/Reason
// in place. It returns the same slice for chaining.
func Verify(repo *Repo, cites []Citation) []Citation {
	for i := range cites {
		c := &cites[i]
		// Direct path hit.
		if lines, ok := repo.byPath[c.File]; ok {
			c.Match = c.File
			c.Status = lineStatus(c.Line, lines)
			if c.Status != StatusOK {
				c.Reason = lineReason(c.Line, lines)
			}
			continue
		}
		// Basename match; pick the unique candidate or flag ambiguous.
		base := filepath.Base(c.File)
		candidates := repo.byBase[base]
		switch len(candidates) {
		case 0:
			c.Status = StatusFileMiss
			c.Reason = "no file named " + base + " in repo"
		case 1:
			c.Match = candidates[0]
			c.Status = lineStatus(c.Line, repo.byPath[candidates[0]])
			if c.Status != StatusOK {
				c.Reason = lineReason(c.Line, repo.byPath[candidates[0]])
			}
		default:
			sort.Strings(candidates)
			c.Status = StatusAmbiguous
			c.Reason = "basename matches " + strconv.Itoa(len(candidates)) + " files: " + strings.Join(candidates, ", ")
		}
	}
	return cites
}

func lineStatus(line, total int) Status {
	if total < 0 {
		// Unknown line count (binary, unreadable, empty). Don't reject.
		return StatusOK
	}
	if line <= total {
		return StatusOK
	}
	return StatusLineMiss
}

func lineReason(line, total int) string {
	if total < 0 {
		return ""
	}
	return "line " + strconv.Itoa(line) + " beyond EOF (" + strconv.Itoa(total) + " lines)"
}
