package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/tmc/nlm/internal/notebooklm/api"
	nlmsync "github.com/tmc/nlm/internal/sync"
)

// command describes a single CLI command.
type command struct {
	name      string
	aliases   []string
	usage     string // one-line description for help text
	argsUsage string // positional args hint for "usage: nlm <name> <argsUsage>"
	section   string // help section header
	minArgs   int    // minimum positional args (after command name)
	maxArgs   int    // maximum positional args; -1 = unlimited
	noAuth    bool   // true if command does not require authentication
	noClient  bool   // true if command does not need an API client (implies noAuth)
	hidden    bool   // true to hide from help text (experimental)
	run       func(c *api.Client, args []string) error
}

// actOnSourcesCmd builds a command entry for the 14 content transform commands.
func actOnSourcesCmd(name, action, usage string) command {
	return command{
		name:      name,
		usage:     usage,
		argsUsage: "<notebook-id> <source-id> [source-id...]",
		section:   "Content Transformation",
		minArgs:   2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			return actOnSources(c, args[0], action, args[1:])
		},
	}
}

// commands is the single source of truth for all CLI commands.
var commands = []command{
	// Notebook operations
	{
		name: "list", aliases: []string{"ls"},
		usage: "List all notebooks", section: "Notebook",
		minArgs: 0, maxArgs: 0,
		run: func(c *api.Client, args []string) error { return list(c) },
	},
	{
		name: "create", argsUsage: "<title>",
		usage: "Create a new notebook", section: "Notebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return create(c, args[0]) },
	},
	{
		name: "rm", argsUsage: "<id>",
		usage: "Delete a notebook", section: "Notebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return remove(c, args[0]) },
	},
	{
		name: "analytics", argsUsage: "<notebook-id>",
		usage: "Show notebook analytics", section: "Notebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return getAnalytics(c, args[0]) },
	},
	{
		name: "list-featured",
		usage: "List featured notebooks", section: "Notebook",
		minArgs: 0, maxArgs: 0,
		run: func(c *api.Client, args []string) error { return listFeaturedProjects(c) },
	},

	// Source operations
	{
		name: "sources", argsUsage: "<notebook-id>",
		usage: "List sources in notebook", section: "Source",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return listSources(c, args[0]) },
	},
	{
		name: "add", argsUsage: "<notebook-id> <file>",
		usage: "Add source to notebook", section: "Source",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			id, err := addSource(c, args[0], args[1])
			if err == nil && replaceSourceID != "" {
				fmt.Fprintf(os.Stderr, "Replacing source %s...\n", replaceSourceID)
				if delErr := c.DeleteSources(args[0], []string{replaceSourceID}); delErr != nil {
					fmt.Fprintf(os.Stderr, "warning: uploaded new source but failed to delete old: %v\n", delErr)
				}
			}
			if err == nil {
				fmt.Println(id)
			}
			return err
		},
	},
	{
		name: "sync-source", argsUsage: "<notebook-id> [paths...]",
		usage: "Sync files as a named source", section: "Source",
		minArgs: 1, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			notebookID := args[0]
			var paths []string
			if len(args) > 1 {
				if args[1] == "-" {
					paths = nil // stdin mode
				} else {
					paths = args[1:]
				}
			} else {
				paths = []string{"."}
			}
			opts := nlmsync.Options{
				MaxBytes: maxBytes,
				Name:     sourceName,
				Force:    force,
				DryRun:   dryRun,
				JSON:     jsonOutput,
			}
			adapter := &syncClientAdapter{client: c}
			return nlmsync.Run(context.Background(), adapter, notebookID, paths, opts, os.Stdout)
		},
	},
	{
		name: "rm-source", aliases: []string{"source-rm"}, argsUsage: "<notebook-id> <source-id>",
		usage: "Remove source from notebook", section: "Source",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return removeSource(c, args[0], args[1]) },
	},
	{
		name: "rename-source", argsUsage: "<source-id> <new-name>",
		usage: "Rename a source", section: "Source",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return renameSource(c, args[0], args[1]) },
	},
	{
		name: "refresh-source", argsUsage: "<notebook-id> <source-id>",
		usage: "Refresh source content", section: "Source",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return refreshSource(c, args[0], args[1]) },
	},
	{
		name: "check-source", argsUsage: "<source-id>",
		usage: "Check source freshness", section: "Source",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return checkSourceFreshness(c, args[0]) },
	},
	{
		name: "discover-sources", argsUsage: "<notebook-id> <query>",
		usage: "Discover relevant sources", section: "Source",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return discoverSources(c, args[0], args[1]) },
	},

	// Note operations
	{
		name: "notes", argsUsage: "<notebook-id>",
		usage: "List notes in notebook", section: "Note",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return listNotes(c, args[0]) },
	},
	{
		name: "read-note", argsUsage: "<notebook-id> <note-id>",
		usage: "Read full note content", section: "Note",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return readNote(c, args[0], args[1]) },
	},
	{
		name: "new-note", argsUsage: "<notebook-id> <title> [content]",
		usage: "Create new note (content via arg or stdin)", section: "Note",
		minArgs: 2, maxArgs: 3,
		run: func(c *api.Client, args []string) error {
			noteContent := ""
			if len(args) > 2 {
				noteContent = args[2]
			} else if fi, stErr := os.Stdin.Stat(); stErr == nil && fi.Mode()&os.ModeCharDevice == 0 {
				data, readErr := io.ReadAll(os.Stdin)
				if readErr != nil {
					return readErr
				}
				noteContent = string(data)
			}
			return createNote(c, args[0], args[1], noteContent)
		},
	},
	{
		name: "update-note", argsUsage: "<notebook-id> <note-id> <content> <title>",
		usage: "Edit note content and title", section: "Note",
		minArgs: 4, maxArgs: 4,
		run: func(c *api.Client, args []string) error { return updateNote(c, args[0], args[1], args[2], args[3]) },
	},
	{
		name: "rm-note", aliases: []string{"note-rm"}, argsUsage: "<notebook-id> <note-id>",
		usage: "Remove note from notebook", section: "Note",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return removeNote(c, args[0], args[1]) },
	},

	// Create operations
	{
		name: "create-audio", argsUsage: "<notebook-id> <instructions>",
		usage: "Create audio overview", section: "Create",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return createAudioOverview(c, args[0], args[1]) },
	},
	{
		name: "create-video", argsUsage: "<notebook-id> <instructions>",
		usage: "Create video overview", section: "Create",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return createVideoOverview(c, args[0], args[1]) },
	},
	{
		name: "create-slides", argsUsage: "<notebook-id> <instructions>",
		usage: "Create slide deck", section: "Create",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			instructions := strings.Join(args[1:], " ")
			artifactID, err := c.CreateSlideDeck(args[0], instructions)
			if err != nil {
				return err
			}
			fmt.Printf("Created slide deck: %s\n", artifactID)
			fmt.Fprintf(os.Stderr, "Use 'nlm artifacts %s' to check status.\n", args[0])
			return nil
		},
	},

	// Audio operations
	{
		name: "audio-list", argsUsage: "<notebook-id>",
		usage: "List audio overviews for a notebook", section: "Audio",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return listAudioOverviews(c, args[0]) },
	},
	{
		name: "audio-get", argsUsage: "<notebook-id>",
		usage: "Get audio overview details", section: "Audio",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return getAudioOverview(c, args[0]) },
	},
	{
		name: "audio-download", argsUsage: "<notebook-id> [filename]",
		usage: "Download audio file", section: "Audio",
		minArgs: 1, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			filename := ""
			if len(args) > 1 {
				filename = args[1]
			}
			return downloadAudioOverview(c, args[0], filename)
		},
	},
	{
		name: "audio-rm", argsUsage: "<notebook-id>",
		usage: "Delete audio overview", section: "Audio",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return deleteAudioOverview(c, args[0]) },
	},
	{
		name: "audio-share", argsUsage: "<notebook-id>",
		usage: "Share audio overview", section: "Audio",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return shareAudioOverview(c, args[0]) },
	},
	{
		name: "audio-interactive", argsUsage: "<notebook-id> [flags]",
		usage: "Start interactive audio session (experimental)", section: "Audio",
		minArgs: 0, maxArgs: -1,
		hidden: true, // requires NLM_EXPERIMENTAL
		run: func(c *api.Client, args []string) error {
			if os.Getenv("NLM_EXPERIMENTAL") == "" {
				return fmt.Errorf("audio-interactive is experimental; set NLM_EXPERIMENTAL=1 to enable")
			}
			opts, notebookID, err := parseInteractiveAudioArgs(args)
			if errors.Is(err, errInteractiveAudioHelp) {
				return nil
			}
			if err != nil {
				return err
			}
			return runInteractiveAudioCommand(c, notebookID, opts)
		},
	},

	// Video operations
	{
		name: "video-list", argsUsage: "<notebook-id>",
		usage: "List video overviews for a notebook", section: "Video",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return listVideoOverviews(c, args[0]) },
	},
	{
		name: "video-get", argsUsage: "<notebook-id>",
		usage: "Get video overview details", section: "Video",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			result, err := c.GetVideoOverview(args[0])
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal video overview: %w", err)
			}
			fmt.Println(string(data))
			return nil
		},
	},
	{
		name: "video-download", argsUsage: "<notebook-id> [filename]",
		usage: "Download video file", section: "Video",
		minArgs: 1, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			filename := ""
			if len(args) > 1 {
				filename = args[1]
			}
			return downloadVideoOverview(c, args[0], filename)
		},
	},

	// Artifact operations
	{
		name: "get-artifact", argsUsage: "<artifact-id>",
		usage: "Get artifact details", section: "Artifact",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return getArtifact(c, args[0]) },
	},
	{
		name: "artifacts", aliases: []string{"list-artifacts"}, argsUsage: "<notebook-id>",
		usage: "List artifacts in notebook", section: "Artifact",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return listArtifacts(c, args[0]) },
	},
	{
		name: "rename-artifact", argsUsage: "<artifact-id> <new-title>",
		usage: "Rename artifact", section: "Artifact",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return renameArtifact(c, args[0], args[1]) },
	},
	{
		name: "delete-artifact", argsUsage: "<artifact-id>",
		usage: "Delete artifact", section: "Artifact",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return deleteArtifact(c, args[0]) },
	},

	// Guidebook operations
	{
		name: "guidebooks",
		usage: "List all guidebooks", section: "Guidebook",
		minArgs: 0, maxArgs: 0,
		run: func(c *api.Client, args []string) error {
			guidebooks, err := c.ListGuidebooks()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
			fmt.Fprintln(w, "ID\tTITLE\tSTATUS")
			for _, gb := range guidebooks {
				fmt.Fprintf(w, "%s\t%s\t%s\n", gb.GetGuidebookId(), gb.GetTitle(), gb.GetStatus().String())
			}
			return w.Flush()
		},
	},
	{
		name: "guidebook", argsUsage: "<guidebook-id>",
		usage: "Get guidebook details", section: "Guidebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			gb, err := c.GetGuidebook(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Guidebook: %s\n", gb.GetTitle())
			fmt.Printf("ID: %s\n", gb.GetGuidebookId())
			fmt.Printf("Status: %s\n", gb.GetStatus().String())
			if content := gb.GetContent(); content != "" {
				fmt.Printf("\n%s\n", content)
			}
			return nil
		},
	},
	{
		name: "guidebook-details", argsUsage: "<guidebook-id>",
		usage: "Get detailed guidebook info with sections and analytics", section: "Guidebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			details, err := c.GetGuidebookDetails(args[0])
			if err != nil {
				return err
			}
			if gb := details.GetGuidebook(); gb != nil {
				fmt.Printf("Guidebook: %s\n", gb.GetTitle())
				fmt.Printf("ID: %s\n", gb.GetGuidebookId())
				fmt.Printf("Status: %s\n", gb.GetStatus().String())
			}
			if sections := details.GetSections(); len(sections) > 0 {
				fmt.Printf("\nSections (%d):\n", len(sections))
				for i, s := range sections {
					fmt.Printf("  %d. %s\n", i+1, s.GetTitle())
				}
			}
			if analytics := details.GetAnalytics(); analytics != nil {
				data, err := json.MarshalIndent(analytics, "", "  ")
				if err == nil {
					fmt.Printf("\nAnalytics:\n%s\n", string(data))
				}
			}
			return nil
		},
	},
	{
		name: "guidebook-publish", argsUsage: "<guidebook-id>",
		usage: "Publish a guidebook", section: "Guidebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			_, err := c.PublishGuidebook(args[0])
			if err == nil {
				fmt.Fprintf(os.Stderr, "Guidebook published.\n")
			}
			return err
		},
	},
	{
		name: "guidebook-share", argsUsage: "<guidebook-id>",
		usage: "Share a guidebook", section: "Guidebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			_, err := c.ShareGuidebook(args[0])
			if err == nil {
				fmt.Fprintf(os.Stderr, "Guidebook shared.\n")
			}
			return err
		},
	},
	{
		name: "guidebook-ask", argsUsage: "<guidebook-id> <question>",
		usage: "Ask a guidebook question", section: "Guidebook",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			question := strings.Join(args[1:], " ")
			resp, err := c.GuidebookAsk(args[0], question)
			if err != nil {
				return err
			}
			fmt.Println(resp.GetAnswer())
			return nil
		},
	},
	{
		name: "guidebook-rm", argsUsage: "<guidebook-id>",
		usage: "Delete a guidebook", section: "Guidebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			err := c.DeleteGuidebook(args[0])
			if err == nil {
				fmt.Fprintf(os.Stderr, "Guidebook deleted.\n")
			}
			return err
		},
	},

	// Generation operations
	{
		name: "generate-guide", argsUsage: "<notebook-id>",
		usage: "Generate notebook guide", section: "Generation",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return generateNotebookGuide(c, args[0]) },
	},
	{
		name: "generate-magic", argsUsage: "<notebook-id> <source-id> [source-id...]",
		usage: "Generate magic view from sources", section: "Generation",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error { return generateMagicView(c, args[0], args[1:]) },
	},
	{
		name: "generate-mindmap", argsUsage: "<notebook-id> <source-id> [source-id...]",
		usage: "Generate mindmap from sources", section: "Generation",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error { return generateMindmap(c, args[0], args[1:]) },
	},
	{
		name: "generate-chat", argsUsage: "<notebook-id> <prompt>",
		usage: "Free-form chat generation", section: "Generation",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			return generateFreeFormChat(c, args[0], strings.Join(args[1:], " "))
		},
	},
	{
		name: "generate-outline", argsUsage: "<notebook-id>",
		usage: "Generate outline from notebook", section: "Generation",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			resp, err := c.GenerateOutline(args[0])
			if err != nil {
				return err
			}
			fmt.Println(resp.GetContent())
			return nil
		},
	},
	{
		name: "generate-section", argsUsage: "<notebook-id>",
		usage: "Generate a specific report section", section: "Generation",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			resp, err := c.GenerateSection(args[0])
			if err != nil {
				return err
			}
			fmt.Println(resp.GetContent())
			return nil
		},
	},
	{
		name: "report-suggestions", argsUsage: "<notebook-id>",
		usage: "Suggest report topics for notebook", section: "Generation",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			resp, err := c.GenerateReportSuggestions(args[0])
			if err != nil {
				return err
			}
			for _, s := range resp.GetSuggestions() {
				fmt.Println(s)
			}
			return nil
		},
	},
	{
		name: "start-draft", argsUsage: "<notebook-id>",
		usage: "Start a draft document from notebook", section: "Generation",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			_, err := c.StartDraft(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Draft started.\n")
			return nil
		},
	},

	// Chat operations
	{
		name: "chat", argsUsage: "<notebook-id> [conversation-id | prompt]",
		usage: "Interactive chat (or one-shot with prompt)", section: "Chat",
		minArgs: 1, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			if len(args) >= 2 {
				rest := strings.Join(args[1:], " ")
				if isConversationID(rest) {
					return interactiveChatWithConv(c, args[0], rest)
				}
				return oneShotChat(c, args[0], rest)
			}
			return interactiveChat(c, args[0])
		},
	},
	{
		name: "chat-list", argsUsage: "[notebook-id]",
		usage: "List chat sessions (server-side if notebook given)", section: "Chat",
		minArgs: 0, maxArgs: 1,
		noAuth: true,
		run: func(c *api.Client, args []string) error {
			if len(args) == 1 {
				return listChatConversations(c, args[0])
			}
			return listChatSessions()
		},
	},
	{
		name: "chat-history", argsUsage: "<notebook-id> <conversation-id>",
		usage: "View conversation history", section: "Chat",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			messages, err := c.GetConversationHistory(args[0], args[1])
			if err != nil {
				return err
			}
			for _, m := range messages {
				role := "UNKNOWN"
				switch m.Role {
				case 1:
					role = "USER"
				case 2:
					role = "ASSISTANT"
				}
				fmt.Printf("[%s]\n%s\n\n", role, m.Content)
			}
			return nil
		},
	},
	{
		name: "delete-chat", argsUsage: "<notebook-id>",
		usage: "Delete server-side chat history", section: "Chat",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return deleteChatHistory(c, args[0]) },
	},
	{
		name: "chat-config", argsUsage: "<notebook-id> <setting> [value]",
		usage: "Configure chat settings", section: "Chat",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error { return setChatConfig(c, args) },
	},
	{
		name: "set-instructions", argsUsage: "<notebook-id> \"prompt\"",
		usage: "Set system instructions", section: "Chat",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			prompt := strings.Join(args[1:], " ")
			return setInstructions(c, args[0], prompt)
		},
	},
	{
		name: "get-instructions", argsUsage: "<notebook-id>",
		usage: "Show current system instructions", section: "Chat",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return getInstructions(c, args[0]) },
	},

	// Content transformation commands
	actOnSourcesCmd("rephrase", "rephrase", "Rephrase content from sources"),
	actOnSourcesCmd("expand", "expand", "Expand on content from sources"),
	actOnSourcesCmd("summarize", "summarize", "Summarize content from sources"),
	actOnSourcesCmd("critique", "critique", "Provide critique of content"),
	actOnSourcesCmd("brainstorm", "brainstorm", "Brainstorm ideas from sources"),
	actOnSourcesCmd("verify", "verify", "Verify facts in sources"),
	actOnSourcesCmd("explain", "explain", "Explain concepts from sources"),
	actOnSourcesCmd("outline", "outline", "Create outline from sources"),
	actOnSourcesCmd("study-guide", "study_guide", "Generate study guide"),
	actOnSourcesCmd("faq", "faq", "Generate FAQ from sources"),
	actOnSourcesCmd("briefing-doc", "briefing_doc", "Create briefing document"),
	actOnSourcesCmd("mindmap", "interactive_mindmap", "Generate interactive mindmap"),
	actOnSourcesCmd("timeline", "timeline", "Create timeline from sources"),
	actOnSourcesCmd("toc", "table_of_contents", "Generate table of contents"),

	// Research operations
	{
		name: "research", argsUsage: "<notebook-id> \"query\"",
		usage: "Start deep research and poll for results", section: "Research",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			query := strings.Join(args[1:], " ")
			return deepResearch(c, args[0], query)
		},
	},

	// Sharing operations
	{
		name: "share", argsUsage: "<notebook-id>",
		usage: "Share notebook publicly", section: "Sharing",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return shareNotebook(c, args[0]) },
	},
	{
		name: "share-private", argsUsage: "<notebook-id>",
		usage: "Share notebook privately", section: "Sharing",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return shareNotebookPrivate(c, args[0]) },
	},
	{
		name: "share-details", argsUsage: "<share-id>",
		usage: "Get details of shared project", section: "Sharing",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return getShareDetails(c, args[0]) },
	},

	// Other operations
	{
		name: "mcp",
		usage: "Start MCP server (stdin/stdout)", section: "Other",
		minArgs: 0, maxArgs: 0,
		run: func(c *api.Client, args []string) error { return runMCP(c) },
	},
	{
		name: "auth", argsUsage: "[profile]",
		usage: "Setup authentication", section: "Other",
		minArgs: 0, maxArgs: -1,
		noAuth: true, noClient: true,
		run: func(c *api.Client, args []string) error {
			_, _, err := handleAuth(args, debug)
			return err
		},
	},
	{
		name: "refresh",
		usage: "Refresh authentication credentials", section: "Other",
		minArgs: 0, maxArgs: -1,
		noAuth: true, noClient: true,
		run: func(c *api.Client, args []string) error { return refreshCredentials(debug) },
	},
	{
		name: "feedback", argsUsage: "<message>",
		usage: "Submit feedback", section: "Other",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return submitFeedback(c, args[0]) },
	},
	{
		name: "hb",
		usage: "Send heartbeat", section: "Other",
		minArgs: 0, maxArgs: 0,
		run: func(c *api.Client, args []string) error { return heartbeat(c) },
	},
}

