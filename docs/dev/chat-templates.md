# Markdown chat templates

Render a saved conversation with a Go text/template file:

```sh
nlm chat show NOTEBOOK CONVERSATION \
  --template docs/examples/chat-templates/report.tmpl \
  --template-var title='Release review' --template-var author='Review team' \
  --out report.md
```

Use `--last` instead of a conversation ID for the newest saved chat. Omit both
to render all saved conversations in the existing newest-first order.
`--template` implies Markdown; without `--out`, output goes to stdout.
`--out -` also selects stdout. Ordinary `--format markdown` supports `--out`.

Template variables are strings. Split each `key=value` at the first `=`;
empty values are allowed and the last repeated key wins. Access a variable as
`.Vars.title` or `index .Vars "project-name"`. Dot access to missing keys fails;
Go's built-in `index` returns the zero value for a missing map key.

| Data | Fields |
| --- | --- |
| Root | NotebookID, Vars, Conversations |
| Conversation | ID, Title, Messages, Markdown |
| Message | Role, Content, Thinking, Citations, CitationsMarkdown, Markdown |
| Citation | Index, SourceID, ParentSourceID, Title, Excerpt, Confidence, AnswerSpan, SourceSpan, Location |

Titles may be empty when not saved. `Content` is the original message content.
`Markdown` reuses the existing transcript rendering, including role headings.
`CitationsMarkdown` contains the existing citation table or excerpt blocks.
Citation Index is an integer; other citation fields are display strings.
Indices remain local to a message, not unique across the notebook.

Thinking is exposed only with `--thinking`. `--citations off` removes citation
data and fragments. Excerpts require `--citation-excerpts`; confidence and spans
respect their hiding flags. Existing optional source resolution happens before
the template executes. Template execution cannot fetch sources or read files.

Functions include Go's standard template functions plus `upper`, `lower`,
`join` (string slice, separator), and `quote` (Go-quoted string, also suitable for
JSON string values and ordinary YAML quoted metadata). Templates control their
own Markdown escaping. Raw HTML is not automatically escaped by text/template.

The explicit file is parsed before loading conversations. Named templates can
be defined within that file using Go's `define` and `template` actions. No
implicit includes, preset directory, stdin template, or global template setting
is used. Templates cannot be combined with HTML, text, JSON output, or `--open`.

Execution completes before output is written. Errors leave stdout empty and an
existing destination unchanged. File output uses a temporary file in the same
directory and a rename; its parent directory must already exist. New reports
are written with private permissions. Saved conversations are unchanged unless
an existing option such as `--backfill` was explicitly requested.

Copyable layouts are in `docs/examples/chat-templates/`: `transcript.tmpl`,
`answers.tmpl`, and `report.tmpl`. This feature applies to saved-chat rendering;
`generate-chat` and `generate-report` keep their existing output behavior.
