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
	"github.com/tmc/nlm/internal/nlmmcp"
	"github.com/tmc/nlm/internal/notebooklm/api"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	nlmsync "github.com/tmc/nlm/internal/sync"
	"golang.org/x/term"
)

// Global flags
var (
	authToken         string
	cookies           string
	debug             bool
	debugDumpPayload  bool
	debugParsing      bool
	debugFieldMapping bool
	chromeProfile     string
	mimeType          string
	chunkedResponse   bool   // Control rt=c parameter for chunked vs JSON array response
	useDirectRPC      bool   // Use direct RPC calls instead of orchestration service
	skipSources       bool   // Skip fetching sources for chat (useful when project is inaccessible)
	yes               bool   // Skip confirmation prompts
	sourceName        string // Custom name for added sources
	showChatHistory   bool   // Show previous chat conversation on start
	showThinking      bool   // Show thinking headers while streaming responses
	verbose           bool   // Show full thinking traces while streaming responses
	replaceSourceID   string // Source ID to replace when adding
	force             bool   // Force re-upload even if unchanged
	dryRun            bool   // Show what would change without uploading
	maxBytes          int    // Chunk threshold for sync-source
	jsonOutput        bool   // NDJSON output for sync-source
	reportPrompt       string // Per-section prompt template for generate-report ({topic} replaced)
	reportInstructions string // Notebook instructions to set before generate-report
	reportSections     int    // Max sections for generate-report (0 = all)
	conversationID    string // Conversation ID to continue (generate-chat)
	useWebChat        bool   // Use most recent server-side conversation (generate-chat)
	citationMode      string // Citation rendering mode: off|block|overlay (default block-on-TTY)
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
	flag.BoolVar(&force, "force", false, "force re-upload even if unchanged (sync-source)")
	flag.BoolVar(&dryRun, "dry-run", false, "show what would change without uploading (sync-source)")
	flag.IntVar(&maxBytes, "max-bytes", 0, "chunk threshold in bytes (sync-source, default 5120000)")
	flag.StringVar(&reportPrompt, "prompt", "", "per-section prompt template for generate-report ({topic} is replaced)")
	flag.StringVar(&reportInstructions, "instructions", "", "set notebook instructions before generate-report")
	flag.IntVar(&reportSections, "sections", 0, "max sections to generate (generate-report, 0=all)")
	flag.StringVar(&conversationID, "conversation", "", "continue an existing conversation by ID (generate-chat)")
	flag.StringVar(&conversationID, "c", "", "continue an existing conversation by ID (shorthand)")
	flag.BoolVar(&useWebChat, "web", false, "use the most recent server-side conversation (generate-chat)")
	flag.BoolVar(&showChatHistory, "history", false, "show previous chat conversation on start")
	flag.BoolVar(&showThinking, "thinking", false, "show thinking headers while streaming chat and generate-chat responses")
	flag.BoolVar(&showThinking, "reasoning", false, "show thinking headers while streaming chat and generate-chat responses")
	flag.BoolVar(&verbose, "verbose", false, "show full thinking traces while streaming chat and generate-chat responses")
	flag.BoolVar(&verbose, "v", false, "show full thinking traces while streaming responses (shorthand)")
	flag.StringVar(&citationMode, "citations", "", "citation rendering: off|block|stream|tail|overlay (default: block on TTY, off when piped)")

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
				flags = append(flags, arg)
				if !boolFlags[name] && !strings.Contains(arg, "=") && i+1 < len(os.Args) {
					i++
					flags = append(flags, os.Args[i])
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
		fmt.Fprintf(os.Stderr, "nlm: %v\n", err)
		os.Exit(1)
	}
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
		os.Exit(1)
	}

	cmdName := flag.Arg(0)
	args := flag.Args()[1:]

	// Handle help aliases.
	if helpAliases[cmdName] {
		flag.Usage()
		os.Exit(0)
	}

	// Look up command in the table.
	entry, ok := lookupCommand(cmdName)
	if !ok {
		flag.Usage()
		os.Exit(1)
	}

	// Check for help flags in subcommand args.
	for _, a := range args {
		if a == "--help" || a == "-h" || a == "-help" {
			fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n  %s\n", cmdName, entry.argsUsage, entry.usage)
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

	for i := 0; i < 3; i++ {
		if i > 0 {
			if i == 1 {
				fmt.Fprintln(os.Stderr, "nlm: authentication expired, refreshing credentials...")
			} else {
				fmt.Fprintln(os.Stderr, "nlm: retrying authentication...")
			}
			debug = true
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

		// Authentication error detected, try to refresh
		if debug {
			fmt.Fprintf(os.Stderr, "nlm: detected authentication error: %v\n", cmdErr)
		}

		var authErr error
		if authToken, cookies, authErr = handleAuth(nil, debug); authErr != nil {
			fmt.Fprintf(os.Stderr, "nlm: authentication refresh failed: %v\n", authErr)
			if i == 2 { // Last attempt
				return fmt.Errorf("authentication failed after 3 attempts: %w", authErr)
			}
		}
	}
	return fmt.Errorf("nlm: authentication failed after 3 attempts")
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
func confirmAction(prompt string) bool {
	if yes {
		return true
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
	fmt.Fprintf(os.Stderr, "%s [Y/n] ", prompt)
	var response string
	fmt.Scanln(&response)
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "" || strings.HasPrefix(response, "y")
}

// Notebook operations
func list(c *api.Client) error {
	notebooks, err := c.ListRecentlyViewedProjects()
	if err != nil {
		return err
	}

	// Display total count
	total := len(notebooks)
	fmt.Fprintf(os.Stderr, "Total notebooks: %d (showing first 10)\n\n", total)

	// Limit to first 10 entries
	limit := 10
	if len(notebooks) < limit {
		limit = len(notebooks)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tSOURCES\tLAST UPDATED")
	for i := 0; i < limit; i++ {
		nb := notebooks[i]
		// Use backspace to compensate for emoji width
		emoji := strings.TrimSpace(nb.Emoji)
		var title string
		if emoji != "" {
			title = emoji + " \b" + nb.Title // Backspace after space to undo emoji extra width
		} else {
			title = nb.Title
		}
		// Truncate title to account for display width with emojis
		if len(title) > 45 {
			title = title[:42] + "..."
		}
		sourceCount := len(nb.Sources)
		ts := nb.GetMetadata().GetModifiedTime()
		if ts == nil {
			ts = nb.GetMetadata().GetCreateTime()
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			nb.ProjectId, title, sourceCount,
			ts.AsTime().Format(time.RFC3339),
		)
	}
	return w.Flush()
}

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

// Source operations
func listSources(c *api.Client, notebookID string) error {
	p, err := c.GetProject(notebookID)
	if err != nil {
		return fmt.Errorf("list sources: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
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
	return w.Flush()
}

func formatSourceStatus(src *pb.Source) string {
	if src.Settings != nil && src.Settings.Status != 0 {
		switch src.Settings.Status {
		case 1:
			return "enabled"
		case 2:
			return "disabled"
		case 3:
			return "error"
		}
	}
	if src.Metadata != nil && src.Metadata.Status != 0 {
		switch src.Metadata.Status {
		case 1:
			return "enabled"
		case 2:
			return "disabled"
		case 3:
			return "error"
		}
	}
	// Show warning codes if present.
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

func addSource(c *api.Client, notebookID, input string) (string, error) {
	// Handle special input designators
	switch input {
	case "-": // stdin
		fmt.Fprintln(os.Stderr, "Reading from stdin...")
		name := "Pasted Text"
		if sourceName != "" {
			name = sourceName
		}
		if mimeType != "" {
			fmt.Fprintf(os.Stderr, "Using specified MIME type: %s\n", mimeType)
			return c.AddSourceFromReader(notebookID, os.Stdin, name, mimeType)
		}
		return c.AddSourceFromReader(notebookID, os.Stdin, name)
	case "": // empty input
		return "", fmt.Errorf("input required (file, URL, or '-' for stdin)")
	}

	// Check if input is a URL
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		fmt.Fprintf(os.Stderr, "Adding source from URL: %s\n", input)
		return c.AddSourceFromURL(notebookID, input)
	}

	// Try as local file
	if _, err := os.Stat(input); err == nil {
		fmt.Fprintf(os.Stderr, "Adding source from file: %s\n", input)
		name := filepath.Base(input)
		if sourceName != "" {
			name = sourceName
		}
		if mimeType != "" {
			fmt.Fprintf(os.Stderr, "Using specified MIME type: %s\n", mimeType)
			file, err := os.Open(input)
			if err != nil {
				return "", fmt.Errorf("open file: %w", err)
			}
			defer file.Close()
			return c.AddSourceFromReader(notebookID, file, name, mimeType)
		}
		if sourceName != "" {
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
	if sourceName != "" {
		textName = sourceName
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
	// Always use text path — sync-source content is txtar, never binary.
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

func removeSource(c *api.Client, notebookID, sourceID string) error {
	if !confirmActionDefaultYes(fmt.Sprintf("Are you sure you want to remove source %s?", sourceID)) {
		return fmt.Errorf("operation cancelled")
	}

	if err := c.DeleteSources(notebookID, []string{sourceID}); err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Removed source %s from notebook %s\n", sourceID, notebookID)
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
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
	return w.Flush()
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

func generateMagicView(c *api.Client, notebookID string, sourceIDs []string) error {
	fmt.Fprintf(os.Stderr, "Generating magic view...\n")
	magicView, err := c.GenerateMagicView(notebookID, sourceIDs)
	if err != nil {
		return fmt.Errorf("generate magic view: %w", err)
	}

	fmt.Printf("Magic View: %s\n", magicView.Title)
	if len(magicView.Items) > 0 {
		fmt.Printf("\nItems:\n")
		for i, item := range magicView.Items {
			fmt.Printf("%d. %s\n", i+1, item.Title)
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

// Other operations
// createArtifact dispatches to the type-specific creation methods.
// All types use R7cb6c under the hood.
func createArtifact(c *api.Client, notebookID, artifactType, instructions string) error {
	switch strings.ToLower(artifactType) {
	case "audio":
		return createAudioOverview(c, notebookID, instructions)
	case "video":
		return createVideoOverview(c, notebookID, instructions)
	case "slides":
		artifactID, err := c.CreateSlideDeck(notebookID, instructions)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Created slide deck: %s\n", artifactID)
		fmt.Fprintf(os.Stderr, "Use 'nlm artifacts %s' to check status.\n", notebookID)
		return nil
	case "report":
		if instructions == "" {
			return fmt.Errorf("report type requires instructions: nlm create-artifact <nb> report <topic> [description]")
		}
		parts := strings.SplitN(instructions, " ", 2)
		topic := parts[0]
		desc := ""
		if len(parts) > 1 {
			desc = parts[1]
		}
		return createReport(c, notebookID, topic, []string{desc})
	default:
		return fmt.Errorf("unknown artifact type %q (audio, video, slides, report)", artifactType)
	}
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
	fmt.Printf("Instructions: %s\n", instructions)

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

// Analytics and featured projects
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tDESCRIPTION")

	for _, project := range resp.Projects {
		description := ""
		if project.Presentation != nil && strings.TrimSpace(project.Presentation.Description) != "" {
			description = strings.TrimSpace(project.Presentation.Description)
		} else if len(project.Sources) > 0 {
			description = fmt.Sprintf("%d sources", len(project.Sources))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			project.ProjectId,
			strings.TrimSpace(project.Emoji)+" "+project.Title,
			description)
	}
	return w.Flush()
}

// Enhanced source operations
func refreshSource(c *api.Client, notebookID, sourceID string) error {
	fmt.Fprintf(os.Stderr, "Refreshing source %s...\n", sourceID)
	source, err := c.RefreshSource(notebookID, sourceID)
	if err != nil {
		return fmt.Errorf("refresh source: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Refreshed source: %s\n", source.Title)
	return nil
}

func checkSourceFreshness(c *api.Client, sourceID string) error {
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	req := &pb.CheckSourceFreshnessRequest{
		SourceId: sourceID,
	}

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

func discoverSources(c *api.Client, projectID, query string) error {
	fmt.Fprintf(os.Stderr, "DiscoverSources is deprecated upstream; using deep research workflow instead.\n")
	if err := deepResearch(c, projectID, query); err == nil {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Deep research is unavailable; falling back to notebook suggestions.\n")
	res, err := streamChatResponse(c, api.ChatRequest{
		ProjectID: projectID,
		Prompt:    fmt.Sprintf("Suggest sources to add for this query: %s. Respond with a short bullet list of specific documents, sites, or search directions.", query),
	})
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
		fmt.Println("No artifacts found in project.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tSTATE\tSOURCES")

	for _, artifact := range artifacts {
		sourceCount := fmt.Sprintf("%d", len(artifact.Sources))

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			artifact.ArtifactId,
			artifact.Type.String(),
			artifact.State.String(),
			sourceCount)
	}
	return w.Flush()
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

	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	req := &pb.DeleteArtifactRequest{
		ArtifactId: artifactID,
	}

	_, err := orchClient.DeleteArtifact(context.Background(), req)
	if err != nil {
		return fmt.Errorf("delete artifact: %w", err)
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
	out             io.Writer
	status          io.Writer
	showThinking    bool
	verbose         bool
	citationMode    citationRenderMode
	tailWindow      int                          // tail mode: max bytes held back for splicing
	resolveTitle    func(sourceID string) string // optional; returns "" if unknown
	lastThinkingLen int
	answerBuf       strings.Builder
	thinking        string
	citations       []api.Citation
	followUps       []string

	// tail-mode bookkeeping
	flushedLen int // absolute cumulative-answer offset of bytes already written to r.out
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

func (r *chatStreamRenderer) Finish() {
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
	// Footer: show inline entries as superscripts, spilled as bracket indices.
	fmt.Fprintln(r.status)
	fmt.Fprintln(r.status, ansiGrey+strings.Repeat("─", 3)+ansiReset)
	for _, c := range r.citations {
		label := r.citationLabel(c)
		marker := superscript(c.SourceIndex)
		for _, s := range spilled {
			if s.SourceIndex == c.SourceIndex {
				marker = fmt.Sprintf("[%d]", c.SourceIndex)
				break
			}
		}
		fmt.Fprintf(r.status, "%s%s %s%s\n", ansiGrey, marker, label, ansiReset)
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
		fmt.Fprintf(r.status, "%s%s %s%s\n", ansiGrey, superscript(c.SourceIndex), label, ansiReset)
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
		fmt.Fprintf(r.status, "%s  [%d] chars %d-%d — %s%s\n",
			ansiGrey, c.SourceIndex, c.StartChar, c.EndChar, label, ansiReset)
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
		fmt.Fprintf(r.status, "%s  [%d] %s%s\n", ansiGrey, c.SourceIndex, label, ansiReset)
	}
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
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
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

func streamChatResponse(c *api.Client, req api.ChatRequest) (chatResult, error) {
	mode := resolveCitationMode(citationMode, isTerminal(os.Stdout))
	renderer := newChatStreamRenderer(os.Stdout, os.Stderr, showThinking || verbose || isTerminal(os.Stdout), verbose, mode)
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

// Generation operations
func generateFreeFormChat(c *api.Client, projectID, prompt string) error {
	fmt.Fprintf(os.Stderr, "Generating response for: %s\n", prompt)

	chatReq := api.ChatRequest{
		ProjectID: projectID,
		Prompt:    prompt,
	}

	// Resolve conversation context from flags.
	convID, history, seqNum, err := resolveGenerateChatConversation(c, projectID)
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

	res, err := streamChatResponse(c, chatReq)
	if err != nil {
		// Fall back to non-streaming path (mirrors oneShotChat behavior).
		response, chatErr := c.ChatWithHistory(chatReq)
		if chatErr != nil {
			return fmt.Errorf("generate chat: %w", err)
		}
		fmt.Print(response)
		res.Answer = response
	}
	if res.Answer != "" {
		fmt.Println()
	} else if thinking := strings.TrimSpace(res.Thinking); thinking != "" {
		fmt.Fprintln(os.Stderr, "nlm: no answer token received; printing thinking trace")
		fmt.Println(thinking)
	} else {
		fmt.Println("(No response received)")
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
	if res.Answer != "" {
		session.Messages = append(session.Messages, ChatMessage{
			Role: "assistant", Content: strings.TrimSpace(res.Answer), Timestamp: time.Now(),
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
func resolveGenerateChatConversation(c *api.Client, projectID string) (string, []api.ChatMessage, int, error) {
	if useWebChat {
		// Fetch the most recent server-side conversation.
		convIDs, err := c.GetConversations(projectID)
		if err != nil {
			return "", nil, 0, fmt.Errorf("list server conversations: %w", err)
		}
		if len(convIDs) == 0 {
			return "", nil, 0, fmt.Errorf("no server-side conversations found for this notebook")
		}
		conversationID = convIDs[0]
		fmt.Fprintf(os.Stderr, "Using server conversation: %s\n", conversationID[:8])
	}

	if conversationID == "" {
		return "", nil, 0, nil
	}

	// Try local session first for richer history.
	session, err := loadChatSessionForConv(projectID, conversationID)
	if err == nil && len(session.Messages) > 0 {
		fmt.Fprintf(os.Stderr, "Continuing conversation %s (%d messages)\n",
			session.ConversationID[:8], len(session.Messages))
		wireHistory := buildWireHistory(session)
		return session.ConversationID, wireHistory, len(session.Messages)/2 + 1, nil
	}

	// No local session — use the conversation ID with no history.
	// The server remembers prior messages for server-side conversations.
	fmt.Fprintf(os.Stderr, "Continuing conversation %s (server-side)\n", conversationID[:8])
	return conversationID, nil, 0, nil
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
func createReport(c *api.Client, notebookID, reportType string, extra []string) error {
	description := ""
	instructions := ""
	if len(extra) > 0 {
		description = extra[0]
	}
	if len(extra) > 1 {
		instructions = strings.Join(extra[1:], " ")
	}

	// Try to match reportType against suggestions for targeted source_ids.
	var sourceIDs []string
	resp, err := c.GenerateReportSuggestions(notebookID)
	if err == nil {
		for _, s := range resp.GetSuggestions() {
			if strings.EqualFold(s.GetTitle(), reportType) {
				if description == "" {
					description = s.GetDescription()
				}
				sourceIDs = s.GetSourceIds()
				fmt.Fprintf(os.Stderr, "Matched suggestion %q (%d sources)\n", s.GetTitle(), len(sourceIDs))
				break
			}
		}
	}

	artifactID, err := c.CreateReport(notebookID, reportType, description, instructions, sourceIDs...)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created report: %s\n", artifactID)
	fmt.Fprintf(os.Stderr, "Use 'nlm artifacts %s' to check status.\n", notebookID)
	return nil
}

func generateReport(c *api.Client, notebookID string) error {
	// Optionally set notebook instructions.
	if reportInstructions != "" {
		fmt.Fprintf(os.Stderr, "Setting instructions...\n")
		if err := c.SetInstructions(notebookID, reportInstructions); err != nil {
			return fmt.Errorf("set instructions: %w", err)
		}
	}

	// Read suggestions from stdin or API.
	suggestions, err := readReportSuggestions(c, notebookID)
	if err != nil {
		return err
	}

	// Limit sections if requested.
	if reportSections > 0 && reportSections < len(suggestions) {
		suggestions = suggestions[:reportSections]
	}

	// Resolve per-section prompt template.
	tmpl := defaultReportPrompt
	if reportPrompt != "" {
		tmpl = reportPrompt
	}

	fmt.Fprintf(os.Stderr, "Generating %d sections...\n", len(suggestions))

	for i, s := range suggestions {
		title := s.GetTitle()
		fmt.Fprintf(os.Stderr, "[%d/%d] %s\n", i+1, len(suggestions), title)

		prompt := strings.ReplaceAll(tmpl, "{topic}", title)
		// Use suggestion-specific prompt if available and no custom template set.
		if reportPrompt == "" && s.GetPrompt() != "" {
			prompt = s.GetPrompt()
		}
		chatReq := api.ChatRequest{
			ProjectID: notebookID,
			Prompt:    prompt,
			SourceIDs: s.GetSourceIds(),
		}
		res, err := streamChatResponse(c, chatReq)
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
	fmt.Println("Chat history deleted.")
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
func oneShotChat(c *api.Client, notebookID, prompt string) error {
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
		ConversationID: session.ConversationID,
		History:        wireHistory,
		SeqNum:         len(session.Messages)/2 + 1,
	}

	res, err := streamChatResponse(c, chatReq)
	if err != nil {
		response, chatErr := c.ChatWithHistory(chatReq)
		if chatErr != nil {
			return fmt.Errorf("chat: %w", err)
		}
		fmt.Print(response)
		res.Answer = response
	}
	if res.Answer == "" {
		if thinking := strings.TrimSpace(res.Thinking); thinking != "" {
			fmt.Fprintln(os.Stderr, "nlm: no answer token received; printing thinking trace")
			fmt.Println(thinking)
		}
	}
	fmt.Println()

	// Save response with thinking trace and citations.
	response := strings.TrimSpace(res.Answer)
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
func interactiveChatWithConv(c *api.Client, notebookID, conversationID string) error {
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

	return runInteractiveChat(c, session)
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
	session, localErr := loadChatSessionForConv(notebookID, conversationID)
	if localErr != nil {
		if err != nil {
			return fmt.Errorf("server: %w; no local session found", err)
		}
		return fmt.Errorf("no conversation history found")
	}
	if len(session.Messages) == 0 {
		fmt.Println("No messages in conversation.")
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

// chatShow renders a locally-stored conversation with full citation modes.
// Unlike chat-history (which prefers server-side), chat-show reads only the
// local session so it can surface persisted citation metadata (char ranges,
// source IDs) that the server doesn't return in conversation history.
func chatShow(notebookID, conversationID string) error {
	session, err := loadChatSessionForConv(notebookID, conversationID)
	if err != nil {
		// Fall back to the single-session-per-notebook path (older layout).
		legacy, legacyErr := loadChatSession(notebookID)
		if legacyErr != nil || legacy.ConversationID != conversationID {
			return fmt.Errorf("load local session: %w", err)
		}
		session = legacy
	}
	if len(session.Messages) == 0 {
		fmt.Println("No messages in local session.")
		return nil
	}

	mode := resolveCitationMode(citationMode, isTerminal(os.Stdout))
	for i, m := range session.Messages {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("[%s]\n", strings.ToUpper(m.Role))

		if m.Role != "assistant" {
			fmt.Println(m.Content)
			continue
		}
		if showThinking && m.Thinking != "" {
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
		fmt.Println("No conversations found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CONVERSATION\tMESSAGES\tSTATUS\tLAST UPDATED")
	fmt.Fprintln(w, "------------\t--------\t------\t------------")

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

	return w.Flush()
}

func deepResearch(c *api.Client, notebookID, query string) error {
	rpcClient := rpc.New(authToken, cookies)

	// Start deep research
	fmt.Fprintf(os.Stderr, "Starting deep research: %s\n", query)
	startResp, err := rpcClient.Do(rpc.Call{
		ID:         rpc.RPCStartDeepResearch,
		NotebookID: notebookID,
		Args:       []interface{}{notebookID, query, []interface{}{2}},
	})
	if err != nil {
		return fmt.Errorf("start deep research: %w", err)
	}

	// Extract research ID from response
	var startData []interface{}
	if err := json.Unmarshal(startResp, &startData); err != nil {
		return fmt.Errorf("parse start response: %w", err)
	}

	// Research ID is typically at position [0]
	researchID := ""
	if len(startData) > 0 {
		if id, ok := startData[0].(string); ok {
			researchID = id
		}
	}
	if researchID == "" {
		// Print raw response for debugging
		fmt.Fprintf(os.Stderr, "Research started (raw response: %s)\n", string(startResp))
		researchID = notebookID // fallback: use notebook ID for polling
	} else {
		fmt.Fprintf(os.Stderr, "Research ID: %s\n", researchID)
	}

	// Poll for results
	fmt.Fprintf(os.Stderr, "Polling for results")
	for i := 0; i < 120; i++ { // max 10 minutes (120 * 5s)
		time.Sleep(5 * time.Second)
		fmt.Fprintf(os.Stderr, ".")

		pollResp, err := rpcClient.Do(rpc.Call{
			ID:         rpc.RPCPollDeepResearch,
			NotebookID: notebookID,
			Args:       []interface{}{notebookID, researchID, []interface{}{2}},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n")
			return fmt.Errorf("poll deep research: %w", err)
		}

		var pollData []interface{}
		if err := json.Unmarshal(pollResp, &pollData); err != nil {
			continue // response may not be parseable yet
		}

		// Check if research is complete — look for content in response
		// The response payload grows as the research progresses
		if len(pollResp) > 1000 {
			fmt.Fprintf(os.Stderr, "\nResearch complete.\n\n")
			// Extract and print the research content
			// The content is typically a large text blob in the response
			fmt.Println(string(pollResp))
			return nil
		}
	}

	fmt.Fprintf(os.Stderr, "\nResearch timed out after 10 minutes.\n")
	return fmt.Errorf("research timed out")
}

func setInstructions(c *api.Client, notebookID, prompt string) error {
	if err := c.SetChatConfig(notebookID, api.ChatGoalCustom, prompt, api.ResponseLengthDefault); err != nil {
		return fmt.Errorf("set instructions: %w", err)
	}
	fmt.Println("Instructions updated.")
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
		fmt.Println("No custom instructions set.")
		return nil
	}

	fmt.Println(prompt)
	return nil
}

// Utility functions for commented-out operations
func shareNotebook(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating public share link...\n")

	// Create RPC client directly for sharing project
	rpcClient := rpc.New(authToken, cookies)
	// Wire format from JS analysis (mAb function):
	//   field 1: repeated YM [{field 1: projectId, field 3: Uzb{field 1: true} (link sharing)}]
	//   field 2: bool (M3 flag)
	//   field 4: ProjectContext [2]
	// HAR-verified wire format:
	// [  [["notebook-id", null, [1, 1], [0, ""]]]  , 1, null, [2] ]
	// linkSettings [1, 1] = enabled + public; [1, 0] = enabled + private
	call := rpc.Call{
		ID: "QDyure", // ShareProject RPC ID
		Args: []interface{}{
			[]interface{}{[]interface{}{notebookID, nil, []interface{}{1, 1}, []interface{}{0, ""}}},
			1,                // int, not bool
			nil,              // gap
			[]interface{}{2}, // ProjectContext
		},
	}

	resp, err := rpcClient.Do(call)
	if err != nil {
		return fmt.Errorf("share project: %w", err)
	}

	// Parse response to extract share URL
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if debug {
		raw, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(os.Stderr, "DEBUG: share response: %s\n", raw)
	}

	// Search for a URL string in the response (may be nested at various depths)
	if url := findShareURL(data); url != "" {
		fmt.Printf("Share URL: %s\n", url)
		return nil
	}

	// If no URL in response, the share succeeded but URL is constructed from project ID
	fmt.Printf("Share URL: https://notebooklm.google.com/notebook/%s\n", notebookID)
	return nil
}

// findShareURL recursively searches a JSON structure for a URL string.
func findShareURL(v interface{}) string {
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "http") && strings.Contains(val, "notebooklm") {
			return val
		}
	case []interface{}:
		for _, item := range val {
			if url := findShareURL(item); url != "" {
				return url
			}
		}
	}
	return ""
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
	if resp.GetShareUrl() != "" {
		fmt.Printf("Private Share URL: %s\n", resp.GetShareUrl())
		return nil
	}
	fmt.Printf("Project shared privately (URL format not recognized)\n")
	return nil
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
	title := strings.TrimSpace(strings.TrimSpace(details.Emoji) + " " + details.Title)
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
			return filepath.Join(os.TempDir(), fmt.Sprintf("nlm-chat-%s-%s.json", notebookID, conversationID[:8]))
		}
		return filepath.Join(os.TempDir(), fmt.Sprintf("nlm-chat-%s.json", notebookID))
	}

	nlmDir := filepath.Join(homeDir, ".nlm")
	os.MkdirAll(nlmDir, 0700) // Ensure directory exists
	if conversationID != "" {
		return filepath.Join(nlmDir, fmt.Sprintf("chat-%s-%s.json", notebookID, conversationID[:8]))
	}
	return filepath.Join(nlmDir, fmt.Sprintf("chat-%s.json", notebookID))
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

	return os.WriteFile(path, data, 0600)
}

func listChatSessions() error {
	sessions, err := listLocalChatSessions("")
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("No chat sessions found.")
		return nil
	}

	fmt.Printf("Chat Sessions (%d total)\n", len(sessions))
	fmt.Println(strings.Repeat("=", 41))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NOTEBOOK\tCONVERSATION\tMESSAGES\tLAST UPDATED")
	fmt.Fprintln(w, "--------\t------------\t--------\t------------")

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

	return w.Flush()
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
func interactiveChat(c *api.Client, notebookID string) error {
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
	return runInteractiveChat(c, session)
}

// runInteractiveChat runs the interactive chat loop with the given session.
func runInteractiveChat(c *api.Client, session *ChatSession) error {
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
		if !showChatHistory {
			fmt.Println("  (use -history flag to show previous conversation)")
		}
	}

	fmt.Println("\nCommands: /exit /clear /history /reset /new /fork /conversations /save /help /multiline")
	fmt.Println("Type your message and press Enter to send.")

	scanner := bufio.NewScanner(os.Stdin)
	multiline := false

	if showChatHistory && len(session.Messages) > 0 {
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
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					break
				}
				lines = append(lines, line)
				fmt.Print("... > ")
			}
			input = strings.Join(lines, "\n")
		} else {
			if !scanner.Scan() {
				break
			}
			input = scanner.Text()
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
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
			ConversationID: session.ConversationID,
			History:        wireHistory,
			SeqNum:         session.SeqNum,
		}
		session.SeqNum++

		fmt.Println()
		res, err := streamChatResponse(c, chatReq)

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

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read input: %w", err)
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
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
	return w.Flush()
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
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
	return w.Flush()
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

	fmt.Printf("Audio saved to: %s\n", filename)

	// Show file info
	if stat, err := os.Stat(filename); err == nil {
		fmt.Printf("  File size: %.2f MB\n", float64(stat.Size())/(1024*1024))
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

	fmt.Printf("Video saved to: %s\n", filename)

	// Show file info
	if stat, err := os.Stat(filename); err == nil {
		fmt.Printf("  File size: %.2f MB\n", float64(stat.Size())/(1024*1024))
	}

	return nil
}
