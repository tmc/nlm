# HAR Capture Session — Fixing Broken NLM Commands

## Goal

Capture network traffic for the broken/untested NLM CLI commands so we can
inspect the real wire format and fix our implementations. We need HAR data
for these specific operations:

1. **audio-download** — response parsing returns 0 elements
2. **audio-share** — returns empty share URL
3. **analytics** — source count wire format mismatch
4. **list-artifacts** / **get-artifact** / **create-artifact** — server returns 400
5. **share-details** — response parsing incomplete
6. **delete-chat** — untested
7. **chat-config** (goal/length) — untested
8. **feedback** — untested
9. **chat streaming** — thinking trace format, citation structure

## Environment

### Sessions

| Session | ID | Purpose |
|---------|----|---------|
| CDP shell | `2E84DB09` | Brave browser connected to NotebookLM, HAR capture |
| Bash | `574E776D` | Run nlm commands, inspect captures |
| This session | `B9CE8566` | Coordination |

### Communication Pattern

Send commands to CDP shell:
```bash
CDP="2E84DB09"
BASH="574E776D"
it2 session send-text "$CDP" 'command here
'
sleep 3
it2 session get-screen "$CDP"
```

Send commands to bash session:
```bash
it2 session send-text "$BASH" 'command here
'
sleep 2
it2 session get-buffer "$BASH" --lines 30
```

### Capture Directory
```
CAPDIR=~/go/src/github.com/tmc/misc/chrome-to-har/logs/nlm-capture
```

### Test Notebook
```
NB=2ed71b32-63bb-4c22-a779-210d4f9bec5f
URL=https://notebooklm.google.com/notebook/$NB
```

## Known CDP Limitations

**CRITICAL — read before starting:**

