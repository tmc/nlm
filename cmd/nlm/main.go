package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/api"
	"github.com/tmc/nlm/internal/batchexecute"
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

	// change this so flag usage doesn't print these values..
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
		fmt.Fprintf(os.Stderr, "  add <id> <input>  Add source to notebook\n")
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

func runCmd(client *api.Client, cmd string, args ...string) error {
	var err error
	switch cmd {
	// Notebook operations
	case "list", "ls":
		err = list(client)
	case "create":
		if len(args) != 1 {
			log.Fatal("usage: nlm create <title>")
		}
		err = create(client, args[0])
	case "rm":
		if len(args) != 1 {
			log.Fatal("usage: nlm rm <id>")
		}
		err = remove(client, args[0])

	// Source operations
	case "sources":
		if len(args) != 1 {
			log.Fatal("usage: nlm sources <notebook-id>")
		}
		err = listSources(client, args[0])
	case "add":
		if len(args) != 2 {
			log.Fatal("usage: nlm add <notebook-id> <file>")
		}
		var id string
		id, err = addSource(client, args[0], args[1])
		fmt.Println(id)
	case "rm-source":
		if len(args) != 2 {
			log.Fatal("usage: nlm rm-source <notebook-id> <source-id>")
		}
		err = removeSource(client, args[0], args[1])
	case "rename-source":
		if len(args) != 2 {
			log.Fatal("usage: nlm rename-source <source-id> <new-name>")
		}
		err = renameSource(client, args[0], args[1])

	// Note operations
	case "new-note":
		if len(args) != 2 {
			log.Fatal("usage: nlm new-note <notebook-id> <title>")
		}
		err = createNote(client, args[0], args[1])
	case "update-note":
		if len(args) != 4 {
			log.Fatal("usage: nlm update-note <notebook-id> <note-id> <content> <title>")
		}
		err = updateNote(client, args[0], args[1], args[2], args[3])
	case "rm-note":
		if len(args) != 1 {
			log.Fatal("usage: nlm rm-note <notebook-id> <note-id>")
		}
		err = removeNote(client, args[0], args[1])

		// Audio operations
	case "audio-create":
		if len(args) != 2 {
			log.Fatal("usage: nlm audio-create <notebook-id> <instructions>")
		}
		err = createAudioOverview(client, args[0], args[1])
	case "audio-get":
		if len(args) != 1 {
			log.Fatal("usage: nlm audio-get <notebook-id>")
		}
		err = getAudioOverview(client, args[0])
	case "audio-rm":
		if len(args) != 1 {
			log.Fatal("usage: nlm audio-rm <notebook-id>")
		}
		err = deleteAudioOverview(client, args[0])
	case "audio-share":
		if len(args) != 1 {
			log.Fatal("usage: nlm audio-share <notebook-id>")
		}
		err = shareAudioOverview(client, args[0])

		// Generation operations
	case "generate-guide":
		if len(args) != 1 {
			log.Fatal("usage: nlm generate-guide <notebook-id>")
		}
		err = generateNotebookGuide(client, args[0])
	case "generate-outline":
		if len(args) != 1 {
			log.Fatal("usage: nlm generate-outline <notebook-id>")
		}
		err = generateOutline(client, args[0])
	case "generate-section":
		if len(args) != 1 {
			log.Fatal("usage: nlm generate-section <notebook-id>")
		}
		err = generateSection(client, args[0])

	// Other operations
	// case "analytics":
	// 	if len(args) != 1 {
	// 		log.Fatal("usage: nlm analytics <notebook-id>")
	// 	}
	// 	err = getAnalytics(client, args[0])
	// case "share":
	// 	if len(args) != 1 {
	// 		log.Fatal("usage: nlm share <notebook-id>")
	// 	}
	// 	err = shareNotebook(client, args[0])
	// case "feedback":
	// 	if len(args) != 1 {
	// 		log.Fatal("usage: nlm feedback <message>")
	// 	}
	// 	err = submitFeedback(client, args[0])
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
	fmt.Printf("Created notebook %v\n", notebook)
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

// Source operations
func refreshSource(c *api.Client, sourceID string) error {
	fmt.Fprintf(os.Stderr, "Refreshing source %s...\n", sourceID)
	source, err := c.RefreshSource(sourceID)
	if err != nil {
		return fmt.Errorf("refresh source: %w", err)
	}
	fmt.Printf("✅ Refreshed source: %s\n", source.Title)
	return nil
}

// func checkSourceFreshness(c *api.Client, sourceID string) error {
// 	fmt.Fprintf(os.Stderr, "Checking source %s...\n", sourceID)
// 	resp, err := c.CheckSourceFreshness(sourceID)
// 	if err != nil {
// 		return fmt.Errorf("check source: %w", err)
// 	}
// 	if resp.NeedsRefresh {
// 		fmt.Printf("Source needs refresh (last updated: %s)\n", resp.LastUpdateTime.AsTime().Format(time.RFC3339))
// 	} else {
// 		fmt.Printf("Source is up to date (last updated: %s)\n", resp.LastUpdateTime.AsTime().Format(time.RFC3339))
// 	}
// 	return nil
// }

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
