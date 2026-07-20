package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type globalOptions struct {
	showVersion          bool
	experimental         bool
	authToken            string
	cookies              string
	authUser             string
	debug                bool
	debugDumpPayload     bool
	debugParsing         bool
	debugFieldMapping    bool
	chromeProfile        string
	mimeType             string
	chunkedResponse      bool
	useDirectRPC         bool
	skipSources          bool
	yes                  bool
	sourceName           string
	showChatHistory      bool
	showThinking         bool
	thinkingJSONL        bool
	verbose              bool
	replaceSourceID      string
	force                bool
	dryRun               bool
	maxBytes             int
	jsonOutput           bool
	sourceReadMarkdown   bool
	sourceReadHTML       bool
	packChunk            int
	reportPrompt         string
	reportInstructions   string
	reportSections       int
	conversationID       string
	useWebChat           bool
	citationMode         string
	resolveCitationsFlag bool
	sourceIDsFlag        string
	sourceMatchFlag      string
	sourceExcludeFlag    string
	labelIDsFlag         string
	labelMatchFlag       string
	labelExcludeFlag     string
	promptFile           string
	researchMode         string
	researchMD           bool
	researchPollMs       int
	researchImport       bool
}

type invocationAction int

const (
	invocationRun invocationAction = iota
	invocationRootHelp
	invocationSectionHelp
	invocationCommandHelp
	invocationVersion
)

type invocation struct {
	action  invocationAction
	section string
	name    string
	cmd     *command
	args    []string
	globals globalOptions
}

func defaultGlobalOptions(env func(string) string) globalOptions {
	if env == nil {
		env = os.Getenv
	}
	return globalOptions{
		chromeProfile: env("NLM_BROWSER_PROFILE"),
		authToken:     env("NLM_AUTH_TOKEN"),
		cookies:       env("NLM_COOKIES"),
		authUser:      env("NLM_AUTHUSER"),
	}
}

