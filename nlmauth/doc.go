// Package nlmauth resolves, persists, and refreshes NotebookLM web-session
// credentials so that any tool built on github.com/tmc/nlm can share one
// authentication store.
//
// Credentials come from a signed-in NotebookLM browser session: an auth token
// (the "at" value) and the raw Cookie header. Harvesting them from a live
// browser is a workstation operation and lives behind the nlm command
// (`nlm auth`); this package covers everything else — reading the harvested
// credentials, writing them back, and refreshing an expiring session over
// HTTP.
//
// Typical use by a downstream tool:
//
//	creds, err := nlmauth.Load()
//	if err != nil {
//		log.Fatalf("no NotebookLM session; run 'nlm auth': %v", err)
//	}
//	client := notebooklm.New(notebooklm.Credentials{
//		AuthToken: creds.AuthToken,
//		Cookies:   creds.Cookies,
//	})
//
// Load resolves credentials from the environment (NLM_AUTH_TOKEN, NLM_COOKIES,
// NLM_AUTHUSER). A complete environment session takes precedence; otherwise
// missing fields come from the current stored identity. Save writes a full
// [Session] to that identity and its $HOME/.nlm/env compatibility mirror.
// Refresh renews the current stored session in place.
//
// [OpenStore] supports named sessions in local files. Use [SaveIdentity] to
// update another identity without changing the current selection.
package nlmauth
