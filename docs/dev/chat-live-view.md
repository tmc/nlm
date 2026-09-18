# Chat index and live view

Status: proposed. Covers `nlm chat show --format=html` with no conversation id
(the index), turn status (including "still generating"), a `--live` view, and
the on-disk streaming that makes a live view possible.

## 1. What exists today

`nlm chat show --format=html --open <notebook-id>` already renders every saved
conversation into one page: `chatShowNotebook` in
`cmd/nlm/chat_render_html_notebook.go` loads the sessions,
`renderNotebookHTML` in `internal/richrender/chat_render_html_notebook.go`
renders each one with the single-conversation renderer, namespaces its ids, and
embeds all of them in a JSON blob the page switches between.

Measured on notebook `08b54bf7` (9 saved conversations, store of 7,926 session
files / 4.3 GB under `~/.nlm`):

| | |
|---|---|
| wall time | 24.9 s |
| page size | 1.13 MB |
| session files read | 7,926 |
| session files belonging to the notebook | 10 |

Reading all those bytes costs ~2 s; the rest is `json.Unmarshal` of every
session in the store, messages, citations and rich trees included, to look at
one field. `loadNotebookSessionRecords` walks `~/.nlm` and fully decodes each
file to test `session.NotebookID != notebookID`.

The fast path already exists for the listing commands and is unused here:
`localChatSessionFiles` in `cmd/nlm/chat_session_index.go` filters by filename
(sessions are `chat-<notebook>[-<conv8>].json`), `readChatSessionSummary`
token-walks a session without materializing messages, and a stat-keyed cache
lives at `~/Library/Caches/nlm/chat-index.json`.

Two things the page cannot show because nothing records them:

- **Whether a turn is still generating.** A conversation whose answer never
  arrived (`30397cd8` in the sample: one message, no answer) renders as a
  question with nothing under it, indistinguishable from a chat that failed
  three days ago.
- **Partial answers.** The session file is written when the turn ends. While an
  answer streams, the only copy is in the streaming process's stdout.

## 2. Goals

1. `chat show --format=html --open <notebook-id>` opens an index of the
   notebook's conversations in well under a second, with each conversation one
   click away.
2. Every conversation and turn carries an honest status, including "generating,
   started 12s ago" and "interrupted".
3. `--live` shows a page that updates as answers stream, without re-running the
   command.
4. A streaming answer is on disk while it streams, so any reader — the live
   page, a second terminal, a crash post-mortem — can see it.

Non-goals: a server anyone but the local user can reach; editing from the page;
replacing the terminal stream as the primary chat surface.

## 3. Session status

### 3.1 States

| state | meaning | written by |
|---|---|---|
| `running` | a turn is in flight | the writer, before the request |
| `complete` | the answer arrived and is stored | the writer, on success |
| `incomplete` | the answer was truncated or abandoned (runaway output, exit 8) | the writer |
| `error` | the turn failed; no answer is stored | the writer |

The state lives on the session (`chatSession.Status`), not on the message: it
describes the turn in flight, and a session has at most one.

### 3.2 `running` is a claim about a process, not a fact about the world

A killed process leaves `running` on disk forever. A reader that renders it as
"generating" lies, and the lie is permanent. So the session also records:

```go
Status    string    `json:"status,omitempty"`     // running, complete, incomplete, error
StatusAt  time.Time `json:"status_at,omitempty"`  // when Status was last written
WriterPID int       `json:"writer_pid,omitempty"` // process that set running
```

A reader resolves `running` as:

- `generating` when the sidecar (§4) was appended to within `staleAfter`
  (default 90 s), or `writer_pid` is alive;
- `interrupted` otherwise.

`staleAfter` is generous on purpose: NotebookLM answers routinely exceed two
minutes of thinking with no output, so an idle sidecar is not evidence of
death — the liveness signal is the writer's heartbeat line (§4.2), not answer
text.

The writer refreshes `status_at` on every sidecar flush, so a healthy long
think keeps reading as `generating`.

### 3.3 Where it is set

`generateFreeFormChat` (`cmd/nlm/main.go`) already brackets the request:
`beginGeneratedChatTurn` persists the user turn as `running` before the RPC,
`saveGeneratedChatTurn` closes it out as `complete`/`incomplete`, and the error
paths set `error`. That bracket is the whole mechanism; the rest of this spec
consumes it.

