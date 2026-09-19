package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type chatViewRow struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Status   string    `json:"status"`
	StatusAt time.Time `json:"status_at"`
	Updated  time.Time `json:"updated"`
	Turns    int       `json:"turns"`
	Path     string    `json:"-"`
}

func chatViewRows(notebookID string) ([]chatViewRow, error) {
	summaries, err := listLocalChatSessionSummaries(notebookID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]chatSessionSummary)
	for _, s := range summaries {
		if s.ConversationID == "" || s.MessageCount == 0 {
			continue
		}
		old, ok := byID[s.ConversationID]
		if !ok || s.UpdatedAt.After(old.UpdatedAt) || s.UpdatedAt.Equal(old.UpdatedAt) && s.Path < old.Path {
			byID[s.ConversationID] = s
		}
	}
	rows := make([]chatViewRow, 0, len(byID))
	for _, s := range byID {
		title := s.Title
		if title == "" {
			title = s.ConversationID
		}
		// Prefer the per-conversation sidecar even when the alias won the tie.
		path := getChatSessionPathForConv(notebookID, s.ConversationID)
		if _, err := os.Stat(path); err == nil {
			s.Path = path
		}
		rows = append(rows, chatViewRow{ID: s.ConversationID, Title: title, Turns: (s.MessageCount + 1) / 2, Updated: s.UpdatedAt,
			Status: resolveChatStatus(s.Status, s.StatusAt, s.WriterPID, path, time.Now()), StatusAt: s.StatusAt, Path: s.Path})
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].Updated.Equal(rows[j].Updated) {
			return rows[i].Updated.After(rows[j].Updated)
		}
		return rows[i].ID < rows[j].ID
	})
	return rows, nil
}

// viewRenderContext reads only saved metadata. Serving a page must not issue RPCs.
func viewRenderContext(notebookID string, opts chatRenderOptions) chatRenderContext {
	titles, _ := loadSourceTitles(notebookID)
	return chatRenderContext{ShowThinking: opts.ShowThinking, ExcerptBudget: opts.ExcerptBudget,
		HideConfidence: opts.HideConfidence, HideSpans: opts.HideSpans, IncludeFollowUps: opts.IncludeFollowUps,
		ResolveTitle: func(id string) string { return titles[id] }}
}

func renderChatViewBody(w io.Writer, path string, opts chatRenderOptions) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var session chatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return err
	}
	doc := notebookDocumentFromSession(&session)
	if !opts.Live {
		if err := appendChatPartial(&doc, &session); err != nil {
			return err
		}
	}
	return renderChatHTML(w, doc, viewRenderContext(session.NotebookID, opts))
}

func chatShowIndex(notebookID string, opts chatRenderOptions) error {
	rows, err := chatViewRows(notebookID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("no local chat sessions for notebook %s", notebookID)
	}
	path, err := notebookHTMLDestination(notebookID, opts.OutFile)
	if err != nil {
		return err
	}
	base := path
	if base == "" {
		base, err = notebookHTMLDestination(notebookID, "")
		if err != nil {
			return err
		}
	}
	config := fmt.Sprintf("%v/%d/%v/%v/%v/%v", opts.ShowThinking, opts.ExcerptBudget, opts.HideConfidence, opts.HideSpans, opts.IncludeFollowUps, opts.ResolveCitations)
	signature := fmt.Sprintf("%x", sha256.Sum256([]byte(config)))
	for i := range rows {
		row := &rows[i]
		// IDs are data, not filesystem paths.
		name := url.PathEscape(row.ID) + ".html"
		bodyPath := filepath.Join(filepath.Dir(base), name)
		src, err := os.Stat(row.Path)
		if err != nil {
			return err
		}
		dst, err := os.Stat(bodyPath)
		cachedConfig, _ := os.ReadFile(bodyPath + ".config")
		if opts.Rebuild || row.Status != "" || err != nil || dst.ModTime().Before(src.ModTime()) || string(cachedConfig) != signature {
			if opts.ResolveCitations || opts.ExcerptBudget > 0 {
				bodyOpts := opts
				bodyOpts.OutFile, bodyOpts.Open = bodyPath, false
				if err := chatShow(notebookID, row.ID, bodyOpts); err != nil {
					return err
				}
			} else {
				var b bytes.Buffer
				if err := renderChatViewBody(&b, row.Path, opts); err != nil {
					return err
				}
				if err := writeChatSessionFile(bodyPath, b.Bytes()); err != nil {
					return err
				}
			}
			if err := writeChatSessionFile(bodyPath+".config", []byte(signature)); err != nil {
				return err
			}
		}
		row.URL = url.PathEscape(name)
		if path == "" {
			row.URL = (&url.URL{Scheme: "file", Path: bodyPath}).String()
		}
	}
	var b bytes.Buffer
	if err := chatViewTemplate.Execute(&b, chatViewPage{Notebook: notebookID, Rows: rows}); err != nil {
		return err
	}
	if path == "" {
		_, err = os.Stdout.Write(b.Bytes())
		return err
	}
	if err := writeChatSessionFile(path, b.Bytes()); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "nlm: wrote %s\n", path)
	if opts.Open {
		return openInBrowser(path)
	}
	return nil
}

type chatViewPage struct {
	Notebook     string
	Rows         []chatViewRow
	Live         bool
	Conversation string
	BodyURL      string
	EventsURL    string
}

