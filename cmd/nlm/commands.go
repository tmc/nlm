package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
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

// command describes a single CLI command.
type command struct {
	name      string
	aliases   []string
	usage     string // one-line description for help text
	argsUsage string // positional args hint for "usage: nlm <name> <argsUsage>"
	section   string // help section header
	surface   commandSurface
	minArgs   int  // minimum positional args (after command name)
	maxArgs   int  // maximum positional args; -1 = unlimited
	noAuth    bool // true if command does not require authentication
	noClient  bool // true if command does not need an API client (implies noAuth)
	hidden    bool // true to hide from help text (experimental)
	validate  func(cmdName string, args []string) error
	help      func(cmdName string)
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
		validate: validateSourceSelectionArgs,
		help:     printSourceSelectionUsage,
		run: func(c *api.Client, args []string) error {
			return runSourceSelectionAction(c, args, action)
		},
	}
}

func mustCommand(byName map[string]command, name string) command {
	cmd, ok := byName[name]
	if !ok {
		panic("missing command: " + name)
	}
	return cmd
}

func cloneCommand(base command, name string) command {
	base.name = name
	base.aliases = nil
	base.hidden = false
	base.surface = surfaceStable
	return base
}

func groupedCommandsFromExisting(existing []command) []command {
	byName := make(map[string]command, len(existing))
	for _, cmd := range existing {
		byName[cmd.name] = cmd
	}
	return []command{
		cloneCommand(mustCommand(byName, "list"), "notebook list"),
		cloneCommand(mustCommand(byName, "create"), "notebook create"),
		cloneCommand(mustCommand(byName, "rm"), "notebook delete"),
		cloneCommand(mustCommand(byName, "rename-notebook"), "notebook rename"),
		cloneCommand(mustCommand(byName, "notebook-emoji"), "notebook emoji"),
		cloneCommand(mustCommand(byName, "notebook-description"), "notebook description"),
		cloneCommand(mustCommand(byName, "notebook-cover"), "notebook cover"),
		cloneCommand(mustCommand(byName, "notebook-cover-image"), "notebook cover-image"),
		cloneCommand(mustCommand(byName, "notebook-unrecent"), "notebook unrecent"),
		cloneCommand(mustCommand(byName, "list-featured"), "notebook featured"),

		cloneCommand(mustCommand(byName, "sources"), "source list"),
		cloneCommand(mustCommand(byName, "add"), "source add"),
		cloneCommand(mustCommand(byName, "sync"), "source sync"),
		cloneCommand(mustCommand(byName, "sync-pack"), "source pack"),
		cloneCommand(mustCommand(byName, "rm-source"), "source delete"),
		cloneCommand(mustCommand(byName, "rename-source"), "source rename"),
		cloneCommand(mustCommand(byName, "refresh-source"), "source refresh"),
		cloneCommand(mustCommand(byName, "check-source"), "source check"),
		cloneCommand(mustCommand(byName, "read-source"), "source read"),

		cloneCommand(mustCommand(byName, "notes"), "note list"),
		cloneCommand(mustCommand(byName, "read-note"), "note read"),
		cloneCommand(mustCommand(byName, "new-note"), "note create"),
		cloneCommand(mustCommand(byName, "update-note"), "note update"),
		cloneCommand(mustCommand(byName, "rm-note"), "note delete"),

		cloneCommand(mustCommand(byName, "artifacts"), "artifact list"),
		cloneCommand(mustCommand(byName, "get-artifact"), "artifact get"),
		cloneCommand(mustCommand(byName, "update-artifact"), "artifact update"),
		cloneCommand(mustCommand(byName, "delete-artifact"), "artifact delete"),
		cloneCommand(mustCommand(byName, "revise-artifact"), "artifact revise"),

		cloneCommand(mustCommand(byName, "chat-list"), "chat list"),
		cloneCommand(mustCommand(byName, "chat-history"), "chat history"),
		cloneCommand(mustCommand(byName, "chat-show"), "chat show"),
		cloneCommand(mustCommand(byName, "delete-chat"), "chat delete"),
		cloneCommand(mustCommand(byName, "chat-config"), "chat config"),
		cloneCommand(mustCommand(byName, "set-instructions"), "chat instructions set"),
		cloneCommand(mustCommand(byName, "get-instructions"), "chat instructions get"),
	}
}

