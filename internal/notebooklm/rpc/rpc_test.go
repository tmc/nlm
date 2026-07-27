package rpc

import (
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

func TestNewWithConfigUsesExplicitAuthUser(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "3")

	client := NewWithConfig("token", "cookies", ServiceConfig{
		URLParams: map[string]string{
			"authuser": "1",
		},
	},
		batchexecute.WithURLParams(map[string]string{"authuser": "2"}),
		batchexecute.WithHeaders(map[string]string{"x-goog-authuser": "2"}),
	)

	if got := client.Config.URLParams["authuser"]; got != "2" {
		t.Fatalf("authuser URL param = %q, want 2", got)
	}
	if got := client.Config.Headers["x-goog-authuser"]; got != "2" {
		t.Fatalf("x-goog-authuser header = %q, want 2", got)
	}
}

func TestNewWithConfigRetainsAppliedOptions(t *testing.T) {
	client := NewWithConfig(
		"token",
		"cookies",
		ServiceConfig{},
		batchexecute.WithDebug(true),
		batchexecute.WithProtoDebug(true, true),
	)

	if !client.Config.Debug {
		t.Error("Debug = false, want true")
	}
	if !client.Config.DebugParsing {
		t.Error("DebugParsing = false, want true")
	}
	if !client.Config.DebugFieldMapping {
		t.Error("DebugFieldMapping = false, want true")
	}
}

func TestNewWithConfigUsesCurrentNotebookHost(t *testing.T) {
	for _, host := range []string{"", "notebooklm.google.com", "notebook.google.com"} {
		t.Run(host, func(t *testing.T) {
			client := NewWithConfig("token", "cookies", ServiceConfig{Host: host})
			if got := client.Config.Host; got != "notebook.google.com" {
				t.Fatalf("Host = %q, want notebook.google.com", got)
			}
		})
	}
}
