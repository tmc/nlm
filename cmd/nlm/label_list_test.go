package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

var labelListFixture = []api.Label{
	{LabelID: "lbl-1", Name: "Testing", SourceIDs: []string{"src-a", "src-b"}},
	{LabelID: "lbl-2", Name: "RPC and Networking", SourceIDs: []string{"src-c"}},
	{LabelID: "lbl-3", Name: "Empty Label", SourceIDs: nil},
}

func TestRenderLabelList_TableOutput(t *testing.T) {
	tests := []struct {
		name     string
		labels   []api.Label
		tty      bool
		wantOut  []string
		wantStat []string
	}{
		{
			name:    "tty with rows",
			labels:  labelListFixture,
			tty:     true,
			wantOut: []string{"LABEL ID\tNAME\tSOURCES", "lbl-1\tTesting\t2", "lbl-2\tRPC and Networking\t1", "lbl-3\tEmpty Label\t0"},
			wantStat: []string{"Total labels: 3"},
		},
		{
			name:     "tty empty list",
			labels:   nil,
			tty:      true,
			wantStat: []string{"Total labels: 0", "No labels found. The notebook may not have run autolabel yet."},
		},
		{
			name:    "non-tty rows",
			labels:  labelListFixture,
			tty:     false,
			wantOut: []string{"LABEL ID\tNAME\tSOURCES", "lbl-1\tTesting\t2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, status bytes.Buffer
			if err := renderLabelList(&out, &status, tt.labels, tt.tty); err != nil {
				t.Fatalf("renderLabelList: %v", err)
			}
			outStr := out.String()
			statStr := status.String()
			for _, want := range tt.wantOut {
				if !strings.Contains(outStr, want) {
					t.Errorf("stdout missing %q\ngot:\n%s", want, outStr)
				}
			}
			for _, want := range tt.wantStat {
				if !strings.Contains(statStr, want) {
					t.Errorf("status missing %q\ngot:\n%s", want, statStr)
				}
			}
		})
	}
}

func TestRenderLabelList_JSON(t *testing.T) {
	prev := jsonOutput
	jsonOutput = true
	defer func() { jsonOutput = prev }()

	var out, status bytes.Buffer
	if err := renderLabelList(&out, &status, labelListFixture, false); err != nil {
		t.Fatalf("renderLabelList: %v", err)
	}
	if status.Len() != 0 {
		t.Errorf("status should be empty in JSON mode, got: %q", status.String())
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if got, want := len(lines), len(labelListFixture); got != want {
		t.Fatalf("got %d JSON lines, want %d:\n%s", got, want, out.String())
	}

	want := []labelListRecord{
		{LabelID: "lbl-1", Name: "Testing", SourceCount: 2, SourceIDs: []string{"src-a", "src-b"}},
		{LabelID: "lbl-2", Name: "RPC and Networking", SourceCount: 1, SourceIDs: []string{"src-c"}},
		{LabelID: "lbl-3", Name: "Empty Label", SourceCount: 0, SourceIDs: nil},
	}
	for i, line := range lines {
		var got labelListRecord
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d: unmarshal %q: %v", i, line, err)
		}
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("record %d: got %+v, want %+v", i, got, want[i])
		}
	}
}
