package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/gen/service"
	"github.com/tmc/nlm/internal/api"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/rpc"
)

// Global flags
var (
	authToken     string
	cookies       string
	debug         bool
	chromeProfile string
	mimeType      string
)

func init() {
	flag.BoolVar(&debug, "debug", false, "enable debug output")
	flag.StringVar(&chromeProfile, "profile", os.Getenv("NLM_BROWSER_PROFILE"), "Chrome profile to use")
	flag.StringVar(&authToken, "auth", os.Getenv("NLM_AUTH_TOKEN"), "auth token (or set NLM_AUTH_TOKEN)")
	flag.StringVar(&cookies, "cookies", os.Getenv("NLM_COOKIES"), "cookies for authentication (or set NLM_COOKIES)")
	flag.StringVar(&mimeType, "mime", "", "specify MIME type for content (e.g. 'text/xml', 'application/json')")

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
		fmt.Fprintf(os.Stderr, "  new-note <id> <title>  Create new note\n")
		fmt.Fprintf(os.Stderr, "  edit-note <id> <note-id> <content>  Edit note\n")
		fmt.Fprintf(os.Stderr, "  rm-note <note-id>  Remove note\n\n")

		fmt.Fprintf(os.Stderr, "Audio Commands:\n")
		fmt.Fprintf(os.Stderr, "  audio-create <id> <instructions>  Create audio overview\n")
		fmt.Fprintf(os.Stderr, "  audio-get <id>    Get audio overview\n")
		fmt.Fprintf(os.Stderr, "  audio-rm <id>     Delete audio overview\n")
		fmt.Fprintf(os.Stderr, "  audio-share <id>  Share audio overview\n\n")

		fmt.Fprintf(os.Stderr, "Artifact Commands:\n")
		fmt.Fprintf(os.Stderr, "  create-artifact <id> <type>  Create artifact (note|audio|report|app)\n")
		fmt.Fprintf(os.Stderr, "  get-artifact <artifact-id>  Get artifact details\n")
		fmt.Fprintf(os.Stderr, "  list-artifacts <id>  List artifacts in notebook\n")
		fmt.Fprintf(os.Stderr, "  delete-artifact <artifact-id>  Delete artifact\n\n")

		fmt.Fprintf(os.Stderr, "Generation Commands:\n")
		fmt.Fprintf(os.Stderr, "  generate-guide <id>  Generate notebook guide\n")
		fmt.Fprintf(os.Stderr, "  generate-outline <id>  Generate content outline\n")
		fmt.Fprintf(os.Stderr, "  generate-section <id>  Generate new section\n")
		fmt.Fprintf(os.Stderr, "  generate-chat <id> <prompt>  Free-form chat generation\n\n")

		fmt.Fprintf(os.Stderr, "Sharing Commands:\n")
		fmt.Fprintf(os.Stderr, "  share <id>        Share notebook publicly\n")
		fmt.Fprintf(os.Stderr, "  share-private <id>  Share notebook privately\n")
		fmt.Fprintf(os.Stderr, "  share-details <share-id>  Get details of shared project\n\n")

		fmt.Fprintf(os.Stderr, "Other Commands:\n")
		fmt.Fprintf(os.Stderr, "  auth [profile]    Setup authentication\n")
		fmt.Fprintf(os.Stderr, "  feedback <msg>    Submit feedback\n")
		fmt.Fprintf(os.Stderr, "  hb                Send heartbeat\n\n")
	}
}

