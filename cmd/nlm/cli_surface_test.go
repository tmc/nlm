package main

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func TestCLISurface(t *testing.T) {
	env := func(key string) string {
		switch key {
		case "NLM_AUTH_TOKEN":
			return "env-auth"
		case "NLM_COOKIES":
			return "env-cookies"
		case "NLM_AUTHUSER":
			return "2"
		case "NLM_BROWSER_PROFILE":
			return "Profile 1"
		case "NLM_DEBUG":
			return "true"
		default:
			return ""
		}
	}
	parse := func(t *testing.T, args ...string) (invocation, string, error) {
		t.Helper()
		var stderr bytes.Buffer
		inv, err := parseInvocation(args, env, nil, &stderr)
		return inv, stderr.String(), err
	}

	t.Run("debug environment default", func(t *testing.T) {
		if options := defaultGlobalOptions(env); !options.debug {
			t.Error("debug = false, want true")
		}
	})

	t.Run("root help aliases", func(t *testing.T) {
		for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
			inv, _, err := parse(t, args...)
			if err != nil {
				t.Fatalf("parseInvocation(%v): %v", args, err)
			}
			if inv.action != invocationRootHelp {
				t.Fatalf("action = %v, want root help", inv.action)
			}
		}
	})

	t.Run("no args is bad args with root help", func(t *testing.T) {
		inv, _, err := parse(t)
		if !errors.Is(err, errBadArgs) {
			t.Fatalf("err = %v, want errBadArgs", err)
		}
		if inv.action != invocationRootHelp {
			t.Fatalf("action = %v, want root help", inv.action)
		}
	})

	t.Run("section help", func(t *testing.T) {
		inv, _, err := parse(t, "source", "--help")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.action != invocationSectionHelp || inv.section != "Source" {
			t.Fatalf("action, section = %v, %q; want section help Source", inv.action, inv.section)
		}
	})

	t.Run("artifact read", func(t *testing.T) {
		inv, _, err := parse(t, "--cdp-url", "ws://localhost:9222", "artifact", "read", "artifact-1")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.name != "artifact read" || inv.globals.cdpURL != "ws://localhost:9222" || !reflect.DeepEqual(inv.args, []string{"artifact-1"}) {
			t.Fatalf("name, cdp URL, args = %q, %q, %v; want artifact read ws://localhost:9222 [artifact-1]", inv.name, inv.globals.cdpURL, inv.args)
		}
	})

	t.Run("custom command help", func(t *testing.T) {
		for _, args := range [][]string{{"auth", "-h"}, {"auth", "--help"}} {
			inv, _, err := parse(t, args...)
			if err != nil {
				t.Fatalf("parseInvocation(%v): %v", args, err)
			}
			if inv.action != invocationCommandHelp || inv.name != "auth" {
				t.Fatalf("action, name = %v, %q; want command help auth", inv.action, inv.name)
			}
		}
	})

	t.Run("global flags before command", func(t *testing.T) {
		inv, _, err := parse(t, "--debug", "notebook", "list")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.name != "notebook list" || !inv.globals.debug {
			t.Fatalf("name, debug = %q, %v; want notebook list debug", inv.name, inv.globals.debug)
		}
	})

	t.Run("single dash global flags before command", func(t *testing.T) {
		inv, _, err := parse(t, "-debug", "notebook", "list")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.name != "notebook list" || !inv.globals.debug {
			t.Fatalf("name, debug = %q, %v; want notebook list debug", inv.name, inv.globals.debug)
		}
	})

	t.Run("global flags after help command", func(t *testing.T) {
		inv, _, err := parse(t, "help", "-debug")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.action != invocationRootHelp || !inv.globals.debug {
			t.Fatalf("action, debug = %v, %v; want root help debug", inv.action, inv.globals.debug)
		}
	})

	t.Run("global flag value before command", func(t *testing.T) {
		inv, _, err := parse(t, "--auth", "token", "help")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.action != invocationRootHelp || inv.globals.authToken != "token" {
			t.Fatalf("action, auth = %v, %q; want root help token", inv.action, inv.globals.authToken)
		}
	})

	t.Run("command local flags remain command args", func(t *testing.T) {
		inv, _, err := parse(t, "auth", "--cdp-url", "ws://localhost:9222")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		wantArgs := []string{"--cdp-url", "ws://localhost:9222"}
		if inv.name != "auth" || !reflect.DeepEqual(inv.args, wantArgs) {
			t.Fatalf("name, args = %q, %v; want auth %v", inv.name, inv.args, wantArgs)
		}
	})

	t.Run("source local flags after command remain command args", func(t *testing.T) {
		inv, _, err := parse(t, "source", "add", "--name", "Title", "nb", "-")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		wantArgs := []string{"--name", "Title", "nb", "-"}
		if inv.name != "source add" || inv.globals.sourceName != "" || !reflect.DeepEqual(inv.args, wantArgs) {
			t.Fatalf("name, global name, args = %q, %q, %v; want source add empty global name %v", inv.name, inv.globals.sourceName, inv.args, wantArgs)
		}
	})

	t.Run("chat selector flags after command remain command args", func(t *testing.T) {
		inv, _, err := parse(t, "generate-chat", "nb", "prompt", "--source-match", "docs")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		wantArgs := []string{"nb", "prompt", "--source-match", "docs"}
		if inv.name != "generate-chat" || inv.globals.sourceMatchFlag != "" || !reflect.DeepEqual(inv.args, wantArgs) {
			t.Fatalf("name, global source-match, args = %q, %q, %v; want generate-chat empty global source-match %v", inv.name, inv.globals.sourceMatchFlag, inv.args, wantArgs)
		}
	})

	t.Run("legacy before-command local flags seed command defaults", func(t *testing.T) {
		inv, _, err := parse(t, "--name", "Title", "source", "add", "nb", "-")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		wantArgs := []string{"nb", "-"}
		if inv.name != "source add" || inv.globals.sourceName != "Title" || !reflect.DeepEqual(inv.args, wantArgs) {
			t.Fatalf("name, global name, args = %q, %q, %v; want source add Title %v", inv.name, inv.globals.sourceName, inv.args, wantArgs)
		}
	})

	t.Run("post-command global compatibility flags still parse", func(t *testing.T) {
		inv, _, err := parse(t, "guidebooks", "--json")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.name != "guidebooks" || !inv.globals.jsonOutput {
			t.Fatalf("name, json = %q, %v; want guidebooks json", inv.name, inv.globals.jsonOutput)
		}

		inv, _, err = parse(t, "notebook", "delete", "-y", "nb")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.name != "notebook delete" || !inv.globals.yes || !reflect.DeepEqual(inv.args, []string{"nb"}) {
			t.Fatalf("name, yes, args = %q, %v, %v; want notebook delete yes [nb]", inv.name, inv.globals.yes, inv.args)
		}
	})

	t.Run("command owned flags after command remain command args", func(t *testing.T) {
		inv, _, err := parse(t, "notebook", "list", "--json")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.name != "notebook list" || inv.globals.jsonOutput || !reflect.DeepEqual(inv.args, []string{"--json"}) {
			t.Fatalf("name, global json, args = %q, %v, %v; want notebook list false [--json]", inv.name, inv.globals.jsonOutput, inv.args)
		}

		inv, _, err = parse(t, "source", "sync", "nb", "--force", "--json", "./docs")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		wantArgs := []string{"nb", "--force", "--json", "./docs"}
		if inv.name != "source sync" || inv.globals.force || inv.globals.jsonOutput || !reflect.DeepEqual(inv.args, wantArgs) {
			t.Fatalf("name, force, json, args = %q, %v, %v, %v; want source sync false false %v", inv.name, inv.globals.force, inv.globals.jsonOutput, inv.args, wantArgs)
		}

		inv, _, err = parse(t, "auth", "Work", "--debug")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		wantArgs = []string{"Work", "--debug"}
		if inv.name != "auth" || inv.globals.debug || !reflect.DeepEqual(inv.args, wantArgs) {
			t.Fatalf("name, global debug, args = %q, %v, %v; want auth false %v", inv.name, inv.globals.debug, inv.args, wantArgs)
		}

		inv, _, err = parse(t, "artifact", "update", "art-1", "--name", "New")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		wantArgs = []string{"art-1", "--name", "New"}
		if inv.name != "artifact update" || inv.globals.sourceName != "" || !reflect.DeepEqual(inv.args, wantArgs) {
			t.Fatalf("name, global name, args = %q, %q, %v; want artifact update empty %v", inv.name, inv.globals.sourceName, inv.args, wantArgs)
		}
	})

	t.Run("legacy command resolves", func(t *testing.T) {
		inv, _, err := parse(t, "ls")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		if inv.name != "ls" || inv.cmd.surface != surfaceCompatibility {
			t.Fatalf("name, surface = %q, %v; want compatibility ls", inv.name, inv.cmd.surface)
		}
	})

	t.Run("dash passthrough prevents command help", func(t *testing.T) {
		inv, _, err := parse(t, "source", "add", "nb", "-", "--", "--help")
		if err != nil {
			t.Fatalf("parseInvocation: %v", err)
		}
		wantArgs := []string{"nb", "-", "--", "--help"}
		if inv.action != invocationRun || inv.name != "source add" || !reflect.DeepEqual(inv.args, wantArgs) {
			t.Fatalf("action, name, args = %v, %q, %v; want run source add %v", inv.action, inv.name, inv.args, wantArgs)
		}
	})

	t.Run("unknown command suggests from command table", func(t *testing.T) {
		inv, stderr, err := parse(t, "notebok")
		if !errors.Is(err, errBadArgs) {
			t.Fatalf("err = %v, want errBadArgs", err)
		}
		if inv.action != invocationRootHelp {
			t.Fatalf("action = %v, want root help", inv.action)
		}
		if !bytes.Contains([]byte(stderr), []byte(`Did you mean "notebook"`)) {
			t.Fatalf("stderr = %q, want notebook suggestion", stderr)
		}
	})
}
