package designreview

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Format names a supported output encoding for Write.
type Format string

const (
	FormatJSONL  Format = "jsonl"  // one JSON object per citation.
	FormatGrep   Format = "grep"   // vim quickfix / grep: file:line:col: severity: message.
	FormatSARIF  Format = "sarif"  // SARIF 2.1.0 JSON; parsed by GitHub code scanning.
	FormatGitHub Format = "github" // GitHub Actions workflow commands.
)

// KnownFormats is the ordered list of formats accepted by ParseFormat.
var KnownFormats = []Format{FormatJSONL, FormatGrep, FormatSARIF, FormatGitHub}

// ParseFormat normalizes a CLI-supplied format name.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "", "jsonl", "json":
		return FormatJSONL, nil
	case "grep", "quickfix", "vim", "compile":
		return FormatGrep, nil
	case "sarif":
		return FormatSARIF, nil
	case "github", "gha":
		return FormatGitHub, nil
	}
	return "", fmt.Errorf("unknown format %q (want one of: jsonl, grep, sarif, github)", s)
}

// Write emits cites in the requested format.
func Write(w io.Writer, format Format, repoRoot string, cites []Citation) error {
	switch format {
	case FormatJSONL:
		return writeJSONL(w, cites)
	case FormatGrep:
		return writeGrep(w, cites)
	case FormatSARIF:
		return writeSARIF(w, repoRoot, cites)
	case FormatGitHub:
		return writeGitHub(w, cites)
	}
	return fmt.Errorf("unsupported format %q", format)
}

func writeJSONL(w io.Writer, cites []Citation) error {
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	for _, c := range cites {
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// writeGrep emits `path:line:col: severity: message` lines. Path is the
// resolved Match when available so vim :copen can jump straight to the file;
// we fall back to the raw File (missing files stay in the list so the user
// sees the rejection).
func writeGrep(w io.Writer, cites []Citation) error {
	bw := bufio.NewWriter(w)
	for _, c := range cites {
		path := c.Match
		if path == "" {
			path = c.File
		}
		col := c.Column
		if col <= 0 {
			col = 1
		}
		sev := grepSeverity(c.Status)
		msg := grepMessage(c)
		fmt.Fprintf(bw, "%s:%d:%d: %s: %s\n", path, c.Line, col, sev, msg)
	}
	return bw.Flush()
}

func grepSeverity(s Status) string {
	switch s {
	case StatusOK:
		return "info"
	case StatusFileMiss, StatusLineMiss, StatusHeaderSpan, StatusOffsetMiss:
		return "error"
	case StatusAmbiguous:
		return "warning"
	}
	return "info"
}

func grepMessage(c Citation) string {
	msg := string(c.Status)
	if c.Reason != "" {
		msg += " (" + c.Reason + ")"
	}
	return msg
}

// writeGitHub emits GitHub Actions workflow commands:
//
//	::notice file=foo.go,line=10::ok
//	::error file=bar.go,line=99::line 99 beyond EOF (40 lines)
//
// GH annotates each run's log with these messages and pins them to the
// corresponding file/line in the diff view when the path is relative to the
// repo root.
func writeGitHub(w io.Writer, cites []Citation) error {
	bw := bufio.NewWriter(w)
	for _, c := range cites {
		path := c.Match
		if path == "" {
			path = c.File
		}
		level := githubLevel(c.Status)
		msg := grepMessage(c)
		col := c.Column
		if col <= 0 {
			col = 1
		}
		if c.EndLine > 0 && c.EndColumn > 0 {
			fmt.Fprintf(bw, "::%s file=%s,line=%d,col=%d,endLine=%d,endColumn=%d::%s\n",
				level, escapeGitHub(path), c.Line, col, c.EndLine, c.EndColumn, escapeGitHub(msg))
			continue
		}
		fmt.Fprintf(bw, "::%s file=%s,line=%d,col=%d::%s\n", level, escapeGitHub(path), c.Line, col, escapeGitHub(msg))
	}
	return bw.Flush()
}

func githubLevel(s Status) string {
	switch s {
	case StatusOK:
		return "notice"
	case StatusAmbiguous:
		return "warning"
	case StatusFileMiss, StatusLineMiss, StatusHeaderSpan, StatusOffsetMiss:
		return "error"
	}
	return "notice"
}

// escapeGitHub replaces characters that would break the workflow-command
// parser. See actions/toolkit's command.ts for the reference encoding.
func escapeGitHub(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// writeSARIF emits a SARIF 2.1.0 log with one result per citation. GitHub's
// code-scanning ingester accepts this shape directly (`gh code-scanning
// upload` or `actions/upload-sarif`).
func writeSARIF(w io.Writer, repoRoot string, cites []Citation) error {
	results := make([]sarifResult, 0, len(cites))
	for _, c := range cites {
		path := c.Match
		if path == "" {
			path = c.File
		}
		col := c.Column
		if col <= 0 {
			col = 1
		}
		region := sarifRegion{StartLine: c.Line, StartColumn: col}
		if c.EndLine > 0 {
			region.EndLine = c.EndLine
		}
		if c.EndColumn > 0 {
			region.EndColumn = c.EndColumn
		}
		results = append(results, sarifResult{
			RuleID:  "design-review-citation",
			Level:   sarifLevel(c.Status),
			Message: sarifMessage{Text: grepMessage(c) + ": " + c.Raw},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifact{URI: path},
					Region:           region,
				},
			}},
		})
	}
	run := sarifRun{
		Tool: sarifTool{Driver: sarifDriver{
			Name:           "nlm-design-review",
			InformationURI: "https://github.com/tmc/nlm",
			Rules: []sarifRule{{
				ID:               "design-review-citation",
				Name:             "CitationGrounding",
				ShortDescription: sarifMessage{Text: "Citation grounding check for design-review output."},
				FullDescription:  sarifMessage{Text: "Verifies that file:line citations from the review model point at real files and valid line numbers in the reviewed repo."},
			}},
		}},
		Results: results,
	}
	if repoRoot != "" {
		run.OriginalURIBase = map[string]sarifArtifact{"REPO_ROOT": {URI: repoRoot}}
	}
	log := sarifLog{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs:    []sarifRun{run},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

func sarifLevel(s Status) string {
	switch s {
	case StatusOK:
		return "note"
	case StatusAmbiguous:
		return "warning"
	case StatusFileMiss, StatusLineMiss, StatusHeaderSpan, StatusOffsetMiss:
		return "error"
	}
	return "none"
}

// SARIF types — minimal subset needed for GitHub code scanning. See
// https://docs.oasis-open.org/sarif/sarif/v2.1.0/os/sarif-v2.1.0-os.html.

type sarifLog struct {
	Schema  string     `json:"$schema,omitempty"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool            sarifTool                `json:"tool"`
	Results         []sarifResult            `json:"results"`
	OriginalURIBase map[string]sarifArtifact `json:"originalUriBaseIds,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri,omitempty"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name,omitempty"`
	ShortDescription sarifMessage `json:"shortDescription,omitempty"`
	FullDescription  sarifMessage `json:"fullDescription,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}
