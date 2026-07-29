package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type commandSurface int

const (
	surfaceStable commandSurface = iota
	surfaceExperimental
	surfaceInternal
	surfaceCompatibility
)

// command is one bound surface of a commandSpec. Stable and compatibility
// surfaces share the same behavior-bearing specification.
type command struct {
	spec        *commandSpec
	surfaceSpec *commandSurfaceSpec
	name        string
	aliases     []string
	usage       string
	argsUsage   string
	section     string
	surface     commandSurface
	hidden      bool
}

var commandSpecs = []*commandSpec{
	// Notebook operations
	{
		ID: "list", aliases: []string{"ls"},
		Summary: "List all notebooks", Section: "Notebook",
	},
	{
		ID: "create", Summary: "Create a new notebook", Section: "Notebook",
	},
	{
		ID: "rm", Summary: "Delete a notebook", Section: "Notebook",
	},
	{
		ID: "rename-notebook", Summary: "Rename a notebook", Section: "Notebook",
	},
	{
		ID: "notebook-emoji", Summary: "Change notebook emoji", Section: "Notebook",
	},
	{
		ID: "notebook-description", aliases: []string{"notebook-notes"},
		Summary: "Set notebook description / creator notes (text via arg or stdin; empty clears)", Section: "Notebook",
	},
	{
		ID: "notebook-cover", Summary: "Pick a built-in cover image (preset ID; HAR-captured value: 4. Other IDs uncatalogued)", Section: "Notebook",
	},
	{
		ID: "notebook-cover-image", Summary: "Upload a custom cover image and associate it with the notebook", Section: "Notebook",
	},
	{
		ID: "notebook-unrecent", Summary: "Remove a notebook from the recently-viewed list (does not delete it)", Section: "Notebook",
	},
	{
		ID: "analytics", Summary: "Show notebook analytics time series", Section: "Notebook",
	},
	{
		ID:      "list-featured",
		Summary: "List featured notebooks", Section: "Notebook",
	},

	// Source operations
	{
		ID: "sources", Summary: "List sources in notebook", Section: "Source",
	},
	{
		ID: "add", Summary: "Add one or more sources (files, URLs, or text; pass '-' to stream stdin as a single source)", Section: "Source",
	},
	{
		ID: "sync", Summary: "Bundle local files into a txtar source and keep it in sync (auto-chunks at 5MB; see --help)", Section: "Source",
		hidden: true, // top-level shortcut for `source sync`; kept first-class but de-duplicated from help
	},
	{
		ID: "sync-pack", Summary: "Preview the txtar bytes that sync would upload (offline)",
		Section:  "Source",
		hidden:   true, // top-level shortcut for `source pack`; kept first-class but de-duplicated from help
		noClient: true,
	},
	{
		ID: "rm-source", aliases: []string{"source-rm"}, Summary: "Remove one or more sources (pass '-' to read newline-delimited IDs from stdin)", Section: "Source",
	},
	{
		ID: "rename-source", Summary: "Rename a source", Section: "Source",
	},
	{
		ID: "refresh-source", Summary: "Refresh source content", Section: "Source",
	},
	{
		ID: "check-source", Summary: "Check source freshness (Google-Drive-only; notebook-id enables client-side source-type validation)", Section: "Source",
	},
	{
		ID: "discover-sources", Summary: "Discover relevant sources via Es3dTe (chat fallback if the server rejects)", Section: "Source",
	},
	{
		ID: "dump-load-source", Summary: "Print the raw JSON wire response of LoadSource (hizoJc) for a source", Section: "Source",
		hidden: true, // developer tool; exposes unmodeled fields (text body fragments, etc.)
	},
	{
		ID: "read-source", Summary: "Read a source body", Section: "Source",
	},

	// Note operations
	{
		ID: "notes", Summary: "List notes in notebook", Section: "Note",
	},
	{
		ID: "read-note", Summary: "Read full note content", Section: "Note",
	},

	{
		ID: "new-note", Summary: "Create new note (content via arg or stdin)", Section: "Note",
	},
	{
		ID: "update-note", Summary: "Edit note content and title", Section: "Note",
	},
	{
		ID: "rm-note", aliases: []string{"note-rm"}, Summary: "Remove a note from a notebook", Section: "Note",
	},

	// Label operations
	{
		ID: "label-list", aliases: []string{"labels"},
		Summary: "List labels (autolabel clusters) in a notebook", Section: "Label",
	},
	{
		ID: "label-generate", aliases: []string{"autolabel"},
		Summary: "Recompute autolabel clusters for a notebook", Section: "Label",
	},
	{
		ID:      "label-create",
		Summary: "Create a new manual label on a notebook", Section: "Label",
	},
	{
		ID:      "label-rename",
		Summary: "Rename an existing label", Section: "Label",
	},
	{
		ID:      "label-emoji",
		Summary: "Set or clear the emoji on a label", Section: "Label",
	},
	{
		ID:      "label-delete",
		Summary: "Delete one or more labels by ID", Section: "Label",
	},
	{
		ID:      "label-unlabeled",
		Summary: "Apply existing labels to currently-unlabeled sources", Section: "Label",
	},
	{
		ID:      "label-relabel-all",
		Summary: "Re-cluster everything (UI's \"Relabel all\")", Section: "Label",
	},
	{
		ID:      "label-attach",
		Summary: "Attach a source to a label (single source per call)", Section: "Label",
	},

	// Create operations
	{
		ID: "create-audio", Summary: "Create audio overview", Section: "Create",
	},
	{
		ID: "create-video", Summary: "Create video overview", Section: "Create",
	},
	{
		ID: "app-create", Summary: "Create a generated app artifact", Section: "Create",
	},
	{
		ID: "mindmap-create", Summary: "Create a generated mind map artifact", Section: "Create",
	},
	{
		ID: "create-slides", Summary: "Create slide deck", Section: "Create",
	},
	{
		ID:      "deck-download",
		Summary: "Download a slide deck (PDF/PPTX)", Section: "Deck",
		hidden: true,
	},
	{
		ID:      "download slide-deck",
		Summary: "Download a slide deck (PDF/PPTX)", Section: "Deck",
		hidden: true,
	},

	// Audio operations
	{
		ID: "audio-list", Summary: "List audio overviews for a notebook", Section: "Audio",
	},
	{
		ID: "audio-get", Summary: "Get audio overview details", Section: "Audio",
	},
	{
		ID: "audio-download", Summary: "Download audio file", Section: "Audio",
	},
	{
		ID: "audio-rm", Summary: "Delete audio overview", Section: "Audio",
	},
	{
		ID: "audio-share", Summary: "Share audio overview", Section: "Audio",
	},

	// Artifact operations
	{
		ID: "get-artifact", Summary: "Get artifact details", Section: "Artifact",
	},
	{
		ID: "read-artifact", Summary: "Print a text artifact", Section: "Artifact",
	},
	{
		ID:      "export-flashcards",
		Summary: "Export an artifact", Section: "Artifact",
		hidden: true,
	},
	{
		ID: "artifacts", aliases: []string{"list-artifacts"}, Summary: "List artifacts in notebook", Section: "Artifact",
	},
	{
		ID: "update-artifact", Summary: "Rename artifact (new title from positional arg or --name)", Section: "Artifact",
	},
	{
		ID: "rename-artifact", Summary: "Rename artifact (alias: update-artifact)", Section: "Artifact",
		hidden: true, // superseded by update-artifact
	},
	{
		ID: "delete-artifact", Summary: "Delete artifact", Section: "Artifact",
	},
	// Guidebook operations
	{
		ID:      "guidebooks",
		Summary: "List all guidebooks", Section: "Guidebook",
	},
	{
		ID: "guidebook", Summary: "Get guidebook details", Section: "Guidebook",
	},
	{
		ID: "guidebook-details", Summary: "Get detailed guidebook info with sections and analytics", Section: "Guidebook",
	},
	{
		ID: "guidebook-publish", Summary: "Publish a guidebook", Section: "Guidebook",
	},
	{
		ID: "guidebook-share", Summary: "Share a guidebook", Section: "Guidebook",
	},
	{
		ID: "guidebook-ask", Summary: "Ask a guidebook question", Section: "Guidebook",
	},
	{
		ID: "guidebook-rm", Summary: "Delete a guidebook", Section: "Guidebook",
	},

	// Generation operations
	{
		ID: "generate-guide", Summary: "Generate notebook guide", Section: "Generation",
	},
	{
		ID: "source-guide", Summary: "Show the per-source auto-summary and keyword chips (cached on disk)", Section: "Generation",
	},
	{
		ID: "generate-chat", Summary: "Stream a one-shot chat answer (use --conversation to follow up)", Section: "Generation",
	},
	{
		ID: "report-suggestions", Summary: "Suggest report topics for notebook", Section: "Generation",
	},
	{
		ID: "audio-suggestions", Summary: "Suggest audio-overview blueprints (emit JSON lines; pipe to create-audio)", Section: "Generation",
	},
	{
		ID: "create-report", Summary: "Create a report artifact (run report-suggestions for valid types)", Section: "Create",
	},
	{
		ID: "generate-report", Summary: "Generate multi-section report via chat (see --prompt, --sections)", Section: "Generation",
	},
	// Chat operations
	{
		ID: "chat", Summary: "Open interactive chat (one-shot if a prompt is given; -f <file> reads a long prompt from file)", Section: "Chat",
	},
	{
		ID: "chat-list", Summary: "List chat sessions (server-side when a notebook is given)", Section: "Chat",
		noAuth: true, noClient: true,
	},
	{
		ID: "chat-history", Summary: "View conversation history", Section: "Chat",
	},
	{
		ID: "chat-show", Summary: "Render a local chat transcript (see --citations)", Section: "Chat",
		noAuth: true, noClient: true,
	},
	{
		ID: "delete-chat", Summary: "Delete server-side chat history", Section: "Chat",
	},
	{
		ID: "chat-config", Summary: "Configure chat settings", Section: "Chat",
	},
	{
		ID: "set-instructions", Summary: "Set system instructions", Section: "Chat",
	},
	{
		ID: "get-instructions", Summary: "Show current system instructions", Section: "Chat",
	},

	// Research operations
	{
		ID: "research", Summary: "Run fast or deep research (JSON-lines by default; --md for markdown; --mode=fast|deep)", Section: "Research",
	},

	// Sharing operations
	{
		ID: "share", Summary: "Share notebook publicly", Section: "Sharing",
	},
	{
		ID: "share-private", Summary: "Share notebook privately", Section: "Sharing",
	},
	{
		ID: "share-details", Summary: "Get details of shared project", Section: "Sharing",
	},

	// Other operations
	{
		ID:      "mcp",
		Summary: "Run the MCP server on stdin/stdout", Section: "Other",
	},
	{
		ID: "betool", Summary: "Translate raw batchexecute payloads to JSON and back (offline codec)",
		Section: "Other",
		noAuth:  true, noClient: true,
		hidden: true, // developer tool; pure wire codec, no network I/O
	},
	{
		ID: "auth", Summary: "Set up authentication from a browser profile", Section: "Other",
		noAuth: true, noClient: true,
	},
	{
		ID:      "refresh",
		Summary: "Refresh stored authentication credentials", Section: "Other",
		noAuth: true, noClient: true,
	},
	{
		ID: "account", Summary: "Show or update the authenticated user's NotebookLM account (ZwVcOc / hT54vc)", Section: "Other",
	},
	{
		ID:      "hb",
		Summary: "Send a session heartbeat", Section: "Other",
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

type badArgsError struct {
	message string
}

func (e badArgsError) Error() string {
	return e.message
}

func (badArgsError) Is(target error) bool {
	return target == errBadArgs
}

func badArgsf(format string, args ...any) error {
	return badArgsError{message: fmt.Sprintf(format, args...)}
}

// errPrecondition marks a locally detected state that makes an otherwise
// valid command inapplicable. It maps to exit 5.
var errPrecondition = errors.New("precondition failed")

// errNotFound marks a missing resource detected by the command layer. It maps
// to exit 4.
var errNotFound = errors.New("not found")

func validateCommandArgs(cmd *command, cmdName string, args []string, opts globalOptions) error {
	_, err := parseBoundCommand(cmd, cmdName, args, opts)
	return err
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