// commandIndex maps command names (including aliases) to their command entry.
var commandIndex map[string]*command

func init() {
	commandIndex = make(map[string]*command, len(commands)*2)
	for i := range commands {
		cmd := &commands[i]
		commandIndex[cmd.name] = cmd
		for _, alias := range cmd.aliases {
			commandIndex[alias] = cmd
		}
	}
}

// lookupCommand returns the command for a given name or alias.
func lookupCommand(name string) (*command, bool) {
	cmd, ok := commandIndex[name]
	return cmd, ok
}

// printUsage prints the full help text derived from the command table.
func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: nlm <command> [arguments]\n\n")

	// Ordered sections matching the original help layout.
	sections := []string{
		"Notebook", "Source", "Note", "Create", "Audio", "Video",
		"Artifact", "Guidebook", "Generation", "Chat",
		"Content Transformation", "Research", "Sharing", "Other",
	}

	for _, section := range sections {
		printed := false
		for i := range commands {
			cmd := &commands[i]
			if cmd.section != section {
				continue
			}
			if cmd.hidden && os.Getenv("NLM_EXPERIMENTAL") == "" {
				continue
			}
			if !printed {
				fmt.Fprintf(os.Stderr, "%s Commands:\n", section)
				printed = true
			}
			label := cmd.name
			if len(cmd.aliases) > 0 {
				label += ", " + strings.Join(cmd.aliases, ", ")
			}
			if cmd.argsUsage != "" {
				label += " " + cmd.argsUsage
			}
			fmt.Fprintf(os.Stderr, "  %-42s %s\n", label, cmd.usage)
		}
		if printed {
			fmt.Fprintf(os.Stderr, "\n")
		}
	}
}

// validateCommandArgs checks positional argument count for a command.
// cmdName is the name the user typed (may be an alias).
func validateCommandArgs(cmd *command, cmdName string, args []string) error {
	// Special case: audio-interactive has its own validation.
	if cmd.name == "audio-interactive" {
		return validateInteractiveAudioArgs(args)
	}

	n := len(args)
	if n < cmd.minArgs {
		fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n", cmdName, cmd.argsUsage)
		return fmt.Errorf("invalid arguments")
	}
	if cmd.maxArgs >= 0 && n > cmd.maxArgs {
		fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n", cmdName, cmd.argsUsage)
		return fmt.Errorf("invalid arguments")
	}
	return nil
}

// commandTableEntries returns all command entries for testing.
func commandTableEntries() []command {
	return commands
}

// helpAliases are recognized as valid commands but handled before table lookup.
var helpAliases = map[string]bool{
	"help": true, "-h": true, "--help": true,
}
