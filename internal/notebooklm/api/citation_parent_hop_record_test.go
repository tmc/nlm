//go:build integration
// +build integration

package api

import (
	"net/http"
	"os"
	"testing"

	"github.com/tmc/nlm/internal/httprr"
)

// TestCaptureCitationParentHop records the two frames Phase 1 of the §9
// parent-hop verification needs, into one httprr fixture: a
// GetConversationHistory (khqZz) response for a citation-bearing conversation,
// and the GetProject source list for the same notebook. Both go through the
// injectable RPC/service client, so httprr can record them — unlike the live
// GenerateFreeFormStreamed stream, whose HTTP client is built internally.
//
// The offline assertion test (non-integration) reads this fixture and proves
// that every citation slot's [5]-outer parent id is a member of the notebook's
// GetProject source ids — i.e. that the discarded parent slot, not the persisted
// [6] chunk id, is the resolvable source. Run to (re)record:
//
//	NLM_AUTH_TOKEN=… NLM_COOKIES=… go test -tags=integration \
//	  -run TestCaptureCitationParentHop -httprecord=. ./internal/notebooklm/api/
//
// The notebook/conversation are the live 6c313fd7 turn already verified to carry
// 145 citations (79 distinct chunk ids) against 8 GetProject sources.
func TestCaptureCitationParentHop(t *testing.T) {
	httprr.SkipIfNoNLMCredentialsOrRecording(t)
	httpClient := httprr.CreateNLMTestClient(t, http.DefaultTransport)

	authToken := "test-auth-token"
	cookies := "test-cookies"
	if os.Getenv("NLM_AUTH_TOKEN") != "" {
		authToken = os.Getenv("NLM_AUTH_TOKEN")
	}
	if os.Getenv("NLM_COOKIES") != "" {
		cookies = os.Getenv("NLM_COOKIES")
	}

	client := New(
		Credentials{AuthToken: authToken, Cookies: cookies},
		WithHTTPClient(httpClient),
		WithDebug(false),
	)

	const projectID = "6c313fd7-049a-4475-aa0f-0fb3ee8de65f"

	// Frame 1: hPTbtc — the notebook's server-side conversation IDs. A
	// generate-chat (live stream) conversation is NOT in this history; only a
	// server-saved conversation is, and only its ID resolves through khqZz. So we
	// discover the ID here rather than hard-coding a local generate-chat ID.
	convs, err := client.GetConversations(projectID)
	if err != nil {
		t.Fatalf("GetConversations: %v", err)
	}
	if len(convs) == 0 {
		t.Fatal("no server-side conversations — nothing for khqZz to return")
	}
	conversationID := convs[0]
	t.Logf("server conversation: %s", conversationID)

	// Frame 2: khqZz — the conversation history carrying the citation slots.
	msgs, err := client.GetConversationHistory(projectID, conversationID)
	if err != nil {
		t.Fatalf("GetConversationHistory: %v", err)
	}
	var cited int
	for _, m := range msgs {
		cited += len(m.Citations)
	}
	t.Logf("history: %d messages, %d citations", len(msgs), cited)
	if cited == 0 {
		t.Fatal("recorded a conversation with no citations — fixture cannot prove the parent hop")
	}

	// Frame 3: GetProject — the source list to match [5]-outer parents against.
	nb, err := client.GetProject(projectID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	t.Logf("project %q: %d sources", nb.Title, len(nb.Sources))
	if len(nb.Sources) == 0 {
		t.Fatal("recorded a project with no sources — nothing to match parents against")
	}
}
