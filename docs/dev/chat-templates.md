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

## External rendering tools

Keep application-specific enrichment in the consuming tool. For example, a
report generator can resolve message IDs against its own archive and emit
ordinary Markdown links. `nlm` renders complete HTTP(S) links in HTML but does
not resolve Discord IDs or read Discord archives. The citation metadata above
is available to custom templates and external report processors.

Templates currently produce Markdown, not HTML fragments or page shells.

## Saved history

Continuing `generate-chat` appends each exchange to its saved conversation,
including local reasoning and citation metadata. A short conversation ID is
expanded from the saved full ID before consulting the server.

For an older incomplete local conversation, `chat show --backfill` merges the
available server turns before rendering and saves only that conversation file.
Existing local text, reasoning, timestamps, citations, and rich trees are retained;
missing metadata is filled from matching server turns. Repeating backfill does
not duplicate turns. The current history request returns at most 20 recent
messages, so recovery cannot guarantee turns older than that window. An empty
server response is an error and leaves the saved conversation unchanged.

### HTML citation markup

The native HTML renderer keeps `citelink`, `grounded`, `data-msg`, and
`data-cite` on citation links and grounded passages. When a passage has several
grounding citations, `data-cite` remains its first index and `data-cites` lists
all indices separated by spaces. Consumers should read `data-cites` when present.
The preview and highlighting include every associated citation.

For repeated grounding, `data-passages` contains `citation:occurrence` pairs,
with occurrences numbered from one within each citation. Fragments created by
inline formatting share an occurrence. Each distinct occurrence gets a sidebar
passage action; a zero-width annotation gets no invented passage target.
Distinct source excerpts under one citation are retained even when their source
IDs match. Excerpt text is preserved without guessing table rows or decoding
literal escape sequences; trimming and the configured length limit still apply.
Hovering or focusing a passage action highlights just that occurrence, including
all of its formatting fragments; clicking scrolls to the first fragment.
