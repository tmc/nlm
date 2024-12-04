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
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  list, ls          List all notebooks\n")
		fmt.Fprintf(os.Stderr, "  create <title>    Create a new notebook\n")
		fmt.Fprintf(os.Stderr, "  rm <id>           Delete a notebook\n")
		fmt.Fprintf(os.Stderr, "  add <id> <input>  Add note to notebook (URL or file)\n")
		fmt.Fprintf(os.Stderr, "  get <id>          Get all notes from notebook\n")
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
	case "auth":
		err = handleAuth(args, debug)
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
	case "add":
		if len(args) != 2 {
			log.Fatal("usage: nlm add <notebook-id> <file>")
		}
		err = addNote(client, args[0], args[1])
	case "get":
		if len(args) != 1 {
			log.Fatal("usage: nlm get <notebook-id>")
		}
		err = getNotes(client, args[0])
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
	return c.DeleteNotebook(id)
}

func addNote(c *api.Client, notebookID, input string) error {
	// Check if input is a URL
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		fmt.Printf("Adding note from URL: %s\n", input)
		return c.AddNoteFromURL(notebookID, input)
	}

	// Check if input is a file
	if _, err := os.Stat(input); err == nil {
		return fmt.Errorf("file upload not implemented yet")
	}

	return fmt.Errorf("invalid input: must be URL or file path")
}

func getNotes(c *api.Client, notebookID string) error {
	return fmt.Errorf("not implemented")
}

func heartbeat(c *api.Client) error {
	return nil
}
