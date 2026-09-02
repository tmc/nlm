//go:build nlmscripttest

// This file is compiled only into the binary the script tests build
// (go build -tags nlmscripttest), never into a released nlm. It replaces the
// two things the auth transcripts cannot exercise offline — the browser and
// the server — with outcomes named by the environment, so
// cmd/nlm/testdata/auth_ux.txt can assert stderr byte for byte.
//
// Every hook is inert unless its variable is set.
//
//	NLM_TEST_COMMAND_RESULTS   per-attempt command outcomes: "401" or
//	                           "ok[:text]" (text is written to stdout)
//	NLM_TEST_BROWSER_AUTH      browser login outcome: "ok" or "fail:<cause>"
//	NLM_TEST_INTERACTIVE       force the interactivity guard on ("1") or off
//	NLM_TEST_PROFILES          profiles that hold cookies for the target

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/auth"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/notebooklm"
)

func init() {
	if results, ok := os.LookupEnv("NLM_TEST_COMMAND_RESULTS"); ok {
		runCommandCall = scriptedCommandResults(splitList(results))
	}
	if outcome, ok := os.LookupEnv("NLM_TEST_BROWSER_AUTH"); ok {
		installScriptedBrowserAuth(outcome)
	}
	if value, ok := os.LookupEnv("NLM_TEST_INTERACTIVE"); ok {
		allowed := value == "1" || strings.EqualFold(value, "true")
		browserAuthAllowed = func() bool { return allowed }
	}
	if names, ok := os.LookupEnv("NLM_TEST_PROFILES"); ok {
		list := splitList(names)
		profilesWithCookies = func(string) []string { return list }
	}
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

// scriptedCommandResults answers each command attempt from results in order,
// then repeats the last answer.
func scriptedCommandResults(results []string) func(commandCall, *notebooklm.Client) error {
	attempt := 0
	return func(commandCall, *notebooklm.Client) error {
		result := "ok"
		if len(results) > 0 {
			if attempt < len(results) {
				result = results[attempt]
			} else {
				result = results[len(results)-1]
			}
		}
		attempt++
		switch {
		case result == "401":
			return batchexecute.ErrUnauthorized
		case strings.HasPrefix(result, "ok:"):
			fmt.Println(strings.TrimPrefix(result, "ok:"))
			return nil
		case result == "ok":
			return nil
		default:
			return errors.New(result)
		}
	}
}

// installScriptedBrowserAuth replaces both browser login paths, the silent
// refresh and explicit `nlm auth`, with a fixed outcome.
func installScriptedBrowserAuth(outcome string) {
	if cause, ok := strings.CutPrefix(outcome, "fail:"); ok {
		err := errors.New(cause)
		reharvestBrowserCredentials = func(bool) (string, string, error) { return "", "", err }
		getAuthData = func(*auth.BrowserAuth, ...auth.Option) (*auth.AuthData, error) { return nil, err }
		return
	}
	reharvestBrowserCredentials = func(bool) (string, string, error) {
		return "test-token", "test-cookies", nil
	}
	getAuthData = func(*auth.BrowserAuth, ...auth.Option) (*auth.AuthData, error) {
		return &auth.AuthData{Token: "test-token", Cookies: "test-cookies"}, nil
	}
}