func newGlobalFlagSet(opts *globalOptions) *flag.FlagSet {
	flags := flag.NewFlagSet("nlm", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	registerGlobalFlags(flags, opts)
	return flags
}

func registerGlobalFlags(flags *flag.FlagSet, opts *globalOptions) {
	flags.BoolVar(&opts.showVersion, "version", false, "print nlm version and exit")
	flags.BoolVar(&opts.experimental, "experimental", false, "enable experimental commands (also: NLM_EXPERIMENTAL=1)")
	flags.BoolVar(&opts.debug, "debug", false, "enable debug output")
	flags.BoolVar(&opts.debugDumpPayload, "debug-dump-payload", false, "dump raw JSON payload and exit (unix-friendly)")
	flags.BoolVar(&opts.debugParsing, "debug-parsing", false, "show detailed protobuf parsing information")
	flags.BoolVar(&opts.debugFieldMapping, "debug-field-mapping", false, "show how JSON array positions map to protobuf fields")
	flags.BoolVar(&opts.chunkedResponse, "chunked", false, "use chunked response format (rt=c)")
	flags.BoolVar(&opts.useDirectRPC, "direct-rpc", false, "use direct RPC calls for audio/video (bypasses orchestration service)")
	flags.BoolVar(&opts.skipSources, "skip-sources", false, "skip fetching sources for chat (useful for testing)")
	flags.BoolVar(&opts.yes, "yes", false, "skip confirmation prompts")
	flags.BoolVar(&opts.yes, "y", false, "skip confirmation prompts")
	flags.StringVar(&opts.chromeProfile, "profile", opts.chromeProfile, "Chrome profile to use")
	flags.StringVar(&opts.authToken, "auth", opts.authToken, "auth token (or set NLM_AUTH_TOKEN)")
	flags.StringVar(&opts.cookies, "cookies", opts.cookies, "cookies for authentication (or set NLM_COOKIES)")
	flags.StringVar(&opts.authUser, "authuser", opts.authUser, "Google account index for multi-account profiles")
	flags.StringVar(&opts.mimeType, "mime", "", "specify MIME type for content (e.g. 'application/pdf', 'text/plain')")
	flags.StringVar(&opts.mimeType, "mime-type", "", "specify MIME type for content (alias for -mime)")
	flags.StringVar(&opts.sourceName, "name", "", "custom name for added source")
	flags.StringVar(&opts.sourceName, "n", "", "custom name for added source (shorthand)")
	flags.StringVar(&opts.replaceSourceID, "replace", "", "source ID to replace (upload new, then delete old)")
	flags.BoolVar(&opts.jsonOutput, "json", false, "emit NDJSON instead of tab-separated tables (notebook list/source list/note list/notebook featured/artifact list/audio list/video list/guidebooks/chat list/label list); also enables NDJSON progress for sync")
	flags.BoolVar(&opts.sourceReadMarkdown, "markdown", false, "render source read with inline image references (source read)")
	flags.BoolVar(&opts.sourceReadHTML, "html", false, "render source read as a self-contained HTML document (source read)")
	flags.BoolVar(&opts.force, "force", false, "force re-upload even if unchanged (sync)")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "show what would change without uploading (sync)")
	flags.IntVar(&opts.maxBytes, "max-bytes", 0, "chunk threshold in bytes (sync, default 5120000)")
	flags.IntVar(&opts.packChunk, "chunk", 0, "1-indexed chunk to emit (sync-pack); omit to list or emit sole chunk")
	flags.StringVar(&opts.reportPrompt, "prompt", "", "per-section prompt template for generate-report ({topic} is replaced)")
	flags.StringVar(&opts.reportInstructions, "instructions", "", "set notebook instructions before generate-report")
	flags.IntVar(&opts.reportSections, "sections", 0, "max sections for generate-report (0 = all)")
	flags.StringVar(&opts.conversationID, "conversation", "", "continue an existing conversation by ID (generate-chat prints the ID on first turn)")
	flags.StringVar(&opts.conversationID, "c", "", "continue an existing conversation by ID (shorthand)")
	flags.BoolVar(&opts.useWebChat, "web", false, "use the most recent server-side conversation (generate-chat)")
	flags.BoolVar(&opts.showChatHistory, "history", false, "show previous chat conversation on start")
	flags.BoolVar(&opts.showThinking, "thinking", false, "show thinking headers while streaming chat and generate-chat responses")
	flags.BoolVar(&opts.showThinking, "reasoning", false, "show thinking headers while streaming chat and generate-chat responses")
	flags.BoolVar(&opts.thinkingJSONL, "thinking-jsonl", false, "deprecated: equivalent to --citations=json --thinking; emits thinking+answer+citation+followup JSON-lines events")
	flags.BoolVar(&opts.verbose, "verbose", false, "show full thinking traces while streaming chat and generate-chat responses")
	flags.BoolVar(&opts.verbose, "v", false, "show full thinking traces while streaming responses (shorthand)")
	flags.StringVar(&opts.citationMode, "citations", "", "citation rendering: auto|off|block|stream|tail|overlay|json (default: block - answer + trailing Sources list; json emits answer+citation JSON-lines)")
	flags.BoolVar(&opts.resolveCitationsFlag, "resolve-citations", false, "resolve each citation back to file:line when the source is a txtar archive (one extra LoadSourceText fetch per cited source)")
	flags.StringVar(&opts.sourceIDsFlag, "source-ids", "", "focus on these source IDs (e.g. 'a,b,c' or '-' for newline-delimited stdin); applies to chat, report, and transform commands")
	flags.StringVar(&opts.sourceMatchFlag, "source-match", "", "focus on sources whose title or UUID matches this regex (e.g. '^nlm internal/' or '^132af'); unioned with --source-ids")
	flags.StringVar(&opts.sourceExcludeFlag, "source-exclude", "", "exclude sources whose title or UUID matches this regex; applied after include selectors")
	flags.StringVar(&opts.labelIDsFlag, "label-ids", "", "include sources tagged with any of these label IDs ('a,b,c'); requires labels to be computed for the notebook")
	flags.StringVar(&opts.labelMatchFlag, "label-match", "", "include sources tagged with any label whose name matches this regex (e.g. '^Testing$')")
	flags.StringVar(&opts.labelExcludeFlag, "label-exclude", "", "exclude sources tagged with any label whose name matches this regex; applied after include selectors")
	flags.StringVar(&opts.promptFile, "prompt-file", "", "read prompt from file for one-shot chat ('-' reads stdin). Reliable for long/automated prompts.")
	flags.StringVar(&opts.promptFile, "f", "", "read prompt from file for one-shot chat ('-' reads stdin) (shorthand)")
	flags.StringVar(&opts.researchMode, "mode", "", "research mode: fast|deep (default: deep; used by nlm research)")
	flags.BoolVar(&opts.researchMD, "md", false, "emit raw markdown report (nlm research; default is JSON-lines events)")
	flags.IntVar(&opts.researchPollMs, "poll-ms", 0, "override research polling interval in milliseconds (default: 5000)")
	flags.BoolVar(&opts.researchImport, "import", false, "after research completes, import the discovered sources into the notebook via LBwxtb BulkImportFromResearch")
}

