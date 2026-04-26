package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	runtimedebug "runtime/debug"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/gen/service"
	"github.com/tmc/nlm/internal/auth"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	intmethod "github.com/tmc/nlm/internal/method"
	"github.com/tmc/nlm/internal/nlmmcp"
	"github.com/tmc/nlm/internal/notebooklm/api"
	nlmsync "github.com/tmc/nlm/internal/sync"
	"golang.org/x/term"
)

// Global flags
var (
	showVersion        bool
	experimental       bool // surface experimental commands in help + allow them to run
	authToken          string
	cookies            string
	debug              bool
	debugDumpPayload   bool
	debugParsing       bool
	debugFieldMapping  bool
	chromeProfile      string
	mimeType           string
	chunkedResponse    bool   // Control rt=c parameter for chunked vs JSON array response
	useDirectRPC       bool   // Use direct RPC calls instead of orchestration service
	skipSources        bool   // Skip fetching sources for chat (useful when project is inaccessible)
	yes                bool   // Skip confirmation prompts
	sourceName         string // Custom name for added sources
	showChatHistory    bool   // Show previous chat conversation on start
	showThinking       bool   // Show thinking headers while streaming responses
	thinkingJSONL      bool   // Emit chat events (thinking/answer/citation/followup) as JSON-lines on stdout
	verbose            bool   // Show full thinking traces while streaming responses
	replaceSourceID    string // Source ID to replace when adding
	force              bool   // Force re-upload even if unchanged
	dryRun             bool   // Show what would change without uploading
	maxBytes           int    // Chunk threshold for sync
	jsonOutput         bool   // NDJSON output for sync
	packChunk          int    // 1-indexed chunk to emit (sync-pack); 0 = auto (single chunk) or list
	reportPrompt       string // Per-section prompt template for generate-report ({topic} replaced)
	reportInstructions string // Notebook instructions to set before generate-report
	reportSections     int    // Max sections for generate-report (0 = all)
	conversationID     string // Conversation ID to continue (generate-chat)
	useWebChat         bool   // Use most recent server-side conversation (generate-chat)
	citationMode       string // Citation rendering mode: off|block|overlay (default block-on-TTY)
	sourceIDsFlag      string // Comma-separated list, or "-" to read newline-delimited IDs from stdin
	sourceMatchFlag    string // Regex matched against source titles and UUIDs; unioned with --source-ids
	promptFile         string // Read prompt from file (nlm chat). "-" reads from stdin.
)