## 4. Streaming partials to disk

### 4.1 Sidecar, not the session file

The session JSON stays what it is: the durable record, written once per turn.
Rewriting a growing JSON document per chunk would re-encode every earlier
message on every delta and leave a reader one `os.WriteFile` truncation away
from parsing garbage (`saveChatSession` writes in place, twice, today).

Instead each in-flight turn gets an append-only sidecar next to the session:

```
~/.nlm/chat-<notebook>-<conv8>.partial.jsonl
```

The lines are the JSONL events `--citations=json` already emits
(`writeChunkJSONL` in `internal/richrender/stream.go`): `{"phase":"thinking"}`,
`{"phase":"answer","text":…}`, `{"phase":"citation",…}`, `{"phase":"done",…}`.
One producer, one format, one set of semantics to get right.

### 4.2 Invariants

1. **Answer events are monotone extensions by offset.** Never the raw wire
   delta. When the server revises text it already sent, the delta restarts at
   the divergence point and a concatenating reader duplicates the tail — the
   bug that produced a 1.37 MB answer from a 25 KB one. The producer emits only
   the bytes past what it has already emitted, exactly as the terminal stream
   does.
2. **A revision is announced, not silently applied.** When a revision lands
   inside the already-emitted range, the producer appends
   `{"phase":"revised","full":"<authoritative answer so far>"}`. A reader
   replaces its buffer with `full`; a reader that ignores the event is stale in
   a bounded, visible way rather than wrong by duplication.
3. **`done` is authoritative.** `{"phase":"done","answer":…}` carries the exact
   final answer, byte-identical to what lands in the session file.
4. **Heartbeat.** The producer appends `{"phase":"alive","t":…}` every 15 s
   while a request is outstanding and no other event has been written. This is
   what separates "thinking hard" from "dead".
5. **Append-only, line-atomic.** Each event is one `Write` of one complete line,
   so a reader tailing the file either sees a whole line or no line. A trailing
   partial line is ignored until it is terminated.

### 4.3 Lifecycle

- Created by `beginGeneratedChatTurn`, alongside the `running` session write.
- Removed after `saveGeneratedChatTurn` writes the session, unless
  `--keep-partial` is set, in which case it is renamed
  `chat-<notebook>-<conv8>.<unix>.jsonl` for forensics.
- An orphan sidecar older than `staleAfter` with no live writer is what the
  index renders as `interrupted`; `nlm chat show <nb> <conv>` offers its text
  as the partial answer, clearly labeled, and never merges it into the session.

### 4.4 Atomic session writes

Independent of the sidecar, `saveChatSession` must stop writing in place:
write to `<path>.tmp` in the same directory, `fsync`, `os.Rename`. A live
reader polls these files; a truncated read is a parse error today. Both the
per-conversation file and the notebook alias file get the same treatment.

## 5. The index view

`nlm chat show --format=html [--open] <notebook-id>` renders an index, not a
bundle.

### 5.1 Data

One row per conversation, built from `readChatSessionSummary` extended with
`status`, `status_at`, and the first user message clipped to a title. No
message bodies, no citations, no rich trees are decoded. File discovery is
`localChatSessionFiles(notebookID)` — 10 files here, not 7,926.

Budget: the index must render in under 250 ms on this store. It is a
directory read, ten token-walks, and a template.

### 5.2 Page

```
Conversations · notebook 08b54bf7                        9 conversations

 ●  In the MLSIROH spec, phase 3 sequences MLS commits…   4 turns  10:55  1e409ff4
 ◍  Review the source "MLSIROH: our spec (3rd baseline…"  2 turns  10:52  566c30f8   generating 00:41
 ○  Review the source "MLSIROH: our spec (3rd baseline…"  1 turn   10:47  30397cd8   interrupted
 ●  Answer only from draft-ietf-mls-extensions and RFC…   2 turns  10:22  92385757
```

Status is a badge, not only a color: `generating hh:mm:ss` (counting from
`status_at`), `interrupted`, `truncated` (incomplete), `failed` (error), and
nothing at all for a complete conversation — the common case stays quiet.

### 5.3 Bodies

