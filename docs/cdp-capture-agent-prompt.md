# Task: Drive NotebookLM Browser Actions via CDP Shell and Capture Network Traffic

## Context

You have a cdp shell running in iTerm2 session `2E84DB09-29DE-4F60-B6B7-BD62EBE160A2` connected to Brave on localhost:9222. It was started with:

```
cdp -shell -remote-host localhost -remote-port 9222 -harl \
    -output-dir ~/go/src/github.com/tmc/misc/chrome-to-har/logs/nlm-capture \
    -chrome-path '/Applications/Brave Browser.app/Contents/MacOS/Brave Browser'
```

The browser is authenticated to NotebookLM. The cdp shell captures network traffic as NDJSON HAR entries, organized by domain into `.jsonl` files in the output directory.

## Your Goal

Build a corpus of captured network traffic for every NotebookLM operation by:
1. Using `push-context <name>` to isolate each action's traffic into its own subdirectory
2. Triggering the action via cdp shell commands (navigate, click, type, eval JS)
3. Using `pop-context` to finalize the capture
4. Inspecting the captures to identify which RPC IDs fired and what data was exchanged

## Environment

### iTerm2 Session Control

Send commands to the cdp shell:
```bash
SID="2E84DB09-29DE-4F60-B6B7-BD62EBE160A2"
it2 session send-text "$SID" "command here"
it2 session get-screen "$SID"                  # read current screen
it2 session get-buffer "$SID" --lines 50       # read scrollback
```

### Capture Directory
```
CAPDIR=~/go/src/github.com/tmc/misc/chrome-to-har/logs/nlm-capture
```
The file of interest within each context subdirectory is `notebooklm.google.com.jsonl` — this is where all NotebookLM batchexecute traffic lands.

### Context-Based Action Isolation

The cdp shell supports `push-context <name>` / `pop-context` to redirect HARL output to named subdirectories. Contexts nest — you can push multiple levels.

**Workflow per action:**
```
cdp> push-context homepage-load
Pushed context: homepage-load
Output directory: .../nlm-capture/homepage-load

cdp> goto https://notebooklm.google.com
# ... wait 3-5 seconds for RPCs to settle ...

cdp> pop-context
Popped context: homepage-load
Output directory: .../nlm-capture
```

Each action's traffic lands in its own directory:
```
nlm-capture/
  homepage-load/
    notebooklm.google.com.jsonl
    accounts.google.com.jsonl
    ...
  open-notebook/
    notebooklm.google.com.jsonl
    ...
```

### CDp Shell Commands

**Navigation:**
- `goto <url>` / `nav <url>` — navigate to URL
- `back` / `forward` — browser history
- `reload` — reload page
- `url` — get current URL
- `title` — get page title

**DOM Interaction:**
- `click <selector>` — click an element
- `type <selector> <text>` — type into an input
- `text <selector>` — get text content of element
- `html <selector>` — get HTML of element
- `eval <js>` — evaluate JavaScript in page context

**Inspection:**
- `screenshot` — full-page screenshot saved to file
- `screenshot <selector>` — element screenshot
- `cookies` — list cookies
- `sources` — list JS/CSS sources

**Output context:**
- `context` — show current output directory
- `push-context <name>` — push new context (creates subdirectory, redirects output)
- `pop-context` — pop context (returns to parent directory)

**Tab management:**
- `tabs` / `lt` — list open browser tabs
- `newtab [url]` / `nt` — open new tab
- `tab <n|text>` / `t` — switch to tab by index or title/URL substring

### Inspecting Captured Traffic

Each line in the `.jsonl` is a HAR entry. Extract RPC info with:

