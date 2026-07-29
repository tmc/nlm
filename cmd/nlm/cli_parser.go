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
	authUserSet          bool
	debug                bool
	debugDumpPayload     bool
	debugParsing         bool
	debugFieldMapping    bool
	chromeProfile        string
	cdpURL               string
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
	sourceReadFormat     string
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
	citationExcerpt      excerptBudgetFlag
	hideCitationConf     offToggleFlag // --citation-confidence=off
	hideCitationSpans    offToggleFlag // --citation-spans=off
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
		cdpURL:        env("NLM_CDP_URL"),
		authToken:     env("NLM_AUTH_TOKEN"),
		cookies:       env("NLM_COOKIES"),
		authUser:      env("NLM_AUTHUSER"),
		debug:         env("NLM_DEBUG") == "true",
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
	flags.StringVar(&opts.authToken, "auth", opts.authToken, "auth token (or set NLM_AUTH_TOKEN)")
	flags.StringVar(&opts.cookies, "cookies", opts.cookies, "cookies for authentication (or set NLM_COOKIES)")
	flags.StringVar(&opts.authUser, "authuser", opts.authUser, "Google account index for multi-account profiles")
}

func parseInvocation(args []string, env func(string) string, stdout, stderr io.Writer) (invocation, error) {
	_ = stdout
	opts := defaultGlobalOptions(env)
	flags := newGlobalFlagSet(&opts)
	flagArgs, positional, stopCommandFlags := splitGlobalArgs(args, flags)
	inv := invocation{globals: opts}
	if err := flags.Parse(flagArgs); err != nil {
		inv.globals = opts
		return inv, fmt.Errorf("%w: %v", errBadArgs, err)
	}
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "authuser" {
			opts.authUserSet = true
		}
	})
	inv.globals = opts
	if opts.showVersion {
		inv.action = invocationVersion
		return inv, nil
	}
	if !stopCommandFlags {
		normalized, warningFlags, normalizeErr := normalizePreCommandFlags(positional)
		if normalizeErr != nil {
			inv.globals = opts
			return inv, normalizeErr
		}
		positional = normalized
		if len(warningFlags) > 0 {
			warnPreCommandFlags(stderr, positional, warningFlags)
		}
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

func splitGlobalArgs(args []string, flags *flag.FlagSet) ([]string, []string, bool) {
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
				return flagArgs, positional, true
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
		if !knownFlags[name] {
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
	return flagArgs, positional, false
}

func commandOwnsFlag(command *command, name string) bool {
	for _, spec := range commandFlagsForSurface(command.spec, command.surfaceSpec) {
		if spec.Name == name {
			return true
		}
		for _, alias := range spec.Aliases {
			if alias == name {
				return true
			}
		}
	}
	return false
}

// preCommandFlagGraceRelease marks the release that introduced warnings for
// command flags before the command path. Remove this compatibility path after
// the next release, as tracked by preCommandFlagRemovalIssue.
const (
	preCommandFlagGraceRelease = "2026-07-29"
	preCommandFlagRemovalIssue = "Phase 6 follow-up commit 5"
)

type preCommandFlagKind uint8

const (
	preCommandBoolFlag preCommandFlagKind = iota
	preCommandValueFlag
)

// preCommandFlagCompatibility is the exact set of command flag spellings and
// arities that registerGlobalFlags accepted before the command path before
// preCommandFlagGraceRelease.
var preCommandFlagCompatibility = map[string]preCommandFlagKind{
	"c":                   preCommandValueFlag,
	"cdp-url":             preCommandValueFlag,
	"chunk":               preCommandValueFlag,
	"chunked":             preCommandBoolFlag,
	"citation-confidence": preCommandBoolFlag,
	"citation-excerpt":    preCommandBoolFlag,
	"citation-excerpts":   preCommandBoolFlag,
	"citation-spans":      preCommandBoolFlag,
	"citations":           preCommandValueFlag,
	"conversation":        preCommandValueFlag,
	"direct-rpc":          preCommandBoolFlag,
	"dry-run":             preCommandBoolFlag,
	"f":                   preCommandValueFlag,
	"force":               preCommandBoolFlag,
	"history":             preCommandBoolFlag,
	"html":                preCommandBoolFlag,
	"import":              preCommandBoolFlag,
	"instructions":        preCommandValueFlag,
	"json":                preCommandBoolFlag,
	"label-exclude":       preCommandValueFlag,
	"label-ids":           preCommandValueFlag,
	"label-match":         preCommandValueFlag,
	"markdown":            preCommandBoolFlag,
	"max-bytes":           preCommandValueFlag,
	"md":                  preCommandBoolFlag,
	"mime":                preCommandValueFlag,
	"mime-type":           preCommandValueFlag,
	"mode":                preCommandValueFlag,
	"n":                   preCommandValueFlag,
	"name":                preCommandValueFlag,
	"poll-ms":             preCommandValueFlag,
	"profile":             preCommandValueFlag,
	"prompt":              preCommandValueFlag,
	"prompt-file":         preCommandValueFlag,
	"reasoning":           preCommandBoolFlag,
	"replace":             preCommandValueFlag,
	"resolve-citations":   preCommandBoolFlag,
	"sections":            preCommandValueFlag,
	"skip-sources":        preCommandBoolFlag,
	"source-exclude":      preCommandValueFlag,
	"source-ids":          preCommandValueFlag,
	"source-match":        preCommandValueFlag,
	"thinking":            preCommandBoolFlag,
	"thinking-jsonl":      preCommandBoolFlag,
	"v":                   preCommandBoolFlag,
	"verbose":             preCommandBoolFlag,
	"web":                 preCommandBoolFlag,
	"y":                   preCommandBoolFlag,
	"yes":                 preCommandBoolFlag,
}

type preCommandFlag struct {
	name    string
	display string
	args    []string
}

func normalizePreCommandFlags(args []string) ([]string, []string, error) {
	var flags []preCommandFlag
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			if len(flags) == 0 {
				return args, nil, nil
			}
			i++
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			break
		}
		name, hasValue := commandFlagName(arg)
		kind, ok := preCommandFlagCompatibility[name]
		if !ok {
			return args, nil, nil
		}
		flagArgs := []string{arg}
		if kind == preCommandValueFlag && !hasValue {
			if i+1 >= len(args) {
				return nil, nil, badArgsf("flag needs an argument: %s", arg)
			}
			i++
			flagArgs = append(flagArgs, args[i])
		}
		flags = append(flags, preCommandFlag{
			name:    name,
			display: commandFlagDisplay(arg),
			args:    flagArgs,
		})
		i++
	}
	if len(flags) == 0 || i >= len(args) {
		return args, nil, nil
	}

	commandName, command, commandArgs, ok := findCommand(args[i:])
	if !ok {
		return args, nil, nil
	}
	for _, flag := range flags {
		if commandOwnsFlag(command, flag.name) {
			continue
		}
		if flag.name == "f" {
			return nil, nil, badArgsf("command flag -f before the command path is ambiguous; move it after the command")
		}
		return nil, nil, badArgsf("flag %s is not valid for %q", flag.display, commandName)
	}

	pathLen := len(args[i:]) - len(commandArgs)
	normalized := append([]string(nil), args[i:i+pathLen]...)
	for _, flag := range flags {
		normalized = append(normalized, flag.args...)
	}
	normalized = append(normalized, commandArgs...)

	seen := make(map[string]bool)
	var warnings []string
	for _, flag := range flags {
		if seen[flag.name] {
			continue
		}
		seen[flag.name] = true
		warnings = append(warnings, flag.display)
	}
	return normalized, warnings, nil
}

func commandFlagName(arg string) (name string, hasValue bool) {
	name = strings.TrimLeft(arg, "-")
	if before, _, ok := strings.Cut(name, "="); ok {
		return before, true
	}
	return name, false
}

func commandFlagDisplay(arg string) string {
	if before, _, ok := strings.Cut(arg, "="); ok {
		return before
	}
	return arg
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func warnPreCommandFlags(stderr io.Writer, args, flags []string) {
	commandName, _, _, ok := findCommand(args)
	if !ok {
		return
	}
	if len(flags) == 1 {
		fmt.Fprintf(stderr, "nlm: warning: command flag %s before %q is deprecated; move it after the command path\n", flags[0], commandName)
		return
	}
	fmt.Fprintf(stderr, "nlm: warning: command flags %s before %q are deprecated; move them after the command path\n", strings.Join(flags, ", "), commandName)
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
}