// ChatSession represents a persistent chat conversation
type ChatSession struct {
	NotebookID     string        `json:"notebook_id"`
	ConversationID string        `json:"conversation_id,omitempty"`
	Messages       []ChatMessage `json:"messages"`
	SeqNum         int           `json:"seq_num,omitempty"`          // Next sequence number for this session
	LastResponseID string        `json:"last_response_id,omitempty"` // ID of last assistant response (for threading)
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// ChatMessage represents a single message in the conversation.
// Local storage preserves transient stream data (reasoning, citations)
// that the server discards after generation completes.
type ChatMessage struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`

	// Conversation threading metadata.
	MessageID string `json:"message_id,omitempty"` // Server-assigned response ID
	SeqNum    int    `json:"seq_num,omitempty"`    // Sequence number within conversation

	// Transient stream data — only available locally, not from server history.
	Thinking  string         `json:"thinking,omitempty"`  // Reasoning traces from intermediate chunks
	Citations []api.Citation `json:"citations,omitempty"` // Source references from the response
}

func init() {
	flag.BoolVar(&showVersion, "version", false, "print nlm version and exit")
	flag.BoolVar(&experimental, "experimental", false, "enable experimental commands (also: NLM_EXPERIMENTAL=1)")
	flag.BoolVar(&debug, "debug", false, "enable debug output")
	flag.BoolVar(&debugDumpPayload, "debug-dump-payload", false, "dump raw JSON payload and exit (unix-friendly)")
	flag.BoolVar(&debugParsing, "debug-parsing", false, "show detailed protobuf parsing information")
	flag.BoolVar(&debugFieldMapping, "debug-field-mapping", false, "show how JSON array positions map to protobuf fields")
	flag.BoolVar(&chunkedResponse, "chunked", false, "use chunked response format (rt=c)")
	flag.BoolVar(&useDirectRPC, "direct-rpc", false, "use direct RPC calls for audio/video (bypasses orchestration service)")
	flag.BoolVar(&skipSources, "skip-sources", false, "skip fetching sources for chat (useful for testing)")
	flag.BoolVar(&yes, "yes", false, "skip confirmation prompts")
	flag.BoolVar(&yes, "y", false, "skip confirmation prompts")
	flag.StringVar(&chromeProfile, "profile", os.Getenv("NLM_BROWSER_PROFILE"), "Chrome profile to use")
	flag.StringVar(&authToken, "auth", os.Getenv("NLM_AUTH_TOKEN"), "auth token (or set NLM_AUTH_TOKEN)")
	flag.StringVar(&cookies, "cookies", os.Getenv("NLM_COOKIES"), "cookies for authentication (or set NLM_COOKIES)")
	flag.StringVar(&mimeType, "mime", "", "specify MIME type for content (e.g. 'application/pdf', 'text/plain')")
	flag.StringVar(&mimeType, "mime-type", "", "specify MIME type for content (alias for -mime)")
	flag.StringVar(&sourceName, "name", "", "custom name for added source")
	flag.StringVar(&sourceName, "n", "", "custom name for added source (shorthand)")
	flag.StringVar(&replaceSourceID, "replace", "", "source ID to replace (upload new, then delete old)")
	flag.BoolVar(&jsonOutput, "json", false, "output in JSON format")
	flag.BoolVar(&force, "force", false, "force re-upload even if unchanged (sync)")
	flag.BoolVar(&dryRun, "dry-run", false, "show what would change without uploading (sync)")
	flag.IntVar(&maxBytes, "max-bytes", 0, "chunk threshold in bytes (sync, default 5120000)")
	flag.IntVar(&packChunk, "chunk", 0, "1-indexed chunk to emit (sync-pack); omit to list or emit sole chunk")
	flag.StringVar(&reportPrompt, "prompt", "", "per-section prompt template for generate-report ({topic} is replaced)")
	flag.StringVar(&reportInstructions, "instructions", "", "set notebook instructions before generate-report")
	flag.IntVar(&reportSections, "sections", 0, "max sections to generate (generate-report, 0=all)")
	flag.StringVar(&conversationID, "conversation", "", "continue an existing conversation by ID (generate-chat prints the ID on first turn)")
	flag.StringVar(&conversationID, "c", "", "continue an existing conversation by ID (shorthand)")
	flag.BoolVar(&useWebChat, "web", false, "use the most recent server-side conversation (generate-chat)")
	flag.BoolVar(&showChatHistory, "history", false, "show previous chat conversation on start")
	flag.BoolVar(&showThinking, "thinking", false, "show thinking headers while streaming chat and generate-chat responses")
	flag.BoolVar(&showThinking, "reasoning", false, "show thinking headers while streaming chat and generate-chat responses")
	flag.BoolVar(&thinkingJSONL, "thinking-jsonl", false, "deprecated: equivalent to --citations=json --thinking; emits thinking+answer+citation+followup JSON-lines events")
	flag.BoolVar(&verbose, "verbose", false, "show full thinking traces while streaming chat and generate-chat responses")
	flag.BoolVar(&verbose, "v", false, "show full thinking traces while streaming responses (shorthand)")
	flag.StringVar(&citationMode, "citations", "", "citation rendering: off|block|stream|tail|overlay|json (default: stream on TTY, off when piped; json emits answer+citation JSON-lines)")
	flag.StringVar(&sourceIDsFlag, "source-ids", "", "focus on these source IDs (e.g. 'a,b,c' or '-' for newline-delimited stdin); applies to chat, report, and transform commands")
	flag.StringVar(&sourceMatchFlag, "source-match", "", "focus on sources whose title or UUID matches this regex (e.g. '^nlm internal/' or '^132af'); unioned with --source-ids")
	flag.StringVar(&promptFile, "prompt-file", "", "read prompt from file for one-shot chat ('-' reads stdin). Reliable for long/automated prompts.")
	flag.StringVar(&promptFile, "f", "", "read prompt from file for one-shot chat ('-' reads stdin) (shorthand)")
	flag.StringVar(&researchMode, "mode", "", "research mode: fast|deep (default: deep; used by nlm research)")
	flag.BoolVar(&researchMD, "md", false, "emit raw markdown report (nlm research; default is JSON-lines events)")
	flag.IntVar(&researchPollMs, "poll-ms", 0, "override research polling interval in milliseconds (default: 5000)")
	flag.BoolVar(&researchImport, "import", false, "after research completes, import the discovered sources into the notebook via LBwxtb BulkImportFromResearch")

	flag.Usage = printUsage
}

// reorderArgs moves known top-level flags that appear after the command name
// to before it, so "nlm rm -y <id>" works the same as "nlm -y rm <id>".
// Unknown flags (e.g. subcommand-specific flags like --cdp-url for auth)
// are left in positional order so the subcommand's FlagSet can parse them.
func reorderArgs() {
	if len(os.Args) < 3 {
		return
	}

	// Build sets of known top-level flags
	knownFlags := map[string]bool{}
	boolFlags := map[string]bool{}
	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		knownFlags[f.Name] = true
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			boolFlags[f.Name] = true
		}
	})

	var flags, positional []string
	i := 1 // skip os.Args[0] (program name)
	for i < len(os.Args) {
		arg := os.Args[i]
		if arg == "--" {
			positional = append(positional, os.Args[i:]...)
			break
		}
		if arg != "-" && strings.HasPrefix(arg, "-") {
			name := strings.TrimLeft(arg, "-")
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name = name[:eq]
			}
			if knownFlags[name] {
				if !boolFlags[name] && !strings.Contains(arg, "=") {
					// Non-bool flag needs a value. Only consume the next arg as
					// that value if it's not itself a command name or another
					// flag — otherwise treat the flag as switch-form and
					// rewrite to flag= so flag.Parse doesn't steal a positional
					// when reorder puts this flag before the command.
					// A bare "-" is a valid value (conventionally "read from
					// stdin"), not a flag, so consume it like any other value.
					if i+1 < len(os.Args) {
						next := os.Args[i+1]
						isFlag := strings.HasPrefix(next, "-") && next != "-"
						if !isCommandStart(next) && !isFlag {
							flags = append(flags, arg, next)
							i += 2
							continue
						}
					}
					flags = append(flags, arg+"=")
				} else {
					flags = append(flags, arg)
				}
			} else {
				// Unknown flag — leave as positional for subcommand parsing
				positional = append(positional, arg)
				// If it looks like it takes a value, pass that through too
				if !strings.Contains(arg, "=") && i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
					i++
					positional = append(positional, os.Args[i])
				}
			}
		} else {
			positional = append(positional, arg)
		}
		i++
	}

	os.Args = append([]string{os.Args[0]}, append(flags, positional...)...)
}

func main() {
	lockInteractiveAudioAppThreadIfNeeded(os.Args[1:])

	reorderArgs()
	flag.Parse()

	if showVersion {
		fmt.Println(versionString())
		return
	}

	if debug {
		fmt.Fprintf(os.Stderr, "nlm: debug mode enabled\n")
		if chromeProfile != "" {
			// Mask potentially sensitive profile names in debug output
			maskedProfile := chromeProfile
			if len(chromeProfile) > 8 {
				maskedProfile = chromeProfile[:4] + "****" + chromeProfile[len(chromeProfile)-4:]
			} else if len(chromeProfile) > 2 {
				maskedProfile = chromeProfile[:2] + "****"
			}
			fmt.Fprintf(os.Stderr, "nlm: using Chrome profile: %s\n", maskedProfile)
		}
	}

	// Load stored environment variables
	loadStoredEnv()

	// Set skip sources flag if specified
	if skipSources {
		os.Setenv("NLM_SKIP_SOURCES", "true")
		if debug {
			fmt.Fprintf(os.Stderr, "nlm: skipping source fetching for chat\n")
		}
	}

	// Set beprotojson debug options if requested
	if debugParsing || debugFieldMapping {
		beprotojson.SetGlobalDebugOptions(debugParsing, debugFieldMapping)
	}

	// Start auto-refresh manager if credentials exist
	startAutoRefreshIfEnabled()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "nlm: %s\n", friendlyError(err))
		code := exitCodeFor(err)
		if name := exitCodeName(code); name != "" {
			fmt.Fprintf(os.Stderr, "nlm: exit-class=%s (exit %d)\n", name, code)
		}
		os.Exit(code)
	}
}

// friendlyError rewrites the "API error <N> (<Type>): <msg>" format produced
// by *batchexecute.APIError into something a user can act on. It keeps the
// wrapping context (e.g. "get project: ...") so callers still see which
// operation failed. If err is not a batchexecute APIError the return value
// is err.Error() unchanged.
func friendlyError(err error) string {
	if errors.Is(err, api.ErrSourceCapReached) {
		return friendlyTypedError(err, api.ErrSourceCapReached, "notebook is at the source limit; remove unused sources before adding more")
	}
	if errors.Is(err, api.ErrSourceTooLarge) {
		return friendlyTypedError(err, api.ErrSourceTooLarge, "source exceeds the per-request size limit; split it, or use `nlm sync` / `nlm sync-pack` which chunk automatically")
	}
	var apiErr *batchexecute.APIError
	if !errors.As(err, &apiErr) {
		return err.Error()
	}
	// Strip the "API error <N> (<Type>): <msg>" suffix from the wrapped
	// error chain so the user sees "<outer context>: <friendly message>".
	full := err.Error()
	suffix := apiErr.Error()
	prefix := strings.TrimSuffix(full, suffix)
	prefix = strings.TrimRight(prefix, ": ")

	msg := friendlyAPIMessage(apiErr)
	if prefix == "" {
		return msg
	}
	return prefix + ": " + msg
}

func friendlyTypedError(err, target error, msg string) string {
	full := err.Error()

	var apiErr *batchexecute.APIError
	if errors.As(err, &apiErr) {
		full = strings.TrimSuffix(full, apiErr.Error())
		full = strings.TrimRight(full, ": ")
	}

	full = strings.TrimSuffix(full, target.Error())
	full = strings.TrimRight(full, ": ")

	if full == "" {
		return msg
	}
	return full + ": " + msg
}

// friendlyAPIMessage returns a human-readable description for an APIError.
// Prefers ErrorCode.Description (from the dictionary) over raw Message, and
// never surfaces the numeric code to the user.
func friendlyAPIMessage(apiErr *batchexecute.APIError) string {
	if apiErr.ErrorCode != nil && apiErr.ErrorCode.Description != "" {
		return apiErr.ErrorCode.Description
	}
	if apiErr.ErrorCode != nil && apiErr.ErrorCode.Message != "" {
		return apiErr.ErrorCode.Message
	}
	if apiErr.Message != "" {
		return apiErr.Message
	}
	return "request failed"
}

// isAuthCommand returns true if the command requires authentication
// validateArgs validates command arguments without requiring authentication

func run() error {
	if authToken == "" {
		authToken = os.Getenv("NLM_AUTH_TOKEN")
	}
	if cookies == "" {
		cookies = os.Getenv("NLM_COOKIES")
	}

	if debug {
		fmt.Fprintf(os.Stderr, "DEBUG: Auth token loaded: %v\n", authToken != "")
		fmt.Fprintf(os.Stderr, "DEBUG: Cookies loaded: %v\n", cookies != "")
		if authToken != "" {
			// Mask token for security - show only first 2 and last 2 chars for tokens > 8 chars
			var tokenDisplay string
			if len(authToken) <= 8 {
				tokenDisplay = strings.Repeat("*", len(authToken))
			} else {
				start := authToken[:2]
				end := authToken[len(authToken)-2:]
				tokenDisplay = start + strings.Repeat("*", len(authToken)-4) + end
			}
			fmt.Fprintf(os.Stderr, "DEBUG: Token: %s\n", tokenDisplay)
		}
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(exitBadArgs)
	}

	rawArgs := flag.Args()
	cmdName := rawArgs[0]

	// Handle help aliases.
	if helpAliases[cmdName] {
		flag.Usage()
		os.Exit(exitSuccess)
	}

	// Look up command in the table, preferring the longest multi-token match.
	cmdName, entry, args, ok := findCommand(rawArgs)
	if !ok {
		flag.Usage()
		os.Exit(exitBadArgs)
	}
	warnCompatibilityCommand(cmdName, entry)

	// Check for help flags in subcommand args.
	for _, a := range args {
		if a == "--help" || a == "-h" || a == "-help" {
			if entry.help != nil {
				entry.help(cmdName)
			} else {
				fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n  %s\n", cmdName, entry.argsUsage, entry.usage)
			}
			return nil
		}
	}

	// Validate arguments.
	if err := validateCommandArgs(entry, cmdName, args); err != nil {
		if errors.Is(err, errInteractiveAudioHelp) {
			return nil
		}
		return err
	}

	// Commands that don't need an API client run directly.
	if entry.noClient {
		return entry.run(nil, args)
	}

	// Check authentication.
	if !entry.noAuth && (authToken == "" || cookies == "") {
		fmt.Fprintf(os.Stderr, "nlm: Authentication required for '%s'. Run 'nlm auth' first, or export NLM_AUTH_TOKEN and NLM_COOKIES (see 'nlm auth --print-env').\n", cmdName)
		return fmt.Errorf("authentication required")
	}

	var opts []batchexecute.Option

	// Add debug option if enabled
	if debug {
		opts = append(opts, batchexecute.WithDebug(true))
	}

	// Add rt=c parameter if chunked response format is requested
	if chunkedResponse {
		opts = append(opts, batchexecute.WithURLParams(map[string]string{
			"rt": "c",
		}))
		if debug {
			fmt.Fprintf(os.Stderr, "DEBUG: Using chunked response format (rt=c)\n")
		}
	} else if debug {
		fmt.Fprintf(os.Stderr, "DEBUG: Using JSON array response format (no rt parameter)\n")
	}

	// Support HTTP recording for testing
	if recordingDir := os.Getenv("HTTPRR_RECORDING_DIR"); recordingDir != "" {
		// In recording mode, we would set up HTTP client options
		// This requires integration with httprr library
		if debug {
			fmt.Fprintf(os.Stderr, "DEBUG: HTTP recording enabled with directory: %s\n", recordingDir)
		}
	}

	// Silent retry is only safe when there is a cached browser profile we can
	// reuse. In env-var-only mode (fresh CI machine) the credentials are
	// fixed for this process lifetime and re-running browser auth cannot
	// help — surface the 401 immediately.
	maxAttempts := 1
	if hasCachedProfile() {
		maxAttempts = 2
	}

	for i := 0; i < maxAttempts; i++ {
		if i > 0 {
			fmt.Fprintln(os.Stderr, "nlm: authentication expired, refreshing credentials...")
		}

		client := api.New(authToken, cookies, opts...)
		if debug {
			client.SetDebug(true)
		}
		// Set authuser for multi-account support
		if v := os.Getenv("NLM_AUTHUSER"); v != "" {
			client.SetAuthUser(v)
		}
		// Set direct RPC flag if specified
		if useDirectRPC {
			client.SetUseDirectRPC(true)
			if debug {
				fmt.Fprintf(os.Stderr, "nlm: using direct RPC for audio/video operations\n")
			}
		}
		cmdErr := entry.run(client, args)
		if cmdErr == nil {
			if i > 0 {
				fmt.Fprintln(os.Stderr, "nlm: authentication refreshed successfully")
			}
			return nil
		} else if !isAuthenticationError(cmdErr) {
			return cmdErr
		}

		// Authentication error detected.
		if debug {
			fmt.Fprintf(os.Stderr, "nlm: detected authentication error: %v\n", cmdErr)
		}

		// Last attempt — surface an actionable message and return the
		// underlying error so callers still see the full server context.
		if i == maxAttempts-1 {
			fmt.Fprintln(os.Stderr, "nlm: session expired. Run `nlm auth` to refresh, or re-export NLM_AUTH_TOKEN / NLM_COOKIES.")
			return cmdErr
		}

		var authErr error
		if authToken, cookies, authErr = handleAuth(nil, debug); authErr != nil {
			fmt.Fprintf(os.Stderr, "nlm: authentication refresh failed: %v\n", authErr)
			fmt.Fprintln(os.Stderr, "nlm: session expired. Run `nlm auth` to refresh, or re-export NLM_AUTH_TOKEN / NLM_COOKIES.")
			return authErr
		}
	}
	return fmt.Errorf("nlm: authentication failed")
}

// isAuthenticationError checks if an error is related to authentication
func isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}

	// Check for batchexecute unauthorized error
	if errors.Is(err, batchexecute.ErrUnauthorized) {
		return true
	}

	// Check for common authentication error messages
	errorStr := strings.ToLower(err.Error())
	authKeywords := []string{
		"unauthenticated",
		"authentication",
		"unauthorized",
		"api error 16", // Google API authentication error
		"error 16",
		"status: 401",
		"status: 403",
		"session invalid",
		"invalid session",
		"session expired",
		"expired session",
		"login required",
		"auth required",
		"invalid credentials",
		"token expired",
		"expired token",
		"cookie invalid",
		"invalid cookie",
	}

	for _, keyword := range authKeywords {
		if strings.Contains(errorStr, keyword) {
			return true
		}
	}

	return false
}

// versionString returns a human-readable version line derived from
// runtime/debug.ReadBuildInfo. It prefers the module version (set by
// `go install module@tag`) and falls back to VCS metadata (commit sha +
// commit date) for source builds. The resulting format is:
//
//	nlm <version-or-sha> (<commit-date>)
//
// For builds without any VCS or module info it emits "nlm devel".
func versionString() string {
	info, ok := runtimedebug.ReadBuildInfo()
	if !ok {
		return "nlm devel"
	}
	version := info.Main.Version
	if version == "(devel)" {
		version = ""
	}
	var commit, commitDate string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
		case "vcs.time":
			commitDate = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if version == "" {
		if commit != "" {
			short := commit
			if len(short) > 12 {
				short = short[:12]
			}
			version = short
		} else {
			version = "devel"
		}
	}
	if dirty {
		version += "-dirty"
	}
	if commitDate != "" {
		return fmt.Sprintf("nlm %s (%s)", version, commitDate)
	}
	return fmt.Sprintf("nlm %s", version)
}

func runMCP(client *api.Client) error {
	info, ok := runtimedebug.ReadBuildInfo()
	version := "devel"
	if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	impl := &mcp.Implementation{
		Name:    "nlm",
		Version: version,
	}
	return nlmmcp.Run(context.Background(), client, impl)
}

// confirmAction prompts the user for confirmation unless --yes is set.
// When stdin is not a TTY and --yes was not passed, the function refuses
// rather than calling fmt.Scanln. Silently consuming piped input is the
// worst possible behavior for destructive operations: scripts that pipe
// source IDs into rm-source would have their input eaten by the prompt
// and the destructive action half-executed.
func confirmAction(prompt string) bool {
	if yes {
		return true
	}
	if !isTerminal(os.Stdin) {
		fmt.Fprintf(os.Stderr, "%s\nrefusing to prompt in non-interactive mode; pass -y to confirm\n", prompt)
		return false
	}
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	var response string
	fmt.Scanln(&response)
	return strings.HasPrefix(strings.ToLower(response), "y")
}

func confirmActionDefaultYes(prompt string) bool {
	if yes {
		return true
	}
	if !isTerminal(os.Stdin) {
		fmt.Fprintf(os.Stderr, "%s\nrefusing to prompt in non-interactive mode; pass -y to confirm\n", prompt)
		return false
	}
	fmt.Fprintf(os.Stderr, "%s [Y/n] ", prompt)
	var response string
	fmt.Scanln(&response)
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "" || strings.HasPrefix(response, "y")
}

// Notebook operations
func create(c *api.Client, title string) error {
	notebook, err := c.CreateProject(title, "📙")
	if err != nil {
		return err
	}
	fmt.Println(notebook.ProjectId)
	return nil
}

func remove(c *api.Client, id string) error {
	if !confirmAction(fmt.Sprintf("Are you sure you want to delete notebook %s?", id)) {
		return fmt.Errorf("operation cancelled")
	}
	return c.DeleteProjects([]string{id})
}

func renameNotebook(c *api.Client, notebookID, newTitle string) error {
	if newTitle == "" {
		return fmt.Errorf("provide a new title")
	}
	fmt.Fprintf(os.Stderr, "Renaming notebook %s...\n", notebookID)
	if _, err := c.MutateProject(notebookID, &pb.Project{Title: newTitle}); err != nil {
		return fmt.Errorf("rename notebook: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Renamed notebook to: %s\n", newTitle)
	return nil
}

func setNotebookEmoji(c *api.Client, notebookID, emoji string) error {
	if emoji == "" {
		return fmt.Errorf("provide an emoji")
	}
	fmt.Fprintf(os.Stderr, "Updating notebook %s emoji...\n", notebookID)
	if _, err := c.MutateProject(notebookID, &pb.Project{Emoji: emoji}); err != nil {
		return fmt.Errorf("set notebook emoji: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Set emoji to: %s\n", emoji)
	return nil
}

func setNotebookDescription(c *api.Client, notebookID, description string) error {
	fmt.Fprintf(os.Stderr, "Updating notebook %s description...\n", notebookID)
	if err := c.SetProjectDescription(notebookID, description); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Description updated.")
	return nil
}

func setNotebookCover(c *api.Client, notebookID string, coverID int) error {
	fmt.Fprintf(os.Stderr, "Setting notebook %s cover to preset %d...\n", notebookID, coverID)
	if err := c.SetProjectCover(notebookID, coverID); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Cover updated.")
	return nil
}

func uploadNotebookCoverImage(c *api.Client, notebookID, imagePath string) error {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}
	displayName := filepath.Base(imagePath)
	fmt.Fprintf(os.Stderr, "Uploading cover image %s (%d bytes) to notebook %s...\n", displayName, len(data), notebookID)
	if err := c.UploadProjectCoverImage(notebookID, displayName, data); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Cover image uploaded.")
	return nil
}

// Source operations
func listSources(c *api.Client, notebookID string) error {
	p, err := c.GetProject(notebookID)
	if err != nil {
		return fmt.Errorf("list sources: %w", err)
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "ID\tTITLE\tTYPE\tSTATUS\tLAST UPDATED")
	for _, src := range p.Sources {
		status := formatSourceStatus(src)
		lastUpdated := formatSourceTime(src)
		sourceType := formatSourceType(src)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			src.SourceId.GetSourceId(),
			strings.TrimSpace(src.Title),
			sourceType,
			status,
			lastUpdated,
		)
	}
	return flush()
}

func formatSourceStatus(src *pb.Source) string {
	// The NotebookLM UI has no disable-source affordance, so the proto's
	// SOURCE_STATUS_DISABLED (=2) never appears on real sources:
	//   - Settings.status ([1][2]) reads 2 on every healthy source — a
	//     server-side constant, not a user-facing state.
	//   - Metadata.status ([3][4]) reads 1 on healthy sources; 2 is
	//     unreachable through any UI path.
	// We render enabled/error/warnings based on what the server actually sends.
	if src.Metadata != nil && src.Metadata.Status == 1 {
		return "enabled"
	}
	if src.Settings != nil && src.Settings.Status == 3 {
		return "error"
	}
	if len(src.Warnings) > 0 {
		var codes []string
		for _, w := range src.Warnings {
			codes = append(codes, fmt.Sprintf("warn:%d", w.GetValue()))
		}
		return strings.Join(codes, ",")
	}
	return "ok"
}

func formatSourceType(src *pb.Source) string {
	if src.Metadata == nil {
		return "-"
	}
	switch src.Metadata.GetSourceType() {
	case 0, 1:
		return "-"
	case 2:
		return "gdoc"
	case 3:
		return "gslides"
	case 4:
		return "text"
	case 5:
		return "web"
	case 6:
		return "file"
	case 7:
		return "gsheets"
	case 8:
		return "note"
	case 9:
		return "youtube"
	default:
		return fmt.Sprintf("type:%d", int(src.Metadata.GetSourceType()))
	}
}

func formatSourceTime(src *pb.Source) string {
	if src.Metadata != nil && src.Metadata.LastModifiedTime != nil {
		return src.Metadata.LastModifiedTime.AsTime().Format(time.RFC3339)
	}
	if src.Metadata != nil && src.Metadata.LastUpdateTimeSeconds != nil {
		return time.Unix(int64(src.Metadata.LastUpdateTimeSeconds.GetValue()), 0).Format(time.RFC3339)
	}
	return "-"
}

func addSource(c *api.Client, notebookID, input string, opts sourceAddOptions) (string, error) {
	// Handle special input designators
	switch input {
	case "-": // stdin
		fmt.Fprintln(os.Stderr, "Reading from stdin...")
		name := "Pasted Text"
		if opts.Name != "" {
			name = opts.Name
		}
		var reader io.Reader = os.Stdin
		if opts.PreProcess != "" {
			fmt.Fprintf(os.Stderr, "Pre-processing stdin through: %s\n", opts.PreProcess)
			piped, err := runPreProcess(opts.PreProcess, "stdin", reader)
			if err != nil {
				return "", err
			}
			reader = piped
		}
		if opts.MIMEType != "" {
			fmt.Fprintf(os.Stderr, "Using specified MIME type: %s\n", opts.MIMEType)
			return c.AddSourceFromReader(notebookID, reader, name, opts.MIMEType)
		}
		return c.AddSourceFromReader(notebookID, reader, name)
	case "": // empty input
		return "", fmt.Errorf("input required (file, URL, or '-' for stdin)")
	}

	// Check if input is a URL
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		fmt.Fprintf(os.Stderr, "Adding source from URL: %s\n", input)
		if opts.PreProcess != "" {
			fmt.Fprintf(os.Stderr, "(--pre-process ignored for URL source)\n")
		}
		return c.AddSourceFromURL(notebookID, input)
	}

	// Try as local file
	if _, err := os.Stat(input); err == nil {
		fmt.Fprintf(os.Stderr, "Adding source from file: %s\n", input)
		name := filepath.Base(input)
		if opts.Name != "" {
			name = opts.Name
		}
		if opts.PreProcess != "" {
			fmt.Fprintf(os.Stderr, "Pre-processing file through: %s\n", opts.PreProcess)
			file, err := os.Open(input)
			if err != nil {
				return "", fmt.Errorf("open file: %w", err)
			}
			defer file.Close()
			piped, err := runPreProcess(opts.PreProcess, input, file)
			if err != nil {
				return "", err
			}
			if opts.MIMEType != "" {
				fmt.Fprintf(os.Stderr, "Using specified MIME type: %s\n", opts.MIMEType)
				return c.AddSourceFromReader(notebookID, piped, name, opts.MIMEType)
			}
			return c.AddSourceFromReader(notebookID, piped, name)
		}
		if opts.MIMEType != "" {
			fmt.Fprintf(os.Stderr, "Using specified MIME type: %s\n", opts.MIMEType)
			file, err := os.Open(input)
			if err != nil {
				return "", fmt.Errorf("open file: %w", err)
			}
			defer file.Close()
			return c.AddSourceFromReader(notebookID, file, name, opts.MIMEType)
		}
		if opts.Name != "" {
			// Use AddSourceFromReader to pass the custom name
			file, err := os.Open(input)
			if err != nil {
				return "", fmt.Errorf("open file: %w", err)
			}
			defer file.Close()
			return c.AddSourceFromReader(notebookID, file, name)
		}
		return c.AddSourceFromFile(notebookID, input)
	}

	// If it's not a URL or file, treat as direct text content
	fmt.Fprintln(os.Stderr, "Adding text content as source...")
	textName := "Text Source"
	if opts.Name != "" {
		textName = opts.Name
	}
	if opts.PreProcess != "" {
		fmt.Fprintf(os.Stderr, "Pre-processing text through: %s\n", opts.PreProcess)
		piped, err := runPreProcess(opts.PreProcess, "text", strings.NewReader(input))
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(piped)
		if err != nil {
			return "", fmt.Errorf("read pre-process output: %w", err)
		}
		return c.AddSourceFromText(notebookID, string(data), textName)
	}
	return c.AddSourceFromText(notebookID, input, textName)
}

// syncClientAdapter wraps *api.Client to satisfy nlmsync.Client.
type syncClientAdapter struct {
	client *api.Client
}

func (a *syncClientAdapter) ListSources(ctx context.Context, notebookID string) ([]nlmsync.Source, error) {
	p, err := a.client.GetProject(notebookID)
	if err != nil {
		return nil, err
	}
	var sources []nlmsync.Source
	for _, src := range p.Sources {
		sources = append(sources, nlmsync.Source{
			ID:    src.SourceId.GetSourceId(),
			Title: strings.TrimSpace(src.Title),
		})
	}
	return sources, nil
}

func (a *syncClientAdapter) AddSource(ctx context.Context, notebookID string, title string, r io.Reader) (string, error) {
	// Always use text path — sync content is txtar, never binary.
	// AddSourceFromReader would MIME-detect and route large content to
	// the binary resumable upload, which the server rejects for text.
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read source: %w", err)
	}
	return a.client.AddSourceFromText(notebookID, string(data), title)
}

func (a *syncClientAdapter) DeleteSources(ctx context.Context, notebookID string, ids []string) error {
	return a.client.DeleteSources(notebookID, ids)
}

func (a *syncClientAdapter) RenameSource(ctx context.Context, sourceID string, title string) error {
	_, err := a.client.MutateSource(sourceID, &pb.Source{Title: title})
	return err
}

func removeSource(c *api.Client, notebookID, sourceArg string) error {
	sourceIDs, err := resolveIDList(sourceArg)
	if err != nil {
		return fmt.Errorf("source IDs: %w", err)
	}
	if len(sourceIDs) == 0 {
		return fmt.Errorf("no source IDs provided")
	}

	var prompt string
	if len(sourceIDs) == 1 {
		prompt = fmt.Sprintf("Are you sure you want to remove source %s?", sourceIDs[0])
	} else {
		prompt = fmt.Sprintf("Are you sure you want to remove %d sources?", len(sourceIDs))
	}
	if !confirmActionDefaultYes(prompt) {
		return fmt.Errorf("operation cancelled")
	}

	if err := c.DeleteSources(notebookID, sourceIDs); err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	for _, id := range sourceIDs {
		fmt.Fprintf(os.Stderr, "Removed source %s from notebook %s\n", id, notebookID)
	}
	return nil
}

func renameSource(c *api.Client, sourceID, newName string) error {
	fmt.Fprintf(os.Stderr, "Renaming source %s to: %s\n", sourceID, newName)
	if _, err := c.MutateSource(sourceID, &pb.Source{
		Title: newName,
	}); err != nil {
		return fmt.Errorf("rename source: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Renamed source to: %s\n", newName)
	return nil
}

// Note operations
func createNote(c *api.Client, notebookID, title, content string) error {
	fmt.Fprintf(os.Stderr, "Creating note in notebook %s...\n", notebookID)
	if _, err := c.CreateNote(notebookID, title, content); err != nil {
		return fmt.Errorf("create note: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Created note: %s\n", title)
	return nil
}

func updateNote(c *api.Client, notebookID, noteID, content, title string) error {
	fmt.Fprintf(os.Stderr, "Updating note %s...\n", noteID)
	if _, err := c.MutateNote(notebookID, noteID, content, title); err != nil {
		return fmt.Errorf("update note: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Updated note: %s\n", title)
	return nil
}

func removeNote(c *api.Client, notebookID, noteID string) error {
	if !confirmAction(fmt.Sprintf("Are you sure you want to remove note %s?", noteID)) {
		return fmt.Errorf("operation cancelled")
	}

	if err := c.DeleteNotes(notebookID, []string{noteID}); err != nil {
		return fmt.Errorf("remove note: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Removed note: %s\n", noteID)
	return nil
}

// Note operations
func listNotes(c *api.Client, notebookID string) error {
	notes, err := c.GetNotes(notebookID)
	if err != nil {
		return fmt.Errorf("list notes: %w", err)
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "ID\tTITLE\tCONTENT PREVIEW")
	for _, note := range notes {
		content := note.GetRichText()
		if content == "" {
			content = note.GetContentText()
		}
		// Strip HTML/markdown for preview, collapse whitespace.
		content = strings.Join(strings.Fields(content), " ")
		if len(content) > 80 {
			content = content[:77] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", note.GetNoteId(), note.GetTitle(), content)
	}
	return flush()
}

func readNote(c *api.Client, notebookID, noteID string) error {
	notes, err := c.GetNotes(notebookID)
	if err != nil {
		return fmt.Errorf("get notes: %w", err)
	}
	for _, note := range notes {
		if note.GetNoteId() == noteID {
			content := note.GetRichText()
			if content == "" {
				content = note.GetContentText()
			}
			fmt.Printf("# %s\n\n%s\n", note.GetTitle(), content)
			return nil
		}
	}
	return fmt.Errorf("note %s not found", noteID)
}

// Audio operations
func getAudioOverview(c *api.Client, projectID string) error {
	fmt.Fprintf(os.Stderr, "Fetching audio overview...\n")

	result, err := c.GetAudioOverview(projectID)
	if err != nil {
		return fmt.Errorf("get audio overview: %w", err)
	}

	if !result.IsReady {
		fmt.Fprintln(os.Stderr, "Audio overview is not ready yet. Try again in a few moments.")
		return nil
	}

	fmt.Printf("Audio Overview:\n")
	fmt.Printf("  Title: %s\n", result.Title)
	fmt.Printf("  ID: %s\n", result.AudioID)
	fmt.Printf("  Ready: %v\n", result.IsReady)

	// Optionally save the audio file
	if result.AudioData != "" {
		audioData, err := result.GetAudioBytes()
		if err != nil {
			return fmt.Errorf("decode audio data: %w", err)
		}

		filename := fmt.Sprintf("audio_overview_%s.wav", result.AudioID)
		if err := os.WriteFile(filename, audioData, 0644); err != nil {
			return fmt.Errorf("save audio file: %w", err)
		}
		fmt.Printf("  Saved audio to: %s\n", filename)
	}

	return nil
}

func deleteAudioOverview(c *api.Client, notebookID string) error {
	if !confirmAction("Are you sure you want to delete the audio overview?") {
		return fmt.Errorf("operation cancelled")
	}

	if err := c.DeleteAudioOverview(notebookID); err != nil {
		return fmt.Errorf("delete audio overview: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Deleted audio overview")
	return nil
}

func shareAudioOverview(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating share link...\n")
	return shareNotebook(c, notebookID)
}

// Generation operations
func generateNotebookGuide(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating notebook guide...\n")
	guide, err := c.GenerateNotebookGuide(notebookID)
	if err != nil {
		return fmt.Errorf("generate guide: %w", err)
	}
	fmt.Printf("%s\n", guide.Content)
	return nil
}

// sourceGuideCacheDir returns the on-disk cache directory for per-source
// guides, creating it on first use. Guides are cached because tr032e is a
// generate call (see --force to re-populate).
func sourceGuideCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "nlm", "source-guides")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// loadCachedSourceGuide returns the cached guide for sourceID, or
// (nil, nil) on cache miss.
func loadCachedSourceGuide(sourceID string) (*api.SourceGuide, error) {
	dir, err := sourceGuideCacheDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, sourceID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var g api.SourceGuide
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func saveCachedSourceGuide(sourceID string, g *api.SourceGuide) error {
	dir, err := sourceGuideCacheDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sourceID+".json"), data, 0o644)
}

func generateSourceGuides(c *api.Client, sourceIDs []string) error {
	enc := json.NewEncoder(os.Stdout)
	for i, sourceID := range sourceIDs {
		var guide *api.SourceGuide
		if !force {
			cached, err := loadCachedSourceGuide(sourceID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cache read %s: %v\n", sourceID, err)
			}
			guide = cached
		}
		if guide == nil {
			fmt.Fprintf(os.Stderr, "Generating source guide for %s...\n", sourceID)
			g, err := c.GenerateSourceGuide(sourceID)
			if err != nil {
				return fmt.Errorf("generate source guide %s: %w", sourceID, err)
			}
			guide = g
			if err := saveCachedSourceGuide(sourceID, guide); err != nil {
				fmt.Fprintf(os.Stderr, "cache write %s: %v\n", sourceID, err)
			}
		}
		if jsonOutput {
			type envelope struct {
				SourceID  string   `json:"source_id"`
				Summary   string   `json:"summary"`
				KeyTopics []string `json:"key_topics"`
			}
			if err := enc.Encode(envelope{SourceID: sourceID, Summary: guide.Summary, KeyTopics: guide.KeyTopics}); err != nil {
				return err
			}
			continue
		}
		if len(sourceIDs) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("── %s ──\n", sourceID)
		}
		if guide.Summary != "" {
			fmt.Println(guide.Summary)
		}
		if len(guide.KeyTopics) > 0 {
			fmt.Println()
			fmt.Println(strings.Join(guide.KeyTopics, ", "))
		}
	}
	return nil
}

func actOnSourcesMindmap(c *api.Client, notebookID string, sourceIDs []string) error {
	fmt.Fprintf(os.Stderr, "Generating interactive mindmap...\n")
	content, err := c.ActOnSources(notebookID, "interactive_mindmap", sourceIDs)
	if err != nil {
		return fmt.Errorf("generate mindmap: %w", err)
	}
	if content != "" {
		fmt.Print(content)
	}
	fmt.Fprintf(os.Stderr, "Mindmap also saved as note — use 'nlm notes' to retrieve.\n")
	return nil
}

func actOnSources(c *api.Client, notebookID string, action string, sourceIDs []string) error {
	actionName := map[string]string{
		"rephrase":            "Rephrasing",
		"expand":              "Expanding",
		"summarize":           "Summarizing",
		"critique":            "Critiquing",
		"brainstorm":          "Brainstorming",
		"verify":              "Verifying",
		"explain":             "Explaining",
		"outline":             "Creating outline",
		"study_guide":         "Generating study guide",
		"faq":                 "Generating FAQ",
		"briefing_doc":        "Creating briefing document",
		"interactive_mindmap": "Generating interactive mindmap",
		"timeline":            "Creating timeline",
		"table_of_contents":   "Generating table of contents",
	}[action]

	if actionName == "" {
		actionName = "Processing"
	}

	fmt.Fprintf(os.Stderr, "%s content from sources...\n", actionName)
	content, err := c.ActOnSources(notebookID, action, sourceIDs)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.ToLower(actionName), err)
	}
	if content != "" {
		fmt.Print(content)
	}
	return nil
}

func createAudioOverview(c *api.Client, projectID string, instructions string) error {
	// NLM limits to one audio overview per notebook. Check for existing.
	existing, _ := c.ListAudioOverviews(projectID)
	if len(existing) > 0 {
		if yes {
			fmt.Fprintf(os.Stderr, "Existing audio overview found. Deleting before creating new one...\n")
			if err := c.DeleteAudioOverview(projectID); err != nil {
				return fmt.Errorf("delete existing audio: %w", err)
			}
			// Wait for server-side propagation of delete
			fmt.Fprintf(os.Stderr, "Waiting for delete to propagate...\n")
			time.Sleep(3 * time.Second)
		} else {
			fmt.Fprintf(os.Stderr, "Notebook already has an audio overview. Use -y to replace it, or 'nlm audio-rm %s' first.\n", projectID)
			return fmt.Errorf("existing audio overview")
		}
	}

	fmt.Fprintf(os.Stderr, "Creating audio overview for notebook %s...\n", projectID)
	fmt.Fprintf(os.Stderr, "Instructions: %s\n", instructions)

	result, err := c.CreateAudioOverview(projectID, instructions)
	if err != nil {
		return fmt.Errorf("create audio overview: %w", err)
	}

	if !result.IsReady {
		fmt.Fprintln(os.Stderr, "Audio overview creation started. Use 'nlm audio-get' to check status.")
		return nil
	}

	// If the result is immediately ready (unlikely but possible)
	fmt.Fprintf(os.Stderr, "Audio overview created:\n")
	fmt.Printf("  Title: %s\n", result.Title)
	fmt.Printf("  ID: %s\n", result.AudioID)

	// Save audio file if available
	if result.AudioData != "" {
		audioData, err := result.GetAudioBytes()
		if err != nil {
			return fmt.Errorf("decode audio data: %w", err)
		}

		filename := fmt.Sprintf("audio_overview_%s.wav", result.AudioID)
		if err := os.WriteFile(filename, audioData, 0644); err != nil {
			return fmt.Errorf("save audio file: %w", err)
		}
		fmt.Printf("  Saved audio to: %s\n", filename)
	}

	return nil
}

func heartbeat(c *api.Client) error {
	return nil
}

// New orchestration service functions

// Analytics and featured projects.
//
// BROKEN — proto contract does not match wire. AUrzMb returns a repeated
// time-series (metric_id + ~30 daily buckets per metric), not the scalar
// counts the generated ProjectAnalytics proto expects. Fields below read
// arbitrary bytes from the time-series encoding; do not trust them.
// Fixture: internal/notebooklm/api/testdata/AUrzMb_analytics_response.json.
// Gated behind --experimental until a proper MetricSeries proto + UX land.
func getAnalytics(c *api.Client, projectID string) error {
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)
	resp, err := orchClient.GetProjectAnalytics(context.Background(), &pb.GetProjectAnalyticsRequest{
		ProjectId: projectID,
	})
	if err != nil {
		return fmt.Errorf("get analytics: %w", err)
	}
	fmt.Printf("Project Analytics for %s:\n", projectID)
	fmt.Printf("  Sources: %d\n", int32Value(resp.GetSourceCount()))
	fmt.Printf("  Notes: %d\n", int32Value(resp.GetNoteCount()))
	fmt.Printf("  Audio Overviews: %d\n", int32Value(resp.GetAudioOverviewCount()))
	fmt.Fprintln(os.Stderr, "warning: analytics output is unreliable; wire returns time-series metrics, not scalar counts")

	return nil
}

func int32Value(v interface{ GetValue() int32 }) int32 {
	if v == nil {
		return 0
	}
	return v.GetValue()
}

func listFeaturedProjects(c *api.Client) error {
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)
	resp, err := orchClient.ListFeaturedProjects(context.Background(), &pb.ListFeaturedProjectsRequest{})
	if err != nil {
		return fmt.Errorf("list featured projects: %w", err)
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "ID\tTITLE\tDESCRIPTION")

	for _, project := range resp.Projects {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			project.ProjectId,
			featuredProjectTitle(project),
			featuredProjectDescription(project))
	}
	return flush()
}

func featuredProjectTitle(project *pb.FeaturedProject) string {
	return collapseWhitespace(strings.TrimSpace(strings.TrimSpace(project.Emoji) + " " + project.Title))
}

func featuredProjectDescription(project *pb.FeaturedProject) string {
	if desc := collapseWhitespace(project.GetPresentation().GetDescription()); desc != "" {
		return desc
	}
	if n := len(project.GetSources()); n > 0 {
		return fmt.Sprintf("%d sources", n)
	}
	return ""
}

// Enhanced source operations
//
// CheckSourceFreshness (yR9Yof) and RefreshSource (FLmJqe) are
// Google-Drive-only in the web UI. The server accepts any source id on
// the wire but returns meaningful results only for Drive sources;
// non-Drive ids are rejected with "One or more arguments are invalid".
// When a notebook-id is available we gate client-side and emit a clear
// error before dispatch; otherwise we pass through and surface the
// server error as-is.
func refreshSource(c *api.Client, notebookID, sourceID string) error {
	if err := assertDriveSource(c, notebookID, sourceID); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Refreshing source %s...\n", sourceID)
	source, err := c.RefreshSource(notebookID, sourceID)
	if err != nil {
		return fmt.Errorf("refresh source: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Refreshed source: %s\n", source.Title)
	return nil
}

func checkSourceFreshness(c *api.Client, sourceID, notebookID string) error {
	if notebookID != "" {
		if err := assertDriveSource(c, notebookID, sourceID); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(os.Stderr, "note: pass notebook-id as the second argument to enable client-side Drive-source validation")
	}
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)
	req := &pb.CheckSourceFreshnessRequest{SourceId: sourceID}
	fmt.Fprintf(os.Stderr, "Checking source %s...\n", sourceID)
	resp, err := orchClient.CheckSourceFreshness(context.Background(), req)
	if err != nil {
		return fmt.Errorf("check source: %w", err)
	}
	if resp.IsFresh {
		fmt.Printf("Source is up to date")
	} else {
		fmt.Printf("Source needs refresh")
	}
	if resp.LastChecked != nil {
		fmt.Printf(" (last checked: %s)", resp.LastChecked.AsTime().Format(time.RFC3339))
	}
	fmt.Println()
	return nil
}

// assertDriveSource returns a precondition error if the source lives in
// notebookID but is not a Google-Drive source type. A lookup failure is
// treated as non-fatal — the caller continues and lets the server
// error speak for itself.
func assertDriveSource(c *api.Client, notebookID, sourceID string) error {
	project, err := c.GetProject(notebookID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "note: could not verify source type (%v); dispatching anyway\n", err)
		return nil
	}
	for _, src := range project.Sources {
		if src.SourceId.GetSourceId() != sourceID {
			continue
		}
		st := src.Metadata.GetSourceType()
		switch st {
		case pb.SourceType_SOURCE_TYPE_GOOGLE_DOCS,
			pb.SourceType_SOURCE_TYPE_GOOGLE_SLIDES,
			pb.SourceType_SOURCE_TYPE_GOOGLE_SHEETS:
			return nil
		}
		return fmt.Errorf("%w: refresh/freshness is Google-Drive-only; source %s is %s", errBadArgs, sourceID, st)
	}
	fmt.Fprintf(os.Stderr, "note: source %s not found in notebook %s; dispatching anyway\n", sourceID, notebookID)
	return nil
}

func discoverSources(c *api.Client, projectID, query string) error {
	fmt.Fprintf(os.Stderr, "DiscoverSources is deprecated upstream; using deep research workflow instead.\n")
	if err := runDeepResearch(c, projectID, query, currentResearchOptions()); err == nil {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Deep research is unavailable; falling back to notebook suggestions.\n")
	res, err := streamChatResponse(c, api.ChatRequest{
		ProjectID: projectID,
		Prompt:    fmt.Sprintf("Suggest sources to add for this query: %s. Respond with a short bullet list of specific documents, sites, or search directions.", query),
	}, currentChatRenderOptions())
	if err != nil {
		return fmt.Errorf("discover sources fallback: %w", err)
	}
	if res.Answer == "" {
		fmt.Println("(No source suggestions returned)")
		return nil
	}
	fmt.Println()
	return nil
}

// Artifact management
func getArtifact(c *api.Client, artifactID string) error {
	artifact, err := c.GetArtifact(artifactID)
	if err != nil {
		return fmt.Errorf("get artifact: %w", err)
	}

	fmt.Printf("Artifact: %s\n", artifact.ArtifactId)
	fmt.Printf("Project:  %s\n", artifact.ProjectId)
	fmt.Printf("Type:     %s\n", artifact.Type.String())
	fmt.Printf("State:    %s\n", artifact.State.String())

	if len(artifact.Sources) > 0 {
		fmt.Printf("Sources:  %d\n", len(artifact.Sources))
		for _, src := range artifact.Sources {
			id := src.SourceId.GetSourceId()
			if len(src.TextFragments) > 0 {
				fmt.Printf("  %s (%d fragments)\n", id, len(src.TextFragments))
			} else {
				fmt.Printf("  %s\n", id)
			}
		}
	}

	// Type-specific content
	if report := artifact.TailoredReport; report != nil {
		if report.Title != "" {
			fmt.Printf("\nReport: %s\n", report.Title)
		}
		if report.Content != "" {
			fmt.Printf("\n%s\n", report.Content)
		}
		for i, section := range report.Sections {
			fmt.Printf("\n## %d. %s\n\n%s\n", i+1, section.Title, section.Content)
		}
	}

	if note := artifact.Note; note != nil {
		fmt.Printf("\nNote: %s (source: %s)\n", note.GetTitle(), note.GetSourceId().GetSourceId())
	}

	if app := artifact.App; app != nil {
		fmt.Printf("\nApp: %s\n", app.Name)
		if app.Description != "" {
			fmt.Printf("  %s\n", app.Description)
		}
		if app.AppId != "" {
			fmt.Printf("  ID: %s\n", app.AppId)
		}
	}

	if audio := artifact.AudioOverview; audio != nil {
		fmt.Printf("\nAudio: status=%s\n", audio.Status)
		if audio.Instructions != "" {
			fmt.Printf("  Instructions: %s\n", audio.Instructions)
		}
	}

	if video := artifact.VideoOverview; video != nil {
		data, err := json.MarshalIndent(video, "", "  ")
		if err == nil {
			fmt.Printf("\nVideo:\n%s\n", string(data))
		}
	}

	return nil
}

func listArtifacts(c *api.Client, projectID string) error {
	artifacts, err := c.ListArtifacts(projectID)
	if err != nil {
		return fmt.Errorf("list artifacts: %w", err)
	}
	return displayArtifacts(artifacts)
}

// displayArtifacts shows artifacts in a formatted table
func displayArtifacts(artifacts []*pb.Artifact) error {

	if len(artifacts) == 0 {
		fmt.Fprintln(os.Stderr, "No artifacts found in project.")
		return nil
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "ID\tTYPE\tSTATE\tSOURCES")

	for _, artifact := range artifacts {
		sourceCount := fmt.Sprintf("%d", len(artifact.Sources))

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			artifact.ArtifactId,
			artifact.Type.String(),
			artifact.State.String(),
			sourceCount)
	}
	return flush()
}

func renameArtifact(c *api.Client, artifactID, newTitle string) error {
	fmt.Fprintf(os.Stderr, "Renaming artifact %s to '%s'...\n", artifactID, newTitle)

	artifact, err := c.RenameArtifact(artifactID, newTitle)
	if err != nil {
		return fmt.Errorf("rename artifact: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Artifact renamed successfully")
	fmt.Printf("ID: %s\n", artifact.ArtifactId)
	fmt.Printf("New Title: %s\n", newTitle)

	return nil
}

func deleteArtifact(c *api.Client, artifactID string) error {
	if !confirmAction(fmt.Sprintf("Are you sure you want to delete artifact %s?", artifactID)) {
		return fmt.Errorf("operation cancelled")
	}
	if err := c.DeleteArtifact(artifactID); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Deleted artifact: %s\n", artifactID)
	return nil
}

// ANSI escape codes for muted/grey thinking output.
const (
	ansiDim   = "\033[2m"  // dim
	ansiGrey  = "\033[90m" // bright black (grey)
	ansiReset = "\033[0m"
)

// citationRenderMode controls how Citation data is surfaced in the CLI.
type citationRenderMode int

const (
	citationModeOff     citationRenderMode = iota // Suppress the trailing Sources block entirely.
	citationModeBlock                             // Stream answer normally; print a trailing Sources block.
	citationModeStream                            // Stream live; trailing footer lists citations with their char ranges.
	citationModeTail                              // Stream live, hold a bounded tail window; splice inline superscripts where possible.
	citationModeOverlay                           // Buffer the whole answer; at Finish, splice inline superscripts at exact positions.
	citationModeJSON                              // Emit answer deltas and citations as JSON-lines events on stdout.
)

// resolveCitationMode maps the user-facing --citations flag to a mode.
// Empty flag defaults to stream when stdout is a TTY, off when piped.
func resolveCitationMode(flag string, outIsTTY bool) citationRenderMode {
	switch strings.ToLower(flag) {
	case "off", "none":
		return citationModeOff
	case "block":
		return citationModeBlock
	case "stream", "inline-footer":
		return citationModeStream
	case "tail":
		return citationModeTail
	case "overlay", "footnote":
		return citationModeOverlay
	case "json", "jsonl":
		return citationModeJSON
	}
	if outIsTTY {
		return citationModeStream
	}
	return citationModeOff
}

// snapToWordBoundary advances pos forward within text until it lies after a
// word character and before a non-word character (or at end-of-string).
// This avoids splicing citation markers mid-word when EndChar lands inside
// one. Walks at most 32 bytes; if no boundary found, returns the original pos.
func snapToWordBoundary(text string, pos int) int {
	if pos < 0 {
		return 0
	}
	if pos >= len(text) {
		return len(text)
	}
	// If we're already at a boundary (next byte is non-word), keep it.
	if !isWordByte(text[pos]) {
		return pos
	}
	// Scan forward for a non-word byte.
	end := pos + 32
	if end > len(text) {
		end = len(text)
	}
	for i := pos; i < end; i++ {
		if !isWordByte(text[i]) {
			return i
		}
	}
	return pos
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_' || b == '`' || b == '\'' || b == '"'
}