func parseInvocation(args []string, env func(string) string, stdout, stderr io.Writer) (invocation, error) {
	_ = stdout
	opts := defaultGlobalOptions(env)
	flags := newGlobalFlagSet(&opts)
	flagArgs, positional := splitGlobalArgs(args, flags)
	inv := invocation{globals: opts}
	if err := flags.Parse(flagArgs); err != nil {
		inv.globals = opts
		return inv, fmt.Errorf("%w: %v", errBadArgs, err)
	}
	inv.globals = opts
	if opts.showVersion {
		inv.action = invocationVersion
		return inv, nil
	}
	if len(positional) == 0 {
		inv.action = invocationRootHelp
		return inv, errBadArgs
	}
	if helpAliases[positional[0]] {
		inv.action = invocationRootHelp
		return inv, nil
	}

	cmdName, entry, cmdArgs, ok := findCommand(positional)
	if !ok {
		if section := nounSectionFromArgs(positional); section != "" {
			inv.action = invocationSectionHelp
			inv.section = section
			return inv, nil
		}
		if guess := suggestionForArgs(positional); guess != "" {
			fmt.Fprintf(stderr, "nlm: unknown command %q. Did you mean %q?\n\n", strings.Join(positional, " "), guess)
		}
		inv.action = invocationRootHelp
		return inv, errBadArgs
	}

	inv.name = cmdName
	inv.cmd = entry
	inv.args = cmdArgs
	if commandHelpRequested(cmdArgs) {
		inv.action = invocationCommandHelp
	}
	return inv, nil
}

func splitGlobalArgs(args []string, flags *flag.FlagSet) ([]string, []string) {
	knownFlags := map[string]bool{}
	boolFlags := map[string]bool{}
	flags.VisitAll(func(f *flag.Flag) {
		knownFlags[f.Name] = true
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			boolFlags[f.Name] = true
		}
	})

	var flagArgs, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if len(positional) == 0 {
				positional = append(positional, args[i+1:]...)
			} else {
				positional = append(positional, args[i:]...)
			}
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}

		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		if !knownFlags[name] || !globalFlagAllowedHere(positional, name) {
			positional = append(positional, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				positional = append(positional, args[i])
			}
			continue
		}

		flagArgs = append(flagArgs, arg)
		if boolFlags[name] || strings.Contains(arg, "=") {
			continue
		}
		if i+1 < len(args) {
			next := args[i+1]
			isFlag := strings.HasPrefix(next, "-") && next != "-"
			if !isCommandStart(next) && !isFlag {
				flagArgs = append(flagArgs, next)
				i++
				continue
			}
		}
		flagArgs[len(flagArgs)-1] = arg + "="
	}
	return flagArgs, positional
}

func globalFlagAllowedHere(positional []string, name string) bool {
	if len(positional) == 0 {
		return true
	}
	if commandOwnsFlag(positional, name) {
		return false
	}
	return postCommandGlobalFlags[name]
}

func commandOwnsFlag(positional []string, name string) bool {
	cmdName, _, _, ok := findCommand(positional)
	if !ok {
		return false
	}
	return commandLocalFlags[cmdName][name]
}

var postCommandGlobalFlags = map[string]bool{
	"auth":                true,
	"authuser":            true,
	"chunked":             true,
	"cookies":             true,
	"debug":               true,
	"debug-dump-payload":  true,
	"debug-field-mapping": true,
	"debug-parsing":       true,
	"direct-rpc":          true,
	"experimental":        true,
	"force":               true,
	"json":                true,
	"markdown":            true,
	"html":                true,
	"skip-sources":        true,
	"version":             true,
	"y":                   true,
	"yes":                 true,
}

