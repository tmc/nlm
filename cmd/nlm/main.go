package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	"google.golang.org/protobuf/proto"
)

// Global flags
var (
	showVersion       bool
	experimental      bool // surface experimental commands in help + allow them to run
	authToken         string
	cookies           string
	authUser          string
	debug             bool
	debugDumpPayload  bool
	debugParsing      bool
	debugFieldMapping bool
	chromeProfile     string
	chunkedResponse   bool // Control rt=c parameter for chunked vs JSON array response
	useDirectRPC      bool // Use direct RPC calls instead of orchestration service
	skipSources       bool // Skip fetching sources for chat (useful when project is inaccessible)
	yes               bool // Skip confirmation prompts
	jsonOutput        bool // NDJSON output for sync
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
	Thinking  string           `json:"thinking,omitempty"`  // Reasoning traces from intermediate chunks
	Citations []api.Citation   `json:"citations,omitempty"` // Source references from the response
	Rich      *api.RichContent `json:"rich,omitempty"`      // Answer-body span tree (paragraphs, lists, inline marks); nil for turns generated before this was captured
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func prepareRuntime(stderr io.Writer) {
	if debug {
		fmt.Fprintf(stderr, "nlm: debug mode enabled\n")
		if chromeProfile != "" {
			// Mask potentially sensitive profile names in debug output
			maskedProfile := chromeProfile
			if len(chromeProfile) > 8 {
				maskedProfile = chromeProfile[:4] + "****" + chromeProfile[len(chromeProfile)-4:]
			} else if len(chromeProfile) > 2 {
				maskedProfile = chromeProfile[:2] + "****"
			}
			fmt.Fprintf(stderr, "nlm: using Chrome profile: %s\n", maskedProfile)
		}
	}

	// Load stored environment variables
	loadStoredEnv()
	if authUser == "" {
		authUser = os.Getenv("NLM_AUTHUSER")
	}
	if authUser != "" {
		os.Setenv("NLM_AUTHUSER", authUser)
	}

	// Set skip sources flag if specified
	if skipSources {
		os.Setenv("NLM_SKIP_SOURCES", "true")
		if debug {
			fmt.Fprintf(stderr, "nlm: skipping source fetching for chat\n")
		}
	}

	// Set beprotojson debug options if requested
	if debugParsing || debugFieldMapping {
		beprotojson.SetGlobalDebugOptions(debugParsing, debugFieldMapping)
	}

	// Start auto-refresh manager if credentials exist
	startAutoRefreshIfEnabled()
}

func runCLI(args []string, env func(string) string, stdout, stderr io.Writer) int {
	inv, err := parseInvocation(args, env, stdout, stderr)
	applyGlobalOptions(inv.globals)
	if inv.action == invocationVersion && err == nil {
		fmt.Fprintln(stdout, versionString())
		return 0
	}

	if err == nil || inv.action != invocationRun || inv.cmd != nil {
		prepareRuntime(stderr)
	}

	if err != nil {
		switch inv.action {
		case invocationRootHelp:
			printUsage()
		case invocationSectionHelp:
			printSectionUsage(inv.section)
		case invocationCommandHelp:
			printCommandHelp(inv.name, inv.cmd)
		}
		return reportRunError(stderr, err)
	}

	switch inv.action {
	case invocationRootHelp:
		printUsage()
		return 0
	case invocationSectionHelp:
		printSectionUsage(inv.section)
		return 0
	case invocationCommandHelp:
		warnCompatibilityCommand(inv.name, inv.cmd)
		printCommandHelp(inv.name, inv.cmd)
		return 0
	}

	if err := run(inv); err != nil {
		return reportRunError(stderr, err)
	}
	return 0
}

func printCommandHelp(cmdName string, cmd *command) {
	if cmd.help != nil {
		cmd.help(cmdName)
		return
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n  %s\n", cmdName, cmd.argsUsage, cmd.usage)
}

func reportRunError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "nlm: %s\n", friendlyError(err))
	code := exitCodeFor(err)
	if name := exitCodeName(code); name != "" {
		fmt.Fprintf(stderr, "nlm: exit-class=%s (exit %d)\n", name, code)
	}
	return code
}

// isAuthCommand returns true if the command requires authentication
// validateArgs validates command arguments without requiring authentication