var chatViewTemplate = template.Must(template.New("chat-view").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Conversations · {{.Notebook}}</title>
<style>
:root{color-scheme:light dark;font:16px/1.5 system-ui,sans-serif}body{max-width:1100px;margin:40px auto;padding:0 24px}h1{font-size:24px;margin-bottom:4px}header p{opacity:.65;margin-top:0}table{width:100%;border-collapse:collapse;margin:24px 0}td,th{text-align:left;padding:12px 8px;border-bottom:1px solid #8884}th{font-size:12px;text-transform:uppercase;letter-spacing:.06em;opacity:.65}a{color:inherit;text-underline-offset:3px}td:first-child{width:60%}.badge{font-size:13px;white-space:nowrap}iframe{width:100%;height:65vh;border:1px solid #8884;border-radius:8px;background:white}pre{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit}#partial{border-left:3px solid #8888;padding:0 20px}#connection{font-size:13px}time{white-space:nowrap} @media(max-width:650px){body{padding:0 12px;margin:20px auto}.updated{display:none}td:first-child{width:auto}}
</style></head><body>
<header><h1>Conversations</h1><p>Notebook {{.Notebook}} · <span id="count">{{len .Rows}}</span> conversations{{if .Live}} · Live{{end}}</p></header>
<table><thead><tr><th>Conversation</th><th>Turns</th><th class="updated">Updated</th><th>Status</th></tr></thead><tbody id="rows">
{{range .Rows}}<tr data-id="{{.ID}}"><td><a href="{{.URL}}">{{.Title}}</a></td><td>{{.Turns}}</td><td class="updated"><time>{{.Updated.Format "Jan 2 15:04"}}</time></td><td class="badge" data-status="{{.Status}}" data-at="{{.StatusAt.Format "2006-01-02T15:04:05Z07:00"}}">{{.Status}}</td></tr>{{end}}
</tbody></table>
{{if .Live}}<p id="connection" role="status">Connecting…</p>
{{if .Conversation}}<iframe id="conversation" title="Saved conversation" src="{{.BodyURL}}" sandbox="allow-scripts allow-same-origin allow-popups"></iframe><section id="partial" hidden><h2 id="phase">Generating</h2><pre id="answer"></pre></section>{{else}}<p>Choose a conversation to follow its answer.</p>{{end}}
<script>
const events = new EventSource({{.EventsURL}}), selected = {{.Conversation}};
let generation='', answer='', finished=false;
const partial=document.getElementById('partial'), output=document.getElementById('answer'), frame=document.getElementById('conversation');
if(frame)frame.addEventListener('load',()=>{frame.style.height=Math.max(240,frame.contentDocument.documentElement.scrollHeight)+'px';});
function reloadBody(){if(frame){const u=new URL(frame.src);u.searchParams.set('v',Date.now());frame.src=u.toString();}}
events.onopen=()=>document.getElementById('connection').textContent='Live updates connected';
events.onerror=()=>document.getElementById('connection').textContent='Disconnected · reconnecting…';
events.addEventListener('rows',event=>{
 const rows=JSON.parse(event.data), tbody=document.getElementById('rows'), ids=new Set();
 for(const row of rows){
  ids.add(row.id);let tr=Array.from(tbody.children).find(e=>e.dataset.id===row.id);
  if(!tr){tr=document.createElement('tr');tr.dataset.id=row.id;for(let i=0;i<4;i++)tr.append(document.createElement('td'));tr.children[0].append(document.createElement('a'));}
  const a=tr.children[0].firstChild;a.href=row.url;a.textContent=row.title;tr.children[1].textContent=row.turns;
  tr.children[2].className='updated';tr.children[2].textContent=new Date(row.updated).toLocaleString();
  const status=tr.children[3];status.className='badge';status.dataset.status=row.status;status.dataset.at=row.status_at;status.textContent=row.status;
  tbody.append(tr);
  if(row.id===selected && frame){
   const next=row.status_at;
   if(next!==generation){generation=next;answer='';output.textContent='';finished=false;reloadBody();}
   if(row.status==='generating'||row.status==='interrupted'||row.status==='truncated'||row.status==='failed'){
    partial.hidden=false;document.getElementById('phase').textContent=row.status==='generating'?'Generating':row.status+' · partial answer';
   }else{partial.hidden=true;if(!finished){reloadBody();finished=true;}}
  }
 }
 for(const tr of Array.from(tbody.children))if(!ids.has(tr.dataset.id))tr.remove();document.getElementById('count').textContent=rows.length;
});
events.addEventListener('partial',event=>{
 if(!output)return;const e=JSON.parse(event.data);
 if(e.phase==='reset'){answer='';output.textContent='';}
 if(e.phase==='answer')answer+=e.text;
 if(e.phase==='revised')answer=e.full;
 if(e.phase==='done'){answer=e.answer;reloadBody();partial.hidden=true;finished=true;}
 output.textContent=answer;
});
setInterval(()=>{for(const e of document.querySelectorAll('[data-status="generating"]')){const seconds=Math.max(0,Math.floor((Date.now()-Date.parse(e.dataset.at))/1000));e.textContent='generating '+String(Math.floor(seconds/60)).padStart(2,'0')+':'+String(seconds%60).padStart(2,'0');}},1000);
</script>{{end}}</body></html>`))
