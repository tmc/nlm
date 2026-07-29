package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
)

func TestUpdateNotePreservesOmittedFields(t *testing.T) {
	tests := []struct {
		name        string
		title       *string
		content     *string
		notePresent bool
		wantTitle   string
		wantContent string
		wantCalls   []string
		wantErr     error
	}{
		{
			name:        "title only",
			title:       stringPointer("New title"),
			notePresent: true,
			wantTitle:   "New title",
			wantContent: "Old body",
			wantCalls:   []string{"cFji9", "cYAfTb"},
		},
		{
			name:        "content only",
			content:     stringPointer("New body"),
			notePresent: true,
			wantTitle:   "Old title",
			wantContent: "New body",
			wantCalls:   []string{"cFji9", "cYAfTb"},
		},
		{
			name:        "empty content clears",
			content:     stringPointer(""),
			notePresent: true,
			wantTitle:   "Old title",
			wantContent: "",
			wantCalls:   []string{"cFji9", "cYAfTb"},
		},
		{
			name:        "both fields skip read",
			title:       stringPointer("New title"),
			content:     stringPointer("New body"),
			notePresent: true,
			wantTitle:   "New title",
			wantContent: "New body",
			wantCalls:   []string{"cYAfTb"},
		},
		{
			name:        "absent note fails closed",
			title:       stringPointer("New title"),
			notePresent: false,
			wantCalls:   []string{"cFji9"},
			wantErr:     ErrNoteNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			var gotTitle, gotContent string
			httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				rpcID, args := noteRequest(t, req)
				calls = append(calls, rpcID)
				switch rpcID {
				case "cFji9":
					payload := `[]`
					if test.notePresent {
						payload = `[[["note-1",["note-1","Old body",null,null,"Old title","",[1]]]]]`
					}
					return noteResponse(req, rpcID, payload), nil
				case "cYAfTb":
					gotContent, gotTitle = mutateNoteFields(t, args)
					return noteResponse(req, rpcID, `[]`), nil
				default:
					t.Fatalf("unexpected RPC %q", rpcID)
					return nil, nil
				}
			})}
			client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(httpClient))

			_, err := client.UpdateNote(
				context.Background(),
				"notebook-1",
				"note-1",
				test.title,
				test.content,
			)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, test.wantCalls) {
				t.Errorf("RPC calls = %v, want %v", calls, test.wantCalls)
			}
			if test.wantErr == nil {
				if gotTitle != test.wantTitle || gotContent != test.wantContent {
					t.Errorf("mutation = title %q, content %q; want title %q, content %q",
						gotTitle, gotContent, test.wantTitle, test.wantContent)
				}
			}
		})
	}
}

func TestUpdateNoteRejectsEmptyMutation(t *testing.T) {
	calls := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected request")
	})}
	client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(httpClient))

	if _, err := client.UpdateNote(context.Background(), "notebook-1", "note-1", nil, nil); err == nil {
		t.Fatal("UpdateNote succeeded")
	}
	if calls != 0 {
		t.Errorf("RPC calls = %d, want 0", calls)
	}
}

func TestUpdateNoteExcludesArtifactBackedNotes(t *testing.T) {
	artifacts := []*pb.Artifact{{
		ArtifactId: "artifact-note",
		Title:      "Generated note",
		Type:       pb.ArtifactType_ARTIFACT_TYPE_NOTE,
		Note: &pb.ArtifactNotePreview{Config: &pb.ArtifactNoteConfig{
			Prompt: "Generated body",
		}},
	}}
	if got := mergeNotes(nil, notesFromArtifacts(artifacts)); len(got) != 1 || got[0].GetNoteId() != "artifact-note" {
		t.Fatalf("public note projection = %#v, want artifact-note", got)
	}

	var calls []string
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rpcID, _ := noteRequest(t, req)
		calls = append(calls, rpcID)
		if rpcID != "cFji9" {
			t.Fatalf("unexpected RPC %q", rpcID)
		}
		return noteResponse(req, rpcID, `[]`), nil
	})}
	client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(httpClient))

	_, err := client.UpdateNote(
		context.Background(),
		"notebook-1",
		"artifact-note",
		stringPointer("New title"),
		nil,
	)
	if !errors.Is(err, ErrNoteNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrNoteNotFound)
	}
	if want := []string{"cFji9"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("RPC calls = %v, want %v", calls, want)
	}
}

func TestUpdateNoteReadErrorDoesNotMutate(t *testing.T) {
	var calls []string
	readErr := errors.New("read failed")
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rpcID, _ := noteRequest(t, req)
		calls = append(calls, rpcID)
		if rpcID != "cFji9" {
			t.Fatalf("unexpected RPC %q", rpcID)
		}
		return nil, readErr
	})}
	client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(httpClient))

	_, err := client.UpdateNote(
		context.Background(),
		"notebook-1",
		"note-1",
		stringPointer("New title"),
		nil,
	)
	if !errors.Is(err, readErr) {
		t.Fatalf("error = %v, want %v", err, readErr)
	}
	if want := []string{"cFji9"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("RPC calls = %v, want %v", calls, want)
	}
}

