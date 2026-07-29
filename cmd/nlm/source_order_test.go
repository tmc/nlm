package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/notebooklm"
)

type sourceMembershipStub struct {
	notebooks map[string]*notebooklm.Notebook
	errors    map[string]error
	calls     []string
}

func (s *sourceMembershipStub) GetProject(_ context.Context, notebookID string) (*notebooklm.Notebook, error) {
	s.calls = append(s.calls, notebookID)
	if err := s.errors[notebookID]; err != nil {
		return nil, err
	}
	if notebook := s.notebooks[notebookID]; notebook != nil {
		return notebook, nil
	}
	return &notebooklm.Notebook{}, nil
}

func sourceNotebook(sourceIDs ...string) *notebooklm.Notebook {
	notebook := &notebooklm.Notebook{}
	for _, sourceID := range sourceIDs {
		notebook.Sources = append(notebook.Sources, &pb.Source{
			SourceId: &pb.SourceId{SourceId: sourceID},
		})
	}
	return notebook
}

func notebookLookupError(notebookID string, errorType batchexecute.ErrorType, status int) error {
	return &notebooklm.NotebookAccessError{
		NotebookID: notebookID,
		Err: &batchexecute.APIError{
			ErrorCode: &batchexecute.ErrorCode{
				Code:    5,
				Type:    errorType,
				Message: errorType.String(),
			},
			HTTPStatus: status,
		},
	}
}

func TestResolveSourceCommand(t *testing.T) {
	tests := []struct {
		name        string
		client      *sourceMembershipStub
		first       string
		second      string
		want        sourceCommandResolution
		wantCalls   []string
		wantErr     error
		wantErrText string
	}{
		{
			name: "documented order",
			client: &sourceMembershipStub{notebooks: map[string]*notebooklm.Notebook{
				"notebook-1": sourceNotebook("source-1"),
				"source-1":   sourceNotebook("notebook-1"),
			}},
			first:     "notebook-1",
			second:    "source-1",
			want:      sourceCommandResolution{NotebookID: "notebook-1", SourceID: "source-1"},
			wantCalls: []string{"notebook-1"},
		},
		{
			name: "old order",
			client: &sourceMembershipStub{notebooks: map[string]*notebooklm.Notebook{
				"source-1":   sourceNotebook(),
				"notebook-1": sourceNotebook("source-1"),
			}},
			first:     "source-1",
			second:    "notebook-1",
			want:      sourceCommandResolution{NotebookID: "notebook-1", SourceID: "source-1", Reversed: true},
			wantCalls: []string{"source-1", "notebook-1"},
		},
		{
			name: "first notebook not found",
			client: &sourceMembershipStub{
				notebooks: map[string]*notebooklm.Notebook{"notebook-1": sourceNotebook("source-1")},
				errors: map[string]error{
					"source-1": notebookLookupError("source-1", batchexecute.ErrorTypeNotFound, http.StatusNotFound),
				},
			},
			first:     "source-1",
			second:    "notebook-1",
			want:      sourceCommandResolution{NotebookID: "notebook-1", SourceID: "source-1", Reversed: true},
			wantCalls: []string{"source-1", "notebook-1"},
		},
		{
			name: "first notebook HTTP 404",
			client: &sourceMembershipStub{
				notebooks: map[string]*notebooklm.Notebook{"notebook-1": sourceNotebook("source-1")},
				errors: map[string]error{
					"source-1": &notebooklm.NotebookAccessError{
						NotebookID: "source-1",
						Err:        &batchexecute.APIError{HTTPStatus: http.StatusNotFound, Message: "not found"},
					},
				},
			},
			first:     "source-1",
			second:    "notebook-1",
			want:      sourceCommandResolution{NotebookID: "notebook-1", SourceID: "source-1", Reversed: true},
			wantCalls: []string{"source-1", "notebook-1"},
		},
		{
			name: "neither relation",
			client: &sourceMembershipStub{notebooks: map[string]*notebooklm.Notebook{
				"first":  sourceNotebook(),
				"second": sourceNotebook(),
			}},
			first:     "first",
			second:    "second",
			wantCalls: []string{"first", "second"},
			wantErr:   errSourceRelationNotFound,
		},
		{
			name: "reverse hard error",
			client: &sourceMembershipStub{
				notebooks: map[string]*notebooklm.Notebook{"first": sourceNotebook()},
				errors:    map[string]error{"second": errors.New("transport failed")},
			},
			first:       "first",
			second:      "second",
			wantCalls:   []string{"first", "second"},
			wantErrText: "list sources in notebook second: transport failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveSourceCommand(context.Background(), test.client, test.first, test.second)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}
			} else if test.wantErrText != "" {
				if err == nil || err.Error() != test.wantErrText {
					t.Fatalf("error = %v, want %q", err, test.wantErrText)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			got.Member = nil
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("resolution = %+v, want %+v", got, test.want)
			}
			if !reflect.DeepEqual(test.client.calls, test.wantCalls) {
				t.Errorf("calls = %v, want %v", test.client.calls, test.wantCalls)
			}
		})
	}
}

