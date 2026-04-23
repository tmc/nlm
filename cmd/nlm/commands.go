package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
// Source IDs come from positional args OR --source-ids/--source-match;
// at least one of those must be provided.
func actOnSourcesCmd(name, action, usage string) command {
	return command{
		name:      name,
		usage:     usage,
		argsUsage: "<notebook-id> [source-id...]",
		section:   "Content Transformation",
		minArgs:   1, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			notebookID := args[0]
			sourceIDs := args[1:]
			if len(sourceIDs) == 0 {
				if sourceIDsFlag == "" && sourceMatchFlag == "" {
					return fmt.Errorf("usage: nlm %s <notebook-id> <source-id> [source-id...]"+
						" (or pass --source-ids / --source-match)", name)
				}
				resolved, err := resolveSourceSelectors(c, notebookID)
				if err != nil {
					return err
				}
				sourceIDs = resolved
			}
			return actOnSources(c, notebookID, action, sourceIDs)
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
		usage: "Show notebook analytics (experimental; wire returns activity time-series, not scalar counts — current decoder misreports)", section: "Notebook",
		minArgs: 1, maxArgs: 1,
		hidden: true, // AUrzMb returns time-series metrics; proto expects scalar counts. Needs redesign.
		run: func(c *api.Client, args []string) error {
			if !experimentalEnabled() {
				return fmt.Errorf("analytics is experimental (wire returns activity time-series, not scalar counts; current output misreports); pass --experimental or set NLM_EXPERIMENTAL=1")
			}
			return getAnalytics(c, args[0])
		},
	},
	{
		name:  "list-featured",
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
		name: "add", argsUsage: "<notebook-id> <source|-> [source...]",
		usage: "Add one or more sources (files, URLs, or text; pass '-' to read newline-delimited entries from stdin)", section: "Source",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			inputs, err := addSourceInputs(args[1:])
			if err != nil {
				return err
			}
			return addSources(c, args[0], inputs)
		},
	},
	{
		name: "sync", argsUsage: "<notebook-id> [paths...]",
		usage: "Sync files into a named source (use --force to re-upload unchanged)", section: "Source",
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
		name: "sync-pack", argsUsage: "[paths...]",
		usage:   "Preview the txtar bytes that sync would upload (offline)",
		section: "Source",
		minArgs: 0, maxArgs: -1,
		noClient: true,
		run: func(_ *api.Client, args []string) error {
			return runSyncPack(args)
		},
	},
	{
		name: "rm-source", aliases: []string{"source-rm"}, argsUsage: "<notebook-id> <source-id|-|a,b,c>",
		usage: "Remove one or more sources (pass '-' to read newline-delimited IDs from stdin)", section: "Source",
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
		name: "check-source", argsUsage: "<source-id> [notebook-id]",
		usage: "Check source freshness (Google-Drive-only; notebook-id enables client-side source-type validation)", section: "Source",
		minArgs: 1, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			notebookID := ""
			if len(args) > 1 {
				notebookID = args[1]
			}
			return checkSourceFreshness(c, args[0], notebookID)
		},
	},
	{
		name: "discover-sources", argsUsage: "<notebook-id> <query>",
		usage: "Discover relevant sources (experimental; upstream RPC deprecated, fallback may return prose)", section: "Source",
		minArgs: 2, maxArgs: 2,
		hidden: true, // upstream qXyaNe deprecated; Ljjv0c (HAR-verified 2026-04-17) is now wired via nlm research --mode=fast
		run: func(c *api.Client, args []string) error {
			if !experimentalEnabled() {
				return fmt.Errorf("discover-sources is experimental (upstream RPC deprecated, fallback may return prose instead of links); pass --experimental or set NLM_EXPERIMENTAL=1")
			}
			return discoverSources(c, args[0], args[1])
		},
	},
	{
		name: "dump-load-source", argsUsage: "<source-id> [notebook-id]",
		usage: "Print the raw JSON wire response of LoadSource (hizoJc) for a source", section: "Source",
		minArgs: 1, maxArgs: 2,
		hidden: true, // developer tool; exposes unmodeled fields (text body fragments, etc.)
		run: func(c *api.Client, args []string) error {
			nb := ""
			if len(args) == 2 {
				nb = args[1]
			}
			raw, err := c.LoadSourceRaw(args[0], nb)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(raw)
			if err == nil {
				fmt.Fprintln(os.Stdout)
			}
			return err
		},
	},
	{
		name: "read-source", argsUsage: "<source-id> [notebook-id]",
		usage: "Print the server-indexed text body of a source (native offsets preserved)", section: "Source",
		minArgs: 1, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			nb := ""
			if len(args) == 2 {
				nb = args[1]
			}
			body, err := c.LoadSourceText(args[0], nb)
			if err != nil {
				return err
			}
			if len(body.Fragments) == 0 {
				return fmt.Errorf("source %s has no text body (non-text source, or body not indexed)", args[0])
			}
			_, err = fmt.Fprint(os.Stdout, body.Full())
			return err
		},
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
		usage: "Remove a note from a notebook", section: "Note",
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
			fmt.Println(artifactID)
			fmt.Fprintf(os.Stderr, "Created slide deck. Use 'nlm artifacts %s' to check status.\n", args[0])
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
		usage: "Start interactive audio session (experimental, limited functionality)", section: "Audio",
		minArgs: 0, maxArgs: -1,
		hidden: true, // requires NLM_EXPERIMENTAL
		run: func(c *api.Client, args []string) error {
			if !experimentalEnabled() {
				return fmt.Errorf("audio-interactive is experimental (limited functionality); pass --experimental or set NLM_EXPERIMENTAL=1")
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
		name: "update-artifact", argsUsage: "<artifact-id> [new-title]",
		usage: "Rename artifact (new title from positional arg or --name)", section: "Artifact",
		minArgs: 1, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			title := sourceName // reuse --name flag
			if len(args) > 1 {
				title = args[1]
			}
			if title == "" {
				return fmt.Errorf("provide new title as second arg or --name flag")
			}
			return renameArtifact(c, args[0], title)
		},
	},
	{
		name: "rename-artifact", argsUsage: "<artifact-id> <new-title>",
		usage: "Rename artifact (alias: update-artifact)", section: "Artifact",
		minArgs: 2, maxArgs: 2,
		hidden: true, // superseded by update-artifact
		run:    func(c *api.Client, args []string) error { return renameArtifact(c, args[0], args[1]) },
	},
	{
		name: "delete-artifact", argsUsage: "<artifact-id>",
		usage: "Delete artifact", section: "Artifact",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return deleteArtifact(c, args[0]) },
	},

	// Guidebook operations
	{
		name:  "guidebooks",
		usage: "List all guidebooks", section: "Guidebook",
		minArgs: 0, maxArgs: 0,
		run: func(c *api.Client, args []string) error {
			guidebooks, err := c.ListGuidebooks()
			if err != nil {
				return err
			}
			w, flush := newListWriter(os.Stdout)
			fmt.Fprintln(w, "ID\tTITLE\tSTATUS")
			for _, gb := range guidebooks {
				fmt.Fprintf(w, "%s\t%s\t%s\n", gb.GetGuidebookId(), gb.GetTitle(), gb.GetStatus().String())
			}
			return flush()
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
		name: "source-guide", argsUsage: "<notebook-id> [source-id...]",
		usage: "Show the per-source auto-summary and keyword chips (cached on disk)", section: "Generation",
		minArgs: 1, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			notebookID := args[0]
			sourceIDs := args[1:]
			if len(sourceIDs) == 0 {
				if sourceIDsFlag == "" && sourceMatchFlag == "" {
					return fmt.Errorf("usage: nlm source-guide <notebook-id> <source-id> [source-id...]" +
						" (or pass --source-ids / --source-match)")
				}
				resolved, err := resolveSourceSelectors(c, notebookID)
				if err != nil {
					return err
				}
				sourceIDs = resolved
			}
			return generateSourceGuides(c, sourceIDs)
		},
	},
	{
		name: "generate-mindmap", argsUsage: "<notebook-id> <source-id> [source-id...]",
		usage: "Generate interactive mindmap (alias for mindmap)", section: "Generation",
		hidden: true, minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			return actOnSourcesMindmap(c, args[0], args[1:])
		},
	},
	{
		name: "generate-chat", argsUsage: "<notebook-id> <prompt>",
		usage: "Stream a one-shot chat answer (use --conversation to follow up)", section: "Generation",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			return generateFreeFormChat(c, args[0], strings.Join(args[1:], " "))
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
			for i, s := range resp.GetSuggestions() {
				if i > 0 {
					fmt.Println()
				}
				fmt.Printf("%s\n", s.GetTitle())
				if s.GetDescription() != "" {
					fmt.Printf("  %s\n", s.GetDescription())
				}
				if s.GetPrompt() != "" {
					fmt.Printf("  Prompt: %s\n", s.GetPrompt())
				}
			}
			return nil
		},
	},
	{
		name: "audio-suggestions", argsUsage: "<notebook-id>",
		usage: "Suggest audio-overview blueprints (emit JSON lines; pipe to create-audio)", section: "Generation",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			return audioSuggestions(c, args[0])
		},
	},
	{
		name: "create-report", argsUsage: "<notebook-id> <report-type> [description] [instructions]",
		usage: "Create a report artifact (run report-suggestions for valid types)", section: "Create",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			return createReport(c, args[0], args[1], args[2:])
		},
	},
	{
		name: "generate-report", argsUsage: "<notebook-id>",
		usage: "Generate multi-section report via chat (see --prompt, --sections)", section: "Generation",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			return generateReport(c, args[0])
		},
	},
	// Chat operations
	{
		name: "chat", argsUsage: "<notebook-id> [conversation-id | prompt]",
		usage: "Open interactive chat (one-shot if a prompt is given; -f <file> reads a long prompt from file)", section: "Chat",
		minArgs: 1, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			// -f/--prompt-file overrides positional prompt: reliable for long
			// prompts that terminals truncate when pasted interactively.
			if promptFile != "" {
				prompt, err := readPromptFile(promptFile)
				if err != nil {
					return fmt.Errorf("read prompt: %w", err)
				}
				if len(args) >= 2 && isConversationID(args[1]) {
					// chat <nb> <conv-id> -f prompt.txt: one-shot into an
					// existing conversation.
					return oneShotChatInConv(c, args[0], args[1], prompt)
				}
				return oneShotChat(c, args[0], prompt)
			}
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
		usage: "List chat sessions (server-side when a notebook is given)", section: "Chat",
		minArgs: 0, maxArgs: 1,
		noAuth: true, noClient: true,
		run: func(_ *api.Client, args []string) error {
			if len(args) == 1 {
				return listChatConversationsWithAuth(args[0])
			}
			return listChatSessions()
		},
	},
	{
		name: "chat-history", argsUsage: "<notebook-id> <conversation-id>",
		usage: "View conversation history", section: "Chat",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			return printChatHistory(c, args[0], args[1])
		},
	},
	{
		name: "chat-show", argsUsage: "<notebook-id> <conversation-id>",
		usage: "Render a local chat transcript (see --citations)", section: "Chat",
		minArgs: 2, maxArgs: 2,
		noAuth: true, noClient: true,
		run: func(_ *api.Client, args []string) error {
			return chatShow(args[0], args[1])
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
	{
		name: "mindmap", argsUsage: "<notebook-id> <source-id> [source-id...]",
		usage:   "Generate interactive mindmap (opens in browser)",
		section: "Content Transformation",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			return actOnSourcesMindmap(c, args[0], args[1:])
		},
	},
	actOnSourcesCmd("timeline", "timeline", "Create timeline from sources"),
	actOnSourcesCmd("toc", "table_of_contents", "Generate table of contents"),

	// Research operations
	{
		name: "research", argsUsage: "<notebook-id> \"query\"",
		usage: "Run fast or deep research (JSON-lines by default; --md for markdown; --mode=fast|deep)", section: "Research",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			query := strings.Join(args[1:], " ")
			return runResearch(c, args[0], query)
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
		name:  "mcp",
		usage: "Run the MCP server on stdin/stdout", section: "Other",
		minArgs: 0, maxArgs: 0,
		run: func(c *api.Client, args []string) error { return runMCP(c) },
	},
	{
		name: "auth", argsUsage: "[profile]",
		usage: "Set up authentication from a browser profile", section: "Other",
		minArgs: 0, maxArgs: -1,
		noAuth: true, noClient: true,
		run: func(c *api.Client, args []string) error {
			_, _, err := handleAuth(args, debug)
			return err
		},
	},
	{
		name:  "refresh",
		usage: "Refresh stored authentication credentials", section: "Other",
		minArgs: 0, maxArgs: -1,
		noAuth: true, noClient: true,
		run: func(c *api.Client, args []string) error { return refreshCredentials(debug) },
	},
	{
		name: "feedback", argsUsage: "<message>",
		usage: "Submit feedback to NotebookLM", section: "Other",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return submitFeedback(c, args[0]) },
	},
	{
		name:  "hb",
		usage: "Send a session heartbeat", section: "Other",
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

// experimentalEnabled reports whether experimental (hidden) commands should
// be surfaced. Either --experimental or NLM_EXPERIMENTAL=<non-empty> enables
// them. Keep both forms: the flag is discoverable via --help, the env var
// is ergonomic for long-running shells and MCP configs.
func experimentalEnabled() bool {
	return experimental || os.Getenv("NLM_EXPERIMENTAL") != ""
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
			if cmd.hidden && !experimentalEnabled() {
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
// errBadArgs is returned by argument-validation paths so the exit-code
// classifier maps them to exit 2 (bad-args). The message is intentionally
// generic — the per-command usage hint is printed separately to stderr.
var errBadArgs = errors.New("invalid arguments")

func validateCommandArgs(cmd *command, cmdName string, args []string) error {
	// Special case: audio-interactive has its own validation.
	if cmd.name == "audio-interactive" {
		return validateInteractiveAudioArgs(args)
	}

	n := len(args)
	if n < cmd.minArgs {
		fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n", cmdName, cmd.argsUsage)
		return errBadArgs
	}
	if cmd.maxArgs >= 0 && n > cmd.maxArgs {
		fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n", cmdName, cmd.argsUsage)
		return errBadArgs
	}
	// Content-transformation commands accept positional source IDs OR a
	// selector flag. Reject the no-source case before we reach auth so the
	// user gets a usage message rather than an auth prompt.
	if cmd.section == "Content Transformation" && n < 2 && sourceIDsFlag == "" && sourceMatchFlag == "" {
		fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n", cmdName, cmd.argsUsage)
		return errBadArgs
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