Each row links to `<conversation-id>.html` in the same directory
(`~/Library/Caches/nlm/render/<notebook>/`, per `chatHTMLDestination`), written
by the existing single-conversation renderer. The index writes a body page when
it is missing or older than the session file; `--rebuild` forces all of them.

This replaces the embedded-blob switcher. `--inline` keeps the old single-file
behavior for anyone who wants one self-contained artifact to send someone; it
is then explicitly a bundle, with its size reported on stderr.

`--format=markdown` is unchanged: it already concatenates conversations, and a
Markdown reader has no click.

## 6. `--live`

`nlm chat show --format=html --live [--open] <notebook-id>` serves instead of
writing:

```
nlm: serving http://127.0.0.1:53417/?k=8f3c… (ctrl-c to stop)
```

- **Server.** `net/http` on `127.0.0.1:0`. A random port and a per-run key in
  the URL, checked on every request. Loopback only; no LAN bind, no flag to
  add one. Chat transcripts are private and the process is a developer tool,
  not a service.
- **Watching.** Poll `localChatSessionFiles(notebookID)` plus the sidecars
  every 250 ms and compare `(size, mtime)`. Ten stat calls; no dependency, no
  fsnotify platform matrix. The poll interval backs off to 2 s after 5 minutes
  with no change, and resets on any change.
- **Transport.** One SSE stream at `/events`. Index rows patch in place
  (status, turn count, timestamp). An open conversation subscribes to its
  sidecar by byte offset: the client sends the offset it has, the server sends
  the events past it. That is the same monotone-by-offset contract as §4.2, so
  the page appends text and never re-renders an answer it already has — except
  on `revised`, which replaces the message body outright.
- **Completion.** When a sidecar ends with `done`, the server re-renders that
  conversation's body page from the session file and tells the page to swap it
  in, so the final view is the real renderer's output (citations, rich text,
  resolved titles) rather than the streaming approximation.
- **Ending.** Ctrl-C. `--live=<duration>` exits after that long; `--live` with
  `--out` is an error (there is no file to write); `--open` opens the URL.

`--live` with a conversation id serves that one conversation's page.

The terminal equivalent, `nlm chat tail <notebook> <conv>` printing a sidecar
as it grows, falls out of the same format for free and is worth having, but is
not part of this spec.

## 7. Delivery

Each stage stands alone and ships on its own.

1. **Fast index data.** Point `chatShowNotebook` at `localChatSessionFiles` +
   `readChatSessionSummary`. No visible change beyond the page appearing in
   ~200 ms instead of ~25 s. Test: a store with sessions from three notebooks
   opens exactly the files of the one asked for.
2. **Status on the session.** `status`, `status_at`, `writer_pid`; the
   `running`→`complete`/`incomplete`/`error` bracket in `generateFreeFormChat`;
   `chat list` gains a status column. Test: an abandoned turn reads
   `interrupted`, a fresh one reads `generating`.
3. **Index page.** Rows, badges, per-conversation body pages, `--inline` for
   the old bundle. Test: golden HTML for a three-conversation notebook with one
   of each status.
4. **Sidecar.** Append-only JSONL, heartbeat, atomic session writes,
   `--keep-partial`. Test: a killed writer leaves a sidecar whose events replay
   to exactly the text the terminal printed; a revision mid-stream replays to
   the same answer the session file holds (this is the regression the
   duplication bug earns).
5. **`--live`.** Server, poll, SSE, swap-on-done. Test: a scripted writer
   appending to a sidecar drives an `/events` client through
   `generating → revised → done` and the final page equals the static render.

## 8. Risks

- **A stale `running` reads as a live chat.** Mitigated by the heartbeat and
  pid check, but a reused pid can still say "generating" about a dead turn.
  Acceptable: the badge is advisory and the timer makes an implausible claim
  obvious.
- **Two writers, one conversation.** Parallel `generate-chat` calls against one
  conversation id already race on the session file; sidecars make the race
  visible rather than worse. The live page shows the last writer's stream.
  Naming the sidecar per-turn (`.<seq>.jsonl`) is the fix if this becomes real.
- **The local server outlives its usefulness.** It exits with the command;
  there is no daemon mode, and adding one needs its own argument.
- **Page weight.** The index is bounded; body pages are not. A 233 KB
  conversation page is already normal, and lazy loading is the point of §5.3.
