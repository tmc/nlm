package main

import (
    "flag"
    "fmt"
    "log"
    "os"

    "github.com/tmc/nlm/cmd/nlm/auth"
    "github.com/tmc/nlm/cmd/nlm/env"
    "github.com/tmc/nlm/internal/api"
)

func main() {
    log.SetPrefix("nlm: ")
    log.SetFlags(0)

    if len(os.Args) < 2 {
        usage()
        os.Exit(2)
    }

    cmd := os.Args[1]
    args := os.Args[2:]

    if err := run(cmd, args); err != nil {
        fmt.Fprintf(os.Stderr, "nlm: %v\n", err)
        os.Exit(1)
    }
}

func usage() {
    fmt.Fprintf(os.Stderr, "Usage: nlm <command> [flags] [arguments]\n\n")
    fmt.Fprintf(os.Stderr, "Commands:\n")
    fmt.Fprintf(os.Stderr, "  list, ls [-json] [-format=template]    List notebooks\n")
    fmt.Fprintf(os.Stderr, "  create [-json] <title>                 Create a notebook\n")
    fmt.Fprintf(os.Stderr, "  rm <id>                               Delete a notebook\n\n")

    fmt.Fprintf(os.Stderr, "  sources [-json] <id>                  List sources\n")
    fmt.Fprintf(os.Stderr, "  add [-filename name] [-base64] <id> <input>  Add source\n")
    fmt.Fprintf(os.Stderr, "  rm-source <id> <source-id>            Remove source\n")
    fmt.Fprintf(os.Stderr, "  rename-source <source-id> <new-name>  Rename source\n\n")

    fmt.Fprintf(os.Stderr, "  notes [-json] <id>                    List notes\n")
    fmt.Fprintf(os.Stderr, "  new-note [-json] <id> <title>         Create note\n")
    fmt.Fprintf(os.Stderr, "  edit-note <id> <note-id> <content>    Edit note\n")
    fmt.Fprintf(os.Stderr, "  rm-note <note-id>                     Remove note\n\n")

    fmt.Fprintf(os.Stderr, "  audio-create [-w] [-json] <id> <instructions>  Create audio\n")
    fmt.Fprintf(os.Stderr, "  audio-get [-json] <id>                Get audio\n")
    fmt.Fprintf(os.Stderr, "  audio-rm <id>                         Remove audio\n")
    fmt.Fprintf(os.Stderr, "  audio-share [-json] <id>              Share audio\n\n")

    fmt.Fprintf(os.Stderr, "  generate-guide [-w] [-json] <id>      Generate guide\n")
    fmt.Fprintf(os.Stderr, "  generate-outline [-w] [-json] <id>    Generate outline\n\n")

    fmt.Fprintf(os.Stderr, "  auth [-debug] [profile]               Setup authentication\n\n")

    fmt.Fprintf(os.Stderr, "Run 'nlm <command> -h' for command details\n")
}

func run(cmd string, args []string) error {
    env.LoadStoredEnv()

    // Create base client
    client := api.New(
        os.Getenv("NLM_AUTH_TOKEN"),
        os.Getenv("NLM_COOKIES"),
    )

    // Try command with auth retry
    for i := 0; i < 3; i++ {
        if i > 1 {
            fmt.Fprintln(os.Stderr, "nlm: attempting again to obtain login information")
        }

        if err := runCmd(client, cmd, args); err == nil {
            return nil
        } else if !api.IsUnauthorized(err) {
            return err
        }

        var err error
        token, cookies, err := auth.HandleAuth(nil, i > 0)
        if err != nil {
            fmt.Fprintf(os.Stderr, "  -> %v\n", err)
            continue
        }
        client = api.New(token, cookies)
    }
    return fmt.Errorf("failed after 3 attempts")
}

func runCmd(c *api.Client, cmd string, args []string) error {
    switch cmd {
    case "list", "ls":
        return handleList(c, args)
    case "create":
        return handleCreate(c, args)
    case "rm":
        return handleRemove(c, args)
    case "sources":
        return handleSources(c, args)
    case "add":
        return handleAdd(c, args)
    case "rm-source":
        return handleRemoveSource(c, args)
    case "rename-source":
        return handleRenameSource(c, args)
    case "notes":
        return handleNotes(c, args)
    case "new-note":
        return handleNewNote(c, args)
    case "edit-note":
        return handleEditNote(c, args)
    case "rm-note":
        return handleRemoveNote(c, args)
    case "audio-create":
        return handleAudioCreate(c, args)
    case "audio-get":
        return handleAudioGet(c, args)
    case "audio-rm":
        return handleAudioRemove(c, args)
    case "audio-share":
        return handleAudioShare(c, args)
    case "generate-guide":
        return handleGenerateGuide(c, args)
    case "generate-outline":
        return handleGenerateOutline(c, args)
    case "auth":
        return handleAuth(args)
    case "help", "-h", "--help":
        usage()
        return nil
    default:
        return fmt.Errorf("unknown command %q (run 'nlm help')", cmd)
    }
}

