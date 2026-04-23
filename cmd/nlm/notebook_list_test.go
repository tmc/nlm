package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestParseNotebookListArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    notebookListOptions
		wantErr string
	}{
		{
			name: "default",
			want: notebookListOptions{Limit: -1},
		},
		{
			name: "all",
			args: []string{"--all"},
			want: notebookListOptions{All: true, Limit: -1},
		},
		{
			name: "limit",
			args: []string{"--limit", "25"},
			want: notebookListOptions{Limit: 25},
		},
		{
			name:    "unexpected positional",
			args:    []string{"extra"},
			wantErr: "unexpected argument: extra",
		},
		{
			name:    "zero limit",
			args:    []string{"--limit", "0"},
			wantErr: "--limit must be greater than 0",
		},
		{
			name:    "negative limit",
			args:    []string{"--limit", "-2"},
			wantErr: "--limit must be greater than 0",
		},
		{
			name:    "all and limit",
			args:    []string{"--all", "--limit", "5"},
			wantErr: "--all and --limit cannot be used together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNotebookListArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseNotebookListArgs(%q) error = %v, want substring %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseNotebookListArgs(%q) error = %v", tt.args, err)
			}
			if got != tt.want {
				t.Fatalf("parseNotebookListArgs(%q) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestNotebookListLimit(t *testing.T) {
	tests := []struct {
		name  string
		total int
		opts  notebookListOptions
		tty   bool
		want  int
	}{
		{name: "tty default cap", total: 12, opts: notebookListOptions{Limit: -1}, tty: true, want: 10},
		{name: "tty default small set", total: 3, opts: notebookListOptions{Limit: -1}, tty: true, want: 3},
		{name: "tty all", total: 12, opts: notebookListOptions{All: true, Limit: -1}, tty: true, want: 12},
		{name: "tty limit", total: 12, opts: notebookListOptions{Limit: 5}, tty: true, want: 5},
		{name: "pipe default", total: 12, opts: notebookListOptions{Limit: -1}, tty: false, want: 12},
		{name: "pipe limit", total: 12, opts: notebookListOptions{Limit: 4}, tty: false, want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notebookListLimit(tt.total, tt.opts, tt.tty); got != tt.want {
				t.Fatalf("notebookListLimit(%d, %+v, %v) = %d, want %d", tt.total, tt.opts, tt.tty, got, tt.want)
			}
		})
	}
}

func TestRenderNotebookList(t *testing.T) {
	notebooks := makeNotebookFixtures(t, 12)

	tests := []struct {
		name          string
		opts          notebookListOptions
		tty           bool
		wantRows      int
		wantStatusSub string
	}{
		{
			name:          "tty default cap",
			opts:          notebookListOptions{Limit: -1},
			tty:           true,
			wantRows:      10,
			wantStatusSub: "showing first 10",
		},
		{
			name:          "tty all",
			opts:          notebookListOptions{All: true, Limit: -1},
			tty:           true,
			wantRows:      12,
			wantStatusSub: "showing all",
		},
		{
			name:          "pipe limit",
			opts:          notebookListOptions{Limit: 4},
			tty:           false,
			wantRows:      4,
			wantStatusSub: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			var status bytes.Buffer
			if err := renderNotebookList(&out, &status, notebooks, tt.opts, tt.tty); err != nil {
				t.Fatalf("renderNotebookList() error = %v", err)
			}

			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if got, want := len(lines), tt.wantRows+1; got != want {
				t.Fatalf("renderNotebookList() wrote %d lines, want %d\n%s", got, want, out.String())
			}
			if !strings.Contains(lines[0], "ID\tTITLE\tSOURCES\tLAST UPDATED") {
				t.Fatalf("renderNotebookList() header = %q, want tab header", lines[0])
			}
			if tt.wantStatusSub == "" {
				if status.Len() != 0 {
					t.Fatalf("status output = %q, want empty", status.String())
				}
				return
			}
			if !strings.Contains(status.String(), tt.wantStatusSub) {
				t.Fatalf("status output = %q, want substring %q", status.String(), tt.wantStatusSub)
			}
		})
	}
}

func makeNotebookFixtures(t *testing.T, n int) []*api.Notebook {
	t.Helper()

	notebooks := make([]*api.Notebook, 0, n)
	base := time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC)
	for i := 0; i < n; i++ {
		notebooks = append(notebooks, &api.Notebook{
			ProjectId: fmt.Sprintf("nb-%02d", i),
			Title:     fmt.Sprintf("Notebook %02d", i),
			Sources: []*pb.Source{
				{},
				{},
			},
			Metadata: &pb.ProjectMetadata{
				ModifiedTime: timestamppb.New(base.Add(time.Duration(i) * time.Hour)),
			},
		})
	}
	return notebooks
}
