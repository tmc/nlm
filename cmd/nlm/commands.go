package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type commandSurface int

const (
	surfaceStable commandSurface = iota
	surfaceExperimental
	surfaceInternal
	surfaceCompatibility
)

// commandDefinition is the temporary Phase 1 behavior scaffold. commandSpec is
// the registry authority; definitions retain the old help and handler fields
// only until each family moves onto typed calls.
type commandDefinition struct {
	name                string
	aliases             []string
	usage               string // one-line description for help text
	argsUsage           string // positional args hint for "usage: nlm <name> <argsUsage>"
	section             string // help section header
	surface             commandSurface
	minArgs             int  // minimum positional args (after command name)
	maxArgs             int  // maximum positional args; -1 = unlimited
	noAuth              bool // true if command does not require authentication
	noClient            bool // true if command does not need an API client (implies noAuth)
	directRPC           bool // true if the command requires direct RPC mode
	hidden              bool // true to hide from help text (experimental)
	validate            func(cmdName string, args []string) error
	validateWithOptions func(cmdName string, args []string, opts globalOptions) error
	help                func(cmdName string)
	run                 func(c *api.Client, args []string) error
	runWithOptions      func(c *api.Client, args []string, opts globalOptions) error
}

// command is one bound surface of a commandSpec. The embedded definition is a
// shared pointer, so stable and compatibility surfaces do not clone handlers.
type command struct {
	*commandDefinition
	spec        *commandSpec
	surfaceSpec *commandSurfaceSpec
	name        string
	aliases     []string
	section     string
	surface     commandSurface
	hidden      bool
}

