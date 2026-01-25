package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// runMCPServer starts the MCP server
func runMCPServer(client *api.Client) error {
	// Create MCP server
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "nlm",
		Version: "0.1.0",
	}, nil)

	// Register tools
	registerTools(s, client)

	// Serve using Stdio
	if debug {
		fmt.Fprintf(os.Stderr, "Starting NLM MCP server on stdio...\n")
	}
	transport := &mcp.StdioTransport{}
	return s.Run(context.Background(), transport)
}

func registerTools(s *mcp.Server, client *api.Client) {
	// Tool: list_notebooks
	type ListNotebooksParams struct{}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_notebooks",
		Description: "List recently viewed notebooks",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params ListNotebooksParams) (*mcp.CallToolResult, struct{}, error) {
		notebooks, err := client.ListRecentlyViewedProjects()
		if err != nil {
			return errorResult(fmt.Sprintf("failed to list notebooks: %v", err)), struct{}{}, nil
		}

		var simpleNotebooks []map[string]interface{}
		for _, nb := range notebooks {
			simpleNotebooks = append(simpleNotebooks, map[string]interface{}{
				"id":         nb.ProjectId,
				"title":      nb.Title,
				"emoji":      nb.Emoji,
				"created_at": nb.GetMetadata().GetCreateTime().AsTime().String(),
			})
		}

		jsonBytes, _ := json.MarshalIndent(simpleNotebooks, "", "  ")
		return textResult(string(jsonBytes)), struct{}{}, nil
	})

	// Tool: list_sources
	type ListSourcesParams struct {
		NotebookID string `json:"notebook_id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_sources",
		Description: "List sources in a notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params ListSourcesParams) (*mcp.CallToolResult, struct{}, error) {
		project, err := client.GetProject(params.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get project: %v", err)), struct{}{}, nil
		}

		var sources []map[string]interface{}
		for _, src := range project.Sources {
			srcID := ""
			if src.SourceId != nil {
				srcID = src.SourceId.SourceId
			}
			sources = append(sources, map[string]interface{}{
				"id":    srcID,
				"title": src.Title,
				// src.Type might be protobuf enum, using String()
				// "type":  src.Type.String(),
			})
		}

		jsonBytes, _ := json.MarshalIndent(sources, "", "  ")
		return textResult(string(jsonBytes)), struct{}{}, nil
	})

	// Tool: list_notes
	type ListNotesParams struct {
		NotebookID string `json:"notebook_id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_notes",
		Description: "List notes in a notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params ListNotesParams) (*mcp.CallToolResult, struct{}, error) {
		notes, err := client.GetNotes(params.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get notes: %v", err)), struct{}{}, nil
		}

		var simpleNotes []map[string]interface{}
		for _, note := range notes {
			noteID := ""
			if note.SourceId != nil {
				noteID = note.SourceId.SourceId
			}
			simpleNotes = append(simpleNotes, map[string]interface{}{
				"id":    noteID,
				"title": note.Title,
				// Content is not accessed
			})
		}

		jsonBytes, _ := json.MarshalIndent(simpleNotes, "", "  ")
		return textResult(string(jsonBytes)), struct{}{}, nil
	})

	// Tool: create_note
	type CreateNoteParams struct {
		NotebookID string `json:"notebook_id"`
		Title      string `json:"title"`
		Content    string `json:"content"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_note",
		Description: "Create a new note in a notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params CreateNoteParams) (*mcp.CallToolResult, struct{}, error) {
		note, err := client.CreateNote(params.NotebookID, params.Title, params.Content)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create note: %v", err)), struct{}{}, nil
		}

		noteID := ""
		if note.SourceId != nil {
			noteID = note.SourceId.SourceId
		}

		return textResult(fmt.Sprintf("Created note %s (ID: %s)", note.Title, noteID)), struct{}{}, nil
	})

	// Tool: add_source_text
	type AddSourceTextParams struct {
		NotebookID string `json:"notebook_id"`
		Title      string `json:"title"`
		Content    string `json:"content"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_source_text",
		Description: "Add text content as a source to a notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params AddSourceTextParams) (*mcp.CallToolResult, struct{}, error) {
		sourceID, err := client.AddSourceFromText(params.NotebookID, params.Content, params.Title)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to add source: %v", err)), struct{}{}, nil
		}
		return textResult(fmt.Sprintf("Added source %s (ID: %s)", params.Title, sourceID)), struct{}{}, nil
	})

	// Tool: delete_note
	type DeleteNoteParams struct {
		NotebookID string `json:"notebook_id"`
		NoteID     string `json:"note_id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_note",
		Description: "Delete a note from a notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params DeleteNoteParams) (*mcp.CallToolResult, struct{}, error) {
		err := client.DeleteNotes(params.NotebookID, []string{params.NoteID})
		if err != nil {
			return errorResult(fmt.Sprintf("failed to delete note: %v", err)), struct{}{}, nil
		}
		return textResult(fmt.Sprintf("Deleted note %s", params.NoteID)), struct{}{}, nil
	})

	// Tool: list_artifacts
	type ListArtifactsParams struct {
		NotebookID string `json:"notebook_id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_artifacts",
		Description: "List artifacts in a notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params ListArtifactsParams) (*mcp.CallToolResult, struct{}, error) {
		artifacts, err := client.ListArtifacts(params.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get artifacts: %v", err)), struct{}{}, nil
		}

		var simpleArtifacts []map[string]interface{}
		for _, art := range artifacts {
			simpleArtifacts = append(simpleArtifacts, map[string]interface{}{
				"id":    art.ArtifactId,
				"type":  art.Type.String(),
				"state": art.State.String(),
			})
		}
		jsonBytes, _ := json.MarshalIndent(simpleArtifacts, "", "  ")
		return textResult(string(jsonBytes)), struct{}{}, nil
	})

	// Tool: create_audio_overview
	type CreateAudioOverviewParams struct {
		NotebookID   string `json:"notebook_id"`
		Instructions string `json:"instructions,omitempty"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_audio_overview",
		Description: "Create a new audio overview generation",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params CreateAudioOverviewParams) (*mcp.CallToolResult, struct{}, error) {
		result, err := client.CreateAudioOverview(params.NotebookID, params.Instructions)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create audio overview: %v", err)), struct{}{}, nil
		}

		msg := fmt.Sprintf("Audio Overview creation started. ID: %s, Title: %s. Use get_audio_overview to check status.", result.AudioID, result.Title)
		return textResult(msg), struct{}{}, nil
	})

	// Tool: get_audio_overview
	type GetAudioOverviewParams struct {
		NotebookID string `json:"notebook_id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_audio_overview",
		Description: "Get audio overview status and details",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetAudioOverviewParams) (*mcp.CallToolResult, struct{}, error) {
		result, err := client.GetAudioOverview(params.NotebookID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get audio overview: %v", err)), struct{}{}, nil
		}

		info := map[string]interface{}{
			"id":       result.AudioID,
			"title":    result.Title,
			"is_ready": result.IsReady,
			// Exclude binary data
		}

		jsonBytes, _ := json.MarshalIndent(info, "", "  ")
		return textResult(string(jsonBytes)), struct{}{}, nil
	})

	// Tool: rename_artifact
	type RenameArtifactParams struct {
		ArtifactID string `json:"artifact_id"`
		NewTitle   string `json:"new_title"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "rename_artifact",
		Description: "Rename an artifact",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params RenameArtifactParams) (*mcp.CallToolResult, struct{}, error) {
		_, err := client.RenameArtifact(params.ArtifactID, params.NewTitle)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to rename artifact: %v", err)), struct{}{}, nil
		}
		return textResult(fmt.Sprintf("Renamed artifact %s to %s", params.ArtifactID, params.NewTitle)), struct{}{}, nil
	})

	// Tool: share_audio
	type ShareAudioParams struct {
		NotebookID string `json:"notebook_id"`
		Public     bool   `json:"public"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "share_audio",
		Description: "Share audio overview (get public URL)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params ShareAudioParams) (*mcp.CallToolResult, struct{}, error) {
		option := api.SharePrivate
		if params.Public {
			option = api.SharePublic
		}
		res, err := client.ShareAudio(params.NotebookID, option)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to share audio: %v", err)), struct{}{}, nil
		}
		if res.IsPublic {
			return textResult(fmt.Sprintf("Audio shared publicly: %s", res.ShareURL)), struct{}{}, nil
		}
		return textResult("Audio sharing disabled (private)"), struct{}{}, nil
	})

	// Tool: create_notebook
	type CreateNotebookParams struct {
		Title string `json:"title"`
		Emoji string `json:"emoji,omitempty"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_notebook",
		Description: "Create a new notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params CreateNotebookParams) (*mcp.CallToolResult, struct{}, error) {
		nb, err := client.CreateProject(params.Title, params.Emoji)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create notebook: %v", err)), struct{}{}, nil
		}
		return textResult(fmt.Sprintf("Created notebook %s (ID: %s)", nb.Title, nb.ProjectId)), struct{}{}, nil
	})

	// Tool: delete_notebook
	type DeleteNotebookParams struct {
		NotebookID string `json:"notebook_id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_notebook",
		Description: "Delete a notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params DeleteNotebookParams) (*mcp.CallToolResult, struct{}, error) {
		err := client.DeleteProjects([]string{params.NotebookID})
		if err != nil {
			return errorResult(fmt.Sprintf("failed to delete notebook: %v", err)), struct{}{}, nil
		}
		return textResult(fmt.Sprintf("Deleted notebook %s", params.NotebookID)), struct{}{}, nil
	})

	// Tool: delete_source
	type DeleteSourceParams struct {
		NotebookID string `json:"notebook_id"`
		SourceID   string `json:"source_id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_source",
		Description: "Remove a source from a notebook",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params DeleteSourceParams) (*mcp.CallToolResult, struct{}, error) {
		err := client.DeleteSources(params.NotebookID, []string{params.SourceID})
		if err != nil {
			return errorResult(fmt.Sprintf("failed to delete source: %v", err)), struct{}{}, nil
		}
		return textResult(fmt.Sprintf("Deleted source %s from notebook %s", params.SourceID, params.NotebookID)), struct{}{}, nil
	})

	// Tool: add_source_url
	type AddSourceURLParams struct {
		NotebookID string `json:"notebook_id"`
		URL        string `json:"url"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_source_url",
		Description: "Add a source from a URL",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params AddSourceURLParams) (*mcp.CallToolResult, struct{}, error) {
		id, err := client.AddSourceFromURL(params.NotebookID, params.URL)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to add source: %v", err)), struct{}{}, nil
		}

		return textResult(fmt.Sprintf("Added source from URL. ID: %s", id)), struct{}{}, nil
	})

	// Tool: generate_chat
	type GenerateChatParams struct {
		NotebookID string `json:"notebook_id"`
		Prompt     string `json:"prompt"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_chat",
		Description: "Generate a chat response from NotebookLM based on a prompt",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GenerateChatParams) (*mcp.CallToolResult, struct{}, error) {
		response, err := client.GenerateFreeFormStreamed(params.NotebookID, params.Prompt, nil)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to generate chat: %v", err)), struct{}{}, nil
		}

		if response != nil && response.Chunk != "" {
			return textResult(response.Chunk), struct{}{}, nil
		}
		return textResult("(No response received)"), struct{}{}, nil

	})

	// Generation Tools
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
		toolName := toolName // capture loop variable
		action := action     // capture loop variable

		type GenParams struct {
			NotebookID string   `json:"notebook_id"`
			SourceIDs  []string `json:"source_ids"`
		}

		mcp.AddTool(s, &mcp.Tool{
			Name:        "generate_" + toolName,
			Description: fmt.Sprintf("Generate a %s from sources", toolName),
		}, func(ctx context.Context, req *mcp.CallToolRequest, params GenParams) (*mcp.CallToolResult, struct{}, error) {
			err := client.ActOnSources(params.NotebookID, action, params.SourceIDs)
			if err != nil {
				return errorResult(fmt.Sprintf("failed to generate %s: %v", toolName, err)), struct{}{}, nil
			}
			return textResult(fmt.Sprintf("Successfully triggered %s generation. Check your notes for the result.", toolName)), struct{}{}, nil
		})
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
	}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: "Error: " + msg,
			},
		},
		IsError: true,
	}
}
