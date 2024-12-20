package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/api"
	"github.com/tmc/nlm/internal/auth"
	"github.com/tmc/nlm/internal/batchexecute"
	"golang.org/x/term"
)

// Global flags
var (
	authToken string
	cookies   string
	debug     bool
)

func main() {
	log.SetPrefix("nlm: ")
	log.SetFlags(0)

	flag.StringVar(&authToken, "auth", os.Getenv("NLM_AUTH_TOKEN"), "auth token (or set NLM_AUTH_TOKEN)")
	flag.StringVar(&cookies, "cookies", os.Getenv("NLM_COOKIES"), "cookies for authentication (or set NLM_COOKIES)")
	flag.BoolVar(&debug, "debug", false, "enable debug output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: nlm <command> [arguments]\n\n")
		fmt.Fprintf(os.Stderr, "Notebook Commands:\n")
		fmt.Fprintf(os.Stderr, "  list, ls          List all notebooks\n")
		fmt.Fprintf(os.Stderr, "  create <title>    Create a new notebook\n")
		fmt.Fprintf(os.Stderr, "  rm <id>           Delete a notebook\n")
		fmt.Fprintf(os.Stderr, "  analytics <id>    Show notebook analytics\n\n")

		fmt.Fprintf(os.Stderr, "Source Commands:\n")
		fmt.Fprintf(os.Stderr, "  sources <id>      List sources in notebook\n")
		fmt.Fprintf(os.Stderr, "  add [-filename name] [-base64] <id> <input>  Add source to notebook\n")
		fmt.Fprintf(os.Stderr, "  rm-source <id> <source-id>  Remove source\n")
		fmt.Fprintf(os.Stderr, "  rename-source <source-id> <new-name>  Rename source\n")
		fmt.Fprintf(os.Stderr, "  refresh-source <source-id>  Refresh source content\n")
		fmt.Fprintf(os.Stderr, "  check-source <source-id>  Check source freshness\n\n")

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

		fmt.Fprintf(os.Stderr, "Generation Commands:\n")
		fmt.Fprintf(os.Stderr, "  generate-guide <id>  Generate notebook guide\n")
		fmt.Fprintf(os.Stderr, "  generate-outline <id>  Generate content outline\n")
		fmt.Fprintf(os.Stderr, "  generate-section <id>  Generate new section\n\n")

		fmt.Fprintf(os.Stderr, "Other Commands:\n")
		fmt.Fprintf(os.Stderr, "  auth [profile]    Setup authentication\n")
		fmt.Fprintf(os.Stderr, "  share <id>        Share notebook\n")
		fmt.Fprintf(os.Stderr, "  feedback <msg>    Submit feedback\n")
		fmt.Fprintf(os.Stderr, "  hb                Send heartbeat\n\n")
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()
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

	var opts []batchexecute.Option
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

func runCmd(c *api.Client, cmd string, args ...string) error {
	var err error
	switch cmd {
	// Notebook operations
	case "list", "ls":
		err = list(c)
	case "create":
		if len(args) != 1 {
			log.Fatal("usage: nlm create <title>")
		}
		err = create(c, args[0])
	case "rm":
		if len(args) != 1 {
			log.Fatal("usage: nlm rm <id>")
		}
		err = remove(c, args[0])

	// Source operations
	case "sources":
		if len(args) != 1 {
			log.Fatal("usage: nlm sources <notebook-id>")
		}
		err = listSources(c, args[0])
	case "add":
		addFlags := flag.NewFlagSet("add", flag.ExitOnError)
		addFlags.Usage = func() {
			fmt.Fprintf(os.Stderr, `Usage: nlm add [options] <notebook-id> <input>

Input can be:
  - A file path
  - A URL (http:// or https://)
  - "-" for stdin
  - Direct text content

Options:
  -filename string
        Custom name for the source (default: derived from input)
  -base64
        Force base64 encoding of content
  -content-type string
        Explicit content type (e.g., text/plain, application/pdf)
  -no-content-type
        Don't set a content type
  -auto-content-type
        Automatically detect content type (default)

Content Type Detection:
  - If -content-type is specified, that value is used
  - If -no-content-type is specified, no content type is set
  - Otherwise, content type is automatically detected:
    • text/markdown   - Files starting with "# "
    • text/html      - HTML documents
    • application/json - JSON content
    • application/xml  - XML content
    • Others based on file content

Examples:
  nlm add notebook-id document.txt                    # Add text file
  nlm add -base64 notebook-id image.png              # Add binary file
  nlm add -filename "Report.pdf" notebook-id doc.pdf  # Add with custom name
  nlm add -content-type "text/markdown" notebook-id - # Add markdown from stdin
  cat file.json | nlm add notebook-id -              # Add JSON from stdin
  nlm add notebook-id https://example.com/doc        # Add from URL

`)
		}

		filename := addFlags.String("filename", "", "custom name for the source")
		base64Encode := addFlags.Bool("base64", false, "force base64 encoding of content")
		contentType := addFlags.String("content-type", "", "explicit content type")
		noContentType := addFlags.Bool("no-content-type", false, "don't set a content type")
		autoContentType := addFlags.Bool("auto-content-type", true, "automatically detect content type (default)")

		if err := addFlags.Parse(args); err != nil {
			return err
		}

		if addFlags.NArg() != 2 {
			addFlags.Usage()
			return fmt.Errorf("requires notebook-id and input")
		}

		notebookID := addFlags.Arg(0)
		input := addFlags.Arg(1)

		var sourceOpts []api.SourceOption
		if *filename != "" {
			sourceOpts = append(sourceOpts, api.WithSourceName(*filename))
		}
		if *base64Encode {
			sourceOpts = append(sourceOpts, api.WithBase64Encoding())
		}

		// Handle content type options
		switch {
		case *contentType != "":
			sourceOpts = append(sourceOpts, api.WithContentType(*contentType))
		case *noContentType:
			sourceOpts = append(sourceOpts, api.WithContentTypeNone())
		case *autoContentType:
			sourceOpts = append(sourceOpts, api.WithContentTypeAuto())
		}

		var id string
		id, err = addSource(c, notebookID, input, sourceOpts...)
		if err == nil {
			fmt.Println(id)
		}
	case "rm-source":
		if len(args) != 2 {
			log.Fatal("usage: nlm rm-source <notebook-id> <source-id>")
		}
		err = removeSource(c, args[0], args[1])
	case "rename-source":
		if len(args) != 2 {
			log.Fatal("usage: nlm rename-source <source-id> <new-name>")
		}
		err = renameSource(c, args[0], args[1])

	// Note operations
	case "new-note":
		if len(args) != 2 {
			log.Fatal("usage: nlm new-note <notebook-id> <title>")
		}
		err = createNote(c, args[0], args[1])
	case "update-note":
		if len(args) != 4 {
			log.Fatal("usage: nlm update-note <notebook-id> <note-id> <content> <title>")
		}
		err = updateNote(c, args[0], args[1], args[2], args[3])
	case "rm-note":
		if len(args) != 1 {
			log.Fatal("usage: nlm rm-note <notebook-id> <note-id>")
		}
		err = removeNote(c, args[0], args[1])

	// Audio operations
	case "audio-create":
		if len(args) != 2 {
			log.Fatal("usage: nlm audio-create <notebook-id> <instructions>")
		}
		err = createAudioOverview(c, args[0], args[1])
	case "audio-get":
		if len(args) != 1 {
			log.Fatal("usage: nlm audio-get <notebook-id>")
		}
		err = getAudioOverview(c, args[0])
	case "audio-rm":
		if len(args) != 1 {
			log.Fatal("usage: nlm audio-rm <notebook-id>")
		}
		err = deleteAudioOverview(c, args[0])
	case "audio-share":
		if len(args) != 1 {
			log.Fatal("usage: nlm audio-share <notebook-id>")
		}
		err = shareAudioOverview(c, args[0])

	// Generation operations
	case "generate-guide":
		if len(args) != 1 {
			log.Fatal("usage: nlm generate-guide <notebook-id>")
		}
		err = generateNotebookGuide(c, args[0])
	case "generate-outline":
		if len(args) != 1 {
			log.Fatal("usage: nlm generate-outline <notebook-id>")
		}
		err = generateOutline(c, args[0])
	case "generate-section":
		if len(args) != 1 {
			log.Fatal("usage: nlm generate-section <notebook-id>")
		}
		err = generateSection(c, args[0])

	case "auth":
		_, _, err = handleAuth(args, debug)
	case "hb":
		err = heartbeat(c)
	default:
		flag.Usage()
		os.Exit(1)
	}

	return err
}

func handleAuth(args []string, debug bool) (string, string, error) {
	isTty := term.IsTerminal(int(os.Stdin.Fd()))

	if !isTty {
		// Parse HAR/curl from stdin
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", "", fmt.Errorf("failed to read stdin: %w", err)
		}
		return detectAuthInfo(string(input))
	}

	profileName := "Default"
	if v := os.Getenv("NLM_BROWSER_PROFILE"); v != "" {
		profileName = v
	}
	if len(args) > 0 {
		profileName = args[0]
	}

	a := auth.New(debug)
	fmt.Fprintf(os.Stderr, "nlm: launching browser to login... (profile:%v)  (set with NLM_BROWSER_PROFILE)\n", profileName)
	token, cookies, err := a.GetAuth(auth.WithProfileName(profileName))
	if err != nil {
		return "", "", fmt.Errorf("browser auth failed: %w", err)
	}
	return persistAuthToDisk(cookies, token, profileName)
}