func run(inv invocation) error {
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

	cmdName, entry, args := inv.name, inv.cmd, inv.args
	warnCompatibilityCommand(cmdName, entry)

	// Validate arguments.
	if err := validateCommandArgs(entry, cmdName, args, inv.globals); err != nil {
		return err
	}

	// Commands that don't need an API client run directly.
	if entry.noClient {
		return runCommand(entry, nil, args, inv.globals)
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
		if authUser != "" {
			client.SetAuthUser(authUser)
		}
		// Set direct RPC flag if specified
		if useDirectRPC {
			client.SetUseDirectRPC(true)
			if debug {
				fmt.Fprintf(os.Stderr, "nlm: using direct RPC for audio/video operations\n")
			}
		}
		cmdErr := runCommand(entry, client, args, inv.globals)
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

	var apiErr *batchexecute.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode != nil {
			switch apiErr.ErrorCode.Type {
			case batchexecute.ErrorTypeAuthentication:
				return true
			case batchexecute.ErrorTypeAuthorization,
				batchexecute.ErrorTypePermissionDenied,
				batchexecute.ErrorTypeNotFound:
				return false
			}
		}
		switch apiErr.HTTPStatus {
		case 401:
			return true
		case 403, 404:
			return false
		}
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

var openControllingTTY = func() (io.ReadWriteCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

// confirmAction prompts the user for confirmation unless --yes is set. The
// prompt uses /dev/tty, not stdin, so commands can pipe IDs or content into
// stdin without the confirmation prompt consuming pipeline data.
func confirmAction(prompt string) bool {
	return confirm(prompt, false)
}

func confirmActionDefaultYes(prompt string) bool {
	return confirm(prompt, true)
}

func confirm(prompt string, defaultYes bool) bool {
	if yes {
		return true
	}

	tty, err := openControllingTTY()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\nrefusing to prompt without a controlling terminal; pass -y to confirm\n", prompt)
		return false
	}
	defer tty.Close()

	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(tty, "%s %s ", prompt, suffix)
	response, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "read confirmation: %v\n", err)
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" {
		return defaultYes
	}
	return strings.HasPrefix(response, "y")
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

	// labelsBySource is empty when GetLabels fails or the notebook has no
	// labels. We don't fail the list on label errors; missing labels are a
	// strictly-additive view.
	labelsBySource := make(map[string][]string)
	hasAnyLabels := false
	if labels, err := c.GetLabels(notebookID); err == nil {
		for _, l := range labels {
			for _, sid := range l.SourceIDs {
				labelsBySource[sid] = append(labelsBySource[sid], l.Name)
			}
		}
		hasAnyLabels = len(labels) > 0
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, src := range p.Sources {
			id := src.SourceId.GetSourceId()
			rec := sourceListRecord{
				SourceID:    id,
				Title:       strings.TrimSpace(src.Title),
				Type:        formatSourceType(src),
				Status:      formatSourceStatus(src),
				LastUpdated: sourceTimeRFC3339(src),
				Labels:      labelsBySource[id],
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
	}

	w, flush := newListWriter(os.Stdout)
	if hasAnyLabels {
		fmt.Fprintln(w, "ID\tTITLE\tTYPE\tSTATUS\tLAST UPDATED\tLABELS")
	} else {
		fmt.Fprintln(w, "ID\tTITLE\tTYPE\tSTATUS\tLAST UPDATED")
	}
	for _, src := range p.Sources {
		id := src.SourceId.GetSourceId()
		status := formatSourceStatus(src)
		lastUpdated := formatSourceTime(src)
		sourceType := formatSourceType(src)

		if hasAnyLabels {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				id,
				strings.TrimSpace(src.Title),
				sourceType,
				status,
				lastUpdated,
				strings.Join(labelsBySource[id], ", "),
			)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				id,
				strings.TrimSpace(src.Title),
				sourceType,
				status,
				lastUpdated,
			)
		}
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
	// Order matters: a source can carry Metadata.Status=1 (parsed metadata)
	// alongside Settings.Status=3 (post-parse error) — the UI shows the red
	// error chip in that case, so error/warnings must win over "enabled".
	if src.Settings != nil && src.Settings.Status == 3 {
		return "error"
	}
	if src.Metadata != nil && src.Metadata.Status == 3 {
		return "error"
	}
	if len(src.Warnings) > 0 {
		var codes []string
		for _, w := range src.Warnings {
			codes = append(codes, fmt.Sprintf("warn:%d", w))
		}
		return strings.Join(codes, ",")
	}
	if src.Metadata != nil && src.Metadata.Status == 1 {
		return "enabled"
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
	if t := sourceTimeRFC3339(src); t != "" {
		return t
	}
	return "-"
}

// sourceTimeRFC3339 returns the source's last-modified timestamp as RFC3339,
// or the empty string if no timestamp is set. The table renderer wraps this
// with "-" for human readability; JSON callers leave it as "" so the field
// can be omitted.
func sourceTimeRFC3339(src *pb.Source) string {
	if src.Metadata != nil && src.Metadata.LastModifiedTime != nil {
		return src.Metadata.LastModifiedTime.AsTime().Format(time.RFC3339)
	}
	if src.Metadata != nil && src.Metadata.LastUpdateTimeSeconds != nil {
		return time.Unix(int64(src.Metadata.LastUpdateTimeSeconds.GetValue()), 0).Format(time.RFC3339)
	}
	return ""
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
			return addLocalFileSource(c, notebookID, input, piped, opts)
		}
		file, err := os.Open(input)
		if err != nil {
			return "", fmt.Errorf("open file: %w", err)
		}
		defer file.Close()
		return addLocalFileSource(c, notebookID, input, file, opts)
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

// addLocalFileSource uploads a local file using its basename. The resumable
// upload service uses the filename extension to select a parser, so a display
// name such as "paper" must not replace an input name such as "paper.pdf".
// Rename the source after the upload instead.
type localFileSourceClient interface {
	AddSourceFromReader(projectID string, r io.Reader, filename string, contentType ...string) (string, error)
	MutateSource(sourceID string, updates *pb.Source) (*pb.Source, error)
}

func addLocalFileSource(c localFileSourceClient, notebookID, path string, r io.Reader, opts sourceAddOptions) (string, error) {
	filename := filepath.Base(path)
	var (
		id  string
		err error
	)
	if opts.MIMEType != "" {
		fmt.Fprintf(os.Stderr, "Using specified MIME type: %s\n", opts.MIMEType)
		id, err = c.AddSourceFromReader(notebookID, r, filename, opts.MIMEType)
	} else {
		id, err = c.AddSourceFromReader(notebookID, r, filename)
	}
	if err != nil {
		return "", err
	}
	if opts.Name == "" || opts.Name == filename {
		return id, nil
	}
	if _, err := c.MutateSource(id, &pb.Source{Title: opts.Name}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: uploaded source %s but could not rename it to %q: %v\n", id, opts.Name, err)
	}
	return id, nil
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

func (a *syncClientAdapter) LabelsForSource(ctx context.Context, notebookID, sourceID string) ([]string, error) {
	return labelsForSource(a.client, notebookID, sourceID)
}

func (a *syncClientAdapter) AttachLabelSource(ctx context.Context, notebookID, labelID, sourceID string) error {
	return a.client.AttachLabelSource(notebookID, labelID, sourceID)
}

type sourceDeleteClient interface {
	DeleteSources(projectID string, sourceIDs []string) error
}

func removeSource(c sourceDeleteClient, notebookID, sourceArg string) error {
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

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, rec := range noteListRecords(notes) {
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "ID\tTITLE\tCONTENT PREVIEW")
	for _, rec := range noteListRecords(notes) {
		fmt.Fprintf(w, "%s\t%s\t%s\n", rec.NoteID, rec.Title, rec.ContentPreview)
	}
	return flush()
}

func noteListRecords(notes []*pb.Note) []noteListRecord {
	records := make([]noteListRecord, 0, len(notes))
	for _, note := range notes {
		if note == nil {
			continue
		}
		content := note.GetRichText()
		if content == "" {
			content = note.GetContentText()
		}
		content = strings.Join(strings.Fields(content), " ")
		if len(content) > 80 {
			content = content[:77] + "..."
		}
		records = append(records, noteListRecord{
			NoteID: note.GetNoteId(), Title: note.GetTitle(), ContentPreview: content,
		})
	}
	return records
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
	fmt.Fprintf(os.Stderr, "Generating audio share link...\n")
	res, err := c.ShareAudio(notebookID, api.SharePublic)
	if err != nil {
		return err
	}
	if res.ShareURL != "" {
		fmt.Println(res.ShareURL)
	}
	if res.ShareID != "" {
		fmt.Fprintf(os.Stderr, "Share ID: %s\n", res.ShareID)
	}
	return nil
}

// Generation operations
func generateNotebookGuide(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating notebook guide...\n")
	guide, err := c.GenerateNotebookGuide(notebookID)
	if err != nil {
		return fmt.Errorf("generate guide: %w", err)
	}
	fmt.Printf("%s\n", guide.GetGuide().GetSummary().GetText())
	return nil
}

func runMagicView(c *api.Client, notebookID string, sourceIDs []string) error {
	fmt.Fprintf(os.Stderr, "Generating magic view...\n")
	resp, err := c.GenerateMagicView(notebookID, sourceIDs)
	if err != nil {
		return fmt.Errorf("generate magic view: %w", err)
	}
	fmt.Printf("Magic View status: %d\n", resp.GetStatus())
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
	return generateSourceGuidesWithOptions(c, sourceIDs, packageGlobalOptions())
}

func generateSourceGuidesWithOptions(c *api.Client, sourceIDs []string, globals globalOptions) error {
	enc := json.NewEncoder(os.Stdout)
	for i, sourceID := range sourceIDs {
		var guide *api.SourceGuide
		if !globals.force {
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
		if globals.jsonOutput {
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
	return createAudioOverviewWithOptions(c, projectID, instructions, audioCreateOptions{Length: "default", Language: "en"})
}

func createAudioOverviewWithOptions(c *api.Client, projectID string, instructions string, opts audioCreateOptions) error {
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
			fmt.Fprintf(os.Stderr, "Notebook already has an audio overview. Use -y to replace it, or 'nlm audio delete %s' first.\n", projectID)
			return fmt.Errorf("existing audio overview")
		}
	}

	fmt.Fprintf(os.Stderr, "Creating audio overview for notebook %s...\n", projectID)
	fmt.Fprintf(os.Stderr, "Instructions: %s\n", instructions)

	length, err := parseAudioLength(opts.Length)
	if err != nil {
		return err
	}
	audioType, err := parseAudioType(opts.AudioType, pb.AudioType_AUDIO_TYPE_DEEP_DIVE)
	if err != nil {
		return err
	}
	result, err := c.CreateAudioOverviewWithOptions(projectID, api.CreateAudioOverviewOptions{
		Instructions: instructions,
		AudioType:    audioType,
		Length:       length,
		Language:     opts.Language,
	})
	if err != nil {
		return fmt.Errorf("create audio overview: %w", err)
	}

	if !result.IsReady {
		fmt.Fprintln(os.Stderr, "Audio overview creation started. Use 'nlm audio get' to check status.")
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

func getAnalytics(c *api.Client, projectID string) error {
	resp, err := c.GetProjectAnalytics(projectID)
	if err != nil {
		return fmt.Errorf("get analytics: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, series := range resp.Series {
			for _, point := range series.Points {
				rec := analyticsRecord{
					ProjectID: projectID,
					MetricID:  series.MetricID,
					Time:      point.Time.Format(time.RFC3339),
					Value:     point.Value,
				}
				if err := enc.Encode(rec); err != nil {
					return err
				}
			}
		}
		return nil
	}

	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "METRIC\tDATE\tVALUE")
	for _, series := range resp.Series {
		for _, point := range series.Points {
			fmt.Fprintf(w, "%d\t%s\t%d\n",
				series.MetricID,
				point.Time.Format("2006-01-02"),
				point.Value)
		}
	}
	return flush()
}

func listFeaturedProjects(c *api.Client) error {
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)
	resp, err := orchClient.ListFeaturedProjects(context.Background(), &pb.ListFeaturedProjectsRequest{})
	if err != nil {
		return fmt.Errorf("list featured projects: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, project := range resp.Projects {
			rec := featuredProjectRecord{
				ProjectID:   project.ProjectId,
				Title:       collapseWhitespace(project.GetTitle()),
				Emoji:       strings.TrimSpace(project.GetEmoji()),
				Description: collapseWhitespace(project.GetPresentation().GetDescription()),
				SourceCount: len(project.GetSources()),
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
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
	req := &pb.CheckSourceFreshnessRequest{Source: &pb.SourceIdList{SourceId: sourceID}, Context: &pb.RequestContext{
		Version: proto.Int32(2),
		Surface: &pb.RequestSurface{Value: proto.Int32(1)},
		Caps:    &pb.RequestClientCaps{Version: proto.Int32(1), CapabilityCodes: []int32{1, 3}},
	}}
	fmt.Fprintf(os.Stderr, "Checking source %s...\n", sourceID)
	resp, err := orchClient.CheckSourceFreshness(context.Background(), req)
	if err != nil {
		return fmt.Errorf("check source: %w", err)
	}
	if resp.GetIsFresh() {
		fmt.Printf("Source is up to date")
	} else {
		fmt.Printf("Source needs refresh")
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
	resp, err := c.DiscoverSources(projectID, query)
	if err != nil {
		// Earlier rounds routed this through deep-research as a
		// workaround when Es3dTe was thought to be deprecated. The
		// JS bundle still binds Es3dTe and the proto carries an
		// arg_format, so we now hit it directly. If the server
		// rejects the call, fall back to a chat suggestion so
		// users still get something usable.
		fmt.Fprintf(os.Stderr, "DiscoverSources (Es3dTe) returned %v; falling back to chat suggestions.\n", err)
		res, fbErr := streamChatResponse(c, api.ChatRequest{
			ProjectID: projectID,
			Prompt:    fmt.Sprintf("Suggest sources to add for this query: %s. Respond with a short bullet list of specific documents, sites, or search directions.", query),
		}, currentChatRenderOptions())
		if fbErr != nil {
			return fmt.Errorf("discover sources fallback: %w", fbErr)
		}
		if res.Answer == "" {
			fmt.Println("(No source suggestions returned)")
		}
		fmt.Println()
		return nil
	}
	sources := resp.GetSources()
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "(No source candidates returned)")
		return nil
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, s := range sources {
			if err := enc.Encode(s); err != nil {
				return err
			}
		}
		return nil
	}
	w, flush := newListWriter(os.Stdout)
	fmt.Fprintln(w, "ID\tTITLE")
	for _, s := range sources {
		fmt.Fprintf(w, "%s\t%s\n",
			s.GetSourceId().GetSourceId(),
			s.GetTitle(),
		)
	}
	return flush()
}

// Artifact management
func getArtifact(c *api.Client, artifactID string) error {
	artifact, err := c.GetArtifact(artifactID)
	if err != nil {
		return fmt.Errorf("get artifact: %w", err)
	}

	fmt.Printf("Artifact: %s\n", artifact.ArtifactId)
	fmt.Printf("Title:    %s\n", artifact.Title)
	fmt.Printf("Type:     %s\n", artifact.Type.String())
	// Print the raw state code alongside the enum name. The state position in
	// the gArtLc wire response is observed but not HAR-pinned, so exposing the
	// integer lets callers distinguish a genuine FAILED (3) from an unparsed
	// state (0/UNSPECIFIED) without --debug.
	fmt.Printf("State:    %s (%d)\n", artifact.State.String(), artifact.State.Number())
	if artifact.State == pb.ArtifactState_ARTIFACT_STATE_FAILED {
		fmt.Fprintln(os.Stderr, "note: artifact reports FAILED. Run with --debug to see the raw artifact record.")
	}

	// Surface rendered-output download links (e.g. a slide deck's .pdf/.pptx).
	// These come from the direct-RPC payload; absence is not an error (the
	// artifact may still be generating, or the account lacks the direct RPC).
	if urls, err := c.GetArtifactDownloadURLs(artifactID); err == nil && len(urls) > 0 {
		fmt.Println("Downloads:")
		for _, u := range urls {
			fmt.Printf("  %s\n", u)
		}
	}

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

	// Type-specific content. These read the gArtLc-local preview/config
	// shapes (ArtifactNotePreview, ArtifactAudioOverview, etc.), which are
	// distinct message types from the top-level Note/AudioOverview/Report
	// used by the Create*/Get* RPCs — see the Artifact message comment in
	// orchestration.proto.
	if report := artifact.TailoredReport; report != nil {
		if opts := report.Options; opts != nil {
			if opts.Instructions != "" {
				fmt.Printf("\nReport instructions: %s\n", opts.Instructions)
			}
			if opts.Language != "" {
				fmt.Printf("  Language: %s\n", opts.Language)
			}
		}
	}

	if note := artifact.Note; note != nil {
		fmt.Printf("\nNote: (source refs: %d)\n", len(note.GetConfig().GetSourceRefs()))
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
		if audio.Content != nil && audio.Content.Prompt != "" {
			fmt.Printf("  Instructions: %s\n", audio.Content.Prompt)
		}
	}

	if video := artifact.VideoPreview; video != nil {
		data, err := json.MarshalIndent(video, "", "  ")
		if err == nil {
			fmt.Printf("\nVideo:\n%s\n", string(data))
		}
	}

	return nil
}

func readArtifact(c *api.Client, artifactID string, opts globalOptions) error {
	directErr := c.ReadArtifactFile(artifactID, "md", os.Stdout)
	if directErr == nil {
		return nil
	}
	if opts.cdpURL == "" {
		return fmt.Errorf("read artifact: direct download failed: %w", directErr)
	}
	url, err := c.ArtifactDownloadURLForFormat(artifactID, "md")
	if err != nil {
		return fmt.Errorf("read artifact: get download URL: %w", err)
	}
	data, err := auth.New(false).ReadTextWithRemoteBrowser(url, opts.cdpURL)
	if err != nil {
		return fmt.Errorf("read artifact: remote browser download: %w", err)
	}
	_, err = os.Stdout.Write(data)
	return err
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
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, artifact := range artifacts {
			rec := artifactListRecord{
				ArtifactID:  artifact.ArtifactId,
				Type:        artifact.Type.String(),
				State:       artifact.State.String(),
				StateCode:   int32(artifact.State.Number()),
				SourceCount: len(artifact.Sources),
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
	}

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

// ANSI escape codes. Grey/dim style secondary output (thinking traces,
// follow-ups); bold sets off headings in default-color blocks; amber flags a
// weakly-grounded citation (confidence below weakConfidence).
const (
	ansiDim   = "\033[2m"  // dim
	ansiGrey  = "\033[90m" // bright black (grey)
	ansiBold  = "\033[1m"  // bold
	ansiAmber = "\033[33m" // yellow/amber
	ansiReset = "\033[0m"
)

// weakConfidence is the grounding-score threshold below which a citation reads
// as weakly supported and its confidence renders amber. It is a reading signal,
// not a filter: weak citations still print.
const weakConfidence = 0.75

// citationRenderMode controls how Citation data is surfaced in the CLI.
type citationRenderMode int

const (
	citationModeOff  citationRenderMode = iota // Suppress the trailing citation list entirely.
	citationModeList                           // Stream the answer live, then print the enriched citation list. The default.
	citationModeJSON                           // Emit answer deltas and citations as JSON-lines events on stdout.
)

// resolveCitationMode maps the user-facing --citations flag to a mode.
// The default (empty, "auto", "tail", or unknown) is the enriched list: the
// answer streams cleanly, then a trailing list names each source with its
// confidence, char span, and — under --citation-excerpts — the cited text.
//
// The historical "block", "stream", and "tail" modes were separate list
// variants that differed only by which metadata columns they showed; they are
// now one list with per-column toggles. They remain accepted as aliases for
// one release (see maybeWarnDeprecatedCitationMode). "overlay"/"footnote" —
// which spliced inline superscripts into the answer body — are removed: the
// splice used byte offsets against rune-based server spans and corrupted any
// answer containing multibyte characters.
func resolveCitationMode(flag string) citationRenderMode {
	switch strings.ToLower(flag) {
	case "off", "none":
		return citationModeOff
	case "json", "jsonl":
		return citationModeJSON
	default:
		// "", "auto", "tail", the deprecated "block"/"stream", and anything
		// unrecognized all render the one enriched list.
		return citationModeList
	}
}

// citationModeIsDeprecatedAlias reports whether the user-facing --citations
// value names a removed/renamed mode that now resolves to the enriched list.
// Used to print a one-time stderr deprecation notice without polluting stdout.
func citationModeIsDeprecatedAlias(flag string) string {
	switch strings.ToLower(flag) {
	case "block":
		return "block"
	case "stream", "inline-footer":
		return "stream"
	case "overlay", "footnote":
		return "overlay"
	}
	return ""
}

// warnDeprecatedCitationMode prints a one-line deprecation notice to w (stderr,
// never stdout — the citation output must stay parseable) when the user passed
// a --citations mode that has been folded into the default list. The notice is
// informational; the command proceeds normally.
func warnDeprecatedCitationMode(w io.Writer, flag string) {
	if alias := citationModeIsDeprecatedAlias(flag); alias != "" {
		fmt.Fprintf(w, "nlm: --citations=%s is deprecated and now renders the default citation list; this alias will be removed in a future release\n", alias)
	}
}

type chatStreamRenderer struct {
	out                  io.Writer
	status               io.Writer
	showThinking         bool
	verbose              bool
	jsonl                bool // when true, emit typed JSON-lines events on r.out instead of human output
	jsonlIncludeThinking bool // when true, thinking chunks are emitted as JSON-lines events (otherwise skipped)
	citationMode         citationRenderMode
	resolveTitle         func(sourceID string) string                      // optional; returns "" if unknown
	sourceRemoved        func(sourceID string) bool                        // optional; reports a source ID no longer in the notebook
	loadSource           func(sourceID string) (api.LoadSourceText, error) // optional; populated when --resolve-citations or --citation-excerpt is set
	excerptBudget        int                                               // >0 enables per-citation excerpts, clipped to this many runes
	showConfidence       bool                                              // list mode: show the (p=…) column (default on)
	showSpans            bool                                              // list mode: show the "chars X-Y" column (default on)
	resolvedLocations    map[citationKey]resolvedCitation                  // computed once at Finish; keyed by citationKey
	lastThinkingLen      int
	answerBuf            strings.Builder
	thinking             string
	citations            []api.Citation
	followUps            []string
	rich                 *api.RichContent // last answer chunk's span tree (cumulative); nil when the stream carried none

	// flushedLen tracks bytes already streamed to r.out so re-renders don't
	// double-print; the answer always streams live now, so it just advances
	// with each answer chunk.
	flushedLen int

	// jsonl bookkeeping: last emitted absolute answer offset so we only
	// emit delta text per event, and track which citations have been emitted.
	jsonlAnswerEmitted int
	jsonlThinkingSeen  string
	jsonlCitationsSeen int
	jsonlSourceBodies  map[string]api.LoadSourceText // per-source cache for lazy JSONL resolution ("" body = negative)
}

func newChatStreamRenderer(out, status io.Writer, showThinking, verbose bool, mode citationRenderMode) *chatStreamRenderer {
	return &chatStreamRenderer{
		out:            out,
		status:         status,
		showThinking:   showThinking,
		verbose:        verbose,
		citationMode:   mode,
		showConfidence: true,
		showSpans:      true,
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
		// The answer always streams live to stdout; citations render as a
		// trailing list at Finish. (Earlier modes buffered or held a tail
		// window to splice inline superscripts; that path corrupted answers
		// with multibyte text and was removed.)
		fmt.Fprint(r.out, chunk.Text)
		r.flushedLen += len(chunk.Text)
		if len(chunk.Citations) > 0 {
			r.citations = chunk.Citations
		}
		if len(chunk.FollowUps) > 0 {
			r.followUps = chunk.FollowUps
		}
		if chunk.Rich != nil {
			r.rich = chunk.Rich // cumulative tree; keep the latest
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
				title := c.Title
				if title == "" && r.resolveTitle != nil {
					title = citationTitle(c, r.resolveTitle)
				}
				event := map[string]any{
					"phase":      "citation",
					"index":      c.SourceIndex,
					"source_id":  c.SourceID,
					"title":      title,
					"start_char": c.StartChar,
					"end_char":   c.EndChar,
					"confidence": c.Confidence,
				}
				// The notebook source that owns the cited passage (source_id is
				// the granular chunk handle). Present only when the frame embedded
				// it; lets consumers resolve the source without knowing the chunk
				// layout.
				if c.ParentSourceID != "" {
					event["parent_source_id"] = c.ParentSourceID
				}
				// The excerpt's offset range within the source document (as
				// opposed to start_char/end_char, which are answer offsets).
				// Present only when the server shipped it — lets offline tools
				// locate the citation in the source without a second fetch.
				if c.SourceStart < c.SourceEnd {
					event["source_start"] = c.SourceStart
					event["source_end"] = c.SourceEnd
				}
				// The server ships the cited source text inline; fold it into
				// the event (clipped to budget) when --citation-excerpts is set
				// so scripts get groundedness inline.
				if r.excerptBudget > 0 {
					if ex := truncateExcerpt(c.Excerpt, r.excerptBudget); ex != "" {
						event["excerpt"] = ex
					}
				}
				// When --resolve-citations is set, fold the resolved txtar
				// location in too so scripts can post-process without a fetch.
				if rc, ok := r.resolveCitationJSONL(c); ok {
					if rc.Location != "" {
						event["location"] = rc.Location
					}
				}
				r.emitJSONLEvent(event)
			}
			r.jsonlCitationsSeen = len(chunk.Citations)
		}
		if len(chunk.FollowUps) > 0 {
			r.followUps = chunk.FollowUps
		}
		if chunk.Rich != nil {
			r.rich = chunk.Rich // cumulative tree; keep the latest
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
	if r.loadSource != nil && len(r.citations) > 0 {
		r.resolvedLocations = resolveCitationLocations(r.loadSource, r.citations)
	}
	if render := citationRenderers[r.citationMode]; render != nil {
		render(r)
	}
	r.printFollowUps()
}

// citationRenderers maps a render mode to the function that emits its trailing
// citation output at Finish. It is a lookup rather than a fixed switch so new
// modes (a compact one-line-per-source view, a grouped-by-source view) can be
// registered without reworking Finish. citationModeOff has no entry (nothing
// to render); citationModeJSON is handled earlier in Finish.
var citationRenderers = map[citationRenderMode]func(*chatStreamRenderer){
	citationModeList: (*chatStreamRenderer).renderCitationList,
}

// renderCitationList prints the enriched, post-answer citation list under a
// "Citations:" heading, grouped by citation index (the [n] marker):
//
//	Citations:
//	  [1] answer 115-241
//	      p=0.91 46870d5a "7A59"
//	      p=0.71 5cfc41f8 "claude: E347"      ← amber, weakly grounded
//
// The [n] marker is the primary key and heads its group carrying only the
// answer span — the one property that is genuinely per-marker. Everything else
// (source, confidence, excerpt, source-span) is per-source, so each source gets
// its own row led by its own confidence. A marker usually cites several sources
// with differing scores; that structure is the point, not redundancy.
// showConfidence / showSpans toggle the per-row p= column and the answer-span
// header. The expanded view (--citation-excerpts) additionally prints each
// source's file:line / src-offset locator and its verbatim excerpt.
func (r *chatStreamRenderer) renderCitationList() {
	if len(r.citations) == 0 {
		return
	}
	order, groups := groupCitationsByIndex(r.citations)
	fmt.Fprintf(r.status, "\n%sCitations:%s\n", ansiBold, ansiReset)
	// --citation-excerpts switches from the compact scan view to the expanded
	// audit view: each source additionally gets its resolved locator and its
	// own excerpt (both per-source, like confidence).
	expanded := r.excerptBudget > 0
	for _, idx := range order {
		r.renderCitationGroup(idx, groups[idx], expanded)
	}
}

// renderCitationGroup prints one marker: a header line with the answer span,
// then one row per source. Each source row leads with that source's own
// confidence (amber below weakConfidence) — never hoisted to the header,
// because a marker's sources have independent, usually differing scores. In the
// expanded view each source's locator and excerpt follow on indented lines.
func (r *chatStreamRenderer) renderCitationGroup(idx int, group []api.Citation, expanded bool) {
	fmt.Fprintf(r.status, "  %s\n", r.citationMarkerHeader(idx, group))
	for _, c := range group {
		if row := r.citationSourceRow(c, expanded); row != "" {
			fmt.Fprintf(r.status, "      %s\n", row)
		}
		if expanded {
			r.printCitationExcerpt(c, "          ")
		}
	}
}

// citationMarkerHeader formats the "[n] answer N-M" line that heads a marker's
// group. The answer span is the only per-marker property (it locates the [n]
// claim in the answer text); it is labeled "answer" so it is never confused
// with a source offset. All sources under a marker share it. When showSpans is
// off, or the sources somehow disagree on the span, only "[n]" prints.
func (r *chatStreamRenderer) citationMarkerHeader(idx int, group []api.Citation) string {
	header := fmt.Sprintf("[%d]", idx)
	if r.showSpans {
		if start, end, ok := uniformSpan(group); ok {
			if span := formatAnswerSpan(start, end); span != "" {
				header += " " + span
			}
		}
	}
	return header
}

// citationSourceRow formats one source's row: "p=0.87 id8 "title"", with the
// per-source confidence first (amber when weak) and the source handle+title
// after. In the expanded view the row also carries the source-offset locator
// ("src N-M", or a resolved file:line) so the excerpt beneath it can be placed
// in the source document. Returns "" when there is nothing to show.
func (r *chatStreamRenderer) citationSourceRow(c api.Citation, expanded bool) string {
	label := r.citationLabel(c)
	var row string
	if r.showConfidence {
		if conf := r.formatSourceConfidence(c.Confidence); conf != "" {
			row = conf
		}
	}
	if label != "" {
		if row != "" {
			row += " "
		}
		row += label
	}
	if expanded {
		if loc := r.citationSourceLocator(c); loc != "" {
			if row != "" {
				row += "   "
			}
			row += loc
		}
	}
	return row
}

// formatSourceConfidence renders one source's grounding score as "p=0.87",
// colored amber when it is below weakConfidence so a weakly-grounded source
// reads at a glance. Returns "" for a zero/absent score.
func (r *chatStreamRenderer) formatSourceConfidence(conf float64) string {
	if conf <= 0 {
		return ""
	}
	s := fmt.Sprintf("p=%.2f", conf)
	if conf < weakConfidence {
		s = ansiAmber + s + ansiReset
	}
	return s
}

// citationSourceLocator returns the source-document anchor for citation c: a
// resolved "file:line:col" when --resolve-citations pinned a txtar member,
// otherwise the raw source-offset range as "src N-M" (from SourceStart/End).
// Both are source-document coordinates, distinct from the answer span on the
// marker header; returns "" when neither is available.
func (r *chatStreamRenderer) citationSourceLocator(c api.Citation) string {
	if loc := r.resolvedLocationFor(c); loc != "" {
		return "→ " + loc
	}
	if !r.showSpans {
		return ""
	}
	return formatSourceSpan(c.SourceStart, c.SourceEnd)
}

// groupCitationsByIndex buckets citations by SourceIndex, preserving the order
// indices first appear in the stream.
func groupCitationsByIndex(cites []api.Citation) ([]int, map[int][]api.Citation) {
	var order []int
	groups := map[int][]api.Citation{}
	for _, c := range cites {
		if _, ok := groups[c.SourceIndex]; !ok {
			order = append(order, c.SourceIndex)
		}
		groups[c.SourceIndex] = append(groups[c.SourceIndex], c)
	}
	return order, groups
}

// uniformSpan returns the group's shared answer-span range and ok=true when
// every member agrees, else ok=false. All sources under one marker cite the
// same answer claim, so they share this span; the guard is defensive against a
// payload that ever splits it (in which case only "[n]" heads the group).
func uniformSpan(group []api.Citation) (int, int, bool) {
	for _, c := range group[1:] {
		if c.StartChar != group[0].StartChar || c.EndChar != group[0].EndChar {
			return 0, 0, false
		}
	}
	return group[0].StartChar, group[0].EndChar, true
}

// formatAnswerSpan renders a marker's answer-text range as "answer 42-205", or
// "answer 409" for a single non-zero point. The "answer" label is load-bearing:
// this offset indexes the answer, never the source (see formatSourceSpan).
// Returns "" for no real span (negative, inverted, or the (0,0) "no metadata"
// sentinel — never "answer 0").
func formatAnswerSpan(start, end int) string {
	return formatLabeledSpan("answer", start, end)
}

// formatSourceSpan renders a citation's source-document range as
// "src 965670-966914" — where the excerpt lives inside the source, from
// SourceStart/SourceEnd. Distinct from the answer span; same empty-span rules.
func formatSourceSpan(start, end int) string {
	return formatLabeledSpan("src", start, end)
}

// formatLabeledSpan renders "<label> N-M" (or "<label> N" for a point), the
// shared shape behind the answer/source span formatters. Returns "" when there
// is no real span: a negative range, an inverted range, or the zero value
// (0,0), which is the "no span metadata" sentinel and must not render as "N 0".
func formatLabeledSpan(label string, start, end int) string {
	if start < 0 || end < start || (start == 0 && end == 0) {
		return ""
	}
	if end == start {
		return fmt.Sprintf("%s %d", label, start)
	}
	return fmt.Sprintf("%s %d-%d", label, start, end)
}

// printCitationExcerpt renders the verbatim cited excerpt beneath a source row
// (when --citation-excerpts is set), indented by indent and in the default
// color so the cited text stays legible. The source-document locator now rides
// on the source row itself; only the excerpt lands here. Nothing prints when
// the server sent no excerpt.
func (r *chatStreamRenderer) printCitationExcerpt(c api.Citation, indent string) {
	if ex := r.excerptFor(c); ex != "" {
		fmt.Fprintf(r.status, "%s“%s”\n", indent, ex)
	}
}

// citationLabel formats the source handle and title for one citation as
// `<id8> - "title"`, where id8 is the source ID's 8-char prefix (enough to
// disambiguate near-duplicate titles like "claude: E347 (pt2)" while staying
// column-aligned; the full UUID is available in --citations=json). The title
// prefers a resolved notebook title, then the server-supplied one. Degrades to
// just the handle or just the quoted title when only one is available. The
// resolved file:line and excerpt render separately on continuation lines.
func (r *chatStreamRenderer) citationLabel(c api.Citation) string {
	handle := shortSourceID(c.SourceID)
	var title string
	if r.resolveTitle != nil {
		title = citationTitle(c, r.resolveTitle)
	}
	if title == "" {
		title = c.Title
	}
	title = truncateExcerpt(title, 100)
	// When a source has no resolvable title AND its ID is absent from the
	// notebook's source list, say the title is unavailable rather than a bare
	// handle that reads as a rendering gap. We say "unavailable", not "removed":
	// a citation's SourceID is a granular chunk/passage handle, not a top-level
	// source UUID, so it legitimately misses the source list even when the source
	// is present — claiming removal would over-state what a miss actually tells us.
	if title == "" && r.sourceRemoved != nil && r.sourceRemoved(citationSourceID(c)) {
		if handle != "" {
			return handle + " (title unavailable)"
		}
		return "(title unavailable)"
	}
	switch {
	case handle != "" && title != "":
		return fmt.Sprintf("%s %q", handle, title)
	case handle != "":
		return handle
	case title != "":
		return fmt.Sprintf("%q", title)
	}
	return ""
}

// shortSourceID returns the 8-char prefix of a source ID (a UUID), or the
// whole ID when it is already 8 chars or shorter. Empty in, empty out.
func shortSourceID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// resolvedLocationFor returns the editor-style "file:line:col" coordinate for
// citation c when --resolve-citations resolved it against a txtar member, or
// "" otherwise. Footer printers render this on a continuation line beneath the
// citation label so the original source name stays visible.
func (r *chatStreamRenderer) resolvedLocationFor(c api.Citation) string {
	if r.resolvedLocations == nil {
		return ""
	}
	return r.resolvedLocations[keyFor(c)].Location
}

// excerptFor returns the cited source text for citation c, clipped to the
// excerpt budget, or "" when excerpts are disabled or the server sent none.
// The text is the verbatim passage the server shipped inline with the citation
// (api.Citation.Excerpt) — available for any source type (notes included) with
// no source fetch or txtar resolution required.
func (r *chatStreamRenderer) excerptFor(c api.Citation) string {
	if r.excerptBudget <= 0 {
		return ""
	}
	return truncateExcerpt(c.Excerpt, r.excerptBudget)
}

// resolveCitationJSONL resolves a single citation's txtar file:line location on
// demand during JSONL streaming, caching each source body so repeated citations
// into the same source cost one fetch. Returns ok=false when no source loader
// is configured (--resolve-citations was not requested), the source could not
// be loaded, or the source is not a pinnable txtar member. The excerpt is no
// longer resolved here — it ships inline on the citation. Resolution is shared
// with the batch path via resolveOneCitation so the two never diverge.
func (r *chatStreamRenderer) resolveCitationJSONL(c api.Citation) (resolvedCitation, bool) {
	if r.loadSource == nil || c.SourceID == "" {
		return resolvedCitation{}, false
	}
	if r.jsonlSourceBodies == nil {
		r.jsonlSourceBodies = make(map[string]api.LoadSourceText)
	}
	body, ok := r.jsonlSourceBodies[c.SourceID]
	if !ok {
		loaded, err := r.loadSource(c.SourceID)
		if err != nil {
			r.jsonlSourceBodies[c.SourceID] = api.LoadSourceText{} // negative cache
			return resolvedCitation{}, false
		}
		body = loaded
		r.jsonlSourceBodies[c.SourceID] = body
	}
	return resolveOneCitation(body, c)
}

// truncateExcerpt collapses whitespace and clips to max runes with an ellipsis.
// Use it for single-line surfaces (the TUI citation rows, where a row must not
// wrap or break alignment). Surfaces that can display structure — HTML's
// pre-wrap excerpt box, a Markdown blockquote — must use clipExcerpt instead so
// cited code, config, and tables keep their line and indent structure.
func truncateExcerpt(s string, max int) string {
	return clipRunes(collapseWhitespace(s), max)
}

// clipExcerpt clips to max runes with an ellipsis but preserves internal
// whitespace (newlines, tabs, indentation). Cited passages are frequently code
// or config whose meaning lives in their layout; the multi-line surfaces render
// that structure, so flattening it here would corrupt the evidence a citation
// exists to show. Leading/trailing whitespace is trimmed so the excerpt does not
// open on a blank line.
func clipExcerpt(s string, max int) string {
	return clipRunes(strings.TrimSpace(s), max)
}

// clipRunes truncates s to max runes, appending an ellipsis when it clips.
func clipRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
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

// Rich returns the answer-body span tree from the last answer chunk (the
// cumulative tree over the whole answer), or nil when the stream carried none.
func (r *chatStreamRenderer) Rich() *api.RichContent {
	return r.rich
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
	Rich      *api.RichContent // answer-body span tree; nil when the stream carried none
}

func streamChatResponse(c *api.Client, req api.ChatRequest, opts chatRenderOptions) (chatResult, error) {
	mode := resolveCitationMode(opts.CitationMode)
	warnDeprecatedCitationMode(os.Stderr, opts.CitationMode)
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
	responseReceived, stopWaiting := startInitialChatResponseWaiter(os.Stderr, 30*time.Second)
	defer stopWaiting()
	renderer.excerptBudget = opts.ExcerptBudget
	renderer.showConfidence = !opts.HideConfidence
	renderer.showSpans = !opts.HideSpans
	// Only --resolve-citations needs source bodies now (for txtar file:line).
	// Excerpts ship inline on the citation, so they need no loader.
	if opts.ResolveCitations && c != nil && req.ProjectID != "" {
		renderer.loadSource = func(sourceID string) (api.LoadSourceText, error) {
			return c.LoadSourceText(sourceID, req.ProjectID)
		}
	}

	err := c.StreamChat(req, func(chunk api.ChatChunk) bool {
		responseReceived()
		renderer.WriteChunk(chunk)
		return true
	})

	renderer.Finish()

	return chatResult{
		Answer:    renderer.Answer(),
		Thinking:  renderer.Thinking(),
		Citations: persistableCitations(renderer.citations, renderer.resolveTitle),
		FollowUps: renderer.followUps,
		Rich:      renderer.Rich(),
	}, err
}

// startInitialChatResponseWaiter reports that the response stream remains open
// while the server has not yet produced a parsed chat chunk. The notice is a
// client-side liveness indicator, not evidence that NotebookLM has started
// generating an answer.
func startInitialChatResponseWaiter(status io.Writer, interval time.Duration) (received func(), stop func()) {
	first := make(chan struct{})
	done := make(chan struct{})
	var firstOnce sync.Once
	var stopOnce sync.Once
	started := time.Now()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(done)
		for {
			select {
			case <-ticker.C:
				fmt.Fprintf(status, "nlm: waiting for initial NotebookLM response (%s elapsed; stream is open)\n", time.Since(started).Round(time.Second))
			case <-first:
				return
			}
		}
	}()

	received = func() { firstOnce.Do(func() { close(first) }) }
	stop = func() {
		stopOnce.Do(func() {
			firstOnce.Do(func() { close(first) })
			<-done
		})
	}
	return received, stop
}

// persistableCitations returns a copy of cites with each Title filled from the
// notebook source title (via resolveTitle) when the server did not supply one.
// The persisted session then carries a human-readable title, so `chat show`
// can render titles offline without a live GetProject fetch. Citations that
// already have a server title, or whose source has no resolvable title, are
// left as-is.
//
// A citation's SourceID is a granular chunk handle that is absent from the
// project source list, so it never resolves; the title lives under the parent
// source at ParentSourceID. Resolve the parent first and fall back to SourceID
// only for older frames that carried no parent.
func persistableCitations(cites []api.Citation, resolveTitle func(string) string) []api.Citation {
	if resolveTitle == nil || len(cites) == 0 {
		return cites
	}
	out := make([]api.Citation, len(cites))
	copy(out, cites)
	for i := range out {
		if out[i].Title == "" {
			if t := citationTitle(out[i], resolveTitle); t != "" {
				out[i].Title = t
			}
		}
	}
	return out
}

// citationTitle resolves a citation's notebook-source title by preferring its
// ParentSourceID (the source that owns the cited passage and is in the source
// list), then falling back to the chunk-level SourceID for frames that embedded
// no parent. Callers that resolve titles or presence off a citation route
// through this so the parent-vs-chunk distinction lives in one place.
func citationTitle(c api.Citation, resolveTitle func(string) string) string {
	if resolveTitle == nil {
		return ""
	}
	if c.ParentSourceID != "" {
		if t := resolveTitle(c.ParentSourceID); t != "" {
			return t
		}
	}
	return resolveTitle(c.SourceID)
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

// notebookSourceIndex lazily fetches a notebook's source list once and answers
// two questions about a source ID: its title, and whether the ID is still in the
// notebook at all. The presence answer lets a renderer distinguish a source that
// is present-but-untitled from one that has been removed — the common case after
// a re-sync, which mints new source UUIDs and strands a saved chat's citations
// on IDs that no longer resolve.
//
// mapped reports whether the project fetch actually succeeded: only then is a
// "not present" answer trustworthy. Before the first lookup, or after a failed
// fetch (offline / unauthed replay), mapped is false and callers must not claim
// a source was removed — absence of a map is not evidence of a removed source.
type notebookSourceIndex struct {
	c         *api.Client
	projectID string
	titles    map[string]string // nil until a successful fetch
	loaded    bool              // a fetch was attempted
	mapped    bool              // the fetch succeeded and titles is populated
}

// newNotebookSourceIndex returns a lazy index, or nil when no client/notebook is
// available (offline replay). A nil *notebookSourceIndex is safe to call: its
// methods degrade to "unknown".
func newNotebookSourceIndex(c *api.Client, projectID string) *notebookSourceIndex {
	if c == nil || projectID == "" {
		return nil
	}
	return &notebookSourceIndex{c: c, projectID: projectID}
}

// load fetches the source list at most once. Failures are suppressed (leaving
// mapped=false) so a renderer degrades to the citation's own data rather than
// erroring on an unauthed or offline replay.
func (idx *notebookSourceIndex) load() {
	if idx.loaded {
		return
	}
	idx.loaded = true
	proj, err := idx.c.GetProject(idx.projectID)
	if err != nil {
		return
	}
	idx.titles = make(map[string]string, len(proj.Sources))
	for _, s := range proj.Sources {
		if id := s.GetSourceId().GetSourceId(); id != "" {
			idx.titles[id] = s.GetTitle()
		}
	}
	idx.mapped = true
}

// title returns the notebook title for sourceID, or "" when the source has no
// title, is not in the notebook, or the list could not be fetched.
func (idx *notebookSourceIndex) title(sourceID string) string {
	if idx == nil {
		return ""
	}
	idx.load()
	return idx.titles[sourceID]
}

// removed reports whether sourceID is known to be absent from the notebook: the
// source list was fetched successfully and did not contain the ID. It returns
// false when the list is unavailable (offline / fetch failed), so a renderer
// never mislabels a source as removed on incomplete information.
func (idx *notebookSourceIndex) removed(sourceID string) bool {
	if idx == nil {
		return false
	}
	idx.load()
	if !idx.mapped {
		return false
	}
	_, ok := idx.titles[sourceID]
	return !ok
}

// notebookSourceTitles adapts a source index to the bare title lookup the
// live-stream renderer and persistableCitations use. Returns nil when no index
// is available so existing nil-checks keep working.
func notebookSourceTitles(c *api.Client, projectID string) func(string) string {
	idx := newNotebookSourceIndex(c, projectID)
	if idx == nil {
		return nil
	}
	return idx.title
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
		if api.IsChatStreamTimeout(streamErr) {
			return fmt.Errorf("generate chat: %w; the streaming RPC produced no usable response; check 'nlm auth status' and 'nlm sources %s'", streamErr, projectID)
		}
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
			Rich:      res.Rich,
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

func createReport(c *api.Client, notebookID, reportType string, extra []string, opts createReportOptions) error {
	description := ""
	instructions := ""
	if len(extra) > 0 {
		description = extra[0]
	}
	if len(extra) > 1 {
		instructions = strings.Join(extra[1:], " ")
	}

	flagIDs, err := resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
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
				suggestionIDs = reportSuggestionSourceIDs(s)
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
			SourceIDs: unionIDs(flagIDs, reportSuggestionSourceIDs(s)),
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

func reportSuggestionSourceIDs(s *pb.ReportSuggestion) []string {
	if s == nil {
		return nil
	}
	ids := make([]string, 0, len(s.GetSourceIds()))
	for _, source := range s.GetSourceIds() {
		if id := source.GetSourceId(); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
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
		fmt.Fprintf(os.Stderr, "usage: nlm chat config <notebook-id> <setting> [value]\n")
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
			return fmt.Errorf("usage: nlm chat config <id> goal <default|custom \"prompt\">")
		}
		switch args[2] {
		case "default":
			return c.SetChatConfig(notebookID, api.ChatGoalDefault, "", api.ResponseLengthDefault)
		case "custom":
			if len(args) < 4 {
				return fmt.Errorf("usage: nlm chat config <id> goal custom \"your prompt\"")
			}
			prompt := strings.Join(args[3:], " ")
			return c.SetChatConfig(notebookID, api.ChatGoalCustom, prompt, api.ResponseLengthDefault)
		default:
			return fmt.Errorf("unknown goal: %s (use 'default' or 'custom')", args[2])
		}
	case "length":
		if len(args) < 3 {
			return fmt.Errorf("usage: nlm chat config <id> length <default|longer|shorter>")
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

// citationContentKey derives a stable signature from an assistant answer so
// server-history citations can be matched to the locally-persisted message that
// carries the same text. It collapses whitespace (the two copies can differ in
// newlines) and clips to a prefix long enough to disambiguate turns within one
// conversation without being sensitive to late-in-the-answer edits.
func citationContentKey(content string) string {
	s := strings.Join(strings.Fields(content), " ")
	const prefix = 200
	if len(s) > prefix {
		s = s[:prefix]
	}
	return s
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

	mode := resolveCitationMode(opts.CitationMode)
	warnDeprecatedCitationMode(os.Stderr, opts.CitationMode)

	// chat-show is a local-only (noClient) command. Titles, excerpts, and
	// file:line resolution all want the server, so build a client on demand
	// when any is requested and auth is present; otherwise degrade to the
	// name-only rendering rather than erroring, so plain replays keep working
	// offline.
	//
	//   - Excerpts ride on the citation itself. The live-stream copy persisted
	//     locally has none (the stream frame carries no structured citations),
	//     so we refetch conversation history — whose frames DO carry per-source
	//     excerpts — and swap those citations in by message order.
	//   - --resolve-citations still needs source bodies for txtar file:line,
	//     via a memoized loader.
	//   - Titles come from the notebook source list.
	var loadSource func(string) (api.LoadSourceText, error)
	var resolveTitle func(string) string
	var sourceRemoved func(string) bool
	// Server-history citations (which carry excerpts and source-body offsets),
	// keyed by a signature of the assistant answer text. Keying by content
	// rather than position makes the swap robust to the server returning
	// messages in a different order (newest-first) or count than the local
	// session stores them (chronological).
	historyCitations := map[string][]api.Citation{}
	// Parsed answer-body span trees from server history, keyed the same way as
	// historyCitations (a signature of the answer text). Populated only when the
	// history fetch runs and a turn carries a tree; the renderers fall back to
	// flat Content for any turn without one.
	historyRich := map[string]*richDocument{}

	// Title resolution and the removed-source hint are cheap (one source-list
	// fetch) and improve every view, so wire them whenever auth is present —
	// not only under --citation-excerpts/--resolve-citations. Without them a
	// citation whose source was re-synced to a new UUID renders as a bare
	// handle on the default view, with nothing to say why the title is blank.
	// The expensive hydration (conversation-history excerpts, source-body
	// text for file:line) stays gated behind its flags below. Offline replay
	// (no auth) still degrades to name-only rather than erroring.
	if authToken != "" && cookies != "" {
		c := api.New(authToken, cookies)
		if debug {
			c.SetDebug(true)
		}
		// One source-list fetch backs both title resolution and the
		// removed-source hint, so a stranded citation (its source re-synced
		// to a new UUID) reads as "removed" rather than a bare handle.
		srcIndex := newNotebookSourceIndex(c, notebookID)
		resolveTitle = srcIndex.title
		sourceRemoved = srcIndex.removed

		if opts.ResolveCitations || opts.ExcerptBudget > 0 {
			// GetConversationHistory matches on the full conversation UUID; the
			// prefix chat-list/chat-show accept returns a 0-message response.
			// Expand it so the history fetch (and its excerpt-bearing citations)
			// actually resolves.
			fullConversationID := resolveConversationID(c, notebookID, conversationID)

			if opts.ExcerptBudget > 0 {
				if msgs, err := c.GetConversationHistory(notebookID, fullConversationID); err != nil {
					fmt.Fprintf(os.Stderr, "nlm: could not fetch history for excerpts (auth may be expired — run 'nlm auth'): %v\n", err)
				} else {
					for _, sm := range msgs {
						if sm.Role != 2 { // assistant only
							continue
						}
						key := citationContentKey(sm.Content)
						if len(sm.Citations) > 0 {
							historyCitations[key] = sm.Citations
						}
						// The history frame also carries the answer-body span tree
						// (Content ships newline-free, so its structure lives only
						// here). Capture it keyed the same way so the renderers can
						// reconstruct paragraphs/lists instead of one run-on block.
						if sm.Rich != nil {
							historyRich[key] = richDocumentFromAPI(sm.Rich)
						}
					}
				}
			}

			if opts.ResolveCitations {
				cache := make(map[string]api.LoadSourceText)
				var authWarned bool
				loadSource = func(sourceID string) (api.LoadSourceText, error) {
					if body, ok := cache[sourceID]; ok {
						if len(body.Fragments) == 0 {
							return api.LoadSourceText{}, fmt.Errorf("source %s unavailable", sourceID)
						}
						return body, nil
					}
					body, err := c.LoadSourceText(sourceID, notebookID)
					if err != nil {
						cache[sourceID] = api.LoadSourceText{} // negative cache
						if !authWarned {
							authWarned = true
							fmt.Fprintln(os.Stderr, "nlm: could not fetch source text (auth may be expired — run 'nlm auth'); skipping file:line.")
						}
						return api.LoadSourceText{}, err
					}
					cache[sourceID] = body
					return body, nil
				}
			}
		}
	} else if opts.ResolveCitations || opts.ExcerptBudget > 0 {
		// The plain view degrades silently offline, but a user who explicitly
		// asked for excerpts or file:line should hear why they're missing.
		fmt.Fprintln(os.Stderr, "nlm: --citation-excerpts/--resolve-citations need auth; run 'nlm auth'. Rendering names only.")
	}

	// Assemble the format-neutral document: swap in excerpt-bearing history
	// citations per assistant turn (matched by content signature), so every
	// renderer sees the best available citation copy.
	doc := chatDocument{
		NotebookID:     notebookID,
		ConversationID: conversationID,
	}
	for _, m := range session.Messages {
		dm := chatDocMessage{Role: m.Role, Content: m.Content, Thinking: m.Thinking}
		if m.Role == "assistant" {
			key := citationContentKey(m.Content)
			cites := m.Citations
			if hist, ok := historyCitations[key]; ok {
				cites = hist
			}
			dm.Citations = cites
			// The persisted turn carries its own answer-body tree (captured live
			// when it was generated), so the DEFAULT view renders structure
			// offline — no fetch, no --citation-excerpts. A live history refetch
			// (under --citation-excerpts) supplies the server's authoritative
			// copy, so prefer it when present.
			if m.Rich != nil {
				dm.Rich = richDocumentFromAPI(m.Rich)
			}
			if rich, ok := historyRich[key]; ok {
				dm.Rich = rich
			}
		}
		doc.Messages = append(doc.Messages, dm)
	}

	ctx := chatRenderContext{
		ShowThinking:   opts.ShowThinking,
		ExcerptBudget:  opts.ExcerptBudget,
		HideConfidence: opts.HideConfidence,
		HideSpans:      opts.HideSpans,
		resolveTitle:   resolveTitle,
		loadSource:     loadSource,
		sourceRemoved:  sourceRemoved,
	}

	switch opts.Format {
	case "markdown":
		return renderChatMarkdown(os.Stdout, doc, ctx)
	case "html":
		return renderChatHTMLToDestination(doc, ctx, opts)
	default: // "text"
		return renderChatText(os.Stdout, os.Stderr, doc, mode, ctx)
	}
}

// renderChatText is the terminal/plain projection: the original chat-show
// rendering, one turn at a time through the shared live renderer so citation
// output is identical to the streaming path.
func renderChatText(out, status io.Writer, doc chatDocument, mode citationRenderMode, ctx chatRenderContext) error {
	for i, m := range doc.Messages {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "[%s]\n", strings.ToUpper(m.Role))
		if m.Role != "assistant" {
			fmt.Fprintln(out, m.Content)
			continue
		}
		if ctx.ShowThinking && m.Thinking != "" {
			fmt.Fprintf(status, "%s%s%s\n", ansiGrey, m.Thinking, ansiReset)
		}
		// Reconstruct paragraph structure from the span tree only for the
		// run-together case (see shouldReflowFromTree): when Content already
		// carries its own newlines/markers it is the richer source, so it is
		// left as-is. The citation list below stays keyed on the flat answer
		// offsets regardless (its spans are labels, not slices of this text).
		body := m.Content
		if shouldReflowFromTree(m.Rich, m.Content) {
			if reflowed := flattenText(projectRichDocument(m.Rich)); reflowed != "" {
				body = reflowed
			}
		}
		renderPersistedAssistant(out, status, ChatMessage{
			Role:      m.Role,
			Content:   body,
			Thinking:  m.Thinking,
			Citations: m.Citations,
		}, mode, persistedRenderConfig{
			excerptBudget:  ctx.ExcerptBudget,
			hideConfidence: ctx.HideConfidence,
			hideSpans:      ctx.HideSpans,
			loadSource:     ctx.loadSource,
			resolveTitle:   ctx.resolveTitle,
			sourceRemoved:  ctx.sourceRemoved,
		})
	}
	return nil
}

// persistedRenderConfig carries the optional hydration hooks for replaying a
// stored assistant message. Its zero value renders name-only citations, the
// same as a plain offline replay.
type persistedRenderConfig struct {
	excerptBudget  int
	hideConfidence bool
	hideSpans      bool
	loadSource     func(string) (api.LoadSourceText, error) // fetch a source body for txtar file:line hydration
	resolveTitle   func(string) string                      // map a source ID to its notebook title
	sourceRemoved  func(string) bool                        // report a source ID no longer in the notebook
}

// renderPersistedAssistant feeds a stored assistant message through the live
// renderer so citation rendering works identically for replays. Excerpts ride
// on the message's citations; when cfg supplies a loader, --resolve-citations
// file:line locations are hydrated on top from the persisted citation spans.
func renderPersistedAssistant(out, status io.Writer, m ChatMessage, mode citationRenderMode, cfg persistedRenderConfig) {
	r := newChatStreamRenderer(out, status, false, false, mode)
	r.excerptBudget = cfg.excerptBudget
	r.showConfidence = !cfg.hideConfidence
	r.showSpans = !cfg.hideSpans
	r.loadSource = cfg.loadSource
	r.resolveTitle = cfg.resolveTitle
	r.sourceRemoved = cfg.sourceRemoved
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

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		seen := make(map[string]bool)
		for _, id := range convIDs {
			seen[id] = true
			rec := chatConversationRecord{
				ConversationID: id,
				Status:         "server",
			}
			if local, ok := localByConv[id]; ok {
				rec.Status = "synced"
				rec.MessageCount = len(local.Messages)
				rec.LastUpdated = local.UpdatedAt.Format(time.RFC3339)
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		for _, s := range localSessions {
			if s.ConversationID == "" || seen[s.ConversationID] {
				continue
			}
			rec := chatConversationRecord{
				ConversationID: s.ConversationID,
				MessageCount:   len(s.Messages),
				Status:         "local",
				LastUpdated:    s.UpdatedAt.Format(time.RFC3339),
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
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

// runAccount implements 'nlm account' (read) and
// 'nlm account set <key> <value>' (write).
//
// Supported keys for set: 'emoji' (default_project_emoji) and
// 'email-notifications' (true/false).
func runAccount(c *api.Client, args []string) error {
	if len(args) == 0 {
		status, err := c.GetAccountStatus()
		if err != nil {
			return err
		}
		notebooks, err := c.ListRecentlyViewedProjects()
		if err != nil {
			return fmt.Errorf("list notebooks for account status: %w", err)
		}
		notebookCount := len(notebooks)
		if jsonOutput {
			rec := accountStatusRecord{
				NotebookCount: notebookCount,
				NotebookLimit: status.NotebookLimit,
				SourceLimit:   status.SourceLimit,
				UploadLimit:   status.UploadLimit,
				Tier:          status.Tier,
			}
			data, mErr := json.MarshalIndent(rec, "", "  ")
			if mErr != nil {
				return mErr
			}
			fmt.Println(string(data))
			return nil
		}
		fmt.Printf("Notebook count: %d\n", notebookCount)
		if status.NotebookLimit > 0 {
			fmt.Printf("Notebook limit: %d\n", status.NotebookLimit)
		}
		if status.SourceLimit > 0 {
			fmt.Printf("Source limit:   %d\n", status.SourceLimit)
		}
		if status.UploadLimit > 0 {
			fmt.Printf("Upload limit:   %d\n", status.UploadLimit)
		}
		if status.Tier > 0 {
			fmt.Printf("Tier:           %d\n", status.Tier)
		}
		return nil
	}
	if args[0] != "set" {
		return fmt.Errorf("account: unknown subcommand %q (try 'set')", args[0])
	}
	if len(args) != 3 {
		return fmt.Errorf("account set: usage 'nlm account set <key> <value>'")
	}
	switch args[1] {
	case "emoji", "email-notifications":
		return fmt.Errorf("account set %s is not supported: NotebookLM changed the account wire schema", args[1])
	default:
		return fmt.Errorf("account set: unknown key %q (try 'emoji' or 'email-notifications')", args[1])
	}
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
	return createVideoOverviewWithOptions(c, projectID, instructions, videoCreateOptions{Language: "en"})
}

func createVideoOverviewWithOptions(c *api.Client, projectID string, instructions string, opts videoCreateOptions) error {
	// NLM may limit to one video per notebook. Check for existing.
	existingVideos, _ := c.ListVideoOverviews(projectID)
	if len(existingVideos) > 0 && !yes {
		fmt.Fprintf(os.Stderr, "Notebook already has a video overview. Use -y to replace it.\n")
		return fmt.Errorf("existing video overview")
	}

	fmt.Fprintf(os.Stderr, "Creating video overview for notebook %s...\n", projectID)
	fmt.Printf("Instructions: %s\n", instructions)

	style, err := parseVideoStyle(opts.Style)
	if err != nil {
		return err
	}
	audioType, err := parseAudioType(opts.AudioType, pb.AudioType_AUDIO_TYPE_BRIEF)
	if err != nil {
		return err
	}
	result, err := c.CreateVideoOverviewWithOptions(projectID, api.CreateVideoOverviewOptions{
		Instructions: instructions,
		AudioType:    audioType,
		VideoStyle:   style,
		Language:     opts.Language,
	})
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
	if !jsonOutput {
		fmt.Fprintf(os.Stderr, "Listing audio overviews for notebook %s...\n", notebookID)
	}

	audioOverviews, err := c.ListAudioOverviews(notebookID)
	if err != nil {
		return fmt.Errorf("list audio overviews: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, audio := range audioOverviews {
			status := "pending"
			if audio.IsReady {
				status = "ready"
			}
			rec := audioOverviewRecord{
				AudioID: audio.AudioID,
				Title:   audio.Title,
				Status:  status,
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
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
	if !jsonOutput {
		fmt.Fprintf(os.Stderr, "Listing video overviews for notebook %s...\n", notebookID)
	}

	videoOverviews, err := c.ListVideoOverviews(notebookID)
	if err != nil {
		return fmt.Errorf("list video overviews: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, video := range videoOverviews {
			status := "pending"
			if video.IsReady {
				status = "ready"
			}
			rec := videoOverviewRecord{
				VideoID: video.VideoID,
				Title:   video.Title,
				Status:  status,
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
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
		return audioDownloadUnavailableError(notebookID, err)
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

func audioDownloadUnavailableError(notebookID string, err error) error {
	u := printDownloadBrowserFallback("audio overview", notebookID)
	return fmt.Errorf("download audio overview: direct download unavailable (%w); open %s in a browser", err, u)
}

func notebookBrowserURL(notebookID string) string {
	return "https://notebooklm.google.com/notebook/" + url.PathEscape(notebookID)
}

func printDownloadBrowserFallback(kind, notebookID string) string {
	u := notebookBrowserURL(notebookID)
	fmt.Println(u)
	fmt.Fprintf(os.Stderr, "Open %s in a browser to download the %s from NotebookLM.\n", u, kind)
	return u
}

func downloadVideoOverview(c *api.Client, notebookID string, filename string) error {
	fmt.Fprintf(os.Stderr, "Downloading video overview for notebook %s...\n", notebookID)
	c.SetUseDirectRPC(true)

	// Generate default filename if not provided
	if filename == "" {
		filename = fmt.Sprintf("video_overview_%s.mp4", notebookID)
	}

	// Download the video
	videoResult, err := c.DownloadVideoOverview(notebookID)
	if err != nil {
		if strings.Contains(err.Error(), "browser authentication") || strings.Contains(err.Error(), "manual") || strings.Contains(err.Error(), "not available") {
			u := printDownloadBrowserFallback("video overview", notebookID)
			return fmt.Errorf("download video overview: direct download unavailable; open %s in a browser", u)
		}
		return fmt.Errorf("download video overview: %w", err)
	}

	// Check if we got a video URL
	if videoResult.VideoData != "" && (strings.HasPrefix(videoResult.VideoData, "http://") || strings.HasPrefix(videoResult.VideoData, "https://")) {
		// Use authenticated download for URLs
		if err := c.DownloadVideoWithAuth(videoResult.VideoData, filename); err != nil {
			if strings.Contains(err.Error(), "text/html") {
				u := printDownloadBrowserFallback("video overview", notebookID)
				return fmt.Errorf("download video: browser-authenticated download required; open %s in a browser", u)
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