func TestResolveSourceCommandHardErrorDoesNotSwap(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "authentication",
			err: &batchexecute.APIError{ErrorCode: &batchexecute.ErrorCode{
				Code: 16, Type: batchexecute.ErrorTypeAuthentication, Message: "Unauthenticated",
			}},
		},
		{
			name: "permission",
			err:  notebookLookupError("first", batchexecute.ErrorTypePermissionDenied, http.StatusForbidden),
		},
		{name: "transport", err: errors.New("connection reset")},
		{
			name: "server",
			err: &batchexecute.APIError{ErrorCode: &batchexecute.ErrorCode{
				Code: 13, Type: batchexecute.ErrorTypeServerError, Message: "Internal",
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &sourceMembershipStub{errors: map[string]error{"first": test.err}}
			_, err := resolveSourceCommand(context.Background(), client, "first", "second")
			if err == nil {
				t.Fatal("resolve succeeded")
			}
			if got, want := client.calls, []string{"first"}; !reflect.DeepEqual(got, want) {
				t.Errorf("calls = %v, want %v", got, want)
			}
		})
	}
}

func TestRunSourceCommandWarningsAndNoRetry(t *testing.T) {
	tests := []struct {
		name        string
		client      *sourceMembershipStub
		target      sourceCommandTarget
		runErr      error
		wantCalls   []string
		wantOutput  string
		wantRuns    int
		wantErr     error
		wantErrText string
	}{
		{
			name:   "single argument grace",
			client: &sourceMembershipStub{},
			target: sourceCommandTarget{
				Path: "source read", SourceID: "source-1", Grace: true,
			},
			runErr:      errors.New("load source failed"),
			wantOutput:  "nlm: 'source read source-1' is deprecated; use 'read-source source-1'\n",
			wantRuns:    1,
			wantErrText: "load source failed",
		},
		{
			name:   "single check argument grace",
			client: &sourceMembershipStub{},
			target: sourceCommandTarget{
				Path: "source check", SourceID: "source-1", Grace: true,
			},
			runErr:      errors.New("check source failed"),
			wantOutput:  "nlm: 'source check source-1' is deprecated; use 'check-source source-1'\n",
			wantRuns:    1,
			wantErrText: "check source failed",
		},
		{
			name: "documented order",
			client: &sourceMembershipStub{notebooks: map[string]*notebooklm.Notebook{
				"notebook-1": sourceNotebook("source-1"),
			}},
			target: sourceCommandTarget{
				Path: "source read", NotebookID: "notebook-1", SourceID: "source-1", Resolve: true,
			},
			runErr:      errors.New("load source failed"),
			wantCalls:   []string{"notebook-1"},
			wantRuns:    1,
			wantErrText: "load source failed",
		},
		{
			name: "old read order",
			client: &sourceMembershipStub{notebooks: map[string]*notebooklm.Notebook{
				"source-1":   sourceNotebook(),
				"notebook-1": sourceNotebook("source-1"),
			}},
			target: sourceCommandTarget{
				Path: "source read", NotebookID: "source-1", SourceID: "notebook-1", Resolve: true,
			},
			runErr:    errors.New("load source failed"),
			wantCalls: []string{"source-1", "notebook-1"},
			wantOutput: "nlm: 'source read source-1 notebook-1' uses deprecated SOURCE NOTEBOOK order; " +
				"use 'source read notebook-1 source-1'\n",
			wantRuns:    1,
			wantErrText: "load source failed",
		},
		{
			name: "old check order",
			client: &sourceMembershipStub{notebooks: map[string]*notebooklm.Notebook{
				"source-1":   sourceNotebook(),
				"notebook-1": sourceNotebook("source-1"),
			}},
			target: sourceCommandTarget{
				Path: "source check", NotebookID: "source-1", SourceID: "notebook-1", Resolve: true,
			},
			runErr:    errors.New("check source failed"),
			wantCalls: []string{"source-1", "notebook-1"},
			wantOutput: "nlm: 'source check source-1 notebook-1' uses deprecated SOURCE NOTEBOOK order; " +
				"use 'source check notebook-1 source-1'\n",
			wantRuns:    1,
			wantErrText: "check source failed",
		},
		{
			name: "neither order",
			client: &sourceMembershipStub{notebooks: map[string]*notebooklm.Notebook{
				"first":  sourceNotebook(),
				"second": sourceNotebook(),
			}},
			target: sourceCommandTarget{
				Path: "source check", NotebookID: "first", SourceID: "second", Resolve: true,
			},
			wantCalls:   []string{"first", "second"},
			wantOutput:  "usage: nlm source check <notebook-id> <source-id>\n",
			wantErr:     errBadArgs,
			wantErrText: "invalid arguments: neither argument order identifies a source in a notebook",
		},
		{
			name: "membership hard error",
			client: &sourceMembershipStub{errors: map[string]error{
				"notebook-1": errors.New("connection reset"),
			}},
			target: sourceCommandTarget{
				Path: "source read", NotebookID: "notebook-1", SourceID: "source-1", Resolve: true,
			},
			wantCalls:   []string{"notebook-1"},
			wantErrText: "list sources in notebook notebook-1: connection reset",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			runs := 0
			err := runSourceCommand(context.Background(), test.client, &output, test.target, func(sourceCommandResolution) error {
				runs++
				return test.runErr
			})
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if test.wantErrText != "" && (err == nil || err.Error() != test.wantErrText) {
				t.Fatalf("error = %v, want %q", err, test.wantErrText)
			}
			if runs != test.wantRuns {
				t.Errorf("runs = %d, want %d", runs, test.wantRuns)
			}
			if got := output.String(); got != test.wantOutput {
				t.Errorf("output = %q, want %q", got, test.wantOutput)
			}
			if !reflect.DeepEqual(test.client.calls, test.wantCalls) {
				t.Errorf("calls = %v, want %v", test.client.calls, test.wantCalls)
			}
		})
	}
}

