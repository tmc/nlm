package api

import (
	"context"
	"fmt"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// Note operations

// CreateNote creates a note in projectID with the given title and content.
//
// The web UI does this in two steps and so does this method: CYK0Xb allocates
// an empty "New Note" shell on the server, then cYAfTb fills in the title and
// body against the new note_id. Calling only the first step leaves a literal
// "New Note" with no body in the notebook, which is why the chain lives at the
// api.Client layer — every caller (CLI, MCP, future SDK consumers) gets the
// populated note in one call.
//
// Content is sent verbatim as Markdown (the wire format the rich-text editor
// converts to on save); callers do not need to convert from HTML.
func (c *Client) CreateNote(ctx context.Context, projectID string, title string, initialContent string) (*Note, error) {
	req := &pb.CreateNoteRequest{
		ProjectId: projectID,
		Content:   proto.String(initialContent),
		NoteType:  &pb.Int32List{Value: 1},
		Title:     title,
	}
	shell, err := c.orchestrationService.CreateNote(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}
	note, err := c.MutateNote(ctx, projectID, shell.NoteId, initialContent, title)
	if err != nil {
		return nil, fmt.Errorf("create note: set title/body: %w", err)
	}
	return note, nil
}

// MutateNote replaces a note's title and content.
func (c *Client) MutateNote(ctx context.Context, projectID string, noteID string, content string, title string) (*Note, error) {
	req := &pb.MutateNoteRequest{
		ProjectId: projectID,
		NoteId:    noteID,
		Updates: &pb.NoteUpdates{Update: &pb.NoteUpdateGroup{Update: &pb.NoteUpdate{
			Content:    content,
			Title:      title,
			Tags:       &pb.NoteTags{},
			UpdateMode: proto.Int32(0),
			StateCode:  proto.Int32(0),
		}}},
	}
	note, err := c.orchestrationService.MutateNote(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("mutate note: %w", err)
	}
	return &Note{Note: note}, nil
}

// DeleteNotes deletes notes from a notebook.
func (c *Client) DeleteNotes(ctx context.Context, projectID string, noteIDs []string) error {
	req := &pb.DeleteNotesRequest{
		ProjectId: projectID,
		NoteIds:   noteIDs,
		Context:   conversationRequestContext(),
	}
	_, err := c.orchestrationService.DeleteNotes(ctx, req)
	if err != nil {
		return fmt.Errorf("delete notes: %w", err)
	}
	return nil
}

// GetNotes returns all notes in a notebook.
func (c *Client) GetNotes(ctx context.Context, projectID string) ([]*Note, error) {
	req := &pb.GetNotesRequest{ProjectId: projectID}
	response, rpcErr := c.orchestrationService.GetNotes(ctx, req)
	if rpcErr != nil {
		return nil, fmt.Errorf("get notes: %w", rpcErr)
	}
	return notesFromWireResponse(response), nil
}

// notesFromWireResponse adapts the generated response to the public Note
// slice while preserving the legacy parser's ordering and nil-item behavior.
func notesFromWireResponse(response *pb.GetNotesRichWireResponse) []*Note {
	if response == nil {
		return nil
	}
	notes := make([]*Note, 0, len(response.GetEntries()))
	for _, entry := range response.GetEntries() {
		if entry == nil {
			continue
		}
		note := noteFromRecord(entry.GetNote())
		if note == nil {
			// A keyed tombstone has an ID and a null record. Preserve the
			// legacy parser's public ID-only projection.
			if entry.GetNoteId() == "" {
				continue
			}
			note = &Note{Note: &pb.Note{NoteId: entry.GetNoteId()}}
		}
		if note.NoteId == "" {
			note.NoteId = entry.GetNoteId()
		}
		notes = append(notes, note)
	}
	return notes
}

func noteFromRecord(note *pb.GetNotesRichRecord) *Note {
	if note == nil {
		return nil
	}
	// Keep the public GetNotes projection identical to the legacy parser. The
	// wire record also carries metadata and note_type, but that parser never
	// exposed those positions.
	return &Note{
		Note: &pb.Note{
			NoteId:      note.GetNoteId(),
			ContentText: note.GetContentText(),
			Title:       note.GetTitle(),
			RichText:    note.GetRichText().GetPlainText(),
		},
		Rich:      note.GetRichText().GetDocument(),
		Grounding: note.GetGroundingDetails().GetGrounding(),
	}
}

// Audio operations

func universalArtifactRequestContext() *pb.RequestContext {
	return &pb.RequestContext{
		Version: proto.Int32(2),
		Caps: &pb.RequestClientCaps{
			Version:         proto.Int32(1),
			CapabilityCodes: []int32{1},
		},
		ArtifactTypes: &pb.RequestArtifactTypeFilter{Types: []int32{1, 4, 8, 10, 2, 3, 6, 9}},
	}
}

func universalArtifactSourceGroups(sourceIDs []string) []*pb.UniversalArtifactSourceGroup {
	groups := make([]*pb.UniversalArtifactSourceGroup, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		groups = append(groups, &pb.UniversalArtifactSourceGroup{Source: &pb.SourceIdList{SourceId: sourceID}})
	}
	return groups
}
