package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestRunPreProcess(t *testing.T) {
	t.Run("stdout replaces content", func(t *testing.T) {
		out, err := runPreProcess("tr a-z A-Z", "test", strings.NewReader("hello"))
		if err != nil {
			t.Fatalf("runPreProcess error = %v", err)
		}
		got, err := io.ReadAll(out)
		if err != nil {
			t.Fatalf("read out: %v", err)
		}
		if string(got) != "HELLO" {
			t.Fatalf("got %q, want %q", string(got), "HELLO")
		}
	})

	t.Run("non-zero exit surfaces stderr", func(t *testing.T) {
		_, err := runPreProcess("echo oops >&2; exit 3", "test", strings.NewReader("hello"))
		if err == nil {
			t.Fatalf("want error, got nil")
		}
		if !strings.Contains(err.Error(), "oops") {
			t.Fatalf("error %q does not mention stderr %q", err.Error(), "oops")
		}
		if !strings.Contains(err.Error(), "test") {
			t.Fatalf("error %q does not mention label %q", err.Error(), "test")
		}
	})

	t.Run("binary-safe passthrough", func(t *testing.T) {
		input := string([]byte{0x00, 0x01, 0x02, 0xff, 0xfe})
		out, err := runPreProcess("cat", "test", strings.NewReader(input))
		if err != nil {
			t.Fatalf("runPreProcess error = %v", err)
		}
		got, err := io.ReadAll(out)
		if err != nil {
			t.Fatalf("read out: %v", err)
		}
		if string(got) != input {
			t.Fatalf("got %x, want %x", got, input)
		}
	})
}

// TestValidateSourceInputs covers the pre-RPC fail-fast rules. The bulk
// path is value only if callers can rely on an all-or-nothing contract; a
// partial batch would mean some IDs made it to the server while the tail
// did not, which is exactly the mode that breaks shell-pipe retry.
func TestValidateSourceInputs(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(existing, []byte("hi"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	missing := filepath.Join(dir, "does-not-exist.txt")

	tests := []struct {
		name    string
		inputs  []string
		wantErr string // substring; "" means expect nil
	}{
		{"empty batch", nil, "no sources provided"},
		{"one url", []string{"https://example.com/doc"}, ""},
		{"two urls", []string{"https://a/x", "http://b/y"}, ""},
		{"existing file", []string{existing}, ""},
		{"missing file with .txt suffix", []string{missing}, "does-not-exist.txt"},
		{"missing path with slash", []string{"/nope/path"}, "/nope/path"},
		{"text literal (no separator)", []string{"hello"}, ""},
		{"mixed urls and text", []string{"https://a/x", "topic"}, ""},
		{"bare dash in batch", []string{"https://a/x", "-"}, "stdin ('-')"},
		{"empty element", []string{"a", ""}, "empty source argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSourceInputs(tt.inputs)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestAddSourceInputs_PassThrough ensures literal argument lists are
// returned as-is — including a sole "-", which addSource interprets as
// "stream stdin as one source" (not as a line-delimited list).
func TestAddSourceInputs_PassThrough(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"url and text literal", []string{"https://a/x", "topic"}, []string{"https://a/x", "topic"}},
		{"sole dash preserved for stdin blob", []string{"-"}, []string{"-"}},
		{"mixed with dash preserved (validation rejects later)", []string{"https://a/x", "-"}, []string{"https://a/x", "-"}},
		{"single file arg", []string{"./notes.md"}, []string{"./notes.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := addSourceInputs(tt.in)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("addSourceInputs(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitIntoChunks(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		chunkSize int
		want      []string
	}{
		{"empty", "", 5, nil},
		{"smaller than chunk", "abc", 5, []string{"abc"}},
		{"exact multiple", "aaaabbbb", 4, []string{"aaaa", "bbbb"}},
		{"ragged last chunk", "aaaabbbbc", 4, []string{"aaaa", "bbbb", "c"}},
		{"single byte chunks", "abc", 1, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitIntoChunks([]byte(tt.content), tt.chunkSize)
			if len(got) != len(tt.want) {
				t.Fatalf("splitIntoChunks(%q, %d) got %d parts, want %d", tt.content, tt.chunkSize, len(got), len(tt.want))
			}
			for i := range tt.want {
				if string(got[i]) != tt.want[i] {
					t.Errorf("part %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestChunkedSourceNames(t *testing.T) {
	tests := []struct {
		base string
		n    int
		want []string
	}{
		{"notes", 1, []string{"notes"}},
		{"notes", 3, []string{"notes", "notes (pt2)", "notes (pt3)"}},
		{"big.log", 2, []string{"big.log", "big.log (pt2)"}},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			got := chunkedSourceNames(tt.base, tt.n)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("chunkedSourceNames(%q, %d) = %v, want %v", tt.base, tt.n, got, tt.want)
			}
		})
	}
}

type fakeSourceReplaceClient struct {
	labels   []api.Label
	deleted  []string
	attached []struct {
		labelID  string
		sourceID string
	}
}

func (f *fakeSourceReplaceClient) GetLabels(string) ([]api.Label, error) {
	return f.labels, nil
}

func (f *fakeSourceReplaceClient) DeleteSources(_ string, ids []string) error {
	f.deleted = append(f.deleted, ids...)
	return nil
}

func (f *fakeSourceReplaceClient) AttachLabelSource(_, labelID, sourceID string) error {
	f.attached = append(f.attached, struct {
		labelID  string
		sourceID string
	}{labelID, sourceID})
	return nil
}

func TestReplaceUploadedSourceDeletesOldAndLabelsAllParts(t *testing.T) {
	fc := &fakeSourceReplaceClient{
		labels: []api.Label{
			{LabelID: "label-a", SourceIDs: []string{"old-src"}},
			{LabelID: "label-b", SourceIDs: []string{"other", "old-src"}},
			{LabelID: "label-c", SourceIDs: []string{"other"}},
		},
	}

	replaceUploadedSource(fc, "nb", "old-src", []string{"new-1", "new-2"})

	if !reflect.DeepEqual(fc.deleted, []string{"old-src"}) {
		t.Fatalf("deleted = %v, want [old-src]", fc.deleted)
	}
	want := []struct {
		labelID  string
		sourceID string
	}{
		{"label-a", "new-1"},
		{"label-b", "new-1"},
		{"label-a", "new-2"},
		{"label-b", "new-2"},
	}
	if !reflect.DeepEqual(fc.attached, want) {
		t.Fatalf("attached = %v, want %v", fc.attached, want)
	}
}

func TestStaleFailedAddSourceIDs(t *testing.T) {
	before := map[string]struct{}{
		"existing": {},
	}
	project := &pb.Project{
		Sources: []*pb.Source{
			{
				SourceId: &pb.SourceId{SourceId: "existing"},
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
			{
				SourceId: &pb.SourceId{SourceId: "new-error"},
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
			{
				SourceId: &pb.SourceId{SourceId: "new-ok"},
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ENABLED},
			},
			{
				SourceId: &pb.SourceId{SourceId: "new-error-metadata"},
				Metadata: &pb.SourceMetadata{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
		},
	}

	got := staleFailedAddSourceIDs(before, project)
	want := []string{"new-error", "new-error-metadata"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staleFailedAddSourceIDs() = %v, want %v", got, want)
	}
}