func detectAuthInfo(cmd string) (string, string, error) {
	// Extract cookies
	cookieRe := regexp.MustCompile(`-H ['"]cookie: ([^'"]+)['"]`)
	cookieMatch := cookieRe.FindStringSubmatch(cmd)
	if len(cookieMatch) < 2 {
		return "", "", fmt.Errorf("no cookies found")
	}
	cookies := cookieMatch[1]

	// Extract auth token
	atRe := regexp.MustCompile(`at=([^&\s]+)`)
	atMatch := atRe.FindStringSubmatch(cmd)
	if len(atMatch) < 2 {
		return "", "", fmt.Errorf("no auth token found")
	}
	authToken := atMatch[1]
	persistAuthToDisk(cookies, authToken, "")
	return authToken, cookies, nil
}

func persistAuthToDisk(cookies, authToken, profileName string) (string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("get home dir: %w", err)
	}

	// Create .nlm directory if it doesn't exist
	nlmDir := filepath.Join(homeDir, ".nlm")
	if err := os.MkdirAll(nlmDir, 0700); err != nil {
		return "", "", fmt.Errorf("create .nlm directory: %w", err)
	}

	// Create or update env file
	envFile := filepath.Join(nlmDir, "env")
	content := fmt.Sprintf("NLM_COOKIES=%q\nNLM_AUTH_TOKEN=%q\nNLM_BROWSER_PROFILE=%q\n",
		cookies,
		authToken,
		profileName,
	)

	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		return "", "", fmt.Errorf("write env file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "nlm: auth info written to %s\n", envFile)
	return authToken, cookies, nil
}

