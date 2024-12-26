/*
nlm is a command-line interface for NotebookLM.

Usage:
    nlm <command> [flags] [arguments]

Commands:
    list, ls [-json] [-format=template]    List notebooks
    create [-json] <title>                 Create a notebook
    rm <id>                               Delete a notebook

    sources [-json] <id>                  List sources in notebook
    add [-filename name] [-base64] <id> <input>  Add source
    rm-source <id> <source-id>            Remove source
    rename-source <source-id> <new-name>  Rename source
    refresh-source <source-id>            Refresh source

    notes [-json] <id>                    List notes
    new-note [-json] <id> <title>         Create note
    edit-note <id> <note-id> <content>    Edit note
    rm-note <note-id>                     Remove note

    audio-create [-w] [-json] <id> <instructions>  Create audio
    audio-get [-json] <id>                Get audio
    audio-rm <id>                         Remove audio
    audio-share [-json] <id>              Share audio

    generate-guide [-w] [-json] <id>      Generate guide
    generate-outline [-w] [-json] <id>    Generate outline

    auth [-debug] [profile]               Setup authentication

Common Flags:
    -json            Output JSON format
    -format string   Output format (Go template syntax)
    -w               Wait for async operations
    -debug          Enable debug output

Environment:
    NLM_AUTH_TOKEN      Authentication token
    NLM_COOKIES         Session cookies
    NLM_BROWSER_PROFILE Browser profile for auth (default: Default)
    NLM_DEBUG           Enable debug output

Examples:
    # List notebooks
    nlm ls -json
    nlm ls -format '{{.ID}}: {{.Title}}'

    # Add content
    nlm add -filename "Report.pdf" notebook-id doc.pdf
    cat data.txt | nlm add notebook-id -

    # Generate content
    nlm generate-guide -w notebook-id
    nlm audio-create -w notebook-id "Summarize key points"
*/
package main

