package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tmc/nlm/internal/api"
	"github.com/tmc/nlm/internal/batchexecute"
)

func main() {
	log.SetPrefix("nlm: ")
	log.SetFlags(0)

	// Global flags
	var (
		authToken string
		cookies   string
		debug     bool
	)
	flag.StringVar(&authToken, "auth", os.Getenv("NLM_AUTH_TOKEN"), "auth token (or set NLM_AUTH_TOKEN)")
	flag.StringVar(&cookies, "cookies", os.Getenv("NLM_COOKIES"), "cookies for authentication (or set NLM_COOKIES)")
	flag.BoolVar(&debug, "debug", false, "enable debug output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: nlm <command> [arguments]\n\n")
		fmt.Fprintf(os.Stderr, "Notebook Commands:\n")
		fmt.Fprintf(os.Stderr, "  list, ls          List all notebooks\n")
		fmt.Fprintf(os.Stderr, "  create <title>    Create a new notebook\n")
		fmt.Fprintf(os.Stderr, "  rm <id>           Delete a notebook\n\n")
		fmt.Fprintf(os.Stderr, "Source Commands:\n")
		fmt.Fprintf(os.Stderr, "  sources <id>      List sources in a notebook\n")
		fmt.Fprintf(os.Stderr, "  add <id> <input>  Add source to notebook (URL or file)\n")
		fmt.Fprintf(os.Stderr, "  rm-source <id> <source-id>  Remove source from notebook\n")
		fmt.Fprintf(os.Stderr, "  rename-source <source-id> <new-name>  Rename a source\n\n")
		fmt.Fprintf(os.Stderr, "Note Commands:\n")
		fmt.Fprintf(os.Stderr, "  new-note <id> <title>  Create a new note\n")
		fmt.Fprintf(os.Stderr, "  update-note <id> <note-id> <content> <title>  Update a note\n")
		fmt.Fprintf(os.Stderr, "  rm-note <note-id>  Remove a note\n\n")
		fmt.Fprintf(os.Stderr, "Other Commands:\n")
		fmt.Fprintf(os.Stderr, "  auth              Setup authentication\n")
		fmt.Fprintf(os.Stderr, "  audio-overview <id> <instructions>  Create audio overview\n")
		fmt.Fprintf(os.Stderr, "  hb                Send heartbeat\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	opts := []batchexecute.Option{}
	if debug {
		opts = append(opts, batchexecute.WithDebug(true))
	}
	client := api.New(authToken, cookies, opts...)

	cmd := flag.Arg(0)
	args := flag.Args()[1:]

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
		err = addSource(client, args[0], args[1])
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
			log.Fatal("usage: nlm rm-note <note-id>")
		}
		err = removeNote(client, args[0])

	// Other operations
	case "auth":
		err = handleAuth(args, debug)
	case "audio-overview":
		if len(args) != 2 {
			log.Fatal("usage: nlm audio-overview <notebook-id> <instructions>")
		}
		err = createAudioOverview(client, args[0], args[1])
	case "hb":
		err = heartbeat(client)
	default:
		flag.Usage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatal(err)
	}
}

// Notebook operations
func list(c *api.Client) error {
	notebooks, err := c.ListNotebooks()
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
	notebook, err := c.CreateNotebook(title)
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
	return c.DeleteNotebook(id)
}

// Source operations
func listSources(c *api.Client, notebookID string) error {
	sources, err := c.ListSources(notebookID)
	if err != nil {
		return fmt.Errorf("list sources: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tTYPE\tSTATUS\tLAST UPDATED")
	for _, src := range sources {
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

func addSource(c *api.Client, notebookID, input string) error {
	// Handle special input designators
	switch input {
	case "-": // stdin
		fmt.Println("Reading from stdin...")
		return c.AddSourceFromReader(notebookID, os.Stdin, "Pasted Text")
	case "": // empty input
		return fmt.Errorf("input required (file, URL, or '-' for stdin)")
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

	if err := c.RemoveSource(notebookID, sourceID); err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	fmt.Printf("✅ Removed source %s from notebook %s\n", sourceID, notebookID)
	return nil
}

func renameSource(c *api.Client, sourceID, newName string) error {
	fmt.Printf("Renaming source %s to: %s\n", sourceID, newName)

	if err := c.RenameSource(sourceID, newName); err != nil {
		return fmt.Errorf("rename source: %w", err)
	}

	fmt.Printf("✅ Renamed source to: %s\n", newName)
	return nil
}

// Note operations
func createNote(c *api.Client, notebookID, title string) error {
	fmt.Printf("Creating note in notebook %s...\n", notebookID)
	if err := c.CreateNote(notebookID, title); err != nil {
		return fmt.Errorf("create note: %w", err)
	}
	fmt.Printf("✅ Created note: %s\n", title)
	return nil
}

func updateNote(c *api.Client, notebookID, noteID, content, title string) error {
	fmt.Printf("Updating note %s...\n", noteID)
	if err := c.UpdateNote(notebookID, noteID, content, title); err != nil {
		return fmt.Errorf("update note: %w", err)
	}
	fmt.Printf("✅ Updated note: %s\n", title)
	return nil
}

func removeNote(c *api.Client, noteID string) error {
	fmt.Printf("Are you sure you want to remove note %s? [y/N] ", noteID)
	var response string
	fmt.Scanln(&response)
	if !strings.HasPrefix(strings.ToLower(response), "y") {
		return fmt.Errorf("operation cancelled")
	}

	if err := c.RemoveNote(noteID); err != nil {
		return fmt.Errorf("remove note: %w", err)
	}
	fmt.Printf("✅ Removed note: %s\n", noteID)
	return nil
}

// Other operations
func createAudioOverview(c *api.Client, notebookID, instructions string) error {
	fmt.Printf("Creating audio overview for notebook %s...\n", notebookID)

	opts := api.AudioOverviewOptions{
		Instructions: instructions,
	}

	if err := c.CreateAudioOverview(notebookID, opts); err != nil {
		return fmt.Errorf("create audio overview: %w", err)
	}

	fmt.Printf("✅ Created audio overview with instructions:\n%s\n", instructions)
	return nil
}

func heartbeat(c *api.Client) error {
	return nil
}