func loadStoredEnv() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	data, err := os.ReadFile(filepath.Join(home, ".nlm", "env"))
	if err != nil {
		return
	}

	s := bufio.NewScanner(strings.NewReader(string(data)))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if os.Getenv(key) != "" {
			continue
		}

		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		os.Setenv(key, value)
	}
}

// Modify addSource to use options:
func addSource(c *api.Client, notebookID, input string, opts ...api.SourceOption) (string, error) {
	// Handle special input designators
	switch input {
	case "-": // stdin
		fmt.Fprintln(os.Stderr, "Reading from stdin...")
		return c.AddSourceFromReader(notebookID, os.Stdin, opts...)
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
		return c.AddSourceFromFile(notebookID, input, opts...)
	}

	// If it's not a URL or file, treat as direct text content
	fmt.Println("Adding text content as source...")
	return c.AddSourceFromText(notebookID, input, opts...)
}

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

func refreshSource(c *api.Client, sourceID string) error {
	fmt.Fprintf(os.Stderr, "Refreshing source %s...\n", sourceID)
	source, err := c.RefreshSource(sourceID)
	if err != nil {
		return fmt.Errorf("refresh source: %w", err)
	}
	fmt.Printf("✅ Refreshed source: %s\n", source.Title)
	return nil
}

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

	fmt.Printf("✅ Audio Overview created:\n")
	fmt.Printf("  Title: %s\n", result.Title)
	fmt.Printf("  ID: %s\n", result.AudioID)

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

func heartbeat(c *api.Client) error {
	return nil
}
