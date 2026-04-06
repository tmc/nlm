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

	"github.com/google/uuid"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/gen/service"
	"github.com/tmc/nlm/internal/auth"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/notebooklm/api"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
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
	chunkedResponse   bool // Control rt=c parameter for chunked vs JSON array response
	useDirectRPC      bool // Use direct RPC calls instead of orchestration service
	skipSources       bool // Skip fetching sources for chat (useful when project is inaccessible)
	yes               bool   // Skip confirmation prompts
	sourceName        string // Custom name for added sources
)

// ChatSession represents a persistent chat conversation
type ChatSession struct {
	NotebookID     string        `json:"notebook_id"`
	ConversationID string        `json:"conversation_id,omitempty"`
	Messages       []ChatMessage `json:"messages"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// ChatMessage represents a single message in the conversation
type ChatMessage struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
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

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: nlm <command> [arguments]\n\n")
		fmt.Fprintf(os.Stderr, "Notebook Commands:\n")
		fmt.Fprintf(os.Stderr, "  list, ls          List all notebooks\n")
		fmt.Fprintf(os.Stderr, "  create <title>    Create a new notebook\n")
		fmt.Fprintf(os.Stderr, "  rm <id>           Delete a notebook\n")
		fmt.Fprintf(os.Stderr, "  analytics <id>    Show notebook analytics\n")
		fmt.Fprintf(os.Stderr, "  list-featured     List featured notebooks\n\n")

		fmt.Fprintf(os.Stderr, "Source Commands:\n")
		fmt.Fprintf(os.Stderr, "  sources <id>      List sources in notebook\n")
		fmt.Fprintf(os.Stderr, "  add <id> <input>  Add source to notebook\n")
		fmt.Fprintf(os.Stderr, "  rm-source <id> <source-id>  Remove source\n")
		fmt.Fprintf(os.Stderr, "  rename-source <source-id> <new-name>  Rename source\n")
		fmt.Fprintf(os.Stderr, "  refresh-source <source-id>  Refresh source content\n")
		fmt.Fprintf(os.Stderr, "  check-source <source-id>  Check source freshness\n")
		fmt.Fprintf(os.Stderr, "  discover-sources <id> <query>  Discover relevant sources\n\n")

		fmt.Fprintf(os.Stderr, "Note Commands:\n")
		fmt.Fprintf(os.Stderr, "  notes <id>        List notes in notebook\n")
		fmt.Fprintf(os.Stderr, "  new-note <id> <title> [content]  Create new note (content via arg or stdin)\n")
		fmt.Fprintf(os.Stderr, "  update-note <id> <note-id> <content> <title>  Edit note\n")
		fmt.Fprintf(os.Stderr, "  rm-note <note-id>  Remove note\n\n")

		fmt.Fprintf(os.Stderr, "Audio Commands:\n")
		fmt.Fprintf(os.Stderr, "  audio-list <id>   List all audio overviews for a notebook with status\n")
		fmt.Fprintf(os.Stderr, "  audio-create <id> <instructions>  Create audio overview\n")
		fmt.Fprintf(os.Stderr, "  audio-get <id>    Get audio overview\n")
		fmt.Fprintf(os.Stderr, "  audio-download <id> [filename]  Download audio file (requires --direct-rpc)\n")
		fmt.Fprintf(os.Stderr, "  audio-rm <id>     Delete audio overview\n")
		fmt.Fprintf(os.Stderr, "  audio-share <id>  Share audio overview\n\n")

		fmt.Fprintf(os.Stderr, "Video Commands:\n")
		fmt.Fprintf(os.Stderr, "  video-list <id>   List all video overviews for a notebook with status\n")
		fmt.Fprintf(os.Stderr, "  video-create <id> <instructions>  Create video overview\n")
		fmt.Fprintf(os.Stderr, "  video-download <id> [filename]  Download video file (requires --direct-rpc)\n\n")

		fmt.Fprintf(os.Stderr, "Artifact Commands:\n")
		fmt.Fprintf(os.Stderr, "  create-artifact <id> <type>  Create artifact (note|audio|report|app)\n")
		fmt.Fprintf(os.Stderr, "  get-artifact <artifact-id>  Get artifact details\n")
		fmt.Fprintf(os.Stderr, "  artifacts <id>       List artifacts in notebook\n")
		fmt.Fprintf(os.Stderr, "  list-artifacts <id>  List artifacts in notebook (alias)\n")
		fmt.Fprintf(os.Stderr, "  rename-artifact <artifact-id> <new-title>  Rename artifact\n")
		fmt.Fprintf(os.Stderr, "  delete-artifact <artifact-id>  Delete artifact\n\n")

		fmt.Fprintf(os.Stderr, "Generation Commands:\n")
		fmt.Fprintf(os.Stderr, "  generate-guide <id>  Generate notebook guide\n")
		fmt.Fprintf(os.Stderr, "  generate-outline <id>  Generate content outline\n")
		fmt.Fprintf(os.Stderr, "  generate-section <id>  Generate new section\n")
		fmt.Fprintf(os.Stderr, "  generate-chat <id> <prompt>  Free-form chat generation\n")
		fmt.Fprintf(os.Stderr, "  generate-magic <id> <source-ids...>  Generate magic view from sources\n")
		fmt.Fprintf(os.Stderr, "  chat <id>               Interactive chat session\n")
		fmt.Fprintf(os.Stderr, "  chat-list               List all saved chat sessions\n")
		fmt.Fprintf(os.Stderr, "  delete-chat <id>        Delete server-side chat history\n")
		fmt.Fprintf(os.Stderr, "  chat-config <id> <setting> [value]  Configure chat settings\n")
		fmt.Fprintf(os.Stderr, "  set-instructions <id> \"prompt\"      Set system instructions\n")
		fmt.Fprintf(os.Stderr, "  get-instructions <id>               Show current system instructions\n\n")

		fmt.Fprintf(os.Stderr, "Content Transformation Commands:\n")
		fmt.Fprintf(os.Stderr, "  rephrase <id> <source-ids...>     Rephrase content from sources\n")
		fmt.Fprintf(os.Stderr, "  expand <id> <source-ids...>       Expand on content from sources\n")
		fmt.Fprintf(os.Stderr, "  summarize <id> <source-ids...>    Summarize content from sources\n")
		fmt.Fprintf(os.Stderr, "  critique <id> <source-ids...>     Provide critique of content\n")
		fmt.Fprintf(os.Stderr, "  brainstorm <id> <source-ids...>   Brainstorm ideas from sources\n")
		fmt.Fprintf(os.Stderr, "  verify <id> <source-ids...>       Verify facts in sources\n")
		fmt.Fprintf(os.Stderr, "  explain <id> <source-ids...>      Explain concepts from sources\n")
		fmt.Fprintf(os.Stderr, "  outline <id> <source-ids...>      Create outline from sources\n")
		fmt.Fprintf(os.Stderr, "  study-guide <id> <source-ids...>  Generate study guide\n")
		fmt.Fprintf(os.Stderr, "  faq <id> <source-ids...>          Generate FAQ from sources\n")
		fmt.Fprintf(os.Stderr, "  briefing-doc <id> <source-ids...> Create briefing document\n")
		fmt.Fprintf(os.Stderr, "  mindmap <id> <source-ids...>      Generate interactive mindmap\n")
		fmt.Fprintf(os.Stderr, "  timeline <id> <source-ids...>     Create timeline from sources\n")
		fmt.Fprintf(os.Stderr, "  toc <id> <source-ids...>          Generate table of contents\n\n")

		fmt.Fprintf(os.Stderr, "Sharing Commands:\n")
		fmt.Fprintf(os.Stderr, "  share <id>        Share notebook publicly\n")
		fmt.Fprintf(os.Stderr, "  share-private <id>  Share notebook privately\n")
		fmt.Fprintf(os.Stderr, "  share-details <share-id>  Get details of shared project\n\n")

		fmt.Fprintf(os.Stderr, "Research Commands:\n")
		fmt.Fprintf(os.Stderr, "  research <id> \"query\"   Start deep research and poll for results\n\n")

		fmt.Fprintf(os.Stderr, "Other Commands:\n")
		fmt.Fprintf(os.Stderr, "  auth [profile]    Setup authentication\n")
		fmt.Fprintf(os.Stderr, "  refresh           Refresh authentication credentials\n")
		fmt.Fprintf(os.Stderr, "  feedback <msg>    Submit feedback\n")
		fmt.Fprintf(os.Stderr, "  hb                Send heartbeat\n\n")
	}
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
func validateArgs(cmd string, args []string) error {
	switch cmd {
	case "create":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm create <title>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "rm":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm rm <id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "sources":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm sources <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "add":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm add <notebook-id> <file>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "rm-source":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm rm-source <notebook-id> <source-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "rename-source":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm rename-source <source-id> <new-name>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "new-note":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm new-note <notebook-id> <title> [content]\n")
			fmt.Fprintf(os.Stderr, "  content can also be piped via stdin\n")
			return fmt.Errorf("invalid arguments")
		}
	case "update-note":
		if len(args) != 4 {
			fmt.Fprintf(os.Stderr, "usage: nlm update-note <notebook-id> <note-id> <content> <title>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "rm-note":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm rm-note <notebook-id> <note-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "audio-list":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm audio-list <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "audio-download":
		if len(args) < 1 || len(args) > 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm audio-download <notebook-id> [filename]\n")
			return fmt.Errorf("invalid arguments")
		}
	case "video-list":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm video-list <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "video-download":
		if len(args) < 1 || len(args) > 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm video-download <notebook-id> [filename]\n")
			return fmt.Errorf("invalid arguments")
		}
	case "audio-create":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm audio-create <notebook-id> <instructions>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "audio-get":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm audio-get <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "audio-rm":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm audio-rm <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "audio-share":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm audio-share <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "video-create":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm video-create <notebook-id> <instructions>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "share":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm share <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "share-private":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm share-private <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "share-details":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm share-details <share-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "refresh":
		// refresh command optionally takes -debug flag
		// Don't validate here, let the command handle its own flags
	case "generate-guide":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm generate-guide <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "generate-outline":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm generate-outline <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "generate-section":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm generate-section <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "generate-magic":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm generate-magic <notebook-id> <source-id> [source-id...]\n")
			return fmt.Errorf("invalid arguments")
		}
	case "generate-mindmap":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm generate-mindmap <notebook-id> <source-id> [source-id...]\n")
			return fmt.Errorf("invalid arguments")
		}
	case "rephrase", "expand", "summarize", "critique", "brainstorm", "verify", "explain", "outline", "study-guide", "faq", "briefing-doc", "mindmap", "timeline", "toc":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <source-id> [source-id...]\n", cmd)
			return fmt.Errorf("invalid arguments")
		}
	case "generate-chat":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm generate-chat <notebook-id> <prompt>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "set-instructions":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm set-instructions <notebook-id> \"prompt text\"\n")
			return fmt.Errorf("invalid arguments")
		}
	case "get-instructions":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm get-instructions <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "research":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm research <notebook-id> \"query\"\n")
			return fmt.Errorf("invalid arguments")
		}
	case "chat":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm chat <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "chat-list":
		if len(args) != 0 {
			fmt.Fprintf(os.Stderr, "usage: nlm chat-list\n")
			return fmt.Errorf("invalid arguments")
		}
	case "delete-chat":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm delete-chat <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "chat-config":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm chat-config <notebook-id> <setting> [value]\n")
			return fmt.Errorf("invalid arguments")
		}
	case "create-artifact":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm create-artifact <notebook-id> <type>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "get-artifact":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm get-artifact <artifact-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "list-artifacts", "artifacts":
		if len(args) != 1 {
			if cmd == "artifacts" {
				fmt.Fprintf(os.Stderr, "usage: nlm artifacts <notebook-id>\n")
			} else {
				fmt.Fprintf(os.Stderr, "usage: nlm list-artifacts <notebook-id>\n")
			}
			return fmt.Errorf("invalid arguments")
		}
	case "rename-artifact":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm rename-artifact <artifact-id> <new-title>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "delete-artifact":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm delete-artifact <artifact-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "discover-sources":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm discover-sources <notebook-id> <query>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "analytics":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm analytics <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "check-source":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm check-source <source-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "refresh-source":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm refresh-source <source-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "notes":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm notes <notebook-id>\n")
			return fmt.Errorf("invalid arguments")
		}
	case "feedback":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm feedback <message>\n")
			return fmt.Errorf("invalid arguments")
		}
	}
	return nil
}

// isValidCommand checks if a command is valid
func isValidCommand(cmd string) bool {
	validCommands := []string{
		"help", "-h", "--help",
		"list", "ls", "create", "rm", "analytics", "list-featured",
		"sources", "add", "rm-source", "rename-source", "refresh-source", "check-source", "discover-sources",
		"notes", "new-note", "update-note", "rm-note",
		"audio-create", "audio-get", "audio-rm", "audio-share", "audio-list", "audio-download", "video-create", "video-list", "video-download",
		"create-artifact", "get-artifact", "list-artifacts", "artifacts", "rename-artifact", "delete-artifact",
		"generate-guide", "generate-outline", "generate-section", "generate-magic", "generate-mindmap", "generate-chat", "chat", "chat-list", "delete-chat", "chat-config", "set-instructions", "get-instructions",
		"rephrase", "expand", "summarize", "critique", "brainstorm", "verify", "explain", "outline", "study-guide", "faq", "briefing-doc", "mindmap", "timeline", "toc",
		"research",
		"auth", "refresh", "hb", "share", "share-private", "share-details", "feedback",
	}

	for _, valid := range validCommands {
		if cmd == valid {
			return true
		}
	}
	return false
}

func isAuthCommand(cmd string) bool {
	// Only help-related commands don't need auth
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		return false
	}
	// Auth command doesn't need prior auth
	if cmd == "auth" {
		return false
	}
	// Refresh command manages its own auth
	if cmd == "refresh" {
		return false
	}
	// Chat-list just lists local sessions, no auth needed
	if cmd == "chat-list" {
		return false
	}
	return true
}

func run() error {
	if authToken == "" {
		authToken = os.Getenv("NLM_AUTH_TOKEN")
	}
	if cookies == "" {
		cookies = os.Getenv("NLM_COOKIES")
	}

	if debug {
		fmt.Printf("DEBUG: Auth token loaded: %v\n", authToken != "")
		fmt.Printf("DEBUG: Cookies loaded: %v\n", cookies != "")
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
			fmt.Printf("DEBUG: Token: %s\n", tokenDisplay)
		}
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	cmd := flag.Arg(0)
	args := flag.Args()[1:]

	// Check if command is valid first
	if !isValidCommand(cmd) {
		flag.Usage()
		os.Exit(1)
	}

	// Validate arguments first (before authentication check)
	if err := validateArgs(cmd, args); err != nil {
		return err
	}

	// Check if this command needs authentication
	if isAuthCommand(cmd) && (authToken == "" || cookies == "") {
		fmt.Fprintf(os.Stderr, "Authentication required for '%s'. Run 'nlm auth' first.\n", cmd)
		return fmt.Errorf("authentication required")
	}

	// Handle help commands without creating API client
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		flag.Usage()
		os.Exit(0)
	}

	// Handle auth command
	if cmd == "auth" {
		_, _, err := handleAuth(args, debug)
		return err
	}

	// Handle refresh command
	if cmd == "refresh" {
		return refreshCredentials(debug)
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
		// Set direct RPC flag if specified
		if useDirectRPC {
			client.SetUseDirectRPC(true)
			if debug {
				fmt.Fprintf(os.Stderr, "nlm: using direct RPC for audio/video operations\n")
			}
		}
		cmdErr := runCmd(client, cmd, args...)
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
		} else {
			// Save the refreshed credentials
			if saveErr := saveCredentials(authToken, cookies); saveErr != nil && debug {
				fmt.Fprintf(os.Stderr, "nlm: warning: failed to save credentials: %v\n", saveErr)
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
		"session.*invalid",
		"session.*expired",
		"login.*required",
		"auth.*required",
		"invalid.*credentials",
		"token.*expired",
		"cookie.*invalid",
	}

	for _, keyword := range authKeywords {
		if strings.Contains(errorStr, keyword) {
			return true
		}
	}

	return false
}

// saveCredentials saves authentication credentials to environment file
func saveCredentials(authToken, cookies string) error {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	// Create .nlm directory if it doesn't exist
	nlmDir := filepath.Join(home, ".nlm")
	if err := os.MkdirAll(nlmDir, 0755); err != nil {
		return fmt.Errorf("create nlm directory: %w", err)
	}

	// Write environment file
	envFile := filepath.Join(nlmDir, "env")
	content := fmt.Sprintf(`NLM_COOKIES=%q
NLM_AUTH_TOKEN=%q
NLM_BROWSER_PROFILE=%q
`,
		cookies,
		authToken,
		chromeProfile,
	)

	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}

	return nil
}

func runCmd(client *api.Client, cmd string, args ...string) error {
	var err error
	switch cmd {
	// Notebook operations
	case "list", "ls":
		err = list(client)
	case "create":
		err = create(client, args[0])
	case "rm":
		err = remove(client, args[0])
	case "analytics":
		err = getAnalytics(client, args[0])
	case "list-featured":
		err = listFeaturedProjects(client)

	// Source operations
	case "sources":
		err = listSources(client, args[0])
	case "add":
		var id string
		id, err = addSource(client, args[0], args[1])
		fmt.Println(id)
	case "rm-source":
		err = removeSource(client, args[0], args[1])
	case "rename-source":
		err = renameSource(client, args[0], args[1])
	case "refresh-source":
		err = refreshSource(client, args[0])
	case "check-source":
		err = checkSourceFreshness(client, args[0])
	case "discover-sources":
		err = discoverSources(client, args[0], args[1])

	// Note operations
	case "notes":
		err = listNotes(client, args[0])
	case "new-note":
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
		err = createNote(client, args[0], args[1], noteContent)
	case "update-note":
		err = updateNote(client, args[0], args[1], args[2], args[3])
	case "rm-note":
		err = removeNote(client, args[0], args[1])

		// Audio operations
	case "audio-create":
		err = createAudioOverview(client, args[0], args[1])
	case "audio-get":
		err = getAudioOverview(client, args[0])
	case "audio-rm":
		err = deleteAudioOverview(client, args[0])
	case "audio-share":
		err = shareAudioOverview(client, args[0])
	case "audio-list":
		err = listAudioOverviews(client, args[0])
	case "audio-download":
		filename := ""
		if len(args) > 1 {
			filename = args[1]
		}
		err = downloadAudioOverview(client, args[0], filename)
	case "video-create":
		err = createVideoOverview(client, args[0], args[1])
	case "video-list":
		err = listVideoOverviews(client, args[0])
	case "video-download":
		filename := ""
		if len(args) > 1 {
			filename = args[1]
		}
		err = downloadVideoOverview(client, args[0], filename)

	// Artifact operations
	case "create-artifact":
		err = createArtifact(client, args[0], args[1])
	case "get-artifact":
		err = getArtifact(client, args[0])
	case "list-artifacts", "artifacts":
		err = listArtifacts(client, args[0])
	case "rename-artifact":
		err = renameArtifact(client, args[0], args[1])
	case "delete-artifact":
		err = deleteArtifact(client, args[0])

		// Generation operations
	case "generate-guide":
		err = generateNotebookGuide(client, args[0])
	case "generate-outline":
		err = generateOutline(client, args[0])
	case "generate-section":
		err = generateSection(client, args[0])
	case "generate-magic":
		err = generateMagicView(client, args[0], args[1:])
	case "generate-mindmap":
		err = generateMindmap(client, args[0], args[1:])
	case "rephrase":
		err = actOnSources(client, args[0], "rephrase", args[1:])
	case "expand":
		err = actOnSources(client, args[0], "expand", args[1:])
	case "summarize":
		err = actOnSources(client, args[0], "summarize", args[1:])
	case "critique":
		err = actOnSources(client, args[0], "critique", args[1:])
	case "brainstorm":
		err = actOnSources(client, args[0], "brainstorm", args[1:])
	case "verify":
		err = actOnSources(client, args[0], "verify", args[1:])
	case "explain":
		err = actOnSources(client, args[0], "explain", args[1:])
	case "outline":
		err = actOnSources(client, args[0], "outline", args[1:])
	case "study-guide":
		err = actOnSources(client, args[0], "study_guide", args[1:])
	case "faq":
		err = actOnSources(client, args[0], "faq", args[1:])
	case "briefing-doc":
		err = actOnSources(client, args[0], "briefing_doc", args[1:])
	case "mindmap":
		err = actOnSources(client, args[0], "interactive_mindmap", args[1:])
	case "timeline":
		err = actOnSources(client, args[0], "timeline", args[1:])
	case "toc":
		err = actOnSources(client, args[0], "table_of_contents", args[1:])
	case "generate-chat":
		err = generateFreeFormChat(client, args[0], args[1])
	case "chat":
		err = interactiveChat(client, args[0])
	case "chat-list":
		err = listChatSessions()
	case "delete-chat":
		err = deleteChatHistory(client, args[0])
	case "chat-config":
		err = setChatConfig(client, args)
	case "set-instructions":
		prompt := strings.Join(args[1:], " ")
		err = setInstructions(client, args[0], prompt)
	case "get-instructions":
		err = getInstructions(client, args[0])

	// Research operations
	case "research":
		query := strings.Join(args[1:], " ")
		err = deepResearch(client, args[0], query)

	// Sharing operations
	case "share":
		err = shareNotebook(client, args[0])
	case "share-private":
		err = shareNotebookPrivate(client, args[0])
	case "share-details":
		err = getShareDetails(client, args[0])

	// Other operations
	case "feedback":
		err = submitFeedback(client, args[0])
	case "hb":
		err = heartbeat(client)
	default:
		flag.Usage()
		os.Exit(1)
	}

	return err
}

// confirmAction prompts the user for confirmation unless --yes is set.
func confirmAction(prompt string) bool {
	if yes {
		return true
	}
	fmt.Printf("%s [y/N] ", prompt)
	var response string
	fmt.Scanln(&response)
	return strings.HasPrefix(strings.ToLower(response), "y")
}

// Notebook operations
func list(c *api.Client) error {
	notebooks, err := c.ListRecentlyViewedProjects()
	if err != nil {
		return err
	}

	// Display total count
	total := len(notebooks)
	fmt.Printf("Total notebooks: %d (showing first 10)\n\n", total)

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
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			nb.ProjectId, title, sourceCount,
			nb.GetMetadata().GetCreateTime().AsTime().Format(time.RFC3339),
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
		status := "enabled"
		if src.Metadata != nil {
			status = src.Metadata.Status.String()
		}

		lastUpdated := "unknown"
		if src.Metadata != nil && src.Metadata.LastModifiedTime != nil {
			lastUpdated = src.Metadata.LastModifiedTime.AsTime().Format(time.RFC3339)
		}

		sourceType := "unknown"
		if src.Metadata != nil {
			sourceType = src.Metadata.GetSourceType().String()
		}

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
		fmt.Printf("Adding source from URL: %s\n", input)
		return c.AddSourceFromURL(notebookID, input)
	}

	// Try as local file
	if _, err := os.Stat(input); err == nil {
		fmt.Printf("Adding source from file: %s\n", input)
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
	fmt.Println("Adding text content as source...")
	textName := "Text Source"
	if sourceName != "" {
		textName = sourceName
	}
	return c.AddSourceFromText(notebookID, input, textName)
}

func removeSource(c *api.Client, notebookID, sourceID string) error {
	if !confirmAction(fmt.Sprintf("Are you sure you want to remove source %s?", sourceID)) {
		return fmt.Errorf("operation cancelled")
	}

	if err := c.DeleteSources(notebookID, []string{sourceID}); err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	fmt.Printf("✅ Removed source %s from notebook %s\n", sourceID, notebookID)
	return nil
}

func renameSource(c *api.Client, sourceID, newName string) error {
	fmt.Printf("Renaming source %s to: %s\n", sourceID, newName)
	if _, err := c.MutateSource(sourceID, &pb.Source{
		Title: newName,
	}); err != nil {
		return fmt.Errorf("rename source: %w", err)
	}

	fmt.Printf("✅ Renamed source to: %s\n", newName)
	return nil
}

// Note operations
func createNote(c *api.Client, notebookID, title, content string) error {
	fmt.Printf("Creating note in notebook %s...\n", notebookID)
	if _, err := c.CreateNote(notebookID, title, content); err != nil {
		return fmt.Errorf("create note: %w", err)
	}
	fmt.Printf("✅ Created note: %s\n", title)
	return nil
}

func updateNote(c *api.Client, notebookID, noteID, content, title string) error {
	fmt.Printf("Updating note %s...\n", noteID)
	if _, err := c.MutateNote(notebookID, noteID, content, title); err != nil {
		return fmt.Errorf("update note: %w", err)
	}
	fmt.Printf("✅ Updated note: %s\n", title)
	return nil
}

func removeNote(c *api.Client, notebookID, noteID string) error {
	if !confirmAction(fmt.Sprintf("Are you sure you want to remove note %s?", noteID)) {
		return fmt.Errorf("operation cancelled")
	}

	if err := c.DeleteNotes(notebookID, []string{noteID}); err != nil {
		return fmt.Errorf("remove note: %w", err)
	}
	fmt.Printf("✅ Removed note: %s\n", noteID)
	return nil
}

// Note operations
func listNotes(c *api.Client, notebookID string) error {
	notes, err := c.GetNotes(notebookID)
	if err != nil {
		return fmt.Errorf("list notes: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tLAST MODIFIED")
	for _, note := range notes {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			note.GetSourceId(),
			note.Title,
			note.GetMetadata().LastModifiedTime.AsTime().Format(time.RFC3339),
		)
	}
	return w.Flush()
}

// Audio operations
func getAudioOverview(c *api.Client, projectID string) error {
	fmt.Fprintf(os.Stderr, "Fetching audio overview...\n")

	result, err := c.GetAudioOverview(projectID)
	if err != nil {
		return fmt.Errorf("get audio overview: %w", err)
	}

	if !result.IsReady {
		fmt.Println("Audio overview is not ready yet. Try again in a few moments.")
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
	fmt.Printf("✅ Deleted audio overview\n")
	return nil
}

func shareAudioOverview(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating share link...\n")
	resp, err := c.ShareAudio(notebookID, api.SharePublic)
	if err != nil {
		return fmt.Errorf("share audio: %w", err)
	}
	fmt.Printf("Share URL: %s\n", resp.ShareURL)
	return nil
}

// Generation operations
func generateNotebookGuide(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating notebook guide...\n")
	guide, err := c.GenerateNotebookGuide(notebookID)
	if err != nil {
		return fmt.Errorf("generate guide: %w", err)
	}
	fmt.Printf("Guide:\n%s\n", guide.Content)
	return nil
}

func generateOutline(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating outline...\n")
	outline, err := c.GenerateOutline(notebookID)
	if err != nil {
		return fmt.Errorf("generate outline: %w", err)
	}
	fmt.Printf("Outline:\n%s\n", outline.Content)
	return nil
}

func generateSection(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating section...\n")
	section, err := c.GenerateSection(notebookID)
	if err != nil {
		return fmt.Errorf("generate section: %w", err)
	}
	fmt.Printf("Section:\n%s\n", section.Content)
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

func generateMindmap(c *api.Client, notebookID string, sourceIDs []string) error {
	fmt.Fprintf(os.Stderr, "Generating interactive mindmap...\n")
	err := c.ActOnSources(notebookID, "interactive_mindmap", sourceIDs)
	if err != nil {
		return fmt.Errorf("generate mindmap: %w", err)
	}
	fmt.Printf("Interactive mindmap generated successfully.\n")
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
	err := c.ActOnSources(notebookID, action, sourceIDs)
	if err != nil {
		return fmt.Errorf("%s: %w", strings.ToLower(actionName), err)
	}
	fmt.Printf("Content %s successfully.\n", strings.ToLower(actionName))
	return nil
}

// func shareNotebook(c *api.Client, notebookID string) error {
// 	fmt.Fprintf(os.Stderr, "Generating share link...\n")
// 	resp, err := c.ShareProject(notebookID)
// 	if err != nil {
// 		return fmt.Errorf("share notebook: %w", err)
// 	}
// 	fmt.Printf("Share URL: %s\n", resp.ShareUrl)
// 	return nil
// }

// func submitFeedback(c *api.Client, message string) error {
// 	if err := c.SubmitFeedback(message); err != nil {
// 		return fmt.Errorf("submit feedback: %w", err)
// 	}
// 	fmt.Printf("✅ Feedback submitted\n")
// 	return nil
// }

// Other operations
func createAudioOverview(c *api.Client, projectID string, instructions string) error {
	fmt.Printf("Creating audio overview for notebook %s...\n", projectID)
	fmt.Printf("Instructions: %s\n", instructions)

	result, err := c.CreateAudioOverview(projectID, instructions)
	if err != nil {
		return fmt.Errorf("create audio overview: %w", err)
	}

	if !result.IsReady {
		fmt.Println("✅ Audio overview creation started. Use 'nlm audio-get' to check status.")
		return nil
	}

	// If the result is immediately ready (unlikely but possible)
	fmt.Printf("✅ Audio Overview created:\n")
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
	// Create orchestration service client using the same auth as the main client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	req := &pb.GetProjectAnalyticsRequest{
		ProjectId: projectID,
	}

	analytics, err := orchClient.GetProjectAnalytics(context.Background(), req)
	if err != nil {
		return fmt.Errorf("get analytics: %w", err)
	}

	fmt.Printf("Project Analytics for %s:\n", projectID)
	fmt.Printf("  Sources: %d\n", analytics.SourceCount)
	fmt.Printf("  Notes: %d\n", analytics.NoteCount)
	fmt.Printf("  Audio Overviews: %d\n", analytics.AudioOverviewCount)
	if analytics.LastAccessed != nil {
		fmt.Printf("  Last Accessed: %s\n", analytics.LastAccessed.AsTime().Format(time.RFC3339))
	}

	return nil
}

func listFeaturedProjects(c *api.Client) error {
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	req := &pb.ListFeaturedProjectsRequest{
		PageSize: 20,
	}

	resp, err := orchClient.ListFeaturedProjects(context.Background(), req)
	if err != nil {
		return fmt.Errorf("list featured projects: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tDESCRIPTION")

	for _, project := range resp.Projects {
		description := ""
		if len(project.Sources) > 0 {
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
func refreshSource(c *api.Client, sourceID string) error {
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	req := &pb.RefreshSourceRequest{
		SourceId: sourceID,
	}

	fmt.Fprintf(os.Stderr, "Refreshing source %s...\n", sourceID)
	source, err := orchClient.RefreshSource(context.Background(), req)
	if err != nil {
		return fmt.Errorf("refresh source: %w", err)
	}

	fmt.Printf("✅ Refreshed source: %s\n", source.Title)
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
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	req := &pb.DiscoverSourcesRequest{
		ProjectId: projectID,
		Query:     query,
	}

	fmt.Fprintf(os.Stderr, "Discovering sources for query: %s\n", query)
	resp, err := orchClient.DiscoverSources(context.Background(), req)
	if err != nil {
		return fmt.Errorf("discover sources: %w", err)
	}

	if len(resp.Sources) == 0 {
		fmt.Println("No sources found for the query.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tTYPE\tRELEVANCE")

	for _, source := range resp.Sources {
		relevance := "Unknown"
		if source.Metadata != nil {
			relevance = source.Metadata.GetSourceType().String()
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			source.SourceId.GetSourceId(),
			strings.TrimSpace(source.Title),
			source.Metadata.GetSourceType(),
			relevance)
	}
	return w.Flush()
}

// Artifact management
func createArtifact(c *api.Client, projectID, artifactType string) error {
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	// Parse artifact type
	var aType pb.ArtifactType
	switch strings.ToLower(artifactType) {
	case "note":
		aType = pb.ArtifactType_ARTIFACT_TYPE_NOTE
	case "audio":
		aType = pb.ArtifactType_ARTIFACT_TYPE_AUDIO_OVERVIEW
	case "report":
		aType = pb.ArtifactType_ARTIFACT_TYPE_REPORT
	case "app":
		aType = pb.ArtifactType_ARTIFACT_TYPE_APP
	default:
		return fmt.Errorf("invalid artifact type: %s (valid: note, audio, report, app)", artifactType)
	}

	req := &pb.CreateArtifactRequest{
		ProjectId: projectID,
		Artifact: &pb.Artifact{
			ProjectId: projectID,
			Type:      aType,
			State:     pb.ArtifactState_ARTIFACT_STATE_CREATING,
		},
	}

	fmt.Fprintf(os.Stderr, "Creating %s artifact in project %s...\n", artifactType, projectID)
	artifact, err := orchClient.CreateArtifact(context.Background(), req)
	if err != nil {
		return fmt.Errorf("create artifact: %w", err)
	}

	fmt.Printf("✅ Created artifact: %s\n", artifact.ArtifactId)
	fmt.Printf("  Type: %s\n", artifact.Type.String())
	fmt.Printf("  State: %s\n", artifact.State.String())

	return nil
}

func getArtifact(c *api.Client, artifactID string) error {
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	req := &pb.GetArtifactRequest{
		ArtifactId: artifactID,
	}

	artifact, err := orchClient.GetArtifact(context.Background(), req)
	if err != nil {
		return fmt.Errorf("get artifact: %w", err)
	}

	fmt.Printf("Artifact Details:\n")
	fmt.Printf("  ID: %s\n", artifact.ArtifactId)
	fmt.Printf("  Project: %s\n", artifact.ProjectId)
	fmt.Printf("  Type: %s\n", artifact.Type.String())
	fmt.Printf("  State: %s\n", artifact.State.String())

	if len(artifact.Sources) > 0 {
		fmt.Printf("  Sources (%d):\n", len(artifact.Sources))
		for _, src := range artifact.Sources {
			fmt.Printf("    - %s\n", src.SourceId.GetSourceId())
		}
	}

	return nil
}

func listArtifacts(c *api.Client, projectID string) error {
	// The orchestration service returns 400 Bad Request for list-artifacts
	// Use direct RPC instead
	if debug {
		fmt.Fprintf(os.Stderr, "Using direct RPC for list-artifacts\n")
	}

	artifacts, err := c.ListArtifacts(projectID)
	if err != nil {
		return fmt.Errorf("list artifacts: %w", err)
	}

	return displayArtifacts(artifacts)
}

// listArtifactsDirectRPC uses direct RPC to list artifacts
func listArtifactsDirectRPC(c *api.Client, projectID string) ([]*pb.Artifact, error) {
	// Use the client's RPC capabilities
	return c.ListArtifacts(projectID)
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
	fmt.Printf("Renaming artifact %s to '%s'...\n", artifactID, newTitle)

	artifact, err := c.RenameArtifact(artifactID, newTitle)
	if err != nil {
		return fmt.Errorf("rename artifact: %w", err)
	}

	fmt.Printf("✅ Artifact renamed successfully\n")
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

	fmt.Printf("✅ Deleted artifact: %s\n", artifactID)
	return nil
}

// Generation operations
func generateFreeFormChat(c *api.Client, projectID, prompt string) error {
	fmt.Fprintf(os.Stderr, "Generating response for: %s\n", prompt)

	// Use the API client's GenerateFreeFormStreamed method
	response, err := c.GenerateFreeFormStreamed(projectID, prompt, nil)
	if err != nil {
		return fmt.Errorf("generate chat: %w", err)
	}

	// Display the response
	if response != nil && response.Chunk != "" {
		fmt.Println(response.Chunk)
	} else {
		fmt.Println("(No response received)")
	}

	return nil
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
	// GetProject response has chatbot config at wire position [7] which isn't
	// in the proto. Use raw RPC to extract it.
	rpcClient := rpc.New(authToken, cookies)
	resp, err := rpcClient.Do(rpc.Call{
		ID:         rpc.RPCGetProject,
		NotebookID: notebookID,
		Args:       []interface{}{notebookID},
	})
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	// ChatbotConfig is at position [7]: [[goal_type, "prompt"], [length_config]]
	if len(data) <= 7 || data[7] == nil {
		fmt.Println("No custom instructions set.")
		return nil
	}

	config, ok := data[7].([]interface{})
	if !ok || len(config) == 0 {
		fmt.Println("No custom instructions set.")
		return nil
	}

	// Goal config is config[0]: [goal_type, "prompt"] or [goal_type]
	goalConfig, ok := config[0].([]interface{})
	if !ok || len(goalConfig) < 2 {
		fmt.Println("No custom instructions set.")
		return nil
	}

	prompt, ok := goalConfig[1].(string)
	if !ok || prompt == "" {
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
	call := rpc.Call{
		ID: "QDyure", // ShareProject RPC ID
		Args: []interface{}{
			[]interface{}{[]interface{}{notebookID, nil, []interface{}{true}}}, // field 1: [YM{projId, null, Uzb{true}}]
			true,              // field 2: M3 flag
			nil,               // field 3: gap
			[]interface{}{2},  // field 4: ProjectContext
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

	if len(data) > 0 {
		if shareData, ok := data[0].([]interface{}); ok && len(shareData) > 0 {
			if shareURL, ok := shareData[0].(string); ok {
				fmt.Printf("Share URL: %s\n", shareURL)
				return nil
			}
		}
	}

	fmt.Printf("Project shared successfully (URL format not recognized)\n")
	return nil
}

func submitFeedback(c *api.Client, message string) error {
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)

	req := &pb.SubmitFeedbackRequest{
		FeedbackType: "general",
		FeedbackText: message,
	}

	_, err := orchClient.SubmitFeedback(context.Background(), req)
	if err != nil {
		return fmt.Errorf("submit feedback: %w", err)
	}

	fmt.Printf("✅ Feedback submitted\n")
	return nil
}

func shareNotebookPrivate(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating private share link...\n")

	// Create RPC client directly for sharing project
	rpcClient := rpc.New(authToken, cookies)
	// Wire format from JS analysis (mAb function):
	//   field 1: repeated YM [{field 1: projectId, field 3: Uzb{field 1: false} (link sharing off)}]
	//   field 2: bool (M3 flag)
	//   field 4: ProjectContext [2]
	call := rpc.Call{
		ID: "QDyure", // ShareProject RPC ID
		Args: []interface{}{
			[]interface{}{[]interface{}{notebookID, nil, []interface{}{false}}}, // field 1: [YM{projId, null, Uzb{false}}]
			true,              // field 2: M3 flag
			nil,               // field 3: gap
			[]interface{}{2},  // field 4: ProjectContext
		},
	}

	resp, err := rpcClient.Do(call)
	if err != nil {
		return fmt.Errorf("share project privately: %w", err)
	}

	// Parse response to extract share URL
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if len(data) > 0 {
		if shareData, ok := data[0].([]interface{}); ok && len(shareData) > 0 {
			if shareURL, ok := shareData[0].(string); ok {
				fmt.Printf("Private Share URL: %s\n", shareURL)
				return nil
			}
		}
	}

	fmt.Printf("Project shared privately (URL format not recognized)\n")
	return nil
}

func getShareDetails(c *api.Client, shareID string) error {
	fmt.Fprintf(os.Stderr, "Getting share details...\n")

	// Create RPC client directly for getting project details
	rpcClient := rpc.New(authToken, cookies)
	call := rpc.Call{
		ID:   "JFMDGd", // GetProjectDetails RPC ID
		Args: []interface{}{shareID},
	}

	resp, err := rpcClient.Do(call)
	if err != nil {
		return fmt.Errorf("get project details: %w", err)
	}

	// Parse response to extract project details
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	// Display project details in a readable format
	fmt.Printf("Share Details:\n")
	fmt.Printf("Share ID: %s\n", shareID)

	if len(data) > 0 {
		// Try to parse the project details from the response
		// The exact format depends on the API response structure
		fmt.Printf("Details: %v\n", data)
	} else {
		fmt.Printf("No details available for this share ID\n")
	}

	return nil
}

// Chat helper functions
func getChatSessionPath(notebookID string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), fmt.Sprintf("nlm-chat-%s.json", notebookID))
	}

	nlmDir := filepath.Join(homeDir, ".nlm")
	os.MkdirAll(nlmDir, 0700) // Ensure directory exists
	return filepath.Join(nlmDir, fmt.Sprintf("chat-%s.json", notebookID))
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	nlmDir := filepath.Join(homeDir, ".nlm")
	entries, err := os.ReadDir(nlmDir)
	if err != nil {
		fmt.Println("No chat sessions found.")
		return nil
	}

	var sessions []ChatSession
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "chat-") && strings.HasSuffix(entry.Name(), ".json") {
			sessionPath := filepath.Join(nlmDir, entry.Name())
			data, err := os.ReadFile(sessionPath)
			if err != nil {
				continue
			}

			var session ChatSession
			if err := json.Unmarshal(data, &session); err != nil {
				continue
			}

			sessions = append(sessions, session)
		}
	}

	if len(sessions) == 0 {
		fmt.Println("No chat sessions found.")
		return nil
	}

	fmt.Printf("📚 Chat Sessions (%d total)\n", len(sessions))
	fmt.Println("=" + strings.Repeat("=", 40))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NOTEBOOK\tMESSAGES\tLAST UPDATED\tCREATED")
	fmt.Fprintln(w, "--------\t--------\t------------\t-------")

	for _, session := range sessions {
		lastUpdated := session.UpdatedAt.Format("Jan 2 15:04")
		created := session.CreatedAt.Format("Jan 2 15:04")
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
			session.NotebookID,
			len(session.Messages),
			lastUpdated,
			created)
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
		return "I'm having trouble connecting to the chat service right now. You might want to try using specific commands like 'nlm generate-guide " + notebookID + "' or 'nlm generate-outline " + notebookID + "' for detailed content analysis."
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

// Interactive chat interface with history and streaming support
func interactiveChat(c *api.Client, notebookID string) error {
	// Load or create chat session
	session, err := loadChatSession(notebookID)
	if err != nil {
		// Create new session if loading fails
		session = &ChatSession{
			NotebookID:     notebookID,
			ConversationID: uuid.New().String(),
			Messages:       []ChatMessage{},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}
	// Ensure conversation ID exists (for sessions loaded from older format)
	if session.ConversationID == "" {
		session.ConversationID = uuid.New().String()
	}

	// Display welcome message
	fmt.Println("\n📚 NotebookLM Interactive Chat")
	fmt.Println("================================")
	fmt.Printf("Notebook: %s\n", notebookID)

	if len(session.Messages) > 0 {
		fmt.Printf("Chat history: %d messages (started %s)\n",
			len(session.Messages),
			session.CreatedAt.Format("Jan 2 15:04"))
	}

	fmt.Println("\nCommands:")
	fmt.Println("  /exit or /quit - Exit chat")
	fmt.Println("  /clear - Clear screen")
	fmt.Println("  /history - Show recent chat history")
	fmt.Println("  /reset - Clear chat history")
	fmt.Println("  /save - Save current session")
	fmt.Println("  /help - Show this help")
	fmt.Println("  /multiline - Toggle multiline mode (end with empty line)")
	fmt.Println("\nType your message and press Enter to send.")

	scanner := bufio.NewScanner(os.Stdin)
	multiline := false

	// Show recent history if it exists
	if len(session.Messages) > 0 {
		fmt.Println("\n--- Recent Chat History ---")
		showRecentHistory(session, 3)
		fmt.Println("---------------------------")
	}

	for {
		// Show prompt with context indicator
		historyCount := len(session.Messages)
		if multiline {
			fmt.Printf("📝 [%d msgs] (multiline, empty line to send) > ", historyCount)
		} else {
			fmt.Printf("💬 [%d msgs] > ", historyCount)
		}

		// Read input
		var input string
		if multiline {
			var lines []string
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					break // Empty line ends multiline input
				}
				lines = append(lines, line)
				fmt.Print("... > ")
			}
			input = strings.Join(lines, "\n")
		} else {
			if !scanner.Scan() {
				break // EOF or error
			}
			input = scanner.Text()
		}

		// Handle special commands
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch strings.ToLower(input) {
		case "/exit", "/quit":
			fmt.Println("\n👋 Saving session and goodbye!")
			if err := saveChatSession(session); err != nil {
				fmt.Printf("Warning: Failed to save session: %v\n", err)
			}
			return nil
		case "/clear":
			// Clear screen (works on most terminals)
			fmt.Print("\033[H\033[2J")
			fmt.Println("📚 NotebookLM Interactive Chat")
			fmt.Println("================================")
			fmt.Printf("Notebook: %s\n", notebookID)
			fmt.Printf("Chat history: %d messages\n\n", len(session.Messages))
			continue
		case "/history":
			fmt.Println("\n--- Chat History ---")
			showRecentHistory(session, 10)
			fmt.Println("-------------------")
			continue
		case "/reset":
			if confirmAction("Are you sure you want to clear chat history?") {
				session.Messages = []ChatMessage{}
				session.UpdatedAt = time.Now()
				fmt.Println("Chat history cleared.")
			}
			continue
		case "/save":
			if err := saveChatSession(session); err != nil {
				fmt.Printf("Error saving session: %v\n", err)
			} else {
				fmt.Println("Session saved successfully.")
			}
			continue
		case "/help":
			fmt.Println("\nCommands:")
			fmt.Println("  /exit or /quit - Exit chat")
			fmt.Println("  /clear - Clear screen")
			fmt.Println("  /history - Show recent chat history")
			fmt.Println("  /reset - Clear chat history")
			fmt.Println("  /save - Save current session")
			fmt.Println("  /help - Show this help")
			fmt.Println("  /multiline - Toggle multiline mode")
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

		// Add user message to local history
		userMsg := ChatMessage{
			Role:      "user",
			Content:   input,
			Timestamp: time.Now(),
		}
		session.Messages = append(session.Messages, userMsg)

		fmt.Println("\n🤔 Thinking...")

		// Build wire history from session (newest first, as the protocol expects)
		wireHistory := buildWireHistory(session)

		// Send chat request with full history
		chatReq := api.ChatRequest{
			ProjectID:      notebookID,
			Prompt:         input,
			ConversationID: session.ConversationID,
			History:        wireHistory,
			SeqNum:         len(session.Messages)/2 + 1,
		}

		fmt.Print("\n🤖 Assistant: ")
		var fullResponse strings.Builder
		err := c.GenerateFreeFormStreamedWithCallback(notebookID, input, nil, func(chunk string) bool {
			fmt.Print(chunk)
			fullResponse.WriteString(chunk)
			return true
		})

		// Fallback: use ChatWithHistory for non-streaming
		if err != nil {
			response, chatErr := c.ChatWithHistory(chatReq)
			if chatErr != nil {
				fmt.Printf("\n⚠️ Chat API error: %v\n", err)
				fallbackResponse := getFallbackResponse(input, notebookID)
				fmt.Printf("🤖 Assistant: %s\n", fallbackResponse)
				session.Messages = append(session.Messages, ChatMessage{
					Role: "assistant", Content: fallbackResponse, Timestamp: time.Now(),
				})
			} else {
				fmt.Print(response)
				session.Messages = append(session.Messages, ChatMessage{
					Role: "assistant", Content: response, Timestamp: time.Now(),
				})
			}
		} else {
			response := strings.TrimSpace(fullResponse.String())
			if response != "" {
				session.Messages = append(session.Messages, ChatMessage{
					Role: "assistant", Content: response, Timestamp: time.Now(),
				})
			}
		}
		fmt.Println()
		_ = chatReq // used in fallback path

		// Update session timestamp
		session.UpdatedAt = time.Now()

		// Auto-save every few messages
		if len(session.Messages)%6 == 0 { // Save every 3 exchanges
			if err := saveChatSession(session); err != nil && debug {
				fmt.Printf("Debug: Auto-save failed: %v\n", err)
			}
		}

		fmt.Println() // Add a blank line for readability
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	// Save session before exiting
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
	fmt.Printf("Creating video overview for notebook %s...\n", projectID)
	fmt.Printf("Instructions: %s\n", instructions)

	result, err := c.CreateVideoOverview(projectID, instructions)
	if err != nil {
		return fmt.Errorf("create video overview: %w", err)
	}

	if !result.IsReady {
		fmt.Println("✅ Video overview creation started. Video generation may take several minutes.")
		fmt.Printf("  Project ID: %s\n", result.ProjectID)
		return nil
	}

	// If the result is immediately ready (unlikely but possible)
	fmt.Printf("✅ Video Overview created:\n")
	fmt.Printf("  Title: %s\n", result.Title)
	fmt.Printf("  Video ID: %s\n", result.VideoID)

	if result.VideoData != "" {
		fmt.Printf("  Video URL: %s\n", result.VideoData)
	}

	return nil
}

func listAudioOverviews(c *api.Client, notebookID string) error {
	fmt.Printf("Listing audio overviews for notebook %s...\n", notebookID)

	audioOverviews, err := c.ListAudioOverviews(notebookID)
	if err != nil {
		return fmt.Errorf("list audio overviews: %w", err)
	}

	if len(audioOverviews) == 0 {
		fmt.Println("No audio overviews found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tTITLE\tSTATUS")
	for _, audio := range audioOverviews {
		status := "pending"
		if audio.IsReady {
			status = "ready"
		}
		title := audio.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			audio.ProjectID,
			title,
			status,
		)
	}
	return w.Flush()
}

func listVideoOverviews(c *api.Client, notebookID string) error {
	fmt.Printf("Listing video overviews for notebook %s...\n", notebookID)

	videoOverviews, err := c.ListVideoOverviews(notebookID)
	if err != nil {
		return fmt.Errorf("list video overviews: %w", err)
	}

	if len(videoOverviews) == 0 {
		fmt.Println("No video overviews found.")
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
	fmt.Printf("Downloading audio overview for notebook %s...\n", notebookID)

	// Generate default filename if not provided
	if filename == "" {
		filename = fmt.Sprintf("audio_overview_%s.wav", notebookID)
	}

	// Download the audio
	audioResult, err := c.DownloadAudioOverview(notebookID)
	if err != nil {
		return fmt.Errorf("download audio overview: %w", err)
	}

	// Save to file
	if err := audioResult.SaveAudioToFile(filename); err != nil {
		return fmt.Errorf("save audio file: %w", err)
	}

	fmt.Printf("✅ Audio saved to: %s\n", filename)

	// Show file info
	if stat, err := os.Stat(filename); err == nil {
		fmt.Printf("  File size: %.2f MB\n", float64(stat.Size())/(1024*1024))
	}

	return nil
}

func downloadVideoOverview(c *api.Client, notebookID string, filename string) error {
	fmt.Printf("Downloading video overview for notebook %s...\n", notebookID)

	// Generate default filename if not provided
	if filename == "" {
		filename = fmt.Sprintf("video_overview_%s.mp4", notebookID)
	}

	// Download the video
	videoResult, err := c.DownloadVideoOverview(notebookID)
	if err != nil {
		return fmt.Errorf("download video overview: %w", err)
	}

	// Check if we got a video URL
	if videoResult.VideoData != "" && (strings.HasPrefix(videoResult.VideoData, "http://") || strings.HasPrefix(videoResult.VideoData, "https://")) {
		// Use authenticated download for URLs
		if err := c.DownloadVideoWithAuth(videoResult.VideoData, filename); err != nil {
			return fmt.Errorf("download video with auth: %w", err)
		}
	} else {
		// Try to save base64 data or handle other formats
		if err := videoResult.SaveVideoToFile(filename); err != nil {
			return fmt.Errorf("save video file: %w", err)
		}
	}

	fmt.Printf("✅ Video saved to: %s\n", filename)

	// Show file info
	if stat, err := os.Stat(filename); err == nil {
		fmt.Printf("  File size: %.2f MB\n", float64(stat.Size())/(1024*1024))
	}

	return nil
}