// defaultTailWindow is the max byte-length of text held back in tail mode
// for late-arriving citation annotation.
const defaultTailWindow = 512

type chatStreamRenderer struct {
	out                  io.Writer
	status               io.Writer
	showThinking         bool
	verbose              bool
	jsonl                bool // when true, emit typed JSON-lines events on r.out instead of human output
	jsonlIncludeThinking bool // when true, thinking chunks are emitted as JSON-lines events (otherwise skipped)
	citationMode         citationRenderMode
	tailWindow           int                          // tail mode: max bytes held back for splicing
	resolveTitle         func(sourceID string) string // optional; returns "" if unknown
	lastThinkingLen      int
	answerBuf            strings.Builder
	thinking             string
	citations            []api.Citation
	followUps            []string

	// tail-mode bookkeeping
	flushedLen int // absolute cumulative-answer offset of bytes already written to r.out

	// jsonl bookkeeping: last emitted absolute answer offset so we only
	// emit delta text per event, and track which citations have been emitted.
	jsonlAnswerEmitted int
	jsonlThinkingSeen  string
	jsonlCitationsSeen int
}

func newChatStreamRenderer(out, status io.Writer, showThinking, verbose bool, mode citationRenderMode) *chatStreamRenderer {
	return &chatStreamRenderer{
		out:          out,
		status:       status,
		showThinking: showThinking,
		verbose:      verbose,
		citationMode: mode,
		tailWindow:   defaultTailWindow,
	}
}