// commandDefinitions retain Phase 0 behavior while commandSpecs take over the
// registry family by family.
var commandDefinitions = []commandDefinition{
	// Notebook operations
	{
		name: "list", aliases: []string{"ls"},
		usage: "List all notebooks", section: "Notebook",
		argsUsage: "[flags]",
		minArgs:   0, maxArgs: -1,
		validateWithOptions: validateNotebookListArgsWithOptions,
		help:                printNotebookListUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runNotebookListWithOptions(c, args, opts)
		},
	},
	{
		name: "create", argsUsage: "<title>",
		usage: "Create a new notebook", section: "Notebook",
	},
	{
		name: "rm", argsUsage: "<id>",
		usage: "Delete a notebook", section: "Notebook",
	},
	{
		name: "rename-notebook", argsUsage: "<notebook-id> <new-title>",
		usage: "Rename a notebook", section: "Notebook",
	},
	{
		name: "notebook-emoji", argsUsage: "<notebook-id> <emoji>",
		usage: "Change notebook emoji", section: "Notebook",
	},
	{
		name: "notebook-description", aliases: []string{"notebook-notes"},
		argsUsage: "<notebook-id> [text]",
		usage:     "Set notebook description / creator notes (text via arg or stdin; empty clears)", section: "Notebook",
	},
	{
		name: "notebook-cover", argsUsage: "<notebook-id> <preset-id>",
		usage: "Pick a built-in cover image (preset ID; HAR-captured value: 4. Other IDs uncatalogued)", section: "Notebook",
	},
	{
		name: "notebook-cover-image", argsUsage: "<notebook-id> <image-path>",
		usage: "Upload a custom cover image and associate it with the notebook", section: "Notebook",
	},
	{
		name: "notebook-unrecent", argsUsage: "<notebook-id>",
		usage: "Remove a notebook from the recently-viewed list (does not delete it)", section: "Notebook",
	},
	{
		name: "analytics", argsUsage: "<notebook-id>",
		usage: "Show notebook analytics time series", section: "Notebook",
	},
	{
		name:  "list-featured",
		usage: "List featured notebooks", section: "Notebook",
	},

	// Source operations
	{
		name: "sources", argsUsage: "<notebook-id>",
		usage: "List sources in notebook", section: "Source",
	},
	{
		name: "add", argsUsage: "<notebook-id> <source|-> [source...]",
		usage: "Add one or more sources (files, URLs, or text; pass '-' to stream stdin as a single source)", section: "Source",
		minArgs: 2, maxArgs: -1,
		validateWithOptions: validateSourceAddArgsWithOptions,
		help:                printSourceAddUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runSourceAddWithOptions(c, args, opts)
		},
	},
	{
		name: "sync", argsUsage: "<notebook-id> [paths...]",
		usage: "Bundle local files into a txtar source and keep it in sync (auto-chunks at 5MB; see --help)", section: "Source",
		minArgs: 1, maxArgs: -1,
		hidden:              true, // top-level shortcut for `source sync`; kept first-class but de-duplicated from help
		validateWithOptions: validateSourceSyncArgsWithOptions,
		help:                printSourceSyncUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runSourceSyncWithOptions(c, args, opts)
		},
	},
	{
		name: "sync-pack", argsUsage: "[paths...]",
		usage:   "Preview the txtar bytes that sync would upload (offline)",
		section: "Source",
		minArgs: 0, maxArgs: -1,
		hidden:              true, // top-level shortcut for `source pack`; kept first-class but de-duplicated from help
		noClient:            true,
		validateWithOptions: validateSourcePackArgsWithOptions,
		help:                printSourcePackUsage,
		runWithOptions: func(_ *api.Client, args []string, opts globalOptions) error {
			return runSourcePackWithOptions(args, opts)
		},
	},
	{
		name: "rm-source", aliases: []string{"source-rm"}, argsUsage: "<notebook-id> <source-id|-|a,b,c>",
		usage: "Remove one or more sources (pass '-' to read newline-delimited IDs from stdin)", section: "Source",
	},
	{
		name: "rename-source", argsUsage: "<source-id> <new-name>",
		usage: "Rename a source", section: "Source",
	},
	{
		name: "refresh-source", argsUsage: "<notebook-id> <source-id>",
		usage: "Refresh source content", section: "Source",
	},
	{
		name: "check-source", argsUsage: "<source-id> [notebook-id]",
		usage: "Check source freshness (Google-Drive-only; notebook-id enables client-side source-type validation)", section: "Source",
	},
	{
		name: "discover-sources", argsUsage: "<notebook-id> <query>",
		usage: "Discover relevant sources via Es3dTe (chat fallback if the server rejects)", section: "Source",
	},
	{
		name: "dump-load-source", argsUsage: "<source-id> [notebook-id]",
		usage: "Print the raw JSON wire response of LoadSource (hizoJc) for a source", section: "Source",
		hidden: true, // developer tool; exposes unmodeled fields (text body fragments, etc.)
	},
	{
		name: "read-source", argsUsage: "[--format text|markdown|html|json|raw] <source-id> [notebook-id]",
		usage: "Read a source body", section: "Source",
		minArgs: 0, maxArgs: -1,
		validateWithOptions: validateSourceReadArgsWithOptions,
		help:                printSourceReadUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runSourceRead(c, args, opts)
		},
	},

	// Note operations
	{
		name: "notes", argsUsage: "<notebook-id>",
		usage: "List notes in notebook", section: "Note",
	},
	{
		name: "read-note", argsUsage: "[--format text|markdown|html] [--out file] [--open] <notebook-id> <note-id>",
		usage: "Read full note content", section: "Note",
		minArgs: 0, maxArgs: -1,
		validate: validateNoteReadArgs,
		help:     printNoteReadUsage,
		runWithOptions: func(c *api.Client, args []string, _ globalOptions) error {
			return runNoteRead(c, args)
		},
	},

	{
		name: "new-note", argsUsage: "<notebook-id> <title> [content]",
		usage: "Create new note (content via arg or stdin)", section: "Note",
	},
	{
		name: "update-note", argsUsage: "<notebook-id> <note-id> <content> <title>",
		usage: "Edit note content and title", section: "Note",
	},
	{
		name: "rm-note", aliases: []string{"note-rm"}, argsUsage: "<notebook-id> <note-id>",
		usage: "Remove a note from a notebook", section: "Note",
	},

	// Label operations
	{
		name: "label-list", aliases: []string{"labels"},
		argsUsage: "<notebook-id>",
		usage:     "List labels (autolabel clusters) in a notebook", section: "Label",
		help: printLabelListUsage,
	},
	{
		name: "label-generate", aliases: []string{"autolabel"},
		argsUsage: "<notebook-id>",
		usage:     "Recompute autolabel clusters for a notebook", section: "Label",
		help: printLabelGenerateUsage,
	},
	{
		name:      "label-create",
		argsUsage: "<notebook-id> <name> [emoji]",
		usage:     "Create a new manual label on a notebook", section: "Label",
		help: printLabelCreateUsage,
	},
	{
		name:      "label-rename",
		argsUsage: "<notebook-id> <label-id> <new-name>",
		usage:     "Rename an existing label", section: "Label",
		help: printLabelRenameUsage,
	},
	{
		name:      "label-emoji",
		argsUsage: "<notebook-id> <label-id> <emoji>",
		usage:     "Set or clear the emoji on a label", section: "Label",
		help: printLabelEmojiUsage,
	},
	{
		name:      "label-delete",
		argsUsage: "<notebook-id> <label-id> [<label-id>...]",
		usage:     "Delete one or more labels by ID", section: "Label",
		help: printLabelDeleteUsage,
	},
	{
		name:      "label-unlabeled",
		argsUsage: "<notebook-id>",
		usage:     "Apply existing labels to currently-unlabeled sources", section: "Label",
		help: printLabelUnlabeledUsage,
	},
	{
		name:      "label-relabel-all",
		argsUsage: "<notebook-id>",
		usage:     "Re-cluster everything (UI's \"Relabel all\")", section: "Label",
		help: printLabelRelabelAllUsage,
	},
	{
		name:      "label-attach",
		argsUsage: "<notebook-id> <label-id> <source-id>",
		usage:     "Attach a source to a label (single source per call)", section: "Label",
		help: printLabelAttachUsage,
	},

	// Create operations
	{
		name: "create-audio", argsUsage: "<notebook-id> <instructions>",
		usage: "Create audio overview", section: "Create",
		minArgs: 2, maxArgs: -1,
		validateWithOptions: validateAudioCreateArgsWithOptions,
		help:                printAudioCreateUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			createOpts, positional, err := parseAudioCreateArgs(args)
			if err != nil {
				return err
			}
			return createAudioOverviewWithOptions(c, positional[0], strings.Join(positional[1:], " "), createOpts)
		},
	},
	{
		name: "create-video", argsUsage: "<notebook-id> <instructions>",
		usage: "Create video overview", section: "Create",
		minArgs: 2, maxArgs: -1,
		validateWithOptions: validateVideoCreateArgsWithOptions,
		help:                printVideoCreateUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			createOpts, positional, err := parseVideoCreateArgs(args)
			if err != nil {
				return err
			}
			return createVideoOverviewWithOptions(c, positional[0], strings.Join(positional[1:], " "), createOpts)
		},
	},
	{
		name: "app-create", argsUsage: "--type <prototype|mindmap|canvas> <notebook-id> [instructions]",
		usage: "Create a generated app artifact", section: "Create",
		minArgs: 1, maxArgs: -1,
		validateWithOptions: validateAppCreateArgsWithOptions,
		help:                printAppCreateUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runAppCreateWithOptions(c, args, opts)
		},
	},
	{
		name: "mindmap-create", argsUsage: "<notebook-id> [instructions]",
		usage: "Create a generated mind map artifact", section: "Create",
		minArgs: 1, maxArgs: -1,
		validateWithOptions: func(cmdName string, args []string, opts globalOptions) error {
			return validateAppCreateArgsWithOptions(cmdName, append([]string{"--type", "mindmap"}, args...), opts)
		},
		help: printAppCreateUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runMindmapCreateWithOptions(c, args, opts)
		},
	},
	{
		name: "create-slides", argsUsage: "[--format detailed|presenter] [selectors] <notebook-id> [instructions]",
		usage: "Create slide deck", section: "Create",
		minArgs: 1, maxArgs: -1,
		validateWithOptions: validateSlidesCreateArgsWithOptions,
		help:                printSlidesCreateUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runSlidesCreateWithOptions(c, args, opts)
		},
	},
	{
		name:      "deck-download",
		argsUsage: "<notebook-id> --id <artifact-id> [--format pdf|pptx] [--output file]",
		usage:     "Download a slide deck (PDF/PPTX)", section: "Deck",
		minArgs: 1, maxArgs: -1,
		hidden:              true,
		validateWithOptions: validateDeckDownloadArgsWithOptions,
		help:                printDeckDownloadUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runDeckDownload(c, args)
		},
	},
	{
		name:      "download slide-deck",
		argsUsage: "<notebook-id> --id <artifact-id> [--format pdf|pptx] [--output file]",
		usage:     "Download a slide deck (PDF/PPTX)", section: "Deck",
		minArgs: 1, maxArgs: -1,
		hidden:              true,
		validateWithOptions: validateDeckDownloadArgsWithOptions,
		help:                printDeckDownloadUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runDeckDownload(c, args)
		},
	},

	// Audio operations
	{
		name: "audio-list", argsUsage: "<notebook-id>",
		usage: "List audio overviews for a notebook", section: "Audio",
	},
	{
		name: "audio-get", argsUsage: "<notebook-id>",
		usage: "Get audio overview details", section: "Audio",
	},
	{
		name: "audio-download", argsUsage: "<notebook-id> [filename]",
		usage: "Download audio file", section: "Audio",
	},
	{
		name: "audio-rm", argsUsage: "<notebook-id>",
		usage: "Delete audio overview", section: "Audio",
	},
	{
		name: "audio-share", argsUsage: "<notebook-id>",
		usage: "Share audio overview", section: "Audio",
	},

	// Artifact operations
	{
		name: "get-artifact", argsUsage: "<artifact-id>",
		usage: "Get artifact details", section: "Artifact",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error { return getArtifact(c, args[0]) },
	},
	{
		name: "read-artifact", argsUsage: "<artifact-id>",
		usage: "Print a text artifact", section: "Artifact",
		minArgs: 1, maxArgs: 1,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return readArtifact(c, args[0], opts)
		},
	},
	{
		name:      "export-flashcards",
		argsUsage: "<artifact-id> [--format format] [--output file]",
		usage:     "Export an artifact", section: "Artifact",
		minArgs: 1, maxArgs: -1,
		hidden:              true,
		validateWithOptions: validateArtifactExportArgsWithOptions,
		help:                printArtifactExportUsage,
		runWithOptions: func(c *api.Client, args []string, _ globalOptions) error {
			return runArtifactExport(c, args)
		},
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
		validateWithOptions: validateUpdateArtifactArgsWithOptions,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runUpdateArtifactWithOptions(c, args, opts)
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
			guidebooks, err := c.ListGuidebooks(context.Background())
			if err != nil {
				return err
			}
			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				for _, gb := range guidebooks {
					rec := guidebookListRecord{
						GuidebookID: gb.GetGuidebookId(),
						Title:       gb.GetTitle(),
						Status:      gb.GetStatus().String(),
					}
					if err := enc.Encode(rec); err != nil {
						return err
					}
				}
				return nil
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
			gb, err := c.GetGuidebook(context.Background(), args[0])
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
			details, err := c.GetGuidebookDetails(context.Background(), args[0])
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
			_, err := c.PublishGuidebook(context.Background(), args[0])
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
			_, err := c.ShareGuidebook(context.Background(), args[0])
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
			resp, err := c.GuidebookAsk(context.Background(), args[0], question)
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
			err := c.DeleteGuidebook(context.Background(), args[0])
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
		validateWithOptions: validateSourceSelectionArgsWithOptions,
		help:                printSourceSelectionUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runSourceGuideWithOptions(c, args, opts)
		},
	},
	{
		name: "generate-chat", argsUsage: "<notebook-id> <prompt>",
		usage: "Stream a one-shot chat answer (use --conversation to follow up)", section: "Generation",
		minArgs: 2, maxArgs: -1,
		validateWithOptions: validateGenerateChatArgsWithOptions,
		help:                printGenerateChatUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runGenerateChatWithOptions(c, args, opts)
		},
	},
	{
		name: "report-suggestions", argsUsage: "<notebook-id>",
		usage: "Suggest report topics for notebook", section: "Generation",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			resp, err := c.GenerateReportSuggestions(context.Background(), args[0])
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
		validateWithOptions: validateCreateReportArgsWithOptions,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runCreateReportWithOptions(c, args, opts)
		},
	},
	{
		name: "generate-report", argsUsage: "<notebook-id>",
		usage: "Generate multi-section report via chat (see --prompt, --sections)", section: "Generation",
		minArgs: 1, maxArgs: 1,
		validateWithOptions: validateGenerateReportArgsWithOptions,
		help:                printGenerateReportUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runGenerateReportWithOptions(c, args, opts)
		},
	},
	// Chat operations
	{
		name: "chat", argsUsage: "<notebook-id> [conversation-id | prompt]",
		usage: "Open interactive chat (one-shot if a prompt is given; -f <file> reads a long prompt from file)", section: "Chat",
		minArgs: 1, maxArgs: -1,
		validateWithOptions: validateChatArgsWithOptions,
		help:                printChatUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runChatWithOptions(c, args, opts)
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
		name: "chat-show", argsUsage: "<notebook-id> [conversation-id]",
		usage: "Render a local chat transcript (see --citations)", section: "Chat",
		minArgs: 1, maxArgs: 2,
		noAuth: true, noClient: true,
		validateWithOptions: validateChatShowArgsWithOptions,
		help:                printChatShowUsage,
		runWithOptions: func(_ *api.Client, args []string, opts globalOptions) error {
			return runChatShowWithOptions(args, opts)
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

	// Research operations
	{
		name: "research", argsUsage: "<notebook-id> \"query\"",
		usage: "Run fast or deep research (JSON-lines by default; --md for markdown; --mode=fast|deep)", section: "Research",
		minArgs: 2, maxArgs: -1,
		validateWithOptions: validateResearchArgsWithOptions,
		help:                printResearchUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return runResearchCommandWithOptions(c, args, opts)
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
		name: "betool", argsUsage: "<decode-request|encode-request|decode-response|encode-response|infer-proto> [file...]",
		usage:   "Translate raw batchexecute payloads to JSON and back (offline codec)",
		section: "Other",
		minArgs: 0, maxArgs: -1,
		noAuth: true, noClient: true,
		hidden: true, // developer tool; pure wire codec, no network I/O
		help:   func(cmdName string) { printBetoolUsage() },
		runWithOptions: func(_ *api.Client, args []string, opts globalOptions) error {
			return runBetool(args, opts.jsonOutput)
		},
	},
	{
		name: "auth", argsUsage: "[profile]",
		usage: "Set up authentication from a browser profile", section: "Other",
		minArgs: 0, maxArgs: -1,
		noAuth: true, noClient: true,
		help: printAuthUsage,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			_, _, err := handleAuthWithOptions(args, opts)
			return err
		},
	},
	{
		name:  "refresh",
		usage: "Refresh stored authentication credentials", section: "Other",
		minArgs: 0, maxArgs: -1,
		noAuth: true, noClient: true,
		runWithOptions: func(c *api.Client, args []string, opts globalOptions) error {
			return refreshCredentials(opts.debug)
		},
	},
	{
		name: "account", argsUsage: "[set <key> <value>]",
		usage: "Show or update the authenticated user's NotebookLM account (ZwVcOc / hT54vc)", section: "Other",
		minArgs: 0, maxArgs: 3,
		run: func(c *api.Client, args []string) error { return runAccount(c, args) },
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
var commandStarts map[string]bool
var maxCommandWords int

var experimentalCommands = map[string]bool{}

var internalCommands = map[string]bool{
	"dump-load-source": true,
	"hb":               true,
}

var compatibilityCommands = map[string]bool{
	"list":                 true,
	"create":               true,
	"rm":                   true,
	"rename-notebook":      true,
	"notebook-emoji":       true,
	"notebook-description": true,
	"notebook-cover":       true,
	"notebook-cover-image": true,
	"notebook-unrecent":    true,
	"list-featured":        true,
	"sources":              true,
	"add":                  true,
	"rm-source":            true,
	"rename-source":        true,
	"refresh-source":       true,
	"check-source":         true,
	"read-source":          true,
	"notes":                true,
	"read-note":            true,
	"new-note":             true,
	"update-note":          true,
	"rm-note":              true,
	"label-list":           true,
	"label-generate":       true,
	"autolabel":            true,
	"label-create":         true,
	"label-rename":         true,
	"label-emoji":          true,
	"label-delete":         true,
	"label-unlabeled":      true,
	"label-relabel-all":    true,
	"label-attach":         true,
	"get-artifact":         true,
	"artifacts":            true,
	"update-artifact":      true,
	"delete-artifact":      true,
	"rename-artifact":      true,
	"chat-list":            true,
	"chat-history":         true,
	"chat-show":            true,
	"delete-chat":          true,
	"chat-config":          true,
	"set-instructions":     true,
	"get-instructions":     true,
	"audio-list":           true,
	"audio-get":            true,
	"audio-download":       true,
	"audio-rm":             true,
	"audio-share":          true,
	"download slide-deck":  true,
}

var compatibilityReplacements = map[string]string{
	"list":                 "notebook list",
	"ls":                   "notebook list",
	"create":               "notebook create",
	"rm":                   "notebook delete",
	"rename-notebook":      "notebook rename",
	"notebook-emoji":       "notebook emoji",
	"notebook-description": "notebook description",
	"notebook-notes":       "notebook description",
	"notebook-cover":       "notebook cover",
	"notebook-cover-image": "notebook cover-image",
	"notebook-unrecent":    "notebook unrecent",
	"list-featured":        "notebook featured",
	"sources":              "source list",
	"add":                  "source add",
	"rm-source":            "source delete",
	"source-rm":            "source delete",
	"rename-source":        "source rename",
	"refresh-source":       "source refresh",
	"check-source":         "source check",
	"read-source":          "source read",
	"notes":                "note list",
	"read-note":            "note read",
	"new-note":             "note create",
	"update-note":          "note update",
	"rm-note":              "note delete",
	"note-rm":              "note delete",
	"label-list":           "label list",
	"labels":               "label list",
	"label-generate":       "label generate",
	"autolabel":            "label generate",
	"label-create":         "label create",
	"label-rename":         "label rename",
	"label-emoji":          "label emoji",
	"label-delete":         "label delete",
	"label-unlabeled":      "label unlabeled",
	"label-relabel-all":    "label relabel-all",
	"label-attach":         "label attach",
	"artifacts":            "artifact list",
	"list-artifacts":       "artifact list",
	"get-artifact":         "artifact get",
	"read-artifact":        "artifact read",
	"update-artifact":      "artifact update",
	"delete-artifact":      "artifact delete",
	"rename-artifact":      "artifact update",
	"chat-list":            "chat list",
	"chat-history":         "chat history",
	"chat-show":            "chat show",
	"delete-chat":          "chat delete",
	"chat-config":          "chat config",
	"set-instructions":     "chat instructions set",
	"get-instructions":     "chat instructions get",
	"audio-list":           "audio list",
	"audio-get":            "audio get",
	"audio-download":       "audio download",
	"audio-rm":             "audio delete",
	"audio-share":          "audio share",
	"download slide-deck":  "deck download",
}

func init() {
	buildCommandRegistry()
	commandIndex = make(map[string]*command, len(commands)*2)
	commandStarts = make(map[string]bool, len(commands))
	for i := range commands {
		cmd := &commands[i]
		commandIndex[cmd.name] = cmd
		registerCommandStart(cmd.name)
		for _, alias := range cmd.aliases {
			commandIndex[alias] = cmd
			registerCommandStart(alias)
		}
	}
}

func registerCommandStart(name string) {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return
	}
	commandStarts[parts[0]] = true
	maxCommandWords = max(maxCommandWords, len(parts))
}

// lookupCommand returns the command for a given name or alias.
func lookupCommand(name string) (*command, bool) {
	cmd, ok := commandIndex[name]
	return cmd, ok
}

func findCommand(args []string) (string, *command, []string, bool) {
	limit := min(len(args), maxCommandWords)
	for n := limit; n >= 1; n-- {
		name := strings.Join(args[:n], " ")
		if cmd, ok := lookupCommand(name); ok {
			return name, cmd, args[n:], true
		}
	}
	return "", nil, nil, false
}

func isCommandStart(name string) bool {
	return commandStarts[name] || helpAliases[name]
}

// experimentalEnabled reports whether experimental (hidden) commands should
// be surfaced. Either --experimental or NLM_EXPERIMENTAL=<non-empty> enables
// them. Keep both forms: the flag is discoverable via --help, the env var
// is ergonomic for long-running shells and MCP configs.
func experimentalEnabled() bool {
	return experimental || os.Getenv("NLM_EXPERIMENTAL") != ""
}

// helpSections lists the help groupings in the order they should be printed.
// The display order matches the original help layout; new sections appended
// here also become valid arguments for `nlm <noun> --help` narrowing.
var helpSections = []string{
	"Notebook", "Source", "Note", "Label", "Create", "Audio", "Video",
	"Deck", "Artifact", "Guidebook", "Generation", "Chat",
	"Research", "Sharing", "Other",
}

// printUsage prints the full help text derived from the command table,
// preceded by a preamble and followed by the exit-code reference.
func printUsage() {
	printPreamble()
	printSections(helpSections)
	printExitCodes()
}

// printPreamble emits the program tagline, one-line summary, and a quick
// pointer to authentication setup. The preamble runs before any command
// listing so a fresh agent reading `nlm --help` sees orientation first.
func printPreamble() {
	fmt.Fprint(os.Stderr,
		"nlm — Command-line interface to Google's NotebookLM.\n"+
			"Manage notebooks, sources, chat, and generated content from the terminal.\n\n"+
			"First run: `nlm auth` to set up authentication, or set NLM_AUTH_TOKEN and NLM_COOKIES.\n\n"+
			"Usage: nlm <command> [arguments]\n\n")
}

// printSections renders the command table for the given sections in order.
// Commands not visible per shouldShowInHelp are skipped.
func printSections(sections []string) {
	for _, section := range sections {
		printed := false
		for i := range commands {
			cmd := &commands[i]
			if cmd.section != section {
				continue
			}
			if !shouldShowInHelp(cmd) {
				continue
			}
			if !printed {
				fmt.Fprintf(os.Stderr, "%s Commands:\n", section)
				printed = true
			}
			label := cmd.name
			if len(cmd.aliases) > 0 && cmd.surface == surfaceStable {
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

// printExitCodes documents the exit-code taxonomy from exitcode.go so
// scripts and agents can branch on numeric codes without reading source.
func printExitCodes() {
	fmt.Fprint(os.Stderr,
		"Exit Codes:\n"+
			"  0  success\n"+
			"  2  bad arguments\n"+
			"  3  authentication required or invalid\n"+
			"  4  not found (notebook, source, artifact)\n"+
			"  5  precondition failed (quota, source cap, wrong source type)\n"+
			"  6  transient error (rate limit, 5xx, connection)\n"+
			"  7  resource busy (still generating)\n")
}

// sectionForNoun resolves a user-supplied noun to a section name from
// helpSections. Matching is case-insensitive on the section's first word
// (e.g. "notebook" matches "Notebook"). Returns "" if no
// section matches.
func sectionForNoun(noun string) string {
	noun = strings.ToLower(strings.TrimSpace(noun))
	if noun == "" {
		return ""
	}
	for _, s := range helpSections {
		first := strings.ToLower(strings.Fields(s)[0])
		if first == noun {
			return s
		}
	}
	return ""
}

// printSectionUsage renders just one section's commands, framed by the
// preamble so the output stays self-contained. Used for `nlm <noun> --help`
// narrowing.
func printSectionUsage(section string) {
	printPreamble()
	printSections([]string{section})
}

// suggestCommand returns the closest top-level command name (or section
// noun) to query, provided the Levenshtein distance is at most 2. Empty
// string means no suggestion is worth printing.
func suggestCommand(query string) string {
	return suggestFromPool(query, topLevelSuggestionPool())
}

// suggestVerb returns the closest verb in a section (e.g. for
// `nlm notebook bogos-verb` we suggest `notebook list`). The pool is
// every command whose name begins with "<section> ". Distance threshold
// matches suggestCommand.
func suggestVerb(section, query string) string {
	prefix := section + " "
	var pool []string
	for i := range commands {
		cmd := &commands[i]
		if !strings.HasPrefix(cmd.name, prefix) {
			continue
		}
		if !shouldShowInHelp(cmd) {
			continue
		}
		// Suggest just the verb, not the full multi-word command, so the
		// hint matches what the user would type after the noun.
		pool = append(pool, strings.TrimPrefix(cmd.name, prefix))
	}
	return suggestFromPool(query, pool)
}

// topLevelSuggestionPool returns all visible top-level command names plus
// the section nouns. Multi-word commands are reduced to their first token
// (the noun) so suggestions stay short and stable.
func topLevelSuggestionPool() []string {
	seen := map[string]bool{}
	pool := make([]string, 0, len(commands)+len(helpSections))
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		pool = append(pool, s)
	}
	for i := range commands {
		cmd := &commands[i]
		if !shouldShowInHelp(cmd) {
			continue
		}
		// Single-word commands suggest as-is; multi-word commands reduce
		// to the noun so e.g. "audi" suggests "audio" not "audio-list".
		first := strings.Fields(cmd.name)[0]
		if first == cmd.name {
			add(cmd.name)
		} else {
			add(first)
		}
		for _, a := range cmd.aliases {
			add(a)
		}
	}
	for _, s := range helpSections {
		add(strings.ToLower(strings.Fields(s)[0]))
	}
	return pool
}

// suggestFromPool picks the closest pool entry to query and returns it
// only if the edit distance is small enough to be a likely typo. The
// threshold scales loosely with query length: very short tokens require
// distance 1; longer tokens allow up to 2.
func suggestFromPool(query string, pool []string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || len(pool) == 0 {
		return ""
	}
	limit := 2
	if len(query) <= 3 {
		limit = 1
	}
	best := ""
	bestDist := limit + 1
	for _, cand := range pool {
		d := levenshtein(query, strings.ToLower(cand))
		if d < bestDist {
			bestDist = d
			best = cand
		}
	}
	if bestDist > limit {
		return ""
	}
	return best
}

// levenshtein returns the edit distance between a and b. The
// implementation uses a single rolling row to keep allocations small;
// good enough for short command names.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min(min(curr[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

func shouldShowInHelp(cmd *command) bool {
	switch cmd.surface {
	case surfaceStable:
		return !cmd.hidden
	case surfaceExperimental:
		return experimentalEnabled()
	default:
		return false
	}
}

func warnCompatibilityCommand(name string, cmd *command) {
	if cmd.surface != surfaceCompatibility {
		return
	}
	replacement := ""
	if cmd.surfaceSpec != nil {
		replacement = strings.Join(cmd.surfaceSpec.Replacement, " ")
	}
	if replacement == "" {
		replacement = compatibilityReplacements[name]
	}
	if replacement == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "nlm: '%s' is deprecated; use '%s'\n", name, replacement)
}

// validateCommandArgs checks positional argument count for a command.
// cmdName is the name the user typed (may be an alias).
// errBadArgs is returned by argument-validation paths so the exit-code
// classifier maps them to exit 2 (bad-args). The message is intentionally
// generic — the per-command usage hint is printed separately to stderr.
var errBadArgs = errors.New("invalid arguments")

// errPrecondition marks a locally detected state that makes an otherwise
// valid command inapplicable. It maps to exit 5.
var errPrecondition = errors.New("precondition failed")

// errNotFound marks a missing resource detected by the command layer. It maps
// to exit 4.
var errNotFound = errors.New("not found")

func validateCommandArgs(cmd *command, cmdName string, args []string, opts globalOptions) error {
	if cmd.spec != nil {
		_, err := parseBoundCommand(cmd, cmdName, args, opts)
		return err
	}
	if cmd.validateWithOptions != nil {
		return cmd.validateWithOptions(cmdName, args, opts)
	}
	if cmd.validate != nil {
		return cmd.validate(cmdName, args)
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
	return nil
}

func runCommand(cmd *command, c *api.Client, args []string, opts globalOptions) error {
	if cmd.spec != nil {
		parsed, err := parseBoundCommand(cmd, cmd.name, args, opts)
		if err != nil {
			return err
		}
		call, err := cmd.spec.Decode(parsed)
		if err != nil {
			return err
		}
		return call(context.Background(), c)
	}
	if cmd.runWithOptions != nil {
		return cmd.runWithOptions(c, args, opts)
	}
	return cmd.run(c, args)
}

// commandTableEntries returns all command entries for testing.
func commandTableEntries() []command {
	return commands
}

// helpAliases are recognized as valid commands but handled before table lookup.
var helpAliases = map[string]bool{
	"help": true, "-h": true, "--help": true,
}

// nounSectionFromArgs returns the help section that matches when the user
// runs `nlm <noun>` or `nlm <noun> --help` with no further arguments and
// no matching command. Returns "" if the args don't match that exact
// shape. Multi-word commands fall through to the regular not-found path.
func nounSectionFromArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if len(args) >= 2 && !helpAliases[args[1]] {
		return ""
	}
	if len(args) > 2 {
		return ""
	}
	return sectionForNoun(args[0])
}

// suggestionForArgs computes the best-guess command name for a misspelled
// invocation. For single-arg misses it uses the top-level pool; for
// `nlm <known-noun> <verb>` shapes it searches verbs within that section
// (and does not fall back to top-level matches, since the typo is in the
// verb, not the noun).
func suggestionForArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if len(args) >= 2 {
		if section := sectionForNoun(args[0]); section != "" {
			if v := suggestVerb(strings.ToLower(strings.Fields(section)[0]), args[1]); v != "" {
				return args[0] + " " + v
			}
			return ""
		}
	}
	return suggestCommand(args[0])
}
