package designreview

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func sampleCites() []Citation {
	return []Citation{
		{Raw: "foo.go:10", File: "foo.go", Line: 10, Match: "pkg/foo.go", Status: StatusOK},
		{Raw: "gone.go:3", File: "gone.go", Line: 3, Match: "", Status: StatusFileMiss, Reason: "no file named gone.go in repo"},
		{Raw: "bar.go:9999", File: "bar.go", Line: 9999, Match: "bar.go", Status: StatusLineMiss, Reason: "line 9999 beyond EOF (40 lines)"},
		{Raw: "util.go:1", File: "util.go", Line: 1, Match: "", Status: StatusAmbiguous, Reason: "basename matches 2 files: a/util.go, b/util.go"},
	}
}

func TestParseFormat(t *testing.T) {
	cases := []struct {
		in   string
		want Format
		err  bool
	}{
		{"", FormatJSONL, false},
		{"jsonl", FormatJSONL, false},
		{"JSON", FormatJSONL, false},
		{"grep", FormatGrep, false},
		{"quickfix", FormatGrep, false},
		{"sarif", FormatSARIF, false},
		{"github", FormatGitHub, false},
		{"yaml", "", true},
	}
	for _, c := range cases {
		got, err := ParseFormat(c.in)
		if (err != nil) != c.err {
			t.Errorf("ParseFormat(%q) err=%v want err=%v", c.in, err, c.err)
			continue
		}
		if !c.err && got != c.want {
			t.Errorf("ParseFormat(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWriteGrep(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatGrep, "/repo", sampleCites()); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := []string{
		"pkg/foo.go:10:1: info: ok\n",
		"gone.go:3:1: error: file_miss (no file named gone.go in repo)\n",
		"bar.go:9999:1: error: line_miss (line 9999 beyond EOF (40 lines))\n",
		"util.go:1:1: warning: ambiguous (basename matches 2 files: a/util.go, b/util.go)\n",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("grep output missing %q\nfull:\n%s", w, got)
		}
	}
	// Each line must have exactly three colons before the first space (path:line:col:).
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		head, _, ok := strings.Cut(line, " ")
		if !ok {
			t.Errorf("line missing space separator: %q", line)
			continue
		}
		if strings.Count(head, ":") != 3 {
			t.Errorf("line header %q: want exactly 3 colons (path:line:col:)", head)
		}
	}
}

func TestWriteGitHub(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatGitHub, "/repo", sampleCites()); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := []string{
		"::notice file=pkg/foo.go,line=10,col=1::ok",
		"::error file=gone.go,line=3,col=1::file_miss (no file named gone.go in repo)",
		"::error file=bar.go,line=9999,col=1::",
		"::warning file=util.go,line=1,col=1::ambiguous",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("github output missing %q\nfull:\n%s", w, got)
		}
	}
}

func TestWriteGitHub_EscapesCarriageNewline(t *testing.T) {
	var buf bytes.Buffer
	cites := []Citation{{
		Raw: "x.go:1", File: "x.go", Line: 1, Match: "x.go",
		Status: StatusLineMiss,
		Reason: "multi\nline\rmessage with % percent",
	}}
	if err := Write(&buf, FormatGitHub, "/repo", cites); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "\nline") || strings.Contains(got, "\rmessage") {
		t.Errorf("unescaped newline/CR in output: %q", got)
	}
	if !strings.Contains(got, "%25 percent") {
		t.Errorf("expected %% escaped as %%25: %q", got)
	}
	if !strings.Contains(got, "%0A") || !strings.Contains(got, "%0D") {
		t.Errorf("expected %%0A / %%0D escapes: %q", got)
	}
}

func TestWriteSARIF(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatSARIF, "/repo", sampleCites()); err != nil {
		t.Fatal(err)
	}

	// Parse back — must be valid JSON and match the minimal shape GH code
	// scanning expects.
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID  string `json:"ruleId"`
				Level   string `json:"level"`
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine   int `json:"startLine"`
							StartColumn int `json:"startColumn"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("sarif not valid json: %v\n%s", err, buf.String())
	}
	if log.Version != "2.1.0" {
		t.Errorf("version = %q, want 2.1.0", log.Version)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(log.Runs))
	}
	run := log.Runs[0]
	if run.Tool.Driver.Name != "nlm-design-review" {
		t.Errorf("tool name = %q", run.Tool.Driver.Name)
	}
	if len(run.Results) != 4 {
		t.Fatalf("results = %d, want 4", len(run.Results))
	}
	wantLevels := []string{"note", "error", "error", "warning"}
	for i, lvl := range wantLevels {
		if run.Results[i].Level != lvl {
			t.Errorf("results[%d].level = %q, want %q", i, run.Results[i].Level, lvl)
		}
		if run.Results[i].Locations[0].PhysicalLocation.Region.StartLine == 0 {
			t.Errorf("results[%d] missing startLine", i)
		}
	}
}

func TestWriteJSONL_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, FormatJSONL, "/repo", sampleCites()); err != nil {
		t.Fatal(err)
	}
	var got []Citation
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		var c Citation
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("line not valid json: %v (%q)", err, line)
		}
		got = append(got, c)
	}
	if len(got) != 4 {
		t.Fatalf("got %d cites, want 4", len(got))
	}
	if got[0].Status != StatusOK || got[1].Status != StatusFileMiss {
		t.Errorf("round-tripped statuses wrong: %+v", got)
	}
}