```bash
# List all batchexecute RPC IDs in a capture
python3 -c "
import json, re, sys, urllib.parse
for line in open(sys.argv[1]):
    d = json.loads(line)
    req = d['request']
    url = req['url']
    if 'batchexecute' not in url:
        continue
    m = re.search(r'rpcids=([^&]+)', url)
    rpcid = urllib.parse.unquote(m.group(1)) if m else '?'
    method = req['method']
    resp = d.get('response')
    status = resp['status'] if resp else 'pending'
    body_len = len(resp.get('content',{}).get('text','')) if resp and resp.get('content') else 0
    post_len = len(req.get('postData',{}).get('text','')) if req.get('postData') else 0
    print(f'  {rpcid:10s} {method:4s} status={status} post={post_len:>5} resp_body={body_len:>6}')
" "$CAPDIR/<action-name>/notebooklm.google.com.jsonl"
```

### Full dump of a single HAR entry (for debugging):

```bash
python3 -c "
import json, sys
d = json.loads(open(sys.argv[1]).readlines()[int(sys.argv[2])])
print(json.dumps(d, indent=2)[:2000])
" "$CAPDIR/<action-name>/notebooklm.google.com.jsonl" 0
```

### Count requests by domain in a capture:

```bash
for f in "$CAPDIR/<action-name>"/*.jsonl; do
    count=$(wc -l < "$f" | tr -d ' ')
    domain=$(basename "$f" .jsonl)
    echo "  $count $domain"
done | sort -rn
```

## RPC ID → Operation Name Mapping

Defined in `/Volumes/tmc/go/src/github.com/tmc/nlm/internal/notebooklm/rpc/rpc.go`:

**Project ops:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `wXbhsf` | ListRecentlyViewedProjects   |
| `CCqFvf` | CreateProject                |
| `rLM1Ne` | GetProject                   |
| `WWINqb` | DeleteProjects               |
| `s0tc2d` | MutateProject                |
| `fejl7e` | RemoveRecentlyViewedProject  |

**Source ops:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `izAoDd` | AddSources                   |
| `o4cbdc` | AddFileSource                |
| `tGMBJ`  | DeleteSources                |
| `b7Wfje` | MutateSource (rename)        |
| `FLmJqe` | RefreshSource                |
| `hizoJc` | LoadSource                   |
| `yR9Yof` | CheckSourceFreshness         |
| `yyryJe` | ActOnSources                 |
| `qXyaNe` | DiscoverSources              |

**Note ops:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `CYK0Xb` | CreateNote                   |
| `cYAfTb` | MutateNote                   |
| `AH0mwd` | DeleteNotes                  |
| `cFji9`  | GetNotes                     |

**Audio/Video:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `AHyHrd` | CreateAudioOverview          |
| `VUsiyb` | GetAudioOverview             |
| `sJDbic` | DeleteAudioOverview          |
| `sqTeoe` | GetAudioFormats              |
| `R7cb6c` | CreateVideoOverview          |

**Chat:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `hPTbtc` | GetConversations             |
| `khqZz`  | GetConversationHistory       |
| `e3bVqc` | DeleteChatHistory            |
| `J7Gthc` | RateConversationTurn         |

**Generation:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `tr032e` | GenerateDocumentGuides       |
| `VfAZjd` | GenerateNotebookGuide        |
| `lCjAd`  | GenerateOutline              |
| `BeTrYd` | GenerateSection              |
| `exXvGf` | StartDraft                   |
| `pGC7gf` | StartSection                 |
| `GHsKob` | GenerateReportSuggestions    |

**Account/Misc:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `ZwVcOc` | GetOrCreateAccount           |
| `hT54vc` | MutateAccount                |
| `ozz5Z`  | LogEvent                     |
| `uNyJKe` | SubmitFeedback               |

**Sharing:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `RGP97b` | ShareAudio                   |
| `JFMDGd` | GetProjectDetails            |
| `QDyure` | ShareProject                 |

**Artifacts:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `xpWGLf` | CreateArtifact               |
| `BnLyuf` | GetArtifact                  |
| `gArtLc` | ListArtifacts                |
| `rc3d8d` | RenameArtifact               |
| `WxBZtb` | DeleteArtifact               |
| `DJezBc` | UpdateArtifact               |

