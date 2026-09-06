package main

import (
	"os"
	"testing"

	"github.com/tmc/nlm/nlmauth"
)

func isolateIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NLM_CREDENTIAL_STORE", "file")
	oldName, oldSource := activeIdentity, activeIdentitySource
	oldToken, oldCookies, oldUser := authToken, cookies, authUser
	t.Cleanup(func() {
		activeIdentity, activeIdentitySource = oldName, oldSource
		authToken, cookies, authUser = oldToken, oldCookies, oldUser
	})
	activeIdentity, activeIdentitySource = "", identityUnset
	for _, key := range []string{"NLM_AUTH_TOKEN", "NLM_COOKIES", "NLM_AUTHUSER", "NLM_BROWSER_PROFILE", "NLM_SESSION_ID", "NLM_BL_PARAM", "NLM_SIGNALER_AUTH"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestIdentitySelectionKeepsEmptyFields(t *testing.T) {
	isolateIdentity(t)
	if err := nlmauth.Save(nlmauth.Session{Credentials: nlmauth.Credentials{AuthToken: "old", Cookies: "old", AuthUser: "3"}, BrowserProfile: "Old"}); err != nil {
		t.Fatal(err)
	}
	applyIdentitySession("work", identityFromFlag, nlmauth.Session{Credentials: nlmauth.Credentials{AuthToken: "new", Cookies: "new"}})
	loadStoredEnv()
	for _, key := range []string{"NLM_AUTHUSER", "NLM_BROWSER_PROFILE"} {
		if value := os.Getenv(key); value != "" {
			t.Fatalf("%s = %q, want empty", key, value)
		}
	}
}

func TestRefreshPersistsSelectedIdentity(t *testing.T) {
	isolateIdentity(t)
	personal := nlmauth.Session{Credentials: nlmauth.Credentials{AuthToken: "personal", Cookies: "personal", AuthUser: "1"}, BrowserProfile: "Personal"}
	work := nlmauth.Session{Credentials: nlmauth.Credentials{AuthToken: "work", Cookies: "work", AuthUser: "3"}, BrowserProfile: "Work"}
	if err := nlmauth.Save(personal); err != nil {
		t.Fatal(err)
	}
	if err := nlmauth.SaveIdentity("work", work); err != nil {
		t.Fatal(err)
	}
	applyIdentitySession("work", identityFromFlag, work)
	authToken, cookies, authUser = work.AuthToken, work.Cookies, work.AuthUser
	if profile, user, ok := cachedBrowserProfile(); !ok || profile != "Work" || user != "3" {
		t.Fatalf("cached profile = %q, %q, %v", profile, user, ok)
	}
	if _, _, err := persistAuthToDisk("fresh", "fresh", "", "fresh-session", "fresh-build", authUser, ""); err != nil {
		t.Fatal(err)
	}
	if err := persistSignalerAuthorization("fresh-signaler"); err != nil {
		t.Fatal(err)
	}
	store, err := nlmauth.OpenStore()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get("default"); err != nil || got != personal {
		t.Fatalf("personal changed: %v, %v", got, err)
	}
	got, err := store.Get("work")
	if err != nil || got.AuthToken != "fresh" || got.AuthUser != "3" || got.SignalerAuth != "fresh-signaler" {
		t.Fatalf("work = %v, %v", got, err)
	}
	if current, _ := nlmauth.CurrentIdentity(); current != "default" {
		t.Fatalf("current = %q", current)
	}
}

func TestNamedLoginDoesNotInheritOtherIdentity(t *testing.T) {
	isolateIdentity(t)
	t.Setenv("NLM_BROWSER_PROFILE", "Other")
	t.Setenv("NLM_SESSION_ID", "other-session")
	t.Setenv("NLM_BL_PARAM", "other-build")
	t.Setenv("NLM_SIGNALER_AUTH", "other-signaler")
	args := parseAuthCommandForTest(t, []string{"login", "--as", "work"}, globalOptions{})
	if args.Options.ProfileName != "Default" {
		t.Fatalf("profile = %q", args.Options.ProfileName)
	}
	if _, _, err := persistAuthToDisk("work", "work", "Default", "", "", "", "work"); err != nil {
		t.Fatal(err)
	}
	store, _ := nlmauth.OpenStore()
	got, err := store.Get("work")
	if err != nil || got.SessionID != "" || got.BLParam != "" || got.SignalerAuth != "" {
		t.Fatalf("work = %v, %v", got, err)
	}
}

func TestAuthSubcommandIgnoresFlagValues(t *testing.T) {
	isolateIdentity(t)
	for _, args := range [][]string{{"--profile", "list"}, {"--as", "remove"}, {"login", "--as", "use"}} {
		got := parseAuthCommandForTest(t, args, globalOptions{})
		if got.Options.Subcommand != "" {
			t.Fatalf("%v selected %q", args, got.Options.Subcommand)
		}
	}
}