func main() {
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
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm new-note <notebook-id> <title>\n")
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
	case "generate-chat":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: nlm generate-chat <notebook-id> <prompt>\n")
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
	case "list-artifacts":
		if len(args) != 1 {
			fmt.Fprintf(os.Stderr, "usage: nlm list-artifacts <notebook-id>\n")
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
		"audio-create", "audio-get", "audio-rm", "audio-share",
		"create-artifact", "get-artifact", "list-artifacts", "delete-artifact",
		"generate-guide", "generate-outline", "generate-section", "generate-chat",
		"auth", "hb", "share", "share-private", "share-details", "feedback",
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
	return true
}

func run() error {
	loadStoredEnv()

	if authToken == "" {
		authToken = os.Getenv("NLM_AUTH_TOKEN")
	}
	if cookies == "" {
		cookies = os.Getenv("NLM_COOKIES")
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

	var opts []batchexecute.Option
	
	// Support HTTP recording for testing
	if recordingDir := os.Getenv("HTTPRR_RECORDING_DIR"); recordingDir != "" {
		// In recording mode, we would set up HTTP client options
		// This requires integration with httprr library
		if debug {
			fmt.Fprintf(os.Stderr, "DEBUG: HTTP recording enabled with directory: %s\n", recordingDir)
		}
	}
	
	for i := 0; i < 3; i++ {
		if i > 1 {
			fmt.Fprintln(os.Stderr, "nlm: attempting again to obtain login information")
			debug = true
		}

		if err := runCmd(api.New(authToken, cookies, opts...), cmd, args...); err == nil {
			return nil
		} else if !errors.Is(err, batchexecute.ErrUnauthorized) {
			return err
		}

		var err error
		if authToken, cookies, err = handleAuth(nil, debug); err != nil {
			fmt.Fprintf(os.Stderr, "  -> %v\n", err)
		}
	}
	return fmt.Errorf("nlm: failed after 3 attempts")
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
		err = createNote(client, args[0], args[1])
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

	// Artifact operations
	case "create-artifact":
		err = createArtifact(client, args[0], args[1])
	case "get-artifact":
		err = getArtifact(client, args[0])
	case "list-artifacts":
		err = listArtifacts(client, args[0])
	case "delete-artifact":
		err = deleteArtifact(client, args[0])

		// Generation operations
	case "generate-guide":
		err = generateNotebookGuide(client, args[0])
	case "generate-outline":
		err = generateOutline(client, args[0])
	case "generate-section":
		err = generateSection(client, args[0])
	case "generate-chat":
		err = generateFreeFormChat(client, args[0], args[1])

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
	case "auth":
		_, _, err = handleAuth(args, debug)

	case "hb":
		err = heartbeat(client)
	default:
		flag.Usage()
		os.Exit(1)
	}

	return err
}

// Notebook operations
func list(c *api.Client) error {
	notebooks, err := c.ListRecentlyViewedProjects()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tLAST UPDATED")
	for _, nb := range notebooks {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			nb.ProjectId, strings.TrimSpace(nb.Emoji)+" "+nb.Title,
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
	fmt.Printf("Are you sure you want to delete notebook %s? [y/N] ", id)
	var response string
	fmt.Scanln(&response)
	if !strings.HasPrefix(strings.ToLower(response), "y") {
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

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tTYPE\tSTATUS\tLAST UPDATED")
	for _, src := range p.Sources {
		status := "enabled"
		if src.Settings != nil {
			status = src.Settings.Status.String()
		}

		lastUpdated := "unknown"
		if src.Metadata != nil && src.Metadata.LastModifiedTime != nil {
			lastUpdated = src.Metadata.LastModifiedTime.AsTime().Format(time.RFC3339)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			src.SourceId.GetSourceId(),
			strings.TrimSpace(src.Title),
			src.Metadata.GetSourceType(),
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
		if mimeType != "" {
			fmt.Fprintf(os.Stderr, "Using specified MIME type: %s\n", mimeType)
			return c.AddSourceFromReader(notebookID, os.Stdin, "Pasted Text", mimeType)
		}
		return c.AddSourceFromReader(notebookID, os.Stdin, "Pasted Text")
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
		if mimeType != "" {
			fmt.Fprintf(os.Stderr, "Using specified MIME type: %s\n", mimeType)
			// Read the file and use AddSourceFromReader with the specified MIME type
			file, err := os.Open(input)
			if err != nil {
				return "", fmt.Errorf("open file: %w", err)
			}
			defer file.Close()
			return c.AddSourceFromReader(notebookID, file, filepath.Base(input), mimeType)
		}
		return c.AddSourceFromFile(notebookID, input)
	}

	// If it's not a URL or file, treat as direct text content
	fmt.Println("Adding text content as source...")
	return c.AddSourceFromText(notebookID, input, "Text Source")
}

func removeSource(c *api.Client, notebookID, sourceID string) error {
	fmt.Printf("Are you sure you want to remove source %s? [y/N] ", sourceID)
	var response string
	fmt.Scanln(&response)
	if !strings.HasPrefix(strings.ToLower(response), "y") {
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
func createNote(c *api.Client, notebookID, title string) error {
	fmt.Printf("Creating note in notebook %s...\n", notebookID)
	if _, err := c.CreateNote(notebookID, title, ""); err != nil {
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
	fmt.Printf("Are you sure you want to remove note %s? [y/N] ", noteID)
	var response string
	fmt.Scanln(&response)
	if !strings.HasPrefix(strings.ToLower(response), "y") {
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

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
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

func editNote(c *api.Client, notebookID, noteID, content string) error {
	fmt.Fprintf(os.Stderr, "Updating note %s...\n", noteID)
	note, err := c.MutateNote(notebookID, noteID, content, "") // Empty title means keep existing
	if err != nil {
		return fmt.Errorf("update note: %w", err)
	}
	fmt.Printf("✅ Updated note: %s\n", note.Title)
	return nil
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
	fmt.Printf("Are you sure you want to delete the audio overview? [y/N] ")
	var response string
	fmt.Scanln(&response)
	if !strings.HasPrefix(strings.ToLower(response), "y") {
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
	
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
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
	
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
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
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)
	
	req := &pb.ListArtifactsRequest{
		ProjectId: projectID,
		PageSize:  50,
	}
	
	resp, err := orchClient.ListArtifacts(context.Background(), req)
	if err != nil {
		return fmt.Errorf("list artifacts: %w", err)
	}
	
	if len(resp.Artifacts) == 0 {
		fmt.Println("No artifacts found in project.")
		return nil
	}
	
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tSTATE\tSOURCES")
	
	for _, artifact := range resp.Artifacts {
		sourceCount := fmt.Sprintf("%d", len(artifact.Sources))
		
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			artifact.ArtifactId,
			artifact.Type.String(),
			artifact.State.String(),
			sourceCount)
	}
	return w.Flush()
}

func deleteArtifact(c *api.Client, artifactID string) error {
	fmt.Printf("Are you sure you want to delete artifact %s? [y/N] ", artifactID)
	var response string
	fmt.Scanln(&response)
	if !strings.HasPrefix(strings.ToLower(response), "y") {
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
	// Create orchestration service client
	orchClient := service.NewLabsTailwindOrchestrationServiceClient(authToken, cookies)
	
	req := &pb.GenerateFreeFormStreamedRequest{
		ProjectId: projectID,
		Prompt:    prompt,
	}
	
	fmt.Fprintf(os.Stderr, "Generating response for: %s\n", prompt)
	
	stream, err := orchClient.GenerateFreeFormStreamed(context.Background(), req)
	if err != nil {
		return fmt.Errorf("generate chat: %w", err)
	}
	
	// For now, just return the first response
	// In a full implementation, this would stream the responses
	fmt.Printf("Response: %s\n", "Free-form generation not fully implemented yet")
	_ = stream
	
	return nil
}

// Utility functions for commented-out operations
func shareNotebook(c *api.Client, notebookID string) error {
	fmt.Fprintf(os.Stderr, "Generating public share link...\n")
	
	// Create RPC client directly for sharing project
	rpcClient := rpc.New(authToken, cookies)
	call := rpc.Call{
		ID:   "QDyure", // ShareProject RPC ID
		Args: []interface{}{
			notebookID,
			map[string]interface{}{
				"is_public": true,
				"allow_comments": true,
				"allow_downloads": false,
			},
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
	call := rpc.Call{
		ID:   "QDyure", // ShareProject RPC ID
		Args: []interface{}{
			notebookID,
			map[string]interface{}{
				"is_public": false,
				"allow_comments": false,
				"allow_downloads": false,
			},
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