func TestParseSourceCommandTargets(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		stablePath string
		args       []string
		want       sourceCommandTarget
	}{
		{
			name:       "stable check",
			path:       "source check",
			stablePath: "source check",
			args:       []string{"notebook-1", "source-1"},
			want: sourceCommandTarget{
				Path: "source check", NotebookID: "notebook-1", SourceID: "source-1", Resolve: true,
			},
		},
		{
			name:       "stable check grace",
			path:       "source check",
			stablePath: "source check",
			args:       []string{"source-1"},
			want: sourceCommandTarget{
				Path: "source check", SourceID: "source-1", Grace: true,
			},
		},
		{
			name:       "legacy check",
			path:       "check-source",
			stablePath: "source check",
			args:       []string{"source-1", "notebook-1"},
			want: sourceCommandTarget{
				Path: "check-source", SourceID: "source-1", NotebookID: "notebook-1",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd, ok := lookupCommand(test.path)
			if !ok {
				t.Fatalf("%s command not found", test.path)
			}
			parsed, err := parseBoundCommand(cmd, test.path, test.args, globalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeSourceCommandTarget(parsed, test.stablePath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("target = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestLegacySourceCommandsDoNotResolveOrWarn(t *testing.T) {
	tests := []struct {
		path string
		args []string
	}{
		{path: "read-source", args: []string{"source-1", "notebook-1"}},
		{path: "check-source", args: []string{"source-1", "notebook-1"}},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			cmd, ok := lookupCommand(test.path)
			if !ok {
				t.Fatalf("%s command not found", test.path)
			}
			parsed, err := parseBoundCommand(cmd, test.path, test.args, globalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			stablePath := "source read"
			if test.path == "check-source" {
				stablePath = "source check"
			}
			target, err := decodeSourceCommandTarget(parsed, stablePath)
			if err != nil {
				t.Fatal(err)
			}
			if target.Resolve || target.Grace {
				t.Errorf("legacy target = %+v", target)
			}
			client := &sourceMembershipStub{}
			var output bytes.Buffer
			runs := 0
			if err := runSourceCommand(context.Background(), client, &output, target, func(sourceCommandResolution) error {
				runs++
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if runs != 1 || len(client.calls) != 0 || output.Len() != 0 {
				t.Errorf("runs=%d calls=%v output=%q", runs, client.calls, output.String())
			}
		})
	}
}

func TestAssertDriveSourceType(t *testing.T) {
	tests := []struct {
		name       string
		sourceType pb.SourceType
		wantErr    bool
	}{
		{name: "docs", sourceType: pb.SourceType_SOURCE_TYPE_GOOGLE_DOCS},
		{name: "slides", sourceType: pb.SourceType_SOURCE_TYPE_GOOGLE_SLIDES},
		{name: "sheets", sourceType: pb.SourceType_SOURCE_TYPE_GOOGLE_SHEETS},
		{name: "non-drive", sourceType: pb.SourceType_SOURCE_TYPE_LOCAL_FILE, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceType := test.sourceType
			source := &pb.Source{
				SourceId: &pb.SourceId{SourceId: "source-1"},
				Metadata: &pb.SourceMetadata{SourceType: &sourceType},
			}
			err := assertDriveSourceType(source)
			if test.wantErr {
				if !errors.Is(err, errPrecondition) {
					t.Fatalf("error = %v, want precondition", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
