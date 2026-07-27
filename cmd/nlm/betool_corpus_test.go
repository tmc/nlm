package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditCorpusFile(t *testing.T) {
	const request = `f.req=%5B%5B%5B%22LQhfEb%22%2C%22%5B%5B2%2Cnull%2Cnull%2C%5B1%2Cnull%2Cnull%2Cnull%2Cnull%2Cnull%2Cnull%2Cnull%2Cnull%2Cnull%2C%5B1%5D%5D%5D%2C%5C%22project-id%5C%22%2C%5Bnull%2C%5Bnull%2C2%5D%5D%2C%5B%5B%5C%22notebook_lm_state.saved_source_panel_view%5C%22%5D%5D%5D%22%2Cnull%2C%22generic%22%5D%5D%5D&at=TOKEN&`
	const response = `)]}'

[["wrb.fr","LQhfEb","[[null,[null,2]]]",null,null,null,"generic"]]
`
	entry := corpusTrafficEntry{}
	entry.Request.URL = "https://notebooklm.google.com/_/data/batchexecute?rpcids=LQhfEb"
	entry.Request.PostData.Text = base64.StdEncoding.EncodeToString([]byte(request))
	entry.Response.Status = 200
	entry.Response.Content.Text = response
	line, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "traffic.jsonl")
	if err := os.WriteFile(path, append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	var audit corpusAudit
	if err := auditCorpusFile(&audit, path); err != nil {
		t.Fatal(err)
	}
	if len(audit.Records) != 2 {
		t.Fatalf("records = %d, want 2: %+v", len(audit.Records), audit.Records)
	}
	for _, record := range audit.Records {
		if record.RPCID != "LQhfEb" || record.Status != "lossless" {
			t.Fatalf("record = %+v, want lossless LQhfEb", record)
		}
	}
}

func TestAuditCorpusFileAccountsNonRPC(t *testing.T) {
	entry := corpusTrafficEntry{}
	entry.Request.Method = "GET"
	entry.Request.URL = "https://accounts.google.com/ServiceLogin?continue=https%3A%2F%2Fnotebooklm.google.com%2F"
	entry.Response.Status = 302
	line, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "traffic.jsonl")
	if err := os.WriteFile(path, append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	var audit corpusAudit
	if err := auditCorpusFile(&audit, path); err != nil {
		t.Fatal(err)
	}
	if len(audit.Records) != 1 {
		t.Fatalf("records = %d, want 1: %+v", len(audit.Records), audit.Records)
	}
	record := audit.Records[0]
	if record.Side != "record" || record.Status != "non_rpc_http" || record.HTTPStatus != 302 {
		t.Fatalf("record = %+v, want non-RPC HTTP 302", record)
	}
	var evidence struct {
		Method string `json:"method"`
		Host   string `json:"host"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(record.Evidence, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Method != "GET" || evidence.Host != "accounts.google.com" || evidence.Path != "/ServiceLogin" {
		t.Fatalf("evidence = %+v", evidence)
	}
}
