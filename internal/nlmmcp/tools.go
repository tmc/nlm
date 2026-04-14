package nlmmcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
)

type listNotebooksInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Maximum notebooks to return (default 50, max 100)"`
	Offset int `json:"offset,omitempty" jsonschema:"Zero-based offset into the notebook list"`
}

type listSourcesInput struct {
	NotebookID string `json:"notebook_id"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum sources to return (default 50, max 100)"`
	Offset     int    `json:"offset,omitempty" jsonschema:"Zero-based offset into the source list"`
}

type listNotesInput struct {
	NotebookID string `json:"notebook_id"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum notes to return (default 50, max 100)"`
	Offset     int    `json:"offset,omitempty" jsonschema:"Zero-based offset into the note list"`
}

type createNoteInput struct {
	NotebookID string `json:"notebook_id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
}

type addSourceTextInput struct {
	NotebookID string `json:"notebook_id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
}

type deleteNoteInput struct {
	NotebookID string `json:"notebook_id"`
	NoteID     string `json:"note_id"`
}

type listArtifactsInput struct {
	NotebookID string `json:"notebook_id"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum artifacts to return (default 50, max 100)"`
	Offset     int    `json:"offset,omitempty" jsonschema:"Zero-based offset into the artifact list"`
}

type createAudioOverviewInput struct {
	NotebookID   string `json:"notebook_id"`
	Instructions string `json:"instructions,omitempty"`
}

type getAudioOverviewInput struct {
	NotebookID string `json:"notebook_id"`
}

type renameArtifactInput struct {
	ArtifactID string `json:"artifact_id"`
	NewTitle   string `json:"new_title"`
}

type shareAudioInput struct {
	NotebookID string `json:"notebook_id"`
	Public     bool   `json:"public"`
}

type createNotebookInput struct {
	Title string `json:"title"`
	Emoji string `json:"emoji,omitempty"`
}

type deleteNotebookInput struct {
	NotebookID string `json:"notebook_id"`
}

type deleteSourceInput struct {
	NotebookID string `json:"notebook_id"`
	SourceID   string `json:"source_id"`
}

type addSourceURLInput struct {
	NotebookID string `json:"notebook_id"`
	URL        string `json:"url"`
}

type generateChatInput struct {
	NotebookID string `json:"notebook_id"`
	Prompt     string `json:"prompt"`
}

type generateContentInput struct {
	NotebookID string   `json:"notebook_id"`
	SourceIDs  []string `json:"source_ids"`
}

type createVideoOverviewInput struct {
	NotebookID   string `json:"notebook_id"`
	Instructions string `json:"instructions"`
}

type createSlideDeckInput struct {
	NotebookID   string `json:"notebook_id"`
	Instructions string `json:"instructions"`
}

type readNoteInput struct {
	NotebookID string `json:"notebook_id"`
	NoteID     string `json:"note_id"`
}

type setInstructionsInput struct {
	NotebookID   string `json:"notebook_id"`
	Instructions string `json:"instructions"`
}

type getInstructionsInput struct {
	NotebookID string `json:"notebook_id"`
}

type startDeepResearchInput struct {
	NotebookID string `json:"notebook_id"`
	Query      string `json:"query"`
}

type pollDeepResearchInput struct {
	NotebookID string `json:"notebook_id"`
	ResearchID string `json:"research_id"`
}

type notebookSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Emoji     string `json:"emoji,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type sourceSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type noteSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type artifactSummary struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	State string `json:"state"`
}

type audioOverviewSummary struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	IsReady bool   `json:"is_ready"`
}

type pageResult[T any] struct {
	Items      []T  `json:"items"`
	Total      int  `json:"total"`
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	Returned   int  `json:"returned"`
	HasMore    bool `json:"has_more"`
	NextOffset int  `json:"next_offset,omitempty"`
}

func registerTools(server *mcp.Server, client *api.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notebooks",
		Description: "List recently viewed notebooks. Results are paginated; use limit and offset to page through them.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listNotebooksInput) (*mcp.CallToolResult, any, error) {
		notebooks, err := client.ListRecentlyViewedProjects()
		if err != nil {
			return errorResult(fmt.Sprintf("failed to list notebooks: %v", err)), nil, nil
		}

		out := make([]notebookSummary, 0, len(notebooks))
		for _, notebook := range notebooks {
			item := notebookSummary{
				ID:    notebook.ProjectId,
				Title: notebook.Title,
				Emoji: notebook.Emoji,
			}
			if meta := notebook.GetMetadata(); meta != nil && meta.GetCreateTime() != nil {
				item.CreatedAt = meta.GetCreateTime().AsTime().Format("2006-01-02T15:04:05Z07:00")
			}
			out = append(out, item)
		}
		return jsonResult(paginate(out, input.Limit, input.Offset)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_sources",
		Description: "List sources in a notebook. Results are paginated; use limit and offset to page through them.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listSourcesInput) (*mcp.CallToolResult, any, error) {
		project, err := client.GetProject(input.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get project: %v", err)), nil, nil
		}

		out := make([]sourceSummary, 0, len(project.Sources))
		for _, source := range project.Sources {
			var id string
			if source.SourceId != nil {
				id = source.SourceId.SourceId
			}
			out = append(out, sourceSummary{
				ID:    id,
				Title: source.Title,
			})
		}
		return jsonResult(paginate(out, input.Limit, input.Offset)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notes",
		Description: "List notes in a notebook. Results are paginated; use limit and offset to page through them.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listNotesInput) (*mcp.CallToolResult, any, error) {
		notes, err := client.GetNotes(input.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get notes: %v", err)), nil, nil
		}

		out := make([]noteSummary, 0, len(notes))
		for _, note := range notes {
			out = append(out, noteSummary{
				ID:    note.GetNoteId(),
				Title: note.GetTitle(),
			})
		}
		return jsonResult(paginate(out, input.Limit, input.Offset)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_note",
		Description: "Create a note in a notebook.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createNoteInput) (*mcp.CallToolResult, any, error) {
		note, err := client.CreateNote(input.NotebookID, input.Title, input.Content)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create note: %v", err)), nil, nil
		}

		return textResult(fmt.Sprintf("created note %q (id: %s)", note.GetTitle(), note.GetNoteId())), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_source_text",
		Description: "Add text content as a source to a notebook.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input addSourceTextInput) (*mcp.CallToolResult, any, error) {
		sourceID, err := client.AddSourceFromText(input.NotebookID, input.Content, input.Title)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to add source: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("added source %q (id: %s)", input.Title, sourceID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_note",
		Description: "Delete a note from a notebook.",
		Annotations: destructiveAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteNoteInput) (*mcp.CallToolResult, any, error) {
		if err := client.DeleteNotes(input.NotebookID, []string{input.NoteID}); err != nil {
			return errorResult(fmt.Sprintf("failed to delete note: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("deleted note %s", input.NoteID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_artifacts",
		Description: "List artifacts in a notebook. Results are paginated; use limit and offset to page through them.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input listArtifactsInput) (*mcp.CallToolResult, any, error) {
		artifacts, err := client.ListArtifacts(input.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to list artifacts: %v", err)), nil, nil
		}

		out := make([]artifactSummary, 0, len(artifacts))
		for _, artifact := range artifacts {
			out = append(out, artifactSummary{
				ID:    artifact.ArtifactId,
				Type:  artifactTypeLabel(artifact.Type),
				State: artifactStateLabel(artifact.State),
			})
		}
		return jsonResult(paginate(out, input.Limit, input.Offset)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_audio_overview",
		Description: "Create a new audio overview.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createAudioOverviewInput) (*mcp.CallToolResult, any, error) {
		result, err := client.CreateAudioOverview(input.NotebookID, input.Instructions)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create audio overview: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("started audio overview %q (id: %s)", result.Title, result.AudioID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_audio_overview",
		Description: "Get audio overview status and details.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getAudioOverviewInput) (*mcp.CallToolResult, any, error) {
		result, err := client.GetAudioOverview(input.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get audio overview: %v", err)), nil, nil
		}
		return jsonResult(audioOverviewSummary{
			ID:      result.AudioID,
			Title:   result.Title,
			IsReady: result.IsReady,
		}), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_artifact",
		Description: "Rename an artifact.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input renameArtifactInput) (*mcp.CallToolResult, any, error) {
		if _, err := client.RenameArtifact(input.ArtifactID, input.NewTitle); err != nil {
			return errorResult(fmt.Sprintf("failed to rename artifact: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("renamed artifact %s to %q", input.ArtifactID, input.NewTitle)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "share_audio",
		Description: "Share an audio overview and return its public URL when enabled.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input shareAudioInput) (*mcp.CallToolResult, any, error) {
		option := api.SharePrivate
		if input.Public {
			option = api.SharePublic
		}
		result, err := client.ShareAudio(input.NotebookID, option)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to share audio: %v", err)), nil, nil
		}
		if !result.IsPublic {
			return textResult("audio sharing disabled (private)"), nil, nil
		}
		return textResult(result.ShareURL), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_video_overview",
		Description: "Create a new video overview for a notebook.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createVideoOverviewInput) (*mcp.CallToolResult, any, error) {
		result, err := client.CreateVideoOverview(input.NotebookID, input.Instructions)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create video overview: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("started video overview (id: %s)", result.VideoID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_slide_deck",
		Description: "Create a slide deck from notebook sources.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createSlideDeckInput) (*mcp.CallToolResult, any, error) {
		artifactID, err := client.CreateSlideDeck(input.NotebookID, input.Instructions)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create slide deck: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("started slide deck creation (artifact id: %s)", artifactID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_note",
		Description: "Read a specific note by ID from a notebook. Returns the note title and content.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input readNoteInput) (*mcp.CallToolResult, any, error) {
		notes, err := client.GetNotes(input.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get notes: %v", err)), nil, nil
		}
		for _, note := range notes {
			if note.GetNoteId() == input.NoteID {
				return jsonResult(map[string]string{
					"id":      note.GetNoteId(),
					"title":   note.GetTitle(),
					"content": note.GetContentText(),
				}), nil, nil
			}
		}
		return errorResult(fmt.Sprintf("note %s not found in notebook %s", input.NoteID, input.NotebookID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_instructions",
		Description: "Set custom chat instructions (system prompt) for a notebook.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input setInstructionsInput) (*mcp.CallToolResult, any, error) {
		if err := client.SetInstructions(input.NotebookID, input.Instructions); err != nil {
			return errorResult(fmt.Sprintf("failed to set instructions: %v", err)), nil, nil
		}
		return textResult("instructions updated"), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_instructions",
		Description: "Get the current custom chat instructions (system prompt) for a notebook.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input getInstructionsInput) (*mcp.CallToolResult, any, error) {
		prompt, err := client.GetInstructions(input.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get instructions: %v", err)), nil, nil
		}
		if prompt == "" {
			return textResult("no custom instructions set"), nil, nil
		}
		return textResult(prompt), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_deep_research",
		Description: "Start a deep research session. Returns a research ID that can be used with poll_deep_research to check progress.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input startDeepResearchInput) (*mcp.CallToolResult, any, error) {
		result, err := client.StartDeepResearch(input.NotebookID, input.Query)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to start deep research: %v", err)), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "poll_deep_research",
		Description: "Poll an in-progress deep research session for results. Returns done=true with content when research is complete.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input pollDeepResearchInput) (*mcp.CallToolResult, any, error) {
		result, err := client.PollDeepResearch(input.NotebookID, input.ResearchID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to poll deep research: %v", err)), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_notebook",
		Description: "Create a new notebook.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input createNotebookInput) (*mcp.CallToolResult, any, error) {
		notebook, err := client.CreateProject(input.Title, input.Emoji)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create notebook: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("created notebook %q (id: %s)", notebook.Title, notebook.ProjectId)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_notebook",
		Description: "Delete a notebook.",
		Annotations: destructiveAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteNotebookInput) (*mcp.CallToolResult, any, error) {
		if err := client.DeleteProjects([]string{input.NotebookID}); err != nil {
			return errorResult(fmt.Sprintf("failed to delete notebook: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("deleted notebook %s", input.NotebookID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_source",
		Description: "Remove a source from a notebook.",
		Annotations: destructiveAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input deleteSourceInput) (*mcp.CallToolResult, any, error) {
		if err := client.DeleteSources(input.NotebookID, []string{input.SourceID}); err != nil {
			return errorResult(fmt.Sprintf("failed to delete source: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("deleted source %s from notebook %s", input.SourceID, input.NotebookID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_source_url",
		Description: "Add a source from a URL.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input addSourceURLInput) (*mcp.CallToolResult, any, error) {
		sourceID, err := client.AddSourceFromURL(input.NotebookID, input.URL)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to add source: %v", err)), nil, nil
		}
		return textResult(fmt.Sprintf("added source from url (id: %s)", sourceID)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "generate_chat",
		Description: "Generate a NotebookLM chat response from a prompt.",
		Annotations: mutatingAnnotations,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input generateChatInput) (*mcp.CallToolResult, any, error) {
		response, err := client.GenerateFreeFormStreamed(input.NotebookID, input.Prompt, nil)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to generate chat: %v", err)), nil, nil
		}
		if response == nil || response.Chunk == "" {
			return textResult("(no response received)"), nil, nil
		}
		return textResult(response.Chunk), nil, nil
	})

	registerGenerationTools(server, client)
}

func registerGenerationTools(server *mcp.Server, client *api.Client) {
	actions := map[string]string{
		"summarize":    "summarize",
		"briefing_doc": "briefing_doc",
		"faq":          "faq",
		"study_guide":  "study_guide",
		"rephrase":     "rephrase",
		"expand":       "expand",
		"critique":     "critique",
		"brainstorm":   "brainstorm",
		"verify":       "verify",
		"explain":      "explain",
		"outline":      "outline",
		"mindmap":      "interactive_mindmap",
		"timeline":     "timeline",
		"toc":          "table_of_contents",
	}

	for toolName, action := range actions {
		toolName := toolName
		action := action

		mcp.AddTool(server, &mcp.Tool{
			Name:        "generate_" + toolName,
			Description: fmt.Sprintf("Generate a %s from sources.", toolName),
			Annotations: mutatingAnnotations,
		}, func(ctx context.Context, req *mcp.CallToolRequest, input generateContentInput) (*mcp.CallToolResult, any, error) {
			if err := client.ActOnSources(input.NotebookID, action, input.SourceIDs); err != nil {
				return errorResult(fmt.Sprintf("failed to generate %s: %v", toolName, err)), nil, nil
			}
			return textResult(fmt.Sprintf("triggered %s generation", toolName)), nil, nil
		})
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult(fmt.Sprintf("failed to encode result: %v", err))
	}
	return textResult(string(data))
}

func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
		IsError: true,
	}
}

func paginate[T any](items []T, limit, offset int) pageResult[T] {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	total := len(items)
	if offset > total {
		offset = total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	page := pageResult[T]{
		Items:    items[offset:end],
		Total:    total,
		Offset:   offset,
		Limit:    limit,
		Returned: end - offset,
		HasMore:  end < total,
	}
	if page.HasMore {
		page.NextOffset = end
	}
	return page
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultPageLimit
	}
	if limit > maxPageLimit {
		return maxPageLimit
	}
	return limit
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func artifactTypeLabel(t pb.ArtifactType) string {
	switch t {
	case pb.ArtifactType_ARTIFACT_TYPE_UNSPECIFIED:
		return "ARTIFACT_TYPE_UNSPECIFIED"
	case pb.ArtifactType_ARTIFACT_TYPE_NOTE:
		return "ARTIFACT_TYPE_NOTE"
	case pb.ArtifactType_ARTIFACT_TYPE_AUDIO_OVERVIEW:
		return "ARTIFACT_TYPE_AUDIO_OVERVIEW"
	case pb.ArtifactType_ARTIFACT_TYPE_VIDEO_OVERVIEW:
		return "ARTIFACT_TYPE_VIDEO_OVERVIEW"
	case pb.ArtifactType_ARTIFACT_TYPE_REPORT:
		return "ARTIFACT_TYPE_REPORT"
	case pb.ArtifactType_ARTIFACT_TYPE_APP:
		return "ARTIFACT_TYPE_APP"
	case pb.ArtifactType_ARTIFACT_TYPE_8:
		return "ARTIFACT_TYPE_8"
	default:
		return fmt.Sprintf("ARTIFACT_TYPE_%d", int32(t))
	}
}

func artifactStateLabel(s pb.ArtifactState) string {
	switch s {
	case pb.ArtifactState_ARTIFACT_STATE_UNSPECIFIED:
		return "ARTIFACT_STATE_UNSPECIFIED"
	case pb.ArtifactState_ARTIFACT_STATE_CREATING:
		return "ARTIFACT_STATE_CREATING"
	case pb.ArtifactState_ARTIFACT_STATE_READY:
		return "ARTIFACT_STATE_READY"
	case pb.ArtifactState_ARTIFACT_STATE_FAILED:
		return "ARTIFACT_STATE_FAILED"
	case pb.ArtifactState_ARTIFACT_STATE_SUGGESTED:
		return "ARTIFACT_STATE_SUGGESTED"
	case pb.ArtifactState_ARTIFACT_STATE_7:
		return "ARTIFACT_STATE_7"
	case pb.ArtifactState_ARTIFACT_STATE_8:
		return "ARTIFACT_STATE_8"
	case pb.ArtifactState_ARTIFACT_STATE_9:
		return "ARTIFACT_STATE_9"
	default:
		return fmt.Sprintf("ARTIFACT_STATE_%d", int32(s))
	}
}