func (r *chatStreamRenderer) WriteChunk(chunk api.ChatChunk) {
	if r.jsonl {
		r.writeChunkJSONL(chunk)
		return
	}
	switch chunk.Phase {
	case api.ChatChunkThinking:
		// Thinking chunks arrive as full cumulative snapshots, not deltas.
		// Replace instead of appending to avoid quadratic growth.
		r.thinking = chunk.Text
		if !r.showThinking {
			return
		}
		if r.verbose {
			r.clearThinkingLine()
			fmt.Fprintf(r.status, "%s%s%s\n", ansiGrey, chunk.Text, ansiReset)
			return
		}
		r.clearThinkingLine()
		display := strings.TrimPrefix(strings.TrimSuffix(chunk.Header, "**"), "**")
		line := fmt.Sprintf("%s  [thinking] %s%s", ansiGrey, display, ansiReset)
		fmt.Fprint(r.status, line)
		r.lastThinkingLen = len("  [thinking] ") + len(display)
	case api.ChatChunkAnswer:
		r.clearThinkingLine()
		r.answerBuf.WriteString(chunk.Text)
		switch r.citationMode {
		case citationModeOverlay:
			// Hold everything until Finish so we can splice precisely.
		case citationModeTail:
			// Flush any bytes that have aged out of the tail window.
			buf := r.answerBuf.String()
			stable := len(buf) - r.tailWindow
			if stable > r.flushedLen {
				fmt.Fprint(r.out, buf[r.flushedLen:stable])
				r.flushedLen = stable
			}
		default:
			// block / stream / off — live streaming.
			fmt.Fprint(r.out, chunk.Text)
			r.flushedLen += len(chunk.Text)
		}
		if len(chunk.Citations) > 0 {
			r.citations = chunk.Citations
		}
		if len(chunk.FollowUps) > 0 {
			r.followUps = chunk.FollowUps
		}
	}
}

// writeChunkJSONL emits chat-stream events as newline-delimited JSON on r.out.
// Answer text is emitted as deltas so shell consumers can pipeline without
// waiting for the full response. Thinking chunks arrive as cumulative
// snapshots; only emit when the snapshot differs from what we last emitted.
func (r *chatStreamRenderer) writeChunkJSONL(chunk api.ChatChunk) {
	switch chunk.Phase {
	case api.ChatChunkThinking:
		r.thinking = chunk.Text
		if !r.jsonlIncludeThinking {
			return
		}
		if chunk.Text == r.jsonlThinkingSeen {
			return
		}
		r.jsonlThinkingSeen = chunk.Text
		r.emitJSONLEvent(map[string]any{
			"phase": "thinking",
			"text":  chunk.Text,
		})
	case api.ChatChunkAnswer:
		r.answerBuf.WriteString(chunk.Text)
		if chunk.Text != "" {
			r.emitJSONLEvent(map[string]any{
				"phase": "answer",
				"text":  chunk.Text,
			})
			r.jsonlAnswerEmitted += len(chunk.Text)
		}
		if len(chunk.Citations) > 0 {
			r.citations = chunk.Citations
			for i := r.jsonlCitationsSeen; i < len(chunk.Citations); i++ {
				c := chunk.Citations[i]
				r.emitJSONLEvent(map[string]any{
					"phase":      "citation",
					"index":      c.SourceIndex,
					"source_id":  c.SourceID,
					"title":      c.Title,
					"start_char": c.StartChar,
					"end_char":   c.EndChar,
					"confidence": c.Confidence,
				})
			}
			r.jsonlCitationsSeen = len(chunk.Citations)
		}
		if len(chunk.FollowUps) > 0 {
			r.followUps = chunk.FollowUps
		}
	}
}

func (r *chatStreamRenderer) emitJSONLEvent(event map[string]any) {
	buf, err := json.Marshal(event)
	if err != nil {
		fmt.Fprintf(r.status, "nlm: thinking-jsonl marshal failed: %v\n", err)
		return
	}
	fmt.Fprintln(r.out, string(buf))
}

func (r *chatStreamRenderer) Finish() {
	if r.jsonl {
		for _, f := range r.followUps {
			r.emitJSONLEvent(map[string]any{
				"phase": "followup",
				"text":  f,
			})
		}
		r.emitJSONLEvent(map[string]any{
			"phase": "done",
		})
		return
	}
	r.clearThinkingLine()
	switch r.citationMode {
	case citationModeOverlay:
		r.emitOverlay()
	case citationModeTail:
		r.emitTail()
	case citationModeStream:
		r.printCitationsFooter()
	case citationModeBlock:
		r.printCitationsBlock()
	}
	r.printFollowUps()
}

// emitOverlay writes the full answer with superscript markers spliced in at
// the citation char ranges, then prints a numbered footnote block.
func (r *chatStreamRenderer) emitOverlay() {
	answer := r.answerBuf.String()
	if len(r.citations) == 0 {
		fmt.Fprint(r.out, answer)
		return
	}
	fmt.Fprint(r.out, insertSuperscripts(answer, r.citations))
	r.printCitationsFootnotes()
}

// emitTail flushes the held tail window. Citations whose EndChar lands
// inside the held tail get inline superscripts; citations whose EndChar is
// already past-flushed get emitted in the footnote block only.
func (r *chatStreamRenderer) emitTail() {
	buf := r.answerBuf.String()
	tail := buf[r.flushedLen:]

	// Partition citations by whether they fall in the still-held tail.
	var inline, spilled []api.Citation
	for _, c := range r.citations {
		if c.EndChar >= r.flushedLen && c.EndChar <= len(buf) {
			// Translate to a tail-local offset.
			local := c
			local.EndChar = c.EndChar - r.flushedLen
			local.StartChar = c.StartChar - r.flushedLen
			inline = append(inline, local)
		} else {
			spilled = append(spilled, c)
		}
	}

	if len(inline) > 0 {
		fmt.Fprint(r.out, insertSuperscripts(tail, inline))
	} else {
		fmt.Fprint(r.out, tail)
	}
	r.flushedLen = len(buf)

	if len(r.citations) == 0 {
		return
	}
	// Footer always uses [N] — the inline superscript in the answer body
	// already marks which ones got spliced. Mixing bracket and superscript
	// in the footer itself makes the odd-one-out look like a rendering bug.
	fmt.Fprintln(r.status)
	fmt.Fprintln(r.status, ansiGrey+strings.Repeat("─", 3)+ansiReset)
	for _, c := range r.citations {
		label := r.citationLabel(c)
		fmt.Fprintf(r.status, "%s[%d] %s%s\n", ansiGrey, c.SourceIndex, label, ansiReset)
	}
}

// printCitationsFootnotes prints a numbered footnote block separated by a rule.
// Used by overlay mode where inline superscripts already appear in the answer.
func (r *chatStreamRenderer) printCitationsFootnotes() {
	if len(r.citations) == 0 {
		return
	}
	fmt.Fprintln(r.status)
	fmt.Fprintln(r.status, ansiGrey+strings.Repeat("─", 3)+ansiReset)
	for _, c := range r.citations {
		label := r.citationLabel(c)
		fmt.Fprintf(r.status, "%s%s%s %s%s\n",
			ansiGrey, superscript(c.SourceIndex), formatConfidence(c.Confidence), label, ansiReset)
	}
}

// printCitationsFooter prints a post-answer footer that lists each citation
// with its char range. Useful for "stream" mode which cannot splice inline.
func (r *chatStreamRenderer) printCitationsFooter() {
	if len(r.citations) == 0 {
		return
	}
	fmt.Fprintf(r.status, "\n%sCitations:%s\n", ansiGrey, ansiReset)
	for _, c := range r.citations {
		label := r.citationLabel(c)
		fmt.Fprintf(r.status, "%s  [%d]%s chars %d-%d — %s%s\n",
			ansiGrey, c.SourceIndex, formatConfidence(c.Confidence),
			c.StartChar, c.EndChar, label, ansiReset)
	}
}

