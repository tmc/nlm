package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "os"
    "strings"
    "text/tabwriter"
    "text/template"
    "time"

    "github.com/tmc/nlm/internal/api"
)

// Command handlers

func handleList(c *api.Client, args []string) error {
    fs := flag.NewFlagSet("ls", flag.ExitOnError)
    json := fs.Bool("json", false, "output JSON")
    format := fs.String("format", "", "output format (Go template)")
    debug := fs.Bool("debug", false, "enable debug output")

    fs.Usage = func() {
        fmt.Fprintf(os.Stderr, "Usage: nlm ls [-json] [-format=template]\n\n")
        fmt.Fprintf(os.Stderr, "List notebooks in various formats.\n\n")
        fmt.Fprintf(os.Stderr, "Options:\n")
        fs.PrintDefaults()
        fmt.Fprintf(os.Stderr, "\nExample formats:\n")
        fmt.Fprintf(os.Stderr, "  {{.ID}}: {{.Title}}\n")
        fmt.Fprintf(os.Stderr, "  {{range .}}{{.ID}}\\n{{end}}\n")
    }

    if err := fs.Parse(args); err != nil {
        return err
    }

    if *debug {
        c.SetDebug(true)
    }

    return list(c, *json, *format)
}

func handleCreate(c *api.Client, args []string) error {
    fs := flag.NewFlagSet("create", flag.ExitOnError)
    json := fs.Bool("json", false, "output JSON")
    debug := fs.Bool("debug", false, "enable debug output")

    fs.Usage = func() {
        fmt.Fprintf(os.Stderr, "Usage: nlm create [-json] <title>\n\n")
        fmt.Fprintf(os.Stderr, "Create a new notebook.\n\n")
        fmt.Fprintf(os.Stderr, "Options:\n")
        fs.PrintDefaults()
    }

    if err := fs.Parse(args); err != nil {
        return err
    }

    if fs.NArg() != 1 {
        fs.Usage()
        return fmt.Errorf("notebook title required")
    }

    if *debug {
        c.SetDebug(true)
    }

    return create(c, fs.Arg(0), *json)
}

func handleAdd(c *api.Client, args []string) error {
    fs := flag.NewFlagSet("add", flag.ExitOnError)
    filename := fs.String("filename", "", "custom name for the source")
    base64 := fs.Bool("base64", false, "force base64 encoding")
    contentType := fs.String("content-type", "", "explicit content type")
    noContentType := fs.Bool("no-content-type", false, "don't set content type")
    autoContentType := fs.Bool("auto-content-type", true, "auto-detect content type")
    debug := fs.Bool("debug", false, "enable debug output")

    fs.Usage = func() {
        fmt.Fprintf(os.Stderr, `Usage: nlm add [options] <notebook-id> <input>

Input can be:
  - A file path
  - A URL (http:// or https://)
  - "-" for stdin
  - Direct text content

Options:
`)
        fs.PrintDefaults()
        fmt.Fprintf(os.Stderr, `
Examples:
  nlm add notebook-id document.txt
  nlm add -base64 notebook-id image.png
  nlm add -filename "Report.pdf" notebook-id doc.pdf
  cat file.json | nlm add notebook-id -
`)
    }

    if err := fs.Parse(args); err != nil {
        return err
    }

    if fs.NArg() != 2 {
        fs.Usage()
        return fmt.Errorf("notebook ID and input required")
    }

    if *debug {
        c.SetDebug(true)
    }

    var opts []api.SourceOption
    if *filename != "" {
        opts = append(opts, api.WithSourceName(*filename))
    }
    if *base64 {
        opts = append(opts, api.WithBase64Encoding())
    }
    switch {
    case *contentType != "":
        opts = append(opts, api.WithContentType(*contentType))
    case *noContentType:
        opts = append(opts, api.WithContentTypeNone())
    case *autoContentType:
        opts = append(opts, api.WithContentTypeAuto())
    }

    return add(c, fs.Arg(0), fs.Arg(1), opts...)
}

func handleAudioCreate(c *api.Client, args []string) error {
    fs := flag.NewFlagSet("audio-create", flag.ExitOnError)
    wait := fs.Bool("w", false, "wait for completion")
    json := fs.Bool("json", false, "output JSON")
    debug := fs.Bool("debug", false, "enable debug output")

    fs.Usage = func() {
        fmt.Fprintf(os.Stderr, "Usage: nlm audio-create [-w] [-json] <notebook-id> <instructions>\n\n")
        fmt.Fprintf(os.Stderr, "Create an audio overview of a notebook.\n\n")
        fmt.Fprintf(os.Stderr, "Options:\n")
        fs.PrintDefaults()
    }

    if err := fs.Parse(args); err != nil {
        return err
    }

    if fs.NArg() != 2 {
        fs.Usage()
        return fmt.Errorf("notebook ID and instructions required")
    }

    if *debug {
        c.SetDebug(true)
    }

    return createAudio(c, fs.Arg(0), fs.Arg(1), *wait, *json)
}

// Command implementations