func TestUpdateNoteRejectsTitleOnlyStructuredNote(t *testing.T) {
	tests := []struct {
		name string
		note *pb.GetNotesRichRecord
	}{
		{
			name: "rich document",
			note: &pb.GetNotesRichRecord{
				NoteId:      "note-1",
				ContentText: proto.String("Plain projection"),
				Title:       "Old title",
				RichText: &pb.NoteRichText{Value: &pb.NoteRichText_Document{Document: &pb.RichDocument{
					Body: &pb.SpanLayers{Blocks: []*pb.Span{{Start: proto.Int64(0), End: proto.Int64(4)}}},
				}}},
			},
		},
		{
			name: "grounding",
			note: &pb.GetNotesRichRecord{
				NoteId:      "note-1",
				ContentText: proto.String("Plain projection"),
				Title:       "Old title",
				GroundingDetails: &pb.NoteGroundingDetails{
					Grounding: []*pb.Grounding{{Score: proto.Float64(0.75)}},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := beprotojson.Marshal(&pb.GetNotesRichWireResponse{
				Entries: []*pb.GetNotesRichEntry{{NoteId: "note-1", Note: test.note}},
			})
			if err != nil {
				t.Fatal(err)
			}

			var calls []string
			httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				rpcID, _ := noteRequest(t, req)
				calls = append(calls, rpcID)
				if rpcID != "cFji9" {
					t.Fatalf("unexpected RPC %q", rpcID)
				}
				return noteResponse(req, rpcID, string(payload)), nil
			})}
			client := New(Credentials{AuthToken: "auth", Cookies: "cookie"}, WithHTTPClient(httpClient))

			_, err = client.UpdateNote(
				context.Background(),
				"notebook-1",
				"note-1",
				stringPointer("New title"),
				nil,
			)
			if !errors.Is(err, ErrRichNoteTitleUpdateUnsupported) {
				t.Fatalf("error = %v, want %v", err, ErrRichNoteTitleUpdateUnsupported)
			}
			if want := []string{"cFji9"}; !reflect.DeepEqual(calls, want) {
				t.Errorf("RPC calls = %v, want %v", calls, want)
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}

func noteRequest(t *testing.T, req *http.Request) (string, []any) {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatal(err)
	}
	var envelope any
	if err := json.Unmarshal([]byte(form.Get("f.req")), &envelope); err != nil {
		t.Fatal(err)
	}
	rpcID, args, ok := findNoteRequest(envelope)
	if !ok {
		t.Fatalf("note RPC not found in %s", form.Get("f.req"))
	}
	return rpcID, args
}

func findNoteRequest(value any) (string, []any, bool) {
	values, ok := value.([]any)
	if !ok {
		return "", nil, false
	}
	if len(values) >= 2 {
		rpcID, rpcOK := values[0].(string)
		encoded, argsOK := values[1].(string)
		if rpcOK && argsOK && (rpcID == "cFji9" || rpcID == "cYAfTb") {
			var args []any
			if json.Unmarshal([]byte(encoded), &args) == nil {
				return rpcID, args, true
			}
		}
	}
	for _, item := range values {
		if rpcID, args, ok := findNoteRequest(item); ok {
			return rpcID, args, true
		}
	}
	return "", nil, false
}

func mutateNoteFields(t *testing.T, args []any) (content, title string) {
	t.Helper()
	if len(args) < 3 {
		t.Fatalf("mutate args = %#v", args)
	}
	updates, ok := args[2].([]any)
	if !ok || len(updates) != 1 {
		t.Fatalf("updates = %#v", args[2])
	}
	group, ok := updates[0].([]any)
	if !ok || len(group) != 1 {
		t.Fatalf("update group = %#v", updates[0])
	}
	update, ok := group[0].([]any)
	if !ok || len(update) < 2 {
		t.Fatalf("update = %#v", group[0])
	}
	content, contentOK := update[0].(string)
	if update[0] == nil {
		// The generated positional encoder represents a protobuf scalar's
		// empty value as null. MutateNote still treats the update record as a
		// full replacement, so null here is the clear-body request.
		contentOK = true
		content = ""
	}
	title, titleOK := update[1].(string)
	if !contentOK || !titleOK {
		t.Fatalf("update fields = %#v", update)
	}
	return content, title
}

func noteResponse(req *http.Request, rpcID, payload string) *http.Response {
	body := fmt.Sprintf(")]}'\n\n[[\"wrb.fr\",%q,%s,null,null,null,\"generic\"]]", rpcID, payload)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}