// printCitationsBlock prints the default post-answer Sources: block.
func (r *chatStreamRenderer) printCitationsBlock() {
	if len(r.citations) == 0 {
		return
	}
	fmt.Fprintf(r.status, "\n%sSources:%s\n", ansiGrey, ansiReset)
	for _, c := range r.citations {
		label := r.citationLabel(c)
		fmt.Fprintf(r.status, "%s  [%d]%s %s%s\n",
			ansiGrey, c.SourceIndex, formatConfidence(c.Confidence), label, ansiReset)
	}
}

// formatConfidence returns " (p=0.87)" for a non-zero score, or an empty
// string. Returned with a leading space so callers can splice it without
// padding logic.
func formatConfidence(conf float64) string {
	if conf <= 0 {
		return ""
	}
	return fmt.Sprintf(" (p=%.2f)", conf)
}

// citationLabel formats a single citation line: "<source-id> — <title/excerpt>".
// Prefers a resolved notebook title; falls back to the server-supplied excerpt;
// falls back to the raw source ID.
func (r *chatStreamRenderer) citationLabel(c api.Citation) string {
	id := c.SourceID
	var title string
	if r.resolveTitle != nil {
		title = r.resolveTitle(c.SourceID)
	}
	if title == "" {
		title = c.Title
	}
	title = truncateExcerpt(title, 100)
	switch {
	case id != "" && title != "":
		return fmt.Sprintf("%s — %q", id, title)
	case id != "":
		return id
	case title != "":
		return fmt.Sprintf("%q", title)
	}
	return ""
}

// truncateExcerpt collapses whitespace and clips to max runes with an ellipsis.
func truncateExcerpt(s string, max int) string {
	s = collapseWhitespace(s)
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// insertSuperscripts splices citation markers at each citation's EndChar.
// A single citation at a position renders as a Unicode superscript (¹, ²).
// Multiple citations sharing the same position render as a bracketed cluster
// ([1,2]) to avoid digit ambiguity — e.g. "³⁴" reads as "3⁴" rather than
// "cite 3, cite 4". Positions are snapped to the next word boundary.
func insertSuperscripts(answer string, citations []api.Citation) string {
	type insert struct {
		at  int
		idx []int
	}
	byPos := map[int]*insert{}
	var order []int
	for _, c := range citations {
		pos := c.EndChar
		if pos < 0 || pos > len(answer) {
			pos = len(answer)
		}
		pos = snapToWordBoundary(answer, pos)
		if _, ok := byPos[pos]; !ok {
			byPos[pos] = &insert{at: pos}
			order = append(order, pos)
		}
		byPos[pos].idx = append(byPos[pos].idx, c.SourceIndex)
	}
	// Emit positions in ascending order.
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			if order[j] < order[i] {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	var b strings.Builder
	last := 0
	for _, pos := range order {
		if pos < last {
			continue
		}
		b.WriteString(answer[last:pos])
		b.WriteString(formatCitationCluster(byPos[pos].idx))
		last = pos
	}
	b.WriteString(answer[last:])
	return b.String()
}

// formatCitationCluster renders a set of citation indices that share a splice
// position. A single index becomes a Unicode superscript; two or more become
// a bracketed, comma-joined cluster.
func formatCitationCluster(idx []int) string {
	if len(idx) == 1 {
		return superscript(idx[0])
	}
	parts := make([]string, len(idx))
	for i, n := range idx {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// superscript formats a 1-based citation index using Unicode superscript digits.
func superscript(n int) string {
	if n <= 0 {
		return ""
	}
	digits := []rune("⁰¹²³⁴⁵⁶⁷⁸⁹")
	var out []rune
	for _, d := range fmt.Sprintf("%d", n) {
		out = append(out, digits[d-'0'])
	}
	return string(out)
}

func (r *chatStreamRenderer) printFollowUps() {
	if len(r.followUps) == 0 {
		return
	}
	fmt.Fprintf(r.status, "%sFollow-up suggestions:%s\n", ansiGrey, ansiReset)
	for _, q := range r.followUps {
		fmt.Fprintf(r.status, "%s  - %s%s\n", ansiGrey, q, ansiReset)
	}
}

func (r *chatStreamRenderer) Answer() string {
	return r.answerBuf.String()
}

func (r *chatStreamRenderer) Thinking() string {
	return r.thinking
}

func (r *chatStreamRenderer) clearThinkingLine() {
	if r.lastThinkingLen == 0 {
		return
	}
	clearLine := strings.Repeat(" ", r.lastThinkingLen)
	fmt.Fprintf(r.status, "\r%s\r", clearLine)
	r.lastThinkingLen = 0
}

// streamChatResponse streams a chat response with phase-aware rendering.
// Default: thinking headers shown on a single overwriting line in grey.
// With --verbose: full thinking text streams in grey before the answer.
// Final answer text streams normally. Returns the full answer and thinking trace.
type chatResult struct {
	Answer    string
	Thinking  string
	Citations []api.Citation // raw citation metadata for persistence / re-rendering
	FollowUps []string
}

func streamChatResponse(c *api.Client, req api.ChatRequest, opts chatRenderOptions) (chatResult, error) {
	mode := resolveCitationMode(opts.CitationMode, isTerminal(os.Stdout))
	// --thinking-jsonl is the legacy form of `--citations=json --thinking`.
	// Keep it working by folding its effects into the cleaner flags.
	wantThinking := opts.ShowThinking || opts.Verbose || opts.ThinkingJSONL
	if opts.ThinkingJSONL {
		mode = citationModeJSON
	}
	renderer := newChatStreamRenderer(os.Stdout, os.Stderr, wantThinking || (mode != citationModeJSON && isTerminal(os.Stdout)), opts.Verbose, mode)
	renderer.jsonl = mode == citationModeJSON
	renderer.jsonlIncludeThinking = wantThinking
	renderer.resolveTitle = notebookSourceTitles(c, req.ProjectID)

	err := c.StreamChat(req, func(chunk api.ChatChunk) bool {
		renderer.WriteChunk(chunk)
		return true
	})

	renderer.Finish()

	return chatResult{
		Answer:    renderer.Answer(),
		Thinking:  renderer.Thinking(),
		Citations: renderer.citations,
		FollowUps: renderer.followUps,
	}, err
}

// promoteThinkingToAnswer handles the case where the stream parser classified
// the entire response as thinking-phase (typical when the server never
// emitted a wirePhase tag, and the model's answer text happened to start
// with a bold header that the text heuristic also treats as a thinking
// marker). res.Answer is empty but res.Thinking contains what is actually
// a complete answer. Print it on stdout and mirror it into res.Answer so
// downstream session persistence sees a real answer, not a thinking trace.
//
// This is not an error condition — the user got the content they asked for
// — so we stay silent on stderr and only surface the reclassification when
// --debug is set. In JSONL mode, emit a typed event instead of raw text so
// we don't corrupt the event stream.
func promoteThinkingToAnswer(res *chatResult, debug, jsonl bool) {
	if res.Answer != "" {
		return
	}
	thinking := strings.TrimSpace(res.Thinking)
	if thinking == "" {
		return
	}
	if debug {
		fmt.Fprintln(os.Stderr, "nlm: stream had no answer-phase chunks; using thinking trace as answer")
	}
	if jsonl {
		buf, _ := json.Marshal(map[string]any{
			"phase": "answer",
			"text":  thinking,
			"note":  "promoted from thinking trace",
		})
		fmt.Println(string(buf))
	} else {
		fmt.Println(thinking)
	}
	res.Answer = thinking
}

// printStreamFallback prints the non-streaming fallback response without
// duplicating what streamChatResponse already wrote to stdout. The streaming
// renderer appends each answer chunk to an internal buffer and also prints
// it live, so streamed holds the exact bytes already on stdout (in stream,
// block, and off citation modes). If full starts with streamed we emit only
// the unseen suffix; otherwise — e.g. overlay mode spliced superscripts, or
// the fallback diverged — we emit a boundary marker and the full response so
// the duplication is at least labeled.
//
// In JSONL mode raw printing would corrupt the event stream, so we write a
// single typed fallback event instead.
func printStreamFallback(out io.Writer, streamed, full string, jsonl bool) {
	if jsonl {
		buf, _ := json.Marshal(map[string]any{
			"phase": "fallback",
			"text":  full,
		})
		fmt.Fprintln(out, string(buf))
		return
	}
	if streamed == "" {
		fmt.Fprint(out, full)
		return
	}
	if strings.HasPrefix(full, streamed) {
		fmt.Fprint(out, full[len(streamed):])
		return
	}
	fmt.Fprint(out, "\n--- streaming failed, re-rendering full response ---\n")
	fmt.Fprint(out, full)
}

// notebookSourceTitles returns a lazy lookup from source ID to source title.
// The project fetch happens at most once, on first lookup, and failures are
// silently suppressed (callers fall back to the server's citation excerpt).
func notebookSourceTitles(c *api.Client, projectID string) func(string) string {
	if c == nil || projectID == "" {
		return nil
	}
	var (
		titles map[string]string
		loaded bool
	)
	return func(sourceID string) string {
		if !loaded {
			loaded = true
			proj, err := c.GetProject(projectID)
			if err != nil {
				return ""
			}
			titles = make(map[string]string, len(proj.Sources))
			for _, s := range proj.Sources {
				if id := s.GetSourceId().GetSourceId(); id != "" {
					titles[id] = s.GetTitle()
				}
			}
		}
		return titles[sourceID]
	}
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// newListWriter returns a writer for tabular list output that pads columns
// with a tabwriter when w is a TTY and writes raw tab-separated records
// otherwise. The returned flush function must be called after all rows are
// written. Matches the ls/ps convention: humans get aligned columns, pipelines
// get parseable TSV (cut/awk/paste work on literal tabs).
func newListWriter(w *os.File) (io.Writer, func() error) {
	if !isTerminal(w) {
		return w, func() error { return nil }
	}
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
	return tw, tw.Flush
}

// Generation operations
func generateFreeFormChat(c *api.Client, projectID, prompt string, opts generateChatOptions) error {
	fmt.Fprintf(os.Stderr, "Generating response for: %s\n", prompt)

	sourceIDs, err := resolveSourceSelectorsWithOptions(c, projectID, opts.Selectors)
	if err != nil {
		return err
	}

	chatReq := api.ChatRequest{
		ProjectID: projectID,
		Prompt:    prompt,
		SourceIDs: sourceIDs,
	}

	// Resolve conversation context from flags.
	convID, history, seqNum, err := resolveGenerateChatConversation(c, projectID, opts)
	if err != nil {
		return err
	}
	// Fresh conversation: mint a UUID locally so we can surface it to the
	// user for follow-ups (the api client would otherwise generate one
	// internally and never return it).
	isNewConversation := convID == ""
	if isNewConversation {
		convID = uuid.New().String()
	}
	chatReq.ConversationID = convID
	chatReq.History = history
	chatReq.SeqNum = seqNum

	res, streamErr := streamChatResponse(c, chatReq, opts.Render)
	if streamErr != nil {
		// Fall back to non-streaming path (mirrors oneShotChat behavior).
		response, chatErr := c.ChatWithHistory(chatReq)
		if chatErr != nil {
			return fmt.Errorf("generate chat: stream: %w; fallback: %v", streamErr, chatErr)
		}
		printStreamFallback(os.Stdout, res.Answer, response, opts.Render.ThinkingJSONL)
		res.Answer = response
		// Surface the streaming error even when fallback succeeded, so users
		// can diagnose flaky streams rather than silently degrading.
		fmt.Fprintf(os.Stderr, "nlm: streaming failed, used fallback: %v\n", streamErr)
	}
	if res.Answer != "" {
		fmt.Println()
	} else if strings.TrimSpace(res.Thinking) != "" {
		promoteThinkingToAnswer(&res, debug, opts.Render.ThinkingJSONL)
	} else {
		// Empty answer with no streaming error usually means the conversation
		// was rejected server-side, every source is in an error/indexing state,
		// or the API returned an empty payload. Fail loudly with a hint rather
		// than printing a misleading "(No response received)" and exiting 0.
		hint := "nlm generate-chat: empty response from API"
		if streamErr != nil {
			hint = fmt.Sprintf("%s (stream error: %v)", hint, streamErr)
		}
		return fmt.Errorf("%s; check 'nlm sources %s' for source state, re-run with -debug for details", hint, projectID)
	}

	// Save to local session so future --conversation calls can continue.
	session := &ChatSession{
		NotebookID:     projectID,
		ConversationID: convID,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt, Timestamp: time.Now()},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	answer := strings.TrimSpace(res.Answer)
	thinking := strings.TrimSpace(res.Thinking)
	// Persist whichever channel produced content. When the parser misclassifies
	// the response as thinking-only, promote the trace into Content so chat-show
	// can replay it and downstream callers can chain --conversation correctly.
	if answer == "" && thinking != "" {
		answer = thinking
	}
	if answer != "" {
		session.Messages = append(session.Messages, ChatMessage{
			Role: "assistant", Content: answer, Timestamp: time.Now(),
			Thinking:  res.Thinking,
			Citations: res.Citations,
		})
	}
	// Best-effort save; don't fail the command.
	_ = saveChatSession(session)

	// Tell the user how to continue this conversation.
	printContinuationHint(os.Stderr, projectID, convID, isNewConversation)

	return nil
}

// printContinuationHint writes a muted one-line nudge to stderr telling the
// user how to follow up on the conversation they just had. New conversations
// get the full command; continued ones get a shorter acknowledgement.
func printContinuationHint(w *os.File, projectID, convID string, isNew bool) {
	short := convID
	if len(short) > 8 {
		short = short[:8]
	}
	useColor := isTerminal(w)
	openTag, closeTag := "", ""
	if useColor {
		openTag, closeTag = ansiGrey, ansiReset
	}
	if isNew {
		fmt.Fprintf(w, "%snlm: continue with: nlm generate-chat --conversation %s %s \"...\"%s\n",
			openTag, convID, projectID, closeTag)
	} else {
		fmt.Fprintf(w, "%snlm: continued conversation %s (use --conversation %s to follow up)%s\n",
			openTag, short, convID, closeTag)
	}
}

// resolveGenerateChatConversation resolves --conversation and --web flags into
// a conversation ID, wire history, and sequence number for generate-chat.
// Returns empty values when neither flag is set (fresh conversation).
func resolveGenerateChatConversation(c *api.Client, projectID string, opts generateChatOptions) (string, []api.ChatMessage, int, error) {
	conversationID := opts.ConversationID
	if opts.UseWebChat {
		// Fetch the most recent server-side conversation.
		convIDs, err := c.GetConversations(projectID)
		if err != nil {
			return "", nil, 0, fmt.Errorf("list server conversations: %w", err)
		}
		if len(convIDs) == 0 {
			return "", nil, 0, fmt.Errorf("no server-side conversations found for this notebook")
		}
		conversationID = convIDs[0]
		fmt.Fprintf(os.Stderr, "Using server conversation: %s\n", shortID(conversationID))
	}

	if conversationID == "" {
		return "", nil, 0, nil
	}

	// Expand 8-char prefixes (as shown by chat-list) to full UUIDs. The
	// GetConversationHistory RPC matches on full UUID only; a prefix
	// returns a 0-message response that the caller would mistake for an
	// empty conversation.
	conversationID = resolveConversationID(c, projectID, conversationID)

	// Server conversation length is authoritative for SequenceNumber —
	// the local cache may be empty (conversation started from the web UI)
	// or stale (peer edits arrived via another client). Fetch first; fall
	// back to the local session only if the RPC errors.
	serverMsgs, serverErr := c.GetConversationHistory(projectID, conversationID)
	if serverErr == nil {
		fmt.Fprintf(os.Stderr, "Continuing conversation %s (%d server messages)\n",
			shortID(conversationID), len(serverMsgs))
		wireHistory := buildWireHistoryFromServer(serverMsgs)
		// SeqNum is the 1-indexed slot of the message we're about to send.
		return conversationID, wireHistory, len(serverMsgs) + 1, nil
	}

	// Server fetch failed — fall back to local cache if we have one so the
	// user can still resume, but surface the failure to stderr.
	fmt.Fprintf(os.Stderr, "nlm: could not fetch server history (%v); trying local cache\n", serverErr)
	session, err := loadChatSessionForConv(projectID, conversationID)
	if err == nil && len(session.Messages) > 0 {
		fmt.Fprintf(os.Stderr, "Continuing conversation %s (%d local messages)\n",
			shortID(session.ConversationID), len(session.Messages))
		wireHistory := buildWireHistory(session)
		return session.ConversationID, wireHistory, len(session.Messages)/2 + 1, nil
	}

	// No server history and no local session — continue with the ID alone.
	fmt.Fprintf(os.Stderr, "Continuing conversation %s (no history available)\n", shortID(conversationID))
	return conversationID, nil, 0, nil
}

// buildWireHistoryFromServer converts server-fetched ChatMessages into the
// wire format expected by generate-chat: newest-first, with the final
// message (which the server will pair with the current prompt) excluded.
func buildWireHistoryFromServer(msgs []api.ChatMessage) []api.ChatMessage {
	if len(msgs) == 0 {
		return nil
	}
	// Newest-first ordering to match buildWireHistory.
	history := make([]api.ChatMessage, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		history = append(history, msgs[i])
	}
	return history
}

// generateReport orchestrates report-suggestions + generate-chat to produce
// a multi-section report on stdout. If reportPrompt is set, instructions are
// applied to the notebook before generation.
// defaultReportPrompt is the per-section generation template.
// {topic} is replaced with the section topic.
const defaultReportPrompt = `Write a thorough, implementation-level wiki section on: {topic}

Requirements:
- Use a top-level heading (# {topic})
- Include mermaid diagrams where architecture or flow is relevant
- Include tables for configuration, parameters, or comparisons
- Cite sources with numbered references
- Be comprehensive: cover design rationale, key APIs, data structures, error handling, and examples
- Target ~2000 words per section`

// createReport creates a report artifact, optionally matching a suggestion to get
// targeted source_ids and description. If the report type matches a suggestion title,
// the suggestion's description and source_ids are used instead of all sources.
// audioSuggestions prints AI-generated audio-overview blueprints as
// tab-separated lines (title\tdescription), one per line. Newlines in
// the description are replaced with spaces so each blueprint stays on
// a single line, safe for cut/awk/xargs pipelines. Pass --json to emit
// JSON objects instead.
func audioSuggestions(c *api.Client, notebookID string) error {
	suggestions, err := c.GenerateArtifactSuggestions(notebookID, intmethod.ArtifactSuggestionKindAudio, 1)
	if err != nil {
		return err
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, s := range suggestions {
			if err := enc.Encode(s); err != nil {
				return err
			}
		}
		return nil
	}
	for _, s := range suggestions {
		desc := strings.ReplaceAll(s.Description, "\t", " ")
		desc = strings.ReplaceAll(desc, "\n", " ")
		fmt.Printf("%s\t%s\n", s.Title, desc)
	}
	return nil
}

func createReport(c *api.Client, notebookID, reportType string, extra []string) error {
	description := ""
	instructions := ""
	if len(extra) > 0 {
		description = extra[0]
	}
	if len(extra) > 1 {
		instructions = strings.Join(extra[1:], " ")
	}

	flagIDs, err := resolveSourceSelectors(c, notebookID)
	if err != nil {
		return err
	}

	// Try to match reportType against suggestions for targeted source_ids.
	var suggestionIDs []string
	resp, suggErr := c.GenerateReportSuggestions(notebookID)
	if suggErr == nil {
		for _, s := range resp.GetSuggestions() {
			if strings.EqualFold(s.GetTitle(), reportType) {
				if description == "" {
					description = s.GetDescription()
				}
				suggestionIDs = s.GetSourceIds()
				fmt.Fprintf(os.Stderr, "Matched suggestion %q (%d sources)\n", s.GetTitle(), len(suggestionIDs))
				break
			}
		}
	}

	sourceIDs := unionIDs(flagIDs, suggestionIDs)

	artifactID, err := c.CreateReport(notebookID, reportType, description, instructions, sourceIDs...)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created report: %s\n", artifactID)
	fmt.Fprintf(os.Stderr, "Use 'nlm artifacts %s' to check status.\n", notebookID)
	return nil
}

func generateReport(c *api.Client, notebookID string, opts reportOptions) error {
	// Optionally set notebook instructions.
	if opts.Instructions != "" {
		fmt.Fprintf(os.Stderr, "Setting instructions...\n")
		if err := c.SetInstructions(notebookID, opts.Instructions); err != nil {
			return fmt.Errorf("set instructions: %w", err)
		}
	}

	flagIDs, err := resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
	if err != nil {
		return err
	}

	// Read suggestions from stdin or API.
	suggestions, err := readReportSuggestions(c, notebookID)
	if err != nil {
		return err
	}

	// Limit sections if requested.
	if opts.Sections > 0 && opts.Sections < len(suggestions) {
		suggestions = suggestions[:opts.Sections]
	}

	// Resolve per-section prompt template.
	tmpl := defaultReportPrompt
	if opts.Prompt != "" {
		tmpl = opts.Prompt
	}

	fmt.Fprintf(os.Stderr, "Generating %d sections...\n", len(suggestions))

	for i, s := range suggestions {
		title := s.GetTitle()
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(suggestions), title)

		prompt := strings.ReplaceAll(tmpl, "{topic}", title)
		// Use suggestion-specific prompt if available and no custom template set.
		if opts.Prompt == "" && s.GetPrompt() != "" {
			prompt = s.GetPrompt()
		}
		chatReq := api.ChatRequest{
			ProjectID: notebookID,
			Prompt:    prompt,
			SourceIDs: unionIDs(flagIDs, s.GetSourceIds()),
		}
		res, err := streamChatResponse(c, chatReq, opts.Render)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: section %q failed: %v\n", title, err)
			continue
		}
		if res.Answer != "" {
			fmt.Println()
		}
		fmt.Println() // blank line between sections
	}

	return nil
}

// readReportSuggestions reads suggestions from stdin (one title per line) or
// from the report-suggestions API. API suggestions include per-section source
// scoping and prompts; stdin suggestions are title-only.
func readReportSuggestions(c *api.Client, notebookID string) ([]*pb.ReportSuggestion, error) {
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		// stdin is piped — read topics from it.
		var suggestions []*pb.ReportSuggestion
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				suggestions = append(suggestions, &pb.ReportSuggestion{Title: line})
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read topics from stdin: %w", err)
		}
		if len(suggestions) == 0 {
			return nil, fmt.Errorf("no topics provided on stdin")
		}
		fmt.Fprintf(os.Stderr, "Read %d topics from stdin\n", len(suggestions))
		return suggestions, nil
	}

	// Fetch from API.
	fmt.Fprintf(os.Stderr, "Fetching report suggestions...\n")
	resp, err := c.GenerateReportSuggestions(notebookID)
	if err != nil {
		return nil, fmt.Errorf("report suggestions: %w", err)
	}
	suggestions := resp.GetSuggestions()
	if len(suggestions) == 0 {
		return nil, fmt.Errorf("no report suggestions returned")
	}
	return suggestions, nil
}

func deleteChatHistory(c *api.Client, notebookID string) error {
	if !confirmAction(fmt.Sprintf("Delete all chat history for notebook %s?", notebookID)) {
		return fmt.Errorf("operation cancelled")
	}
	if err := c.DeleteChatHistory(notebookID); err != nil {
		return fmt.Errorf("delete chat history: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Chat history deleted.")
	return nil
}

func setChatConfig(c *api.Client, args []string) error {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: nlm chat-config <notebook-id> <setting> [value]\n")
		fmt.Fprintf(os.Stderr, "\nSettings:\n")
		fmt.Fprintf(os.Stderr, "  goal default              Reset to default conversational style\n")
		fmt.Fprintf(os.Stderr, "  goal custom \"<prompt>\"    Set custom system prompt\n")
		fmt.Fprintf(os.Stderr, "  length default            Reset to default response length\n")
		fmt.Fprintf(os.Stderr, "  length longer             Set longer responses\n")
		fmt.Fprintf(os.Stderr, "  length shorter            Set shorter responses\n")
		return fmt.Errorf("invalid arguments")
	}

	notebookID := args[0]
	setting := args[1]

	switch setting {
	case "goal":
		if len(args) < 3 {
			return fmt.Errorf("usage: nlm chat-config <id> goal <default|custom \"prompt\">")
		}
		switch args[2] {
		case "default":
			return c.SetChatConfig(notebookID, api.ChatGoalDefault, "", api.ResponseLengthDefault)
		case "custom":
			if len(args) < 4 {
				return fmt.Errorf("usage: nlm chat-config <id> goal custom \"your prompt\"")
			}
			prompt := strings.Join(args[3:], " ")
			return c.SetChatConfig(notebookID, api.ChatGoalCustom, prompt, api.ResponseLengthDefault)
		default:
			return fmt.Errorf("unknown goal: %s (use 'default' or 'custom')", args[2])
		}
	case "length":
		if len(args) < 3 {
			return fmt.Errorf("usage: nlm chat-config <id> length <default|longer|shorter>")
		}
		switch args[2] {
		case "default":
			return c.SetChatConfig(notebookID, 0, "", api.ResponseLengthDefault)
		case "longer":
			return c.SetChatConfig(notebookID, 0, "", api.ResponseLengthLonger)
		case "shorter":
			return c.SetChatConfig(notebookID, 0, "", api.ResponseLengthShorter)
		default:
			return fmt.Errorf("unknown length: %s (use 'default', 'longer', or 'shorter')", args[2])
		}
	default:
		return fmt.Errorf("unknown setting: %s (use 'goal' or 'length')", setting)
	}
}

// isConversationID returns true if the string looks like a conversation ID
// (UUID format or long alphanumeric string, not natural language).
func isConversationID(s string) bool {
	// UUIDs: 8-4-4-4-12 hex
	if len(s) == 36 && s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-' {
		return true
	}
	// Also accept raw hex strings >= 20 chars with no spaces
	if len(s) >= 20 && !strings.Contains(s, " ") {
		return true
	}
	return false
}