func list(c *api.Client, jsonfmt bool, format string) error {
    fmt.Fprintf(os.Stderr, "Fetching notebooks...\n")

    notebooks, err := c.ListRecentlyViewedProjects()
    if err != nil {
        return err
    }

    if jsonfmt {
        return json.NewEncoder(os.Stdout).Encode(notebooks)
    }

    if format != "" {
        tmpl, err := template.New("output").Parse(format)
        if err != nil {
            return fmt.Errorf("invalid format: %w", err)
        }
        return tmpl.Execute(os.Stdout, notebooks)
    }

    // Default tabwriter output
    w := tabwriter.NewWriter(os.Stdout, 0, 4, 4, ' ', 0)
    fmt.Fprintln(w, "ID\tTITLE\tLAST UPDATED")
    for _, nb := range notebooks {
        fmt.Fprintf(w, "%s\t%s\t%s\n",
            nb.ProjectId,
            strings.TrimSpace(nb.Emoji)+" "+nb.Title,
            nb.GetMetadata().GetCreateTime().AsTime().Format(time.RFC3339),
        )
    }
    return w.Flush()
}

func create(c *api.Client, title string, jsonfmt bool) error {
    fmt.Fprintf(os.Stderr, "Creating notebook %q...\n", title)

    nb, err := c.CreateProject(title, "📙")
    if err != nil {
        return err
    }

    if jsonfmt {
        return json.NewEncoder(os.Stdout).Encode(nb)
    }

    fmt.Printf("%s\n", nb.ProjectId) // Machine-readable ID on stdout
    fmt.Fprintf(os.Stderr, "Created notebook %q\n", nb.Title)
    return nil
}

func add(c *api.Client, notebookID, input string, opts ...api.SourceOption) error {
    switch input {
    case "-":
        fmt.Fprintf(os.Stderr, "Reading from stdin...\n")
        data, err := io.ReadAll(os.Stdin)
        if err != nil {
            return fmt.Errorf("read stdin: %w", err)
        }
        id, err := c.AddSource(notebookID, "stdin", data, opts...)
        if err != nil {
            return err
        }
        fmt.Printf("%s\n", id) // Machine-readable ID on stdout
        return nil

    case "":
        return fmt.Errorf("input required (file, URL, or '-' for stdin)")

    default:
        if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
            fmt.Fprintf(os.Stderr, "Fetching %s...\n", input)
            id, err := c.AddSourceFromURL(notebookID, input, opts...)
            if err != nil {
                return err
            }
            fmt.Printf("%s\n", id)
            return nil
        }

        fmt.Fprintf(os.Stderr, "Adding %s...\n", input)
        id, err := c.AddSourceFromFile(notebookID, input, opts...)
        if err != nil {
            return err
        }
        fmt.Printf("%s\n", id)
        return nil
    }
}

func createAudio(c *api.Client, notebookID, instructions string, wait, jsonfmt bool) error {
    fmt.Fprintf(os.Stderr, "Creating audio overview...\n")
    fmt.Fprintf(os.Stderr, "Instructions: %s\n", instructions)

    result, err := c.CreateAudioOverview(notebookID, instructions)
    if err != nil {
        return err
    }

    if !result.IsReady && !wait {
        fmt.Fprintf(os.Stderr, "Generation started! Check status with: nlm audio-get %s\n", notebookID)
        if jsonfmt {
            return json.NewEncoder(os.Stdout).Encode(result)
        }
        fmt.Printf("%s\n", result.AudioID) // Machine-readable ID on stdout
        return nil
    }

    if wait {
        fmt.Fprintf(os.Stderr, "Waiting for generation...")
        for !result.IsReady {
            time.Sleep(5 * time.Second)
            fmt.Fprintf(os.Stderr, ".")
            result, err = c.GetAudioOverview(notebookID)
            if err != nil {
                return err
            }
        }
        fmt.Fprintf(os.Stderr, "\n")
    }

    if err := saveAudio(result); err != nil {
        return err
    }

    if jsonfmt {
        return json.NewEncoder(os.Stdout).Encode(result)
    }

    fmt.Printf("%s\n", result.AudioID) // Machine-readable ID on stdout
    fmt.Fprintf(os.Stderr, "Audio ready! Saved to: audio_%s.wav\n", result.AudioID)
    return nil
}

func generateGuide(c *api.Client, notebookID string, wait, jsonfmt bool) error {
    fmt.Fprintf(os.Stderr, "Generating guide...\n")

    guide, err := c.GenerateGuide(notebookID)
    if err != nil {
        return err
    }

    if !guide.Ready && !wait {
        fmt.Fprintf(os.Stderr, "Generation started! Check back later.\n")
        if jsonfmt {
            return json.NewEncoder(os.Stdout).Encode(guide)
        }
        return nil
    }

    if wait {
        fmt.Fprintf(os.Stderr, "Waiting for generation...")
        for !guide.Ready {
            time.Sleep(2 * time.Second)
            fmt.Fprintf(os.Stderr, ".")
            guide, err = c.GenerateGuide(notebookID)
            if err != nil {
                return err
            }
        }
        fmt.Fprintf(os.Stderr, "\n")
    }

    if jsonfmt {
        return json.NewEncoder(os.Stdout).Encode(guide)
    }

    fmt.Println(guide.Content) // Clean content output on stdout
    return nil
}

func confirm(format string, args ...interface{}) bool {
    fmt.Fprintf(os.Stderr, format+" [y/N] ", args...)
    var response string
    fmt.Scanln(&response)
    return strings.HasPrefix(strings.ToLower(response), "y")
}

func saveAudio(audio *api.Audio) error {
    if audio.Data == "" {
        return nil
    }

    filename := fmt.Sprintf("audio_%s.wav", audio.ID)
    if err := os.WriteFile(filename, audio.Bytes(), 0644); err != nil {
        return fmt.Errorf("save audio: %w", err)
    }
    return nil
}