// commands is the single source of truth for all CLI commands.
var commands = []command{
	// Notebook operations
	{
		name: "list", aliases: []string{"ls"},
		usage: "List all notebooks", section: "Notebook",
		argsUsage: "[flags]",
		minArgs:   0, maxArgs: -1,
		validate: validateNotebookListArgs,
		help:     printNotebookListUsage,
		run:      func(c *api.Client, args []string) error { return runNotebookList(c, args) },
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
		name: "rename-notebook", argsUsage: "<notebook-id> <new-title>",
		usage: "Rename a notebook", section: "Notebook",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return renameNotebook(c, args[0], args[1]) },
	},
	{
		name: "notebook-emoji", argsUsage: "<notebook-id> <emoji>",
		usage: "Change notebook emoji", section: "Notebook",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return setNotebookEmoji(c, args[0], args[1]) },
	},
	{
		name: "notebook-description", aliases: []string{"notebook-notes"},
		argsUsage: "<notebook-id> [text]",
		usage:     "Set notebook description / creator notes (text via arg or stdin; empty clears)", section: "Notebook",
		minArgs: 1, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			text := ""
			if len(args) > 1 {
				text = args[1]
			} else if fi, stErr := os.Stdin.Stat(); stErr == nil && fi.Mode()&os.ModeCharDevice == 0 {
				data, readErr := io.ReadAll(os.Stdin)
				if readErr != nil {
					return readErr
				}
				text = string(data)
			}
			return setNotebookDescription(c, args[0], text)
		},
	},
	{
		name: "notebook-cover", argsUsage: "<notebook-id> <preset-id>",
		usage: "Pick a built-in cover image (preset ID; HAR-captured value: 4. Other IDs uncatalogued)", section: "Notebook",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error {
			id, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("preset-id must be an integer: %w", err)
			}
			return setNotebookCover(c, args[0], id)
		},
	},
	{
		name: "notebook-cover-image", argsUsage: "<notebook-id> <image-path>",
		usage: "Upload a custom cover image and associate it with the notebook", section: "Notebook",
		minArgs: 2, maxArgs: 2,
		run: func(c *api.Client, args []string) error { return uploadNotebookCoverImage(c, args[0], args[1]) },
	},
	{
		name: "notebook-unrecent", argsUsage: "<notebook-id>",
		usage: "Remove a notebook from the recently-viewed list (does not delete it)", section: "Notebook",
		minArgs: 1, maxArgs: 1,
		run: func(c *api.Client, args []string) error {
			if err := c.RemoveRecentlyViewedProject(args[0]); err != nil {
				return fmt.Errorf("remove recently viewed: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Removed %s from recently viewed.\n", args[0])
			return nil
		},
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
		usage: "Add one or more sources (files, URLs, or text; pass '-' to stream stdin as a single source)", section: "Source",
		minArgs: 2, maxArgs: -1,
		validate: validateSourceAddArgs,
		help:     printSourceAddUsage,
		run:      func(c *api.Client, args []string) error { return runSourceAdd(c, args) },
	},
	{
		name: "sync", argsUsage: "<notebook-id> [paths...]",
		usage: "Bundle local files into a txtar source and keep it in sync (auto-chunks at 5MB; see --help)", section: "Source",
		minArgs: 1, maxArgs: -1,
		hidden:   true, // top-level shortcut for `source sync`; kept first-class but de-duplicated from help
		validate: validateSourceSyncArgs,
		help:     printSourceSyncUsage,
		run:      func(c *api.Client, args []string) error { return runSourceSync(c, args) },
	},
	{
		name: "sync-pack", argsUsage: "[paths...]",
		usage:   "Preview the txtar bytes that sync would upload (offline)",
		section: "Source",
		minArgs: 0, maxArgs: -1,
		hidden:   true, // top-level shortcut for `source pack`; kept first-class but de-duplicated from help
		noClient: true,
		validate: validateSourcePackArgs,
		help:     printSourcePackUsage,
		run:      func(_ *api.Client, args []string) error { return runSourcePack(args) },
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
		hidden:   true, // requires NLM_EXPERIMENTAL
		validate: func(cmdName string, args []string) error { return validateInteractiveAudioArgs(args) },
		help:     printInteractiveAudioUsage,
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
			// GetVideoOverview requires the direct-RPC path; enable it
			// transparently so callers don't have to pass --direct-rpc.
			c.SetUseDirectRPC(true)
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
	{
		name: "revise-artifact", argsUsage: "<artifact-id> <instructions>",
		usage: "Re-run an artifact generator with revision instructions (KmcKPe; HAR-unverified)", section: "Artifact",
		minArgs: 2, maxArgs: -1,
		run: func(c *api.Client, args []string) error {
			instructions := strings.Join(args[1:], " ")
			art, err := c.ReviseArtifact(args[0], instructions)
			if err != nil {
				return err
			}
			fmt.Println(art.GetArtifactId())
			fmt.Fprintf(os.Stderr, "Revision submitted. Use 'nlm artifact get %s' to check status.\n", art.GetArtifactId())
			return nil
		},
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
		name: "magic", argsUsage: "<notebook-id> [source-id...]",
		usage: "Generate the notebook 'Magic View' (uK8f7c)", section: "Generation",
		minArgs: 1, maxArgs: -1,
		run: func(c *api.Client, args []string) error { return runMagicView(c, args[0], args[1:]) },
	},
	{
		name: "source-guide", argsUsage: "<notebook-id> [source-id...]",
		usage: "Show the per-source auto-summary and keyword chips (cached on disk)", section: "Generation",
		minArgs: 1, maxArgs: -1,
		validate: validateSourceSelectionArgs,
		help:     printSourceSelectionUsage,
		run:      func(c *api.Client, args []string) error { return runSourceGuide(c, args) },
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
		validate: validateGenerateChatArgs,
		help:     printGenerateChatUsage,
		run:      func(c *api.Client, args []string) error { return runGenerateChat(c, args) },
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
		validate: validateGenerateReportArgs,
		help:     printGenerateReportUsage,
		run:      func(c *api.Client, args []string) error { return runGenerateReport(c, args) },
	},
	// Chat operations
	{
		name: "chat", argsUsage: "<notebook-id> [conversation-id | prompt]",
		usage: "Open interactive chat (one-shot if a prompt is given; -f <file> reads a long prompt from file)", section: "Chat",
		minArgs: 1, maxArgs: -1,
		validate: validateChatArgs,
		help:     printChatUsage,
		run:      func(c *api.Client, args []string) error { return runChat(c, args) },
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
		validate: validateChatShowArgs,
		help:     printChatShowUsage,
		run:      func(_ *api.Client, args []string) error { return runChatShow(args) },
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
		validate: validateResearchArgs,
		help:     printResearchUsage,
		run:      func(c *api.Client, args []string) error { return runResearchCommand(c, args) },
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
var commandStarts map[string]bool
var maxCommandWords int

var experimentalCommands = map[string]bool{
	"analytics":         true,
	"discover-sources":  true,
	"audio-interactive": true,
}

var internalCommands = map[string]bool{
	"dump-load-source": true,
	"hb":               true,
}

var compatibilityCommands = map[string]bool{
	"list":             true,
	"create":           true,
	"rm":               true,
	"rename-notebook":  true,
	"notebook-emoji":   true,
	"notebook-description": true,
	"notebook-cover":   true,
	"notebook-cover-image": true,
	"notebook-unrecent": true,
	"list-featured":    true,
	"sources":          true,
	"add":              true,
	"rm-source":        true,
	"rename-source":    true,
	"refresh-source":   true,
	"check-source":     true,
	"read-source":      true,
	"notes":            true,
	"read-note":        true,
	"new-note":         true,
	"update-note":      true,
	"rm-note":          true,
	"get-artifact":     true,
	"artifacts":        true,
	"update-artifact":  true,
	"delete-artifact":  true,
	"rename-artifact":  true,
	"revise-artifact":  true,
	"chat-list":        true,
	"chat-history":     true,
	"chat-show":        true,
	"delete-chat":      true,
	"chat-config":      true,
	"set-instructions": true,
	"get-instructions": true,
}

var compatibilityReplacements = map[string]string{
	"list":             "notebook list",
	"ls":               "notebook list",
	"create":           "notebook create",
	"rm":               "notebook delete",
	"rename-notebook":  "notebook rename",
	"notebook-emoji":   "notebook emoji",
	"notebook-description": "notebook description",
	"notebook-notes":   "notebook description",
	"notebook-cover":   "notebook cover",
	"notebook-cover-image": "notebook cover-image",
	"notebook-unrecent": "notebook unrecent",
	"list-featured":    "notebook featured",
	"sources":          "source list",
	"add":              "source add",
	"rm-source":        "source delete",
	"source-rm":        "source delete",
	"rename-source":    "source rename",
	"refresh-source":   "source refresh",
	"check-source":     "source check",
	"read-source":      "source read",
	"notes":            "note list",
	"read-note":        "note read",
	"new-note":         "note create",
	"update-note":      "note update",
	"rm-note":          "note delete",
	"note-rm":          "note delete",
	"artifacts":        "artifact list",
	"list-artifacts":   "artifact list",
	"get-artifact":     "artifact get",
	"update-artifact":  "artifact update",
	"delete-artifact":  "artifact delete",
	"rename-artifact":  "artifact update",
	"revise-artifact":  "artifact revise",
	"chat-list":        "chat list",
	"chat-history":     "chat history",
	"chat-show":        "chat show",
	"delete-chat":      "chat delete",
	"chat-config":      "chat config",
	"set-instructions": "chat instructions set",
	"get-instructions": "chat instructions get",
}

func init() {
	commands = append(groupedCommandsFromExisting(commands), commands...)
	commandIndex = make(map[string]*command, len(commands)*2)
	commandStarts = make(map[string]bool, len(commands))
	for i := range commands {
		cmd := &commands[i]
		switch {
		case experimentalCommands[cmd.name]:
			cmd.surface = surfaceExperimental
		case internalCommands[cmd.name]:
			cmd.surface = surfaceInternal
		case compatibilityCommands[cmd.name]:
			cmd.surface = surfaceCompatibility
		}
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

// printUsage prints the full help text derived from the command table.
func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: nlm <command> [arguments]\n\n")

	// Ordered sections matching the original help layout.
	sections := []string{
		"Notebook", "Source", "Note", "Label", "Create", "Audio", "Video",
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
	replacement := compatibilityReplacements[name]
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

func validateCommandArgs(cmd *command, cmdName string, args []string) error {
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

// commandTableEntries returns all command entries for testing.
func commandTableEntries() []command {
	return commands
}

// helpAliases are recognized as valid commands but handled before table lookup.
var helpAliases = map[string]bool{
	"help": true, "-h": true, "--help": true,
}