// oneShotChat sends a single prompt and streams the response without entering interactive mode.
func oneShotChat(c *api.Client, notebookID, prompt string, opts chatOptions) error {
	sourceIDs, err := resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
	if err != nil {
		return err
	}

	// Load or create session for history continuity
	session, err := loadChatSession(notebookID)
	if err != nil {
		session = &ChatSession{
			NotebookID:     notebookID,
			ConversationID: uuid.New().String(),
			Messages:       []ChatMessage{},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}
	if session.ConversationID == "" {
		session.ConversationID = uuid.New().String()
	}

	// Add user message
	session.Messages = append(session.Messages, ChatMessage{
		Role: "user", Content: prompt, Timestamp: time.Now(),
	})

	wireHistory := buildWireHistory(session)
	chatReq := api.ChatRequest{
		ProjectID:      notebookID,
		Prompt:         prompt,
		SourceIDs:      sourceIDs,
		ConversationID: session.ConversationID,
		History:        wireHistory,
		SeqNum:         len(session.Messages)/2 + 1,
	}

	res, err := streamChatResponse(c, chatReq, opts.Render)
	if err != nil {
		response, chatErr := c.ChatWithHistory(chatReq)
		if chatErr != nil {
			return fmt.Errorf("chat: %w", err)
		}
		printStreamFallback(os.Stdout, res.Answer, response, opts.Render.ThinkingJSONL)
		res.Answer = response
	}
	if res.Answer == "" {
		promoteThinkingToAnswer(&res, debug, opts.Render.ThinkingJSONL)
	}
	fmt.Println()

	// Save response with thinking trace and citations. When the parser
	// classified the response as thinking-only, promote the trace to Content
	// so chat-show can replay it and history persists across runs.
	response := strings.TrimSpace(res.Answer)
	if response == "" {
		response = strings.TrimSpace(res.Thinking)
	}
	if response != "" {
		session.Messages = append(session.Messages, ChatMessage{
			Role: "assistant", Content: response, Timestamp: time.Now(),
			Thinking:  res.Thinking,
			Citations: res.Citations,
		})
	}
	session.UpdatedAt = time.Now()
	return saveChatSession(session)
}

// readPromptFile returns the prompt text from path, or from stdin when path is "-".
// Trailing whitespace/newlines are stripped so the prompt matches what users
// typed interactively.
func readPromptFile(path string) (string, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	prompt := strings.TrimRight(string(data), " \t\r\n")
	if prompt == "" {
		return "", fmt.Errorf("empty prompt")
	}
	return prompt, nil
}

// oneShotChatInConv sends a single prompt to an existing conversation, then exits.
// Mirrors oneShotChat but preserves the server-side conversation ID so callers
// can chain turns via automation.
func oneShotChatInConv(c *api.Client, notebookID, conversationID, prompt string, opts chatOptions) error {
	sourceIDs, err := resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
	if err != nil {
		return err
	}
	conversationID = resolveConversationID(c, notebookID, conversationID)
	session, err := loadChatSessionForConv(notebookID, conversationID)
	if err != nil {
		session = &ChatSession{
			NotebookID:     notebookID,
			ConversationID: conversationID,
			Messages:       []ChatMessage{},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}
	session.ConversationID = conversationID
	session.Messages = append(session.Messages, ChatMessage{
		Role: "user", Content: prompt, Timestamp: time.Now(),
	})
	wireHistory := buildWireHistory(session)
	chatReq := api.ChatRequest{
		ProjectID:      notebookID,
		Prompt:         prompt,
		SourceIDs:      sourceIDs,
		ConversationID: conversationID,
		History:        wireHistory,
		SeqNum:         len(session.Messages)/2 + 1,
	}
	res, err := streamChatResponse(c, chatReq, opts.Render)
	if err != nil {
		response, chatErr := c.ChatWithHistory(chatReq)
		if chatErr != nil {
			return fmt.Errorf("chat: %w", err)
		}
		printStreamFallback(os.Stdout, res.Answer, response, opts.Render.ThinkingJSONL)
		res.Answer = response
	}
	if res.Answer == "" {
		promoteThinkingToAnswer(&res, debug, opts.Render.ThinkingJSONL)
	}
	fmt.Println()
	response := strings.TrimSpace(res.Answer)
	if response == "" {
		response = strings.TrimSpace(res.Thinking)
	}
	if response != "" {
		session.Messages = append(session.Messages, ChatMessage{
			Role: "assistant", Content: response, Timestamp: time.Now(),
			Thinking:  res.Thinking,
			Citations: res.Citations,
		})
	}
	session.UpdatedAt = time.Now()
	return saveChatSession(session)
}

// interactiveChatWithConv starts or resumes an interactive chat with a specific conversation ID.
func interactiveChatWithConv(c *api.Client, notebookID, conversationID string, opts chatOptions) error {
	sourceIDs, err := resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
	if err != nil {
		return err
	}
	// Expand partial IDs (chat-list prints the first 8 chars of the UUID).
	conversationID = resolveConversationID(c, notebookID, conversationID)

	// Try to load local session for this conversation
	session, err := loadChatSessionForConv(notebookID, conversationID)
	if err != nil {
		// Try fetching server-side history
		serverMsgs, fetchErr := c.GetConversationHistory(notebookID, conversationID)
		if fetchErr != nil && debug {
			fmt.Fprintf(os.Stderr, "nlm: could not fetch server history: %v\n", fetchErr)
		}

		session = &ChatSession{
			NotebookID:     notebookID,
			ConversationID: conversationID,
			Messages:       []ChatMessage{},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Populate from server history
		if fetchErr == nil && len(serverMsgs) > 0 {
			for _, m := range serverMsgs {
				role := "user"
				if m.Role == 2 {
					role = "assistant"
				}
				session.Messages = append(session.Messages, ChatMessage{
					Role:    role,
					Content: m.Content,
				})
			}
			fmt.Printf("Loaded %d messages from server history.\n", len(serverMsgs))
		}
	}

	// Override the conversation ID (the loaded session might have an old one)
	session.ConversationID = conversationID

	return runInteractiveChat(c, session, sourceIDs, opts)
}

// printChatHistory prints conversation history, trying the server first then
// falling back to local session storage.
func printChatHistory(c *api.Client, notebookID, conversationID string) error {
	// Resolve partial conversation IDs (e.g. "abcd1234" from chat-list output).
	conversationID = resolveConversationID(c, notebookID, conversationID)

	// Try server-side history first.
	messages, err := c.GetConversationHistory(notebookID, conversationID)
	if err == nil && len(messages) > 0 {
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
	}

	// Fall back to local session.
	session, localErr := loadChatSessionByConversation(notebookID, conversationID)
	if localErr != nil {
		if err != nil {
			return fmt.Errorf("server: %w; no local session found", err)
		}
		return fmt.Errorf("no conversation history found")
	}
	if len(session.Messages) == 0 {
		fmt.Fprintln(os.Stderr, "No messages in conversation.")
		return nil
	}
	for _, m := range session.Messages {
		role := strings.ToUpper(m.Role)
		fmt.Printf("[%s]\n%s\n\n", role, m.Content)
	}
	return nil
}

// resolveConversationID expands a partial conversation ID prefix (as shown by
// chat-list) to the full UUID by checking server-side conversations.
func resolveConversationID(c *api.Client, notebookID, partial string) string {
	if len(partial) >= 36 {
		return partial // already full UUID
	}
	convIDs, err := c.GetConversations(notebookID)
	if err != nil {
		return partial
	}
	for _, id := range convIDs {
		if strings.HasPrefix(id, partial) {
			return id
		}
	}
	return partial
}

func loadChatSessionByConversation(notebookID, conversationID string) (*ChatSession, error) {
	if session, err := loadChatSessionForConv(notebookID, conversationID); err == nil {
		return session, nil
	}

	sessions, _ := listLocalChatSessions(notebookID)
	for i := range sessions {
		if sessions[i].ConversationID == conversationID || strings.HasPrefix(sessions[i].ConversationID, conversationID) {
			return &sessions[i], nil
		}
	}

	legacy, err := loadChatSession(notebookID)
	if err != nil {
		return nil, err
	}
	if legacy.ConversationID == conversationID || strings.HasPrefix(legacy.ConversationID, conversationID) {
		return legacy, nil
	}
	return nil, os.ErrNotExist
}

// chatShow renders a locally-stored conversation with full citation modes.
// Unlike chat-history (which prefers server-side), chat-show reads only the
// local session so it can surface persisted citation metadata (char ranges,
// source IDs) that the server doesn't return in conversation history.
func chatShow(notebookID, conversationID string, opts chatRenderOptions) error {
	session, err := loadChatSessionByConversation(notebookID, conversationID)
	if err != nil {
		return fmt.Errorf("load local session: %w", err)
	}
	if len(session.Messages) == 0 {
		fmt.Fprintln(os.Stderr, "No messages in local session.")
		return nil
	}

	mode := resolveCitationMode(opts.CitationMode, isTerminal(os.Stdout))
	for i, m := range session.Messages {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("[%s]\n", strings.ToUpper(m.Role))

		if m.Role != "assistant" {
			fmt.Println(m.Content)
			continue
		}
		if opts.ShowThinking && m.Thinking != "" {
			fmt.Fprintf(os.Stderr, "%s%s%s\n", ansiGrey, m.Thinking, ansiReset)
		}
		renderPersistedAssistant(os.Stdout, os.Stderr, m, mode)
	}
	return nil
}

// renderPersistedAssistant feeds a stored assistant message through the live
// renderer so all citation modes (block/stream/tail/overlay) work identically
// for replays.
func renderPersistedAssistant(out, status io.Writer, m ChatMessage, mode citationRenderMode) {
	r := newChatStreamRenderer(out, status, false, false, mode)
	r.WriteChunk(api.ChatChunk{
		Phase:     api.ChatChunkAnswer,
		Text:      m.Content,
		Citations: m.Citations,
	})
	r.Finish()
	if !strings.HasSuffix(m.Content, "\n") {
		fmt.Fprintln(out)
	}
}

// listChatConversationsWithAuth creates a client and lists server-side
// conversations. Used by chat-list which is noClient (local-only path needs no client).
func listChatConversationsWithAuth(notebookID string) error {
	if authToken == "" || cookies == "" {
		return fmt.Errorf("authentication required for server-side listing; run 'nlm auth' first")
	}
	c := api.New(authToken, cookies)
	if debug {
		c.SetDebug(true)
	}
	return listChatConversations(c, notebookID)
}

// listChatConversations lists server-side conversations for a notebook.
func listChatConversations(c *api.Client, notebookID string) error {
	convIDs, err := c.GetConversations(notebookID)
	if err != nil {
		return fmt.Errorf("list conversations: %w", err)
	}

	// Also get local sessions for this notebook
	localSessions, _ := listLocalChatSessions(notebookID)
	localByConv := make(map[string]*ChatSession)
	for i := range localSessions {
		if localSessions[i].ConversationID != "" {
			localByConv[localSessions[i].ConversationID] = &localSessions[i]
		}
	}

	if len(convIDs) == 0 && len(localSessions) == 0 {
		fmt.Fprintln(os.Stderr, "No conversations found.")
		return nil
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "CONVERSATION\tMESSAGES\tSTATUS\tLAST UPDATED")
	if isTerminal(os.Stdout) {
		fmt.Fprintln(w, "------------\t--------\t------\t------------")
	}

	seen := make(map[string]bool)
	for _, id := range convIDs {
		seen[id] = true
		msgs := "-"
		status := "server"
		lastUpdated := "-"
		if local, ok := localByConv[id]; ok {
			msgs = fmt.Sprintf("%d", len(local.Messages))
			status = "synced"
			lastUpdated = local.UpdatedAt.Format("Jan 2 15:04")
		}
		short := id
		if len(id) > 8 {
			short = id[:8]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", short, msgs, status, lastUpdated)
	}

	// Show local-only sessions
	for _, s := range localSessions {
		if s.ConversationID != "" && !seen[s.ConversationID] {
			short := s.ConversationID
			if len(short) > 8 {
				short = short[:8]
			}
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
				short, len(s.Messages), "local", s.UpdatedAt.Format("Jan 2 15:04"))
		}
	}

	return flush()
}

func setInstructions(c *api.Client, notebookID, prompt string) error {
	if err := c.SetChatConfig(notebookID, api.ChatGoalCustom, prompt, api.ResponseLengthDefault); err != nil {
		return fmt.Errorf("set instructions: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Instructions updated.")
	return nil
}

func getInstructions(c *api.Client, notebookID string) error {
	project, err := c.GetProject(notebookID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	if debug {
		if cfg := project.GetChatbotConfig(); cfg != nil {
			fmt.Fprintf(os.Stderr, "DEBUG: chat goal=%d response_length=%d\n",
				cfg.GetGoal().GetGoal(),
				cfg.GetResponseLength().GetValue(),
			)
		}
	}

	prompt := strings.TrimSpace(project.GetChatbotConfig().GetGoal().GetCustomPrompt())
	if prompt == "" {
		// Empty stdout + zero exit signals "no instructions"; scripts can
		// branch on `[ -z "$(nlm get-instructions NB)" ]`.
		fmt.Fprintln(os.Stderr, "No custom instructions set.")
		return nil
	}

	fmt.Println(prompt)
	return nil
}

// Utility functions for commented-out operations
func shareNotebook(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating public share link...\n")
	resp, err := c.ShareProject(notebookID, &pb.ShareSettings{IsPublic: true})
	if err != nil {
		return fmt.Errorf("share project: %w", err)
	}
	if resp.GetShareUrl() == "" {
		return fmt.Errorf("share project: server did not return a public share URL")
	}
	fmt.Printf("Share URL: %s\n", resp.GetShareUrl())
	return nil
}

func submitFeedback(c *api.Client, message string) error {
	if err := c.SubmitFeedback("", "general", message); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "Feedback submitted")
	return nil
}

func shareNotebookPrivate(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating private share link...\n")
	resp, err := c.ShareProject(notebookID, &pb.ShareSettings{IsPublic: false})
	if err != nil {
		return fmt.Errorf("share project privately: %w", err)
	}
	printPrivateShareResult(os.Stdout, notebookID, resp)
	return nil
}

func printPrivateShareResult(w io.Writer, notebookID string, resp *pb.ShareProjectResponse) {
	if resp == nil {
		fmt.Fprintf(w, "Project shared privately, but the server returned no share metadata. Open https://notebooklm.google.com/notebook/%s in the browser to copy the invite link.\n", notebookID)
		return
	}
	if resp.GetShareUrl() != "" {
		fmt.Fprintf(w, "Private Share URL: %s\n", resp.GetShareUrl())
		return
	}
	if resp.GetShareId() != "" {
		fmt.Fprintf(w, "Private Share ID: %s\n", resp.GetShareId())
		fmt.Fprintf(w, "Open https://notebooklm.google.com/notebook/%s in the browser to copy the invite link.\n", notebookID)
		return
	}
	fmt.Fprintf(w, "Project shared privately, but the server returned no share URL or share ID. Open https://notebooklm.google.com/notebook/%s in the browser to copy the invite link.\n", notebookID)
}

func getShareDetails(c *api.Client, shareID string) error {
	fmt.Fprintf(os.Stderr, "Getting share details...\n")
	details, err := c.GetProjectDetails(shareID)
	if err != nil {
		return err
	}
	printShareDetails(os.Stdout, shareID, details)
	return nil
}

func printShareDetails(w io.Writer, shareID string, details *pb.ProjectDetails) {
	fmt.Fprintln(w, "Share Details:")
	fmt.Fprintf(w, "Share ID: %s\n", shareID)
	if details == nil {
		fmt.Fprintln(w, "No details available for this share ID.")
		return
	}
	if details.ProjectId != "" {
		fmt.Fprintf(w, "Project ID: %s\n", details.ProjectId)
	}
	title := collapseWhitespace(strings.TrimSpace(strings.TrimSpace(details.Emoji) + " " + details.Title))
	if title != "" {
		fmt.Fprintf(w, "Title: %s\n", title)
	}
	if details.OwnerName != "" {
		fmt.Fprintf(w, "Owner: %s\n", details.OwnerName)
	}
	visibility := "private"
	if details.IsPublic {
		visibility = "public"
	}
	fmt.Fprintf(w, "Visibility: %s\n", visibility)
	if ts := details.SharedAt; ts != nil && ts.IsValid() {
		fmt.Fprintf(w, "Shared At: %s\n", ts.AsTime().Format(time.RFC3339))
	}
	if len(details.Sources) == 0 {
		if details.ProjectId == "" && title == "" {
			fmt.Fprintln(w, "Note: current share-details responses only include owner/visibility metadata.")
		}
		return
	}
	fmt.Fprintf(w, "Sources: %d\n", len(details.Sources))
	for _, src := range details.Sources {
		fmt.Fprintf(w, "  - %s (%s)\n", src.Title, src.SourceType.String())
	}
}

// Chat helper functions
func getChatSessionPath(notebookID string) string {
	return getChatSessionPathForConv(notebookID, "")
}

func getChatSessionPathForConv(notebookID, conversationID string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		if conversationID != "" {
			return filepath.Join(os.TempDir(), fmt.Sprintf("nlm-chat-%s-%s.json", notebookID, shortID(conversationID)))
		}
		return filepath.Join(os.TempDir(), fmt.Sprintf("nlm-chat-%s.json", notebookID))
	}

	nlmDir := filepath.Join(homeDir, ".nlm")
	os.MkdirAll(nlmDir, 0700) // Ensure directory exists
	if conversationID != "" {
		return filepath.Join(nlmDir, fmt.Sprintf("chat-%s-%s.json", notebookID, shortID(conversationID)))
	}
	return filepath.Join(nlmDir, fmt.Sprintf("chat-%s.json", notebookID))
}

// shortID returns the first 8 characters of id, or all of id if shorter.
// Used to build short suffixes for chat session filenames without panicking
// on truncated or malformed conversation IDs.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func loadChatSessionForConv(notebookID, conversationID string) (*ChatSession, error) {
	path := getChatSessionPathForConv(notebookID, conversationID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// listLocalChatSessions returns all local chat sessions for a given notebook ID.
// If notebookID is empty, returns sessions for all notebooks.
func listLocalChatSessions(notebookID string) ([]ChatSession, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	nlmDir := filepath.Join(homeDir, ".nlm")
	entries, err := os.ReadDir(nlmDir)
	if err != nil {
		return nil, nil
	}
	var sessions []ChatSession
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "chat-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(nlmDir, entry.Name()))
		if err != nil {
			continue
		}
		var session ChatSession
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}
		if notebookID == "" || session.NotebookID == notebookID {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func loadChatSession(notebookID string) (*ChatSession, error) {
	path := getChatSessionPath(notebookID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func saveChatSession(session *ChatSession) error {
	path := getChatSessionPath(session.NotebookID)

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	if session.ConversationID == "" {
		return nil
	}
	return os.WriteFile(getChatSessionPathForConv(session.NotebookID, session.ConversationID), data, 0600)
}

func listChatSessions() error {
	sessions, err := listLocalChatSessions("")
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "No chat sessions found.")
		return nil
	}

	isTTY := isTerminal(os.Stdout)
	if isTTY {
		fmt.Fprintf(os.Stderr, "Chat Sessions (%d total)\n", len(sessions))
		fmt.Fprintln(os.Stderr, strings.Repeat("=", 41))
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "NOTEBOOK\tCONVERSATION\tMESSAGES\tLAST UPDATED")
	if isTTY {
		fmt.Fprintln(w, "--------\t------------\t--------\t------------")
	}

	for _, session := range sessions {
		convShort := session.ConversationID
		if len(convShort) > 8 {
			convShort = convShort[:8]
		}
		if convShort == "" {
			convShort = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			session.NotebookID,
			convShort,
			len(session.Messages),
			session.UpdatedAt.Format("Jan 2 15:04"))
	}

	return flush()
}

func showRecentHistory(session *ChatSession, maxMessages int) {
	messages := session.Messages
	start := 0
	if len(messages) > maxMessages {
		start = len(messages) - maxMessages
	}

	for _, msg := range messages[start:] {
		timestamp := msg.Timestamp.Format("15:04")
		if msg.Role == "user" {
			fmt.Printf("[%s] 👤 You: %s\n", timestamp, msg.Content)
		} else {
			fmt.Printf("[%s] 🤖 Assistant: %s\n", timestamp, msg.Content)
		}
	}
}

// buildWireHistory converts a ChatSession's messages into the wire format expected
// by the NotebookLM chat API. Messages are ordered newest-first, with each entry
// being [content, null, role] where role 1=user, 2=assistant.
func buildWireHistory(session *ChatSession) []api.ChatMessage {
	msgs := session.Messages
	// Exclude the last message (it's the current user prompt, sent separately)
	if len(msgs) > 1 {
		msgs = msgs[:len(msgs)-1]
	} else {
		return nil
	}

	// Build in reverse chronological order (newest first)
	var history []api.ChatMessage
	for i := len(msgs) - 1; i >= 0; i-- {
		role := 1 // user
		if msgs[i].Role == "assistant" {
			role = 2
		}
		history = append(history, api.ChatMessage{
			Content: msgs[i].Content,
			Role:    role,
		})
	}
	return history
}

func getFallbackResponse(input, notebookID string) string {
	lowerInput := strings.ToLower(input)

	// Greeting responses
	if strings.Contains(lowerInput, "hello") || strings.Contains(lowerInput, "hi") || strings.Contains(lowerInput, "hey") {
		return "Hello! I'm here to help you explore and understand your notebook content. What would you like to know?"
	}

	// Content questions
	if strings.Contains(lowerInput, "what") || strings.Contains(lowerInput, "explain") || strings.Contains(lowerInput, "tell me") {
		return "I'm having trouble connecting to the chat service right now. You might want to try using specific commands like 'nlm generate-guide " + notebookID + "' or 'nlm create-report " + notebookID + "' for detailed content analysis."
	}

	// Summary requests
	if strings.Contains(lowerInput, "summary") || strings.Contains(lowerInput, "summarize") {
		return "For a summary of your notebook, try running 'nlm generate-guide " + notebookID + "' which will provide a comprehensive overview of your content."
	}

	// Questions about sources
	if strings.Contains(lowerInput, "source") || strings.Contains(lowerInput, "document") {
		return "To see the sources in your notebook, try 'nlm sources " + notebookID + "'. If you want to analyze specific sources, you can use commands like 'nlm summarize'."
	}

	// Help requests
	if strings.Contains(lowerInput, "help") || strings.Contains(lowerInput, "how") {
		return "I can help you explore your notebook! Try asking me about your content, or use '/help' to see chat commands. For more functionality, check 'nlm help' for all available commands."
	}

	// Default response
	return "I'm unable to process your request right now due to connectivity issues. The chat service may be temporarily unavailable. You can try using other nlm commands or rephrase your question."
}

// interactiveChat starts a new or resumes the default interactive chat session for a notebook.
func interactiveChat(c *api.Client, notebookID string, opts chatOptions) error {
	sourceIDs, err := resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
	if err != nil {
		return err
	}
	session, err := loadChatSession(notebookID)
	if err != nil {
		session = &ChatSession{
			NotebookID:     notebookID,
			ConversationID: uuid.New().String(),
			Messages:       []ChatMessage{},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}
	if session.ConversationID == "" {
		session.ConversationID = uuid.New().String()
	}
	return runInteractiveChat(c, session, sourceIDs, opts)
}

// runInteractiveChat runs the interactive chat loop with the given session.
// sourceIDs, when non-empty, scopes every request in the loop to that subset.
func runInteractiveChat(c *api.Client, session *ChatSession, sourceIDs []string, opts chatOptions) error {
	notebookID := session.NotebookID

	fmt.Println("\nNotebookLM Interactive Chat")
	fmt.Println("================================")
	fmt.Printf("Notebook: %s\n", notebookID)
	convShort := session.ConversationID
	if len(convShort) > 8 {
		convShort = convShort[:8]
	}
	fmt.Printf("Conversation: %s\n", convShort)

	if len(session.Messages) > 0 {
		fmt.Printf("Chat history: %d messages (started %s)\n",
			len(session.Messages),
			session.CreatedAt.Format("Jan 2 15:04"))
		if !opts.ShowHistory {
			fmt.Println("  (use -history flag to show previous conversation)")
		}
	}

	fmt.Println("\nCommands: /exit /clear /history /reset /new /fork /conversations /save /help /multiline /file")
	fmt.Println("Type your message and press Enter to send.")

	// bufio.Reader (not Scanner): Scanner's 64KB token cap truncates pasted
	// prompts, and it refuses to return a partial line on EOF. Reader.ReadString
	// grows unbounded and promotes a trailing no-newline chunk on EOF, so
	// automation that sends text without a final "\n" still submits.
	reader := bufio.NewReader(os.Stdin)
	readLine := func() (string, bool) {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "nlm: read input: %v\n", err)
			return "", false
		}
		// On EOF with buffered chars, submit them as the final line.
		if err == io.EOF && line == "" {
			return "", false
		}
		return strings.TrimRight(line, "\r\n"), true
	}

	multiline := false

	if opts.ShowHistory && len(session.Messages) > 0 {
		fmt.Println("\n--- Recent Chat History ---")
		showRecentHistory(session, 10)
		fmt.Println("---------------------------")
	}

	for {
		historyCount := len(session.Messages)
		if multiline {
			fmt.Printf("[%s %d msgs] (multiline) > ", convShort, historyCount)
		} else {
			fmt.Printf("[%s %d msgs] > ", convShort, historyCount)
		}

		var input string
		if multiline {
			var lines []string
			for {
				line, ok := readLine()
				if !ok {
					break
				}
				if line == "" {
					break
				}
				lines = append(lines, line)
				fmt.Print("... > ")
			}
			input = strings.Join(lines, "\n")
		} else {
			line, ok := readLine()
			if !ok {
				break
			}
			input = line
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// /file <path> — load prompt text from disk. Bypasses terminal paste
		// limits that plague long prompts sent via automation.
		if strings.HasPrefix(input, "/file ") || strings.HasPrefix(input, "/file\t") {
			path := strings.TrimSpace(input[len("/file"):])
			prompt, err := readPromptFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "nlm: /file %s: %v\n", path, err)
				continue
			}
			input = prompt
			fmt.Printf("(loaded %d bytes from %s)\n", len(prompt), path)
		}

		switch strings.ToLower(input) {
		case "/exit", "/quit":
			fmt.Println("\nSaving session and goodbye!")
			if err := saveChatSession(session); err != nil {
				fmt.Printf("Warning: Failed to save session: %v\n", err)
			}
			return nil
		case "/clear":
			fmt.Print("\033[H\033[2J")
			fmt.Printf("Notebook: %s  Conversation: %s  Messages: %d\n\n",
				notebookID, convShort, len(session.Messages))
			continue
		case "/history":
			fmt.Println("\n--- Chat History ---")
			showRecentHistory(session, 10)
			fmt.Println("-------------------")
			continue
		case "/reset":
			if confirmAction("Are you sure you want to clear chat history?") {
				session.Messages = []ChatMessage{}
				session.ConversationID = uuid.New().String()
				convShort = session.ConversationID[:8]
				session.UpdatedAt = time.Now()
				fmt.Printf("Chat history cleared. New conversation: %s\n", convShort)
			}
			continue
		case "/new":
			// Start a new conversation within the same notebook
			if err := saveChatSession(session); err != nil && debug {
				fmt.Fprintf(os.Stderr, "Debug: save failed: %v\n", err)
			}
			session = &ChatSession{
				NotebookID:     notebookID,
				ConversationID: uuid.New().String(),
				Messages:       []ChatMessage{},
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			convShort = session.ConversationID[:8]
			fmt.Printf("Started new conversation: %s\n", convShort)
			continue
		case "/fork":
			// Fork: save current, create new conversation with same history
			if err := saveChatSession(session); err != nil && debug {
				fmt.Fprintf(os.Stderr, "Debug: save failed: %v\n", err)
			}
			oldShort := convShort
			// Deep copy messages
			forkedMsgs := make([]ChatMessage, len(session.Messages))
			copy(forkedMsgs, session.Messages)
			session = &ChatSession{
				NotebookID:     notebookID,
				ConversationID: uuid.New().String(),
				Messages:       forkedMsgs,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			convShort = session.ConversationID[:8]
			fmt.Printf("Forked from %s -> %s (%d messages carried over)\n",
				oldShort, convShort, len(forkedMsgs))
			continue
		case "/conversations":
			convIDs, err := c.GetConversations(notebookID)
			if err != nil {
				fmt.Printf("Error fetching conversations: %v\n", err)
				continue
			}
			if len(convIDs) == 0 {
				fmt.Println("No server-side conversations found.")
				continue
			}
			fmt.Printf("\nConversations for notebook %s:\n", notebookID)
			for i, id := range convIDs {
				marker := "  "
				if id == session.ConversationID {
					marker = "* "
				}
				short := id
				if len(short) > 8 {
					short = short[:8]
				}
				fmt.Printf("  %s%d. %s\n", marker, i+1, short)
			}
			fmt.Println("\nUse 'nlm chat <notebook-id> <conversation-id>' to resume a conversation.")
			continue
		case "/save":
			if err := saveChatSession(session); err != nil {
				fmt.Printf("Error saving session: %v\n", err)
			} else {
				fmt.Println("Session saved.")
			}
			continue
		case "/help":
			fmt.Println("\nCommands:")
			fmt.Println("  /exit or /quit     - Exit chat")
			fmt.Println("  /clear             - Clear screen")
			fmt.Println("  /history           - Show recent chat history")
			fmt.Println("  /reset             - Clear history and start new conversation")
			fmt.Println("  /new               - Start a new conversation (keeps old one)")
			fmt.Println("  /fork              - Fork: new conversation with current history")
			fmt.Println("  /conversations     - List server-side conversations")
			fmt.Println("  /save              - Save current session")
			fmt.Println("  /multiline         - Toggle multiline mode")
			fmt.Println("  /file <path>       - Send contents of file as the next message")
			fmt.Println("  /help              - Show this help")
			continue
		case "/multiline":
			multiline = !multiline
			if multiline {
				fmt.Println("Multiline mode ON (send with empty line)")
			} else {
				fmt.Println("Multiline mode OFF")
			}
			continue
		}

		userMsg := ChatMessage{
			Role:      "user",
			Content:   input,
			Timestamp: time.Now(),
		}
		session.Messages = append(session.Messages, userMsg)

		if session.SeqNum == 0 {
			session.SeqNum = 1
		}
		userMsg.SeqNum = session.SeqNum

		wireHistory := buildWireHistory(session)
		chatReq := api.ChatRequest{
			ProjectID:      notebookID,
			Prompt:         input,
			SourceIDs:      sourceIDs,
			ConversationID: session.ConversationID,
			History:        wireHistory,
			SeqNum:         session.SeqNum,
		}
		session.SeqNum++

		fmt.Println()
		res, err := streamChatResponse(c, chatReq, opts.Render)

		if err != nil {
			response, chatErr := c.ChatWithHistory(chatReq)
			if chatErr != nil {
				fmt.Printf("\nChat API error: %v\n", err)
				fallbackResponse := getFallbackResponse(input, notebookID)
				fmt.Printf("Assistant: %s\n", fallbackResponse)
				session.Messages = append(session.Messages, ChatMessage{
					Role: "assistant", Content: fallbackResponse, Timestamp: time.Now(),
					SeqNum: session.SeqNum,
				})
			} else {
				fmt.Print(response)
				session.Messages = append(session.Messages, ChatMessage{
					Role: "assistant", Content: response, Timestamp: time.Now(),
					SeqNum: session.SeqNum,
				})
			}
		} else {
			response := strings.TrimSpace(res.Answer)
			if response != "" {
				session.Messages = append(session.Messages, ChatMessage{
					Role: "assistant", Content: response, Timestamp: time.Now(),
					Thinking:  res.Thinking,
					Citations: res.Citations,
					SeqNum:    session.SeqNum,
				})
			}
		}
		fmt.Println()

		session.UpdatedAt = time.Now()

		if len(session.Messages)%6 == 0 {
			if err := saveChatSession(session); err != nil && debug {
				fmt.Printf("Debug: Auto-save failed: %v\n", err)
			}
		}

		fmt.Println()
	}

	if err := saveChatSession(session); err != nil && debug {
		fmt.Printf("Debug: Failed to save session on exit: %v\n", err)
	}

	return nil
}

// startAutoRefreshIfEnabled starts the auto-refresh manager if credentials exist
func startAutoRefreshIfEnabled() {
	// Check if NLM_AUTO_REFRESH is disabled
	if os.Getenv("NLM_AUTO_REFRESH") == "false" {
		return
	}

	// Check if we have stored credentials
	token, err := auth.GetStoredToken()
	if err != nil {
		// No stored credentials, skip auto-refresh
		return
	}

	// Parse token to check if it's valid
	_, expiryTime, err := auth.ParseAuthToken(token)
	if err != nil {
		// Invalid token format, skip auto-refresh
		return
	}

	// Check if token is already expired
	if time.Until(expiryTime) < 0 {
		if debug {
			fmt.Fprintf(os.Stderr, "nlm: stored token expired, skipping auto-refresh\n")
		}
		return
	}

	// Create and start token manager
	tokenManager := auth.NewTokenManager(debug || os.Getenv("NLM_DEBUG") == "true")
	if err := tokenManager.StartAutoRefreshManager(); err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "nlm: failed to start auto-refresh: %v\n", err)
		}
		return
	}

	if debug {
		fmt.Fprintf(os.Stderr, "nlm: auto-refresh enabled (token expires in %v)\n", time.Until(expiryTime).Round(time.Minute))
	}
}

func createVideoOverview(c *api.Client, projectID string, instructions string) error {
	// NLM may limit to one video per notebook. Check for existing.
	existingVideos, _ := c.ListVideoOverviews(projectID)
	if len(existingVideos) > 0 && !yes {
		fmt.Fprintf(os.Stderr, "Notebook already has a video overview. Use -y to replace it.\n")
		return fmt.Errorf("existing video overview")
	}

	fmt.Fprintf(os.Stderr, "Creating video overview for notebook %s...\n", projectID)
	fmt.Printf("Instructions: %s\n", instructions)

	result, err := c.CreateVideoOverview(projectID, instructions)
	if err != nil {
		return fmt.Errorf("create video overview: %w", err)
	}

	if !result.IsReady {
		fmt.Fprintln(os.Stderr, "Video overview creation started. Video generation may take several minutes.")
		fmt.Fprintf(os.Stderr, "  Project ID: %s\n", result.ProjectID)
		return nil
	}

	// If the result is immediately ready (unlikely but possible)
	fmt.Fprintf(os.Stderr, "Video overview created:\n")
	fmt.Printf("  Title: %s\n", result.Title)
	fmt.Printf("  Video ID: %s\n", result.VideoID)

	if result.VideoData != "" {
		fmt.Printf("  Video URL: %s\n", result.VideoData)
	}

	return nil
}

func listAudioOverviews(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Listing audio overviews for notebook %s...\n", notebookID)

	audioOverviews, err := c.ListAudioOverviews(notebookID)
	if err != nil {
		return fmt.Errorf("list audio overviews: %w", err)
	}

	if len(audioOverviews) == 0 {
		fmt.Fprintln(os.Stderr, "No audio overviews found.")
		return nil
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "ID\tTITLE\tSTATUS")
	for _, audio := range audioOverviews {
		status := "pending"
		if audio.IsReady {
			status = "ready"
		}
		title := audio.Title
		if title == "" {
			title = "(untitled)"
		}
		id := audio.AudioID
		if id == "" {
			id = "(unknown)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			id,
			title,
			status,
		)
	}
	return flush()
}

func listVideoOverviews(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Listing video overviews for notebook %s...\n", notebookID)

	videoOverviews, err := c.ListVideoOverviews(notebookID)
	if err != nil {
		return fmt.Errorf("list video overviews: %w", err)
	}

	if len(videoOverviews) == 0 {
		fmt.Fprintln(os.Stderr, "No video overviews found.")
		return nil
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "VIDEO_ID\tTITLE\tSTATUS")
	for _, video := range videoOverviews {
		status := "pending"
		if video.IsReady {
			status = "ready"
		}
		title := video.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			video.VideoID,
			title,
			status,
		)
	}
	return flush()
}

func downloadAudioOverview(c *api.Client, notebookID string, filename string) error {
	fmt.Fprintf(os.Stderr, "Downloading audio overview for notebook %s...\n", notebookID)

	// Generate default filename if not provided
	if filename == "" {
		filename = fmt.Sprintf("audio_overview_%s.wav", notebookID)
	}

	// Download the audio
	audioResult, err := c.DownloadAudioOverview(notebookID)
	if err != nil {
		// Provide actionable guidance for the known CDN auth issue
		if strings.Contains(err.Error(), "browser authentication") || strings.Contains(err.Error(), "text/html") {
			return fmt.Errorf("download audio overview: Google CDN requires browser session cookies that cannot be forwarded via CLI; download manually from https://notebooklm.google.com/notebook/%s", notebookID)
		}
		return fmt.Errorf("download audio overview: %w", err)
	}

	// Save to file
	if err := audioResult.SaveAudioToFile(filename); err != nil {
		return fmt.Errorf("save audio file: %w", err)
	}

	fmt.Println(filename)

	// Show file info on stderr so scripts can capture the filename from stdout.
	fmt.Fprintf(os.Stderr, "Audio saved to: %s\n", filename)
	if stat, err := os.Stat(filename); err == nil {
		fmt.Fprintf(os.Stderr, "  File size: %.2f MB\n", float64(stat.Size())/(1024*1024))
	}

	return nil
}

func downloadVideoOverview(c *api.Client, notebookID string, filename string) error {
	fmt.Fprintf(os.Stderr, "Downloading video overview for notebook %s...\n", notebookID)

	// Generate default filename if not provided
	if filename == "" {
		filename = fmt.Sprintf("video_overview_%s.mp4", notebookID)
	}

	// Download the video
	videoResult, err := c.DownloadVideoOverview(notebookID)
	if err != nil {
		if strings.Contains(err.Error(), "browser authentication") || strings.Contains(err.Error(), "manual") || strings.Contains(err.Error(), "not available") {
			return fmt.Errorf("download video overview: Google CDN requires browser session cookies that cannot be forwarded via CLI; download manually from https://notebooklm.google.com/notebook/%s", notebookID)
		}
		return fmt.Errorf("download video overview: %w", err)
	}

	// Check if we got a video URL
	if videoResult.VideoData != "" && (strings.HasPrefix(videoResult.VideoData, "http://") || strings.HasPrefix(videoResult.VideoData, "https://")) {
		// Use authenticated download for URLs
		if err := c.DownloadVideoWithAuth(videoResult.VideoData, filename); err != nil {
			if strings.Contains(err.Error(), "text/html") {
				return fmt.Errorf("download video: Google CDN requires browser session cookies that cannot be forwarded via CLI; download manually from https://notebooklm.google.com/notebook/%s", notebookID)
			}
			return fmt.Errorf("download video with auth: %w", err)
		}
	} else {
		// Try to save base64 data or handle other formats
		if err := videoResult.SaveVideoToFile(filename); err != nil {
			return fmt.Errorf("save video file: %w", err)
		}
	}

	fmt.Println(filename)

	// Show file info on stderr so scripts can capture the filename from stdout.
	fmt.Fprintf(os.Stderr, "Video saved to: %s\n", filename)
	if stat, err := os.Stat(filename); err == nil {
		fmt.Fprintf(os.Stderr, "  File size: %.2f MB\n", float64(stat.Size())/(1024*1024))
	}

	return nil
}