**Guidebooks:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `EYqtU`  | GetGuidebook                 |
| `R6smae` | PublishGuidebook             |
| `LJyzeb` | GetGuidebookDetails          |
| `OTl0K`  | ShareGuidebook               |
| `itA0pc` | GuidebookGenerateAnswer      |
| `ARGkVc` | DeleteGuidebook              |
| `YJBpHc` | ListRecentlyViewedGuidebooks |

**Featured:**
| RPC ID   | Operation                    |
|----------|------------------------------|
| `nS9Qlc` | ListFeaturedProjects         |
| `rJKx8e` | ReportContent                |

## Test Notebook

Notebook ID: `2ed71b32-63bb-4c22-a779-210d4f9bec5f` ("nlm source code") — has 18 sources, good for testing source/notes/chat/generation operations.

URL: `https://notebooklm.google.com/notebook/2ed71b32-63bb-4c22-a779-210d4f9bec5f`

## Actions to Capture

Work through these systematically, one at a time. For each action:
1. `push-context <action-name>`
2. Trigger the action
3. Wait 3-5 seconds for RPCs to settle
4. `pop-context`
5. Inspect the capture with the python snippet above

### Phase 1: Page Load & Navigation
1. **homepage-load** — Navigate to `https://notebooklm.google.com` (captures GetOrCreateAccount, ListRecentlyViewedProjects, etc.)
2. **open-notebook** — Click into the test notebook (captures GetProject, GetNotes, ListArtifacts, GetConversations, etc.)

### Phase 2: Source Operations
3. **list-sources** — View the sources panel (may already be loaded from step 2)
4. **add-text-source** — Add a small text source via the UI
5. **rename-source** — Rename a source
6. **delete-source** — Delete the source just added

### Phase 3: Note Operations
7. **list-notes** — View notes panel
8. **create-note** — Create a new note
9. **delete-note** — Delete the note

### Phase 4: Generation
10. **generate-guide** — Trigger notebook guide generation
11. **generate-outline** — Trigger outline generation

### Phase 5: Chat
12. **chat-send** — Send a chat message (this uses gRPC-Web, not batchexecute — different protocol, look for requests to a different endpoint)
13. **chat-history** — Load chat history

### Phase 6: Audio
14. **audio-create** — Create an audio overview
15. **audio-list** — List audio overviews

## Important Notes

- **Body capture limitation**: The current cdp `-harl` simple mode may not capture POST request bodies or response bodies (see `/Volumes/tmc/go/src/github.com/tmc/nlm/cdp-issues.txt` issue #4). Even without bodies, the RPC IDs, URLs, headers, and timing are valuable for understanding the protocol.
- **Wait for RPCs**: After triggering an action, wait 3-5 seconds before `pop-context`. Some operations trigger multiple sequential RPCs.
- **NotebookLM is a SPA**: Most interactions don't cause page navigations — they fire batchexecute RPCs via XHR. Use `eval` to trigger clicks on elements that are hard to target with CSS selectors.
- **The batchexecute URL pattern**: All NLM API calls go to `https://notebooklm.google.com/_/LabsTailwindUi/data/batchexecute?rpcids=XXX&...`
- **Chat uses gRPC-Web**: Chat messages use a different endpoint pattern — look for requests that don't match batchexecute.
- **Unknown RPC IDs**: If you see an RPC ID not in the mapping above, note it — it may be a new or undocumented endpoint.

## Output

After capturing each action, report:
1. What RPC IDs fired (with operation names from the mapping)
2. How many requests total (batchexecute + other)
3. Whether POST data and response bodies were present (non-zero lengths)
4. Any unexpected or unknown RPCs
5. Any errors observed

Save a summary to `/Volumes/tmc/go/src/github.com/tmc/nlm/docs/captured-actions-summary.md` with a table like:

```markdown
| Action | Context Dir | RPC IDs | Notes |
|--------|------------|---------|-------|
| homepage-load | homepage-load/ | ZwVcOc, wXbhsf, ... | Initial page load RPCs |
| open-notebook | open-notebook/ | rLM1Ne, cFji9, ... | Notebook detail RPCs |
```