1. **No response bodies** (cdp-issues.txt #4): The HAR recorder streams entries
   on `EventResponseReceived` before bodies are fetched. All `.jsonl` entries have
   empty response bodies. **Workaround**: After triggering the action in CDP, also
   run the equivalent `nlm -debug` command in 574E to see the actual response data.

2. **No gRPC-Web capture** (cdp-issues.txt #7): Chat streaming uses `fetch()` with
   `ReadableStream`, which the HAR recorder misses entirely. **Workaround**: Use
   `nlm -debug chat` in 574E to capture chat protocol data.

3. **No POST bodies**: Request bodies are also missing from HAR entries.
   **Workaround**: Use `nlm -debug` which logs request payloads.

4. **`eval` is broken** (cdp-issues.txt #6): `eval <js>` fails with "expected
   Domain.method". **Workaround**: Use `Runtime.evaluate {"expression":"<js>"}` directly.

5. **`click` breaks on quoted selectors** (#8): Use `Runtime.evaluate` instead of `click`.

### What HAR CAN capture
- RPC IDs from URL query params (`?rpcids=XXX`)
- Request/response headers
- Status codes
- Timing information
- Number and sequence of RPCs fired per action

### Supplemental capture strategy
For each action, do TWO things in parallel:
1. **CDP**: `push-context` → trigger UI action → `pop-context` (captures RPC IDs, headers, timing)
2. **574E**: Run `nlm -debug <command>` (captures request/response bodies, error details)

## Capture Plan

### Phase 1: Audio Operations (audio-download, audio-share)

**1a. Audio download via UI**
```
cdp> push-context audio-download
```
Navigate to a notebook that has an existing audio overview. Click the download
button in the audio player UI. The RPC ID should be related to `GetAudioFormats`
(`sqTeoe`) or a direct media URL fetch.
```
cdp> pop-context
```

In 574E, simultaneously:
```bash
nlm -debug audio-download $NB test.wav 2>&1 | tee /tmp/audio-download-debug.txt
```

**1b. Audio share via UI**
```
cdp> push-context audio-share
```
Click the share button on the audio overview. Look for `RGP97b` (ShareAudio) RPC.
```
cdp> pop-context
```

In 574E:
```bash
nlm -debug audio-share $NB 2>&1 | tee /tmp/audio-share-debug.txt
```

### Phase 2: Analytics

**2a. Analytics via UI**
```
cdp> push-context analytics
```
Navigate to the notebook. The analytics data may load automatically with
`GetProject` (`rLM1Ne`), or there may be a separate analytics panel. Look for
`AUrzMb` (GetProjectAnalytics) in the captured RPC IDs.
```
cdp> pop-context
```

In 574E:
```bash
nlm -debug analytics $NB 2>&1 | tee /tmp/analytics-debug.txt
```

The analytics proto now uses wrapper counts, so the main remaining question is
whether the server has added any new fields beyond the current count/timestamp
shape.

### Phase 3: Artifacts

**3a. Create artifact via UI**
```
cdp> push-context create-artifact
```
In the notebook, find the "Create" or "New artifact" button. Click it and select
each type (note, audio, report). Look for `xpWGLf` (CreateArtifact) RPC.
```
cdp> pop-context
```

**3b. List artifacts via UI**
```
cdp> push-context list-artifacts
```
Open the artifacts panel. Look for `gArtLc` (ListArtifacts) RPC.
```
cdp> pop-context
```

In 574E:
```bash
nlm -debug list-artifacts $NB 2>&1 | tee /tmp/list-artifacts-debug.txt
nlm -debug create-artifact $NB note 2>&1 | tee /tmp/create-artifact-debug.txt
```

### Phase 4: Chat Config

**4a. Set chat goal via UI**
```
cdp> push-context chat-config-goal
```
Open chat settings. Change the conversation goal (e.g., to "Custom" with a
prompt). Look for `s0tc2d` (MutateProject) RPC — chat config is set via
project mutation.
```
cdp> pop-context
```

**4b. Set response length via UI**
```
cdp> push-context chat-config-length
```
Change the response length setting (shorter/longer). Same RPC: `s0tc2d`.
```
cdp> pop-context
```

In 574E:
```bash
nlm -debug chat-config $NB goal custom "Be concise" 2>&1 | tee /tmp/chat-config-goal-debug.txt
nlm -debug chat-config $NB length shorter 2>&1 | tee /tmp/chat-config-length-debug.txt
```

### Phase 5: Delete Chat History

```
cdp> push-context delete-chat
```
In the chat panel, find "Delete conversation" or "Clear history". Look for
`e3bVqc` (DeleteChatHistory) RPC.
```
cdp> pop-context
```

In 574E:
```bash
nlm -debug -yes delete-chat $NB 2>&1 | tee /tmp/delete-chat-debug.txt
```

### Phase 6: Feedback

```
cdp> push-context feedback
```
Find the feedback/report button in the UI. Submit a test feedback message.
Look for `uNyJKe` (SubmitFeedback) RPC.
```
cdp> pop-context
```

In 574E:
```bash
nlm -debug feedback "test feedback from cli" 2>&1 | tee /tmp/feedback-debug.txt
```

### Phase 7: Chat Streaming (citations + thinking)

gRPC-Web traffic won't appear in HAR captures (issue #7). Use 574E exclusively:

```bash
nlm -debug -verbose chat $NB "What are the main topics covered in my sources? Cite specific sources." 2>&1 | tee /tmp/chat-stream-debug.txt
```

This captures:
- Whether thinking chunks start with `**` (needed for phase detection)
- Citation format in inner JSON position `[0][4]`
- Full chunk structure for debugging the first-answer truncation bug

### Phase 8: Share Details

```
cdp> push-context share-details
```
First share the notebook to get a share ID, then view the share details page.
Look for `JFMDGd` (GetProjectDetails) RPC.
```
cdp> pop-context
```

## Inspecting Captures

After each action, inspect the HAR with:

```bash
# List RPC IDs in a capture
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
    print(f'  {rpcid:10s} {method:4s} status={status}')
" "$CAPDIR/<context>/notebooklm.google.com.jsonl"
```

```bash
# Count requests per domain
for f in "$CAPDIR/<context>"/*.jsonl; do
    count=$(wc -l < "$f" | tr -d ' ')
    domain=$(basename "$f" .jsonl)
    echo "  $count $domain"
done | sort -rn
```

## RPC ID Quick Reference

| RPC ID | Operation | Broken Command |
|--------|-----------|----------------|
| `sqTeoe` | GetAudioFormats | audio-download |
| `RGP97b` | ShareAudio | audio-share |
| `AUrzMb` | GetProjectAnalytics | analytics |
| `xpWGLf` | CreateArtifact | create-artifact |
| `gArtLc` | ListArtifacts | list-artifacts |
| `BnLyuf` | GetArtifact | get-artifact |
| `s0tc2d` | MutateProject | chat-config |
| `e3bVqc` | DeleteChatHistory | delete-chat |
| `uNyJKe` | SubmitFeedback | feedback |
| `JFMDGd` | GetProjectDetails | share-details |

## Output

After each capture phase, save findings to `/tmp/har-analysis-<phase>.txt`
with:
- RPC IDs observed
- Status codes
- Any error messages from debug output
- Wire format observations (from `nlm -debug` output)
- Suggested fixes for the broken command

Final summary goes to:
```
/Volumes/tmc/go/src/github.com/tmc/nlm/docs/har-capture-results.md
```