var commandLocalFlags = map[string]map[string]bool{
	"auth": {
		"all": true, "a": true, "profile": true, "p": true,
		"url": true, "u": true, "notebooks": true, "debug": true,
		"d": true, "help": true, "h": true, "print-env": true,
		"keep-open": true, "k": true, "cdp-url": true, "c": true,
		"authuser": true, "au": true,
	},
	"notebook list": {"all": true, "limit": true, "json": true},
	"list":          {"all": true, "limit": true, "json": true},
	"ls":            {"all": true, "limit": true, "json": true},
	"source add": {
		"name": true, "n": true, "mime": true, "mime-type": true,
		"replace": true, "pre-process": true, "chunk": true,
	},
	"add": {
		"name": true, "n": true, "mime": true, "mime-type": true,
		"replace": true, "pre-process": true, "chunk": true,
	},
	"source sync": {
		"name": true, "n": true, "force": true, "dry-run": true,
		"max-bytes": true, "json": true, "exclude": true, "x": true,
		"include-untracked": true, "parallel": true, "pre-process": true,
	},
	"sync": {
		"name": true, "n": true, "force": true, "dry-run": true,
		"max-bytes": true, "json": true, "exclude": true, "x": true,
		"include-untracked": true, "parallel": true, "pre-process": true,
	},
	"source pack": {
		"name": true, "n": true, "max-bytes": true, "chunk": true,
		"exclude": true, "x": true, "pre-process": true,
	},
	"sync-pack": {
		"name": true, "n": true, "max-bytes": true, "chunk": true,
		"exclude": true, "x": true, "pre-process": true,
	},
	"generate-chat":   chatCommandLocalFlags(),
	"chat":            chatCommandLocalFlags(),
	"chat show":       chatShowCommandLocalFlags(),
	"chat-show":       chatShowCommandLocalFlags(),
	"app-create":      appCreateCommandLocalFlags(),
	"app create":      appCreateCommandLocalFlags(),
	"mindmap-create":  appCreateCommandLocalFlags(),
	"mindmap create":  appCreateCommandLocalFlags(),
	"create-audio":    audioCreateCommandLocalFlags(),
	"audio create":    audioCreateCommandLocalFlags(),
	"create-video":    videoCreateCommandLocalFlags(),
	"video create":    videoCreateCommandLocalFlags(),
	"create-slides":   slidesCreateCommandLocalFlags(),
	"deck create":     slidesCreateCommandLocalFlags(),
	"create-report":   selectorCommandLocalFlags(),
	"generate-report": reportCommandLocalFlags(),
	"research": {
		"mode": true, "md": true, "poll-ms": true, "import": true,
	},
	"deck download": {
		"id": true, "artifact-id": true, "format": true, "f": true,
		"output": true, "o": true,
	},
	"download slide-deck": {
		"id": true, "artifact-id": true, "format": true, "f": true,
		"output": true, "o": true,
	},
	"artifact update": {"name": true, "n": true},
	"update-artifact": {"name": true, "n": true},
}

func chatCommandLocalFlags() map[string]bool {
	return map[string]bool{
		"conversation": true, "c": true, "web": true, "prompt-file": true,
		"f": true, "history": true, "thinking": true, "reasoning": true,
		"thinking-jsonl": true, "verbose": true, "v": true,
		"citations": true, "resolve-citations": true,
		"source-ids": true, "source-match": true, "source-exclude": true,
		"label-ids": true, "label-match": true, "label-exclude": true,
	}
}

func reportCommandLocalFlags() map[string]bool {
	flags := chatCommandLocalFlags()
	flags["prompt"] = true
	flags["instructions"] = true
	flags["sections"] = true
	return flags
}

func chatShowCommandLocalFlags() map[string]bool {
	return map[string]bool{
		"thinking": true, "reasoning": true,
		"citations": true, "resolve-citations": true,
	}
}

func selectorCommandLocalFlags() map[string]bool {
	return map[string]bool{
		"source-ids": true, "source-match": true, "source-exclude": true,
		"label-ids": true, "label-match": true, "label-exclude": true,
	}
}

func commandHelpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--help" || arg == "-h" || arg == "-help" {
			return true
		}
	}
	return false
}

func packageGlobalOptions() globalOptions {
	return globalOptions{
		showVersion:       showVersion,
		experimental:      experimental,
		authToken:         authToken,
		cookies:           cookies,
		authUser:          authUser,
		debug:             debug,
		debugDumpPayload:  debugDumpPayload,
		debugParsing:      debugParsing,
		debugFieldMapping: debugFieldMapping,
		chromeProfile:     chromeProfile,
		chunkedResponse:   chunkedResponse,
		useDirectRPC:      useDirectRPC,
		skipSources:       skipSources,
		yes:               yes,
		jsonOutput:        jsonOutput,
	}
}

func applyGlobalOptions(opts globalOptions) {
	showVersion = opts.showVersion
	experimental = opts.experimental
	authToken = opts.authToken
	cookies = opts.cookies
	authUser = opts.authUser
	debug = opts.debug
	debugDumpPayload = opts.debugDumpPayload
	debugParsing = opts.debugParsing
	debugFieldMapping = opts.debugFieldMapping
	chromeProfile = opts.chromeProfile
	chunkedResponse = opts.chunkedResponse
	useDirectRPC = opts.useDirectRPC
	skipSources = opts.skipSources
	yes = opts.yes
	jsonOutput = opts.jsonOutput
}
