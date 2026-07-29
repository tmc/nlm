package main

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const updateCommandParityEnv = "UPDATE_COMMAND_PARITY"

type commandParityGolden struct {
	RootHelp    string                 `json:"root_help"`
	SectionHelp []commandParitySection `json:"section_help"`
	Commands    []commandParityCommand `json:"commands"`
}

type commandParitySection struct {
	Name string `json:"name"`
	Help string `json:"help"`
}

type commandParityCommand struct {
	Path      string                  `json:"path"`
	Name      string                  `json:"name"`
	Surface   commandSurface          `json:"surface"`
	Section   string                  `json:"section"`
	Summary   string                  `json:"summary"`
	ArgsUsage string                  `json:"args_usage"`
	Hidden    bool                    `json:"hidden"`
	Help      string                  `json:"help"`
	Cases     []commandParityArgsCase `json:"cases"`
}

type commandParityArgsCase struct {
	Args       []string `json:"args"`
	Accepted   bool     `json:"accepted"`
	Error      string   `json:"error,omitempty"`
	UsageError bool     `json:"usage_error,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
}

func TestCommandParityGolden(t *testing.T) {
	name := filepath.Join("testdata", "command_parity.golden.json")
	want, err := os.ReadFile(name)
	if err != nil && os.Getenv(updateCommandParityEnv) == "" {
		t.Fatalf("read golden: %v; regenerate with %s=1", err, updateCommandParityEnv)
	}
	var frozen commandParityGolden
	if len(want) > 0 {
		if err := json.Unmarshal(want, &frozen); err != nil {
			t.Fatal(err)
		}
	}
	got := collectCommandParity(t, frozenCommandParityCases(frozen))
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')

	if os.Getenv(updateCommandParityEnv) != "" {
		if err := os.WriteFile(name, data, 0o644); err != nil {
			t.Fatal(err)
		}
		want = data
	}
	if string(data) != string(want) {
		t.Fatalf("command behavior differs from %s; regenerate only before a behavior-preserving migration\nwant %d bytes\ngot  %d bytes", name, len(want), len(data))
	}
}

func TestCommandParityPhase1Baseline(t *testing.T) {
	baseline := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.phase1.golden.json"))
	current := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.golden.json"))
	compareCommandParityPhase1(t, baseline, current)
}

func TestCommandParityPhase2Baseline(t *testing.T) {
	baseline := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.phase2.golden.json"))
	current := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.golden.json"))
	compareCommandParityPhase2(t, baseline, current)
}

func TestCommandParityPhase4Baseline(t *testing.T) {
	baseline := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.phase4.golden.json"))
	current := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.golden.json"))
	compareCommandParityPhase4(t, baseline, current)
}

func TestCommandParityPhase5Baseline(t *testing.T) {
	baseline := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.phase5.golden.json"))
	current := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.golden.json"))
	compareCommandParityPhase5(t, baseline, current)
}

func TestCommandParityPhase6UnknownFlags(t *testing.T) {
	baseline := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.phase5.golden.json"))
	current := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.golden.json"))
	if len(baseline.Commands) != len(current.Commands) {
		t.Fatalf("command count changed: got %d, want %d", len(current.Commands), len(baseline.Commands))
	}
	for i := range baseline.Commands {
		before, after := baseline.Commands[i], current.Commands[i]
		if before.Path != after.Path {
			t.Fatalf("command %d path changed: got %q, want %q", i, after.Path, before.Path)
		}
		beforeCase := commandParityCaseForArgs(t, before, []string{"--unknown"})
		afterCase := commandParityCaseForArgs(t, after, []string{"--unknown"})
		if beforeCase.Accepted != phase6UnknownAcceptedPaths[before.Path] {
			t.Errorf("%s baseline accepted = %v, want %v", before.Path, beforeCase.Accepted, phase6UnknownAcceptedPaths[before.Path])
		}
		if afterCase.Accepted ||
			afterCase.Error != `unknown flag --unknown for "`+after.Path+`"` ||
			!afterCase.UsageError {
			t.Errorf("%s unknown case = %+v", after.Path, afterCase)
		}
	}
}

func TestCommandParityPhase6IgnoredArguments(t *testing.T) {
	baseline := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.phase5.golden.json"))
	current := readCommandParityGolden(t, filepath.Join("testdata", "command_parity.golden.json"))
	if len(baseline.Commands) != len(current.Commands) {
		t.Fatalf("command count changed: got %d, want %d", len(current.Commands), len(baseline.Commands))
	}

	var changed []string
	for i := range baseline.Commands {
		before, after := baseline.Commands[i], current.Commands[i]
		if before.Path != after.Path {
			t.Fatalf("command %d path changed: got %q, want %q", i, after.Path, before.Path)
		}
		beforeCases := commandParityCaseSemantics(before.Cases)
		afterCases := commandParityCaseSemantics(after.Cases)
		if reflect.DeepEqual(beforeCases, afterCases) {
			continue
		}
		changed = append(changed, after.Path)
		if !phase6IgnoredArgumentPaths[after.Path] {
			continue
		}
		for j := range beforeCases {
			if reflect.DeepEqual(beforeCases[j], afterCases[j]) {
				continue
			}
			if !beforeCases[j].Accepted || afterCases[j].Accepted ||
				!afterCases[j].UsageError ||
				!strings.HasPrefix(afterCases[j].Error, "unexpected argument ") {
				t.Errorf("%s case %v changed from %+v to %+v", after.Path, afterCases[j].Args, beforeCases[j], afterCases[j])
			}
		}
	}
	slices.Sort(changed)
	want := slices.Sorted(maps.Keys(phase6IgnoredArgumentPaths))
	if !slices.Equal(changed, want) {
		t.Fatalf("semantic case changes = %v, want %v", changed, want)
	}
}

func readCommandParityGolden(t *testing.T, name string) commandParityGolden {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var golden commandParityGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	return golden
}

func compareCommandParityPhase1(t *testing.T, baseline, current commandParityGolden) {
	t.Helper()
	if filterPhase1HelpLines(current.RootHelp) != filterPhase1HelpLines(baseline.RootHelp) {
		t.Error("root help differs outside the Phase 2 inventory")
	}
	if len(current.SectionHelp) != len(baseline.SectionHelp) {
		t.Fatalf("section help count changed: got %d, want %d", len(current.SectionHelp), len(baseline.SectionHelp))
	}
	for i := range baseline.SectionHelp {
		got, want := current.SectionHelp[i], baseline.SectionHelp[i]
		if got.Name != want.Name {
			t.Fatalf("section %d name changed: got %q, want %q", i, got.Name, want.Name)
		}
		if filterPhase1HelpLines(got.Help) != filterPhase1HelpLines(want.Help) {
			t.Errorf("%s section help differs outside the Phase 2 inventory", want.Name)
		}
	}
	if len(current.Commands) != len(baseline.Commands) {
		t.Fatalf("command count changed: got %d, want %d", len(current.Commands), len(baseline.Commands))
	}
	for i := range baseline.Commands {
		got, want := current.Commands[i], baseline.Commands[i]
		if got.Path != want.Path {
			t.Fatalf("command %d path changed: got %q, want %q", i, got.Path, want.Path)
		}
		normalizePhase6UnknownCase(t, &got, want)
		normalizePhase6IgnoredArgumentCases(&got, want)
		if phase4CommandPaths[want.Path] || phase5CommandPaths[want.Path] || prototextCommandPaths[want.Path] || phase6OwnershipCommandPaths[want.Path] {
			got.ArgsUsage, got.Help = want.ArgsUsage, want.Help
			if len(got.Cases) != len(want.Cases) {
				t.Errorf("%s argument case count changed", want.Path)
				continue
			}
			for j := range got.Cases {
				got.Cases[j].Stderr = want.Cases[j].Stderr
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed outside authorized later-phase help text", want.Path)
			}
			continue
		}
		if authorizedDetailedHelpPaths[want.Path] {
			got.Help = want.Help
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed outside its detailed-help first line", want.Path)
			}
			continue
		}
		if !inventoryCommandPaths[want.Path] {
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed outside the Phase 2 inventory", want.Path)
			}
			continue
		}
		got.ArgsUsage, got.Help = want.ArgsUsage, want.Help
		if len(got.Cases) != len(want.Cases) {
			t.Errorf("%s argument case count changed", want.Path)
			continue
		}
		for j := range got.Cases {
			got.Cases[j].Stderr = want.Cases[j].Stderr
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s changed outside help text", want.Path)
		}
	}
}

func compareCommandParityPhase2(t *testing.T, baseline, current commandParityGolden) {
	t.Helper()
	if filterLaterPhaseHelpLines(current.RootHelp) != filterLaterPhaseHelpLines(baseline.RootHelp) {
		t.Error("root help differs outside the Phase 4 and Phase 5 paths")
	}
	if len(current.SectionHelp) != len(baseline.SectionHelp) {
		t.Fatalf("section help count changed: got %d, want %d", len(current.SectionHelp), len(baseline.SectionHelp))
	}
	for i := range baseline.SectionHelp {
		got, want := current.SectionHelp[i], baseline.SectionHelp[i]
		if got.Name != want.Name {
			t.Fatalf("section %d name changed: got %q, want %q", i, got.Name, want.Name)
		}
		if filterLaterPhaseHelpLines(got.Help) != filterLaterPhaseHelpLines(want.Help) {
			t.Errorf("%s section help differs outside the Phase 4 and Phase 5 paths", want.Name)
		}
	}
	if len(current.Commands) != len(baseline.Commands) {
		t.Fatalf("command count changed: got %d, want %d", len(current.Commands), len(baseline.Commands))
	}
	for i := range baseline.Commands {
		got, want := current.Commands[i], baseline.Commands[i]
		if got.Path != want.Path {
			t.Fatalf("command %d path changed: got %q, want %q", i, got.Path, want.Path)
		}
		normalizePhase6UnknownCase(t, &got, want)
		normalizePhase6IgnoredArgumentCases(&got, want)
		if !phase4CommandPaths[want.Path] && !phase5CommandPaths[want.Path] && !prototextCommandPaths[want.Path] && !phase6OwnershipCommandPaths[want.Path] {
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed outside Phase 4 and Phase 5", want.Path)
			}
			continue
		}
		got.ArgsUsage, got.Help = want.ArgsUsage, want.Help
		if len(got.Cases) != len(want.Cases) {
			t.Errorf("%s argument case count changed", want.Path)
			continue
		}
		for j := range got.Cases {
			got.Cases[j].Stderr = want.Cases[j].Stderr
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s changed outside authorized later-phase help text", want.Path)
		}
	}
}

func compareCommandParityPhase4(t *testing.T, baseline, current commandParityGolden) {
	t.Helper()
	if filterLaterThanPhase4HelpLines(current.RootHelp) != filterLaterThanPhase4HelpLines(baseline.RootHelp) {
		t.Error("root help differs outside the Phase 5 and prototext paths")
	}
	if len(current.SectionHelp) != len(baseline.SectionHelp) {
		t.Fatalf("section help count changed: got %d, want %d", len(current.SectionHelp), len(baseline.SectionHelp))
	}
	for i := range baseline.SectionHelp {
		got, want := current.SectionHelp[i], baseline.SectionHelp[i]
		if got.Name != want.Name {
			t.Fatalf("section %d name changed: got %q, want %q", i, got.Name, want.Name)
		}
		if filterLaterThanPhase4HelpLines(got.Help) != filterLaterThanPhase4HelpLines(want.Help) {
			t.Errorf("%s section help differs outside the Phase 5 and prototext paths", want.Name)
		}
	}
	if len(current.Commands) != len(baseline.Commands) {
		t.Fatalf("command count changed: got %d, want %d", len(current.Commands), len(baseline.Commands))
	}
	for i := range baseline.Commands {
		got, want := current.Commands[i], baseline.Commands[i]
		if got.Path != want.Path {
			t.Fatalf("command %d path changed: got %q, want %q", i, got.Path, want.Path)
		}
		normalizePhase6UnknownCase(t, &got, want)
		normalizePhase6IgnoredArgumentCases(&got, want)
		if !phase5CommandPaths[want.Path] && !prototextCommandPaths[want.Path] && !phase6OwnershipCommandPaths[want.Path] {
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed outside Phase 5 and prototext", want.Path)
			}
			continue
		}
		got.ArgsUsage, got.Help = want.ArgsUsage, want.Help
		if len(got.Cases) != len(want.Cases) {
			t.Errorf("%s argument case count changed", want.Path)
			continue
		}
		for j := range got.Cases {
			got.Cases[j].Stderr = want.Cases[j].Stderr
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s changed outside Phase 5 help text", want.Path)
		}
	}
}

func compareCommandParityPhase5(t *testing.T, baseline, current commandParityGolden) {
	t.Helper()
	if filterHelpLines(filterHelpLines(current.RootHelp, prototextCommandPaths), phase6OwnershipCommandPaths) !=
		filterHelpLines(filterHelpLines(baseline.RootHelp, prototextCommandPaths), phase6OwnershipCommandPaths) {
		t.Error("root help differs outside the prototext paths")
	}
	if len(current.SectionHelp) != len(baseline.SectionHelp) {
		t.Fatalf("section help count changed: got %d, want %d", len(current.SectionHelp), len(baseline.SectionHelp))
	}
	for i := range baseline.SectionHelp {
		got, want := current.SectionHelp[i], baseline.SectionHelp[i]
		if got.Name != want.Name {
			t.Fatalf("section %d name changed: got %q, want %q", i, got.Name, want.Name)
		}
		if filterHelpLines(filterHelpLines(got.Help, prototextCommandPaths), phase6OwnershipCommandPaths) !=
			filterHelpLines(filterHelpLines(want.Help, prototextCommandPaths), phase6OwnershipCommandPaths) {
			t.Errorf("%s section help differs outside the prototext paths", want.Name)
		}
	}
	if len(current.Commands) != len(baseline.Commands) {
		t.Fatalf("command count changed: got %d, want %d", len(current.Commands), len(baseline.Commands))
	}
	for i := range baseline.Commands {
		got, want := current.Commands[i], baseline.Commands[i]
		if got.Path != want.Path {
			t.Fatalf("command %d path changed: got %q, want %q", i, got.Path, want.Path)
		}
		normalizePhase6UnknownCase(t, &got, want)
		normalizePhase6IgnoredArgumentCases(&got, want)
		if !prototextCommandPaths[want.Path] && !phase6OwnershipCommandPaths[want.Path] {
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed outside the prototext paths", want.Path)
			}
			continue
		}
		got.ArgsUsage, got.Help = want.ArgsUsage, want.Help
		if len(got.Cases) != len(want.Cases) {
			t.Errorf("%s argument case count changed", want.Path)
			continue
		}
		for j := range got.Cases {
			got.Cases[j].Stderr = want.Cases[j].Stderr
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s changed outside prototext help text", want.Path)
		}
	}
}

func filterPhase1HelpLines(help string) string {
	var kept []string
	for _, line := range strings.Split(help, "\n") {
		if !helpLineForPaths(line, inventoryCommandPaths) &&
			!helpLineForPaths(line, phase4CommandPaths) &&
			!helpLineForPaths(line, phase5CommandPaths) &&
			!helpLineForPaths(line, prototextCommandPaths) &&
			!helpLineForPaths(line, phase6OwnershipCommandPaths) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func filterLaterPhaseHelpLines(help string) string {
	var kept []string
	for _, line := range strings.Split(help, "\n") {
		if !helpLineForPaths(line, phase4CommandPaths) &&
			!helpLineForPaths(line, phase5CommandPaths) &&
			!helpLineForPaths(line, prototextCommandPaths) &&
			!helpLineForPaths(line, phase6OwnershipCommandPaths) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func filterLaterThanPhase4HelpLines(help string) string {
	var kept []string
	for _, line := range strings.Split(help, "\n") {
		if !helpLineForPaths(line, phase5CommandPaths) &&
			!helpLineForPaths(line, prototextCommandPaths) &&
			!helpLineForPaths(line, phase6OwnershipCommandPaths) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func filterHelpLines(help string, paths map[string]bool) string {
	var kept []string
	for _, line := range strings.Split(help, "\n") {
		if !helpLineForPaths(line, paths) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func helpLineForPaths(line string, paths map[string]bool) bool {
	line = strings.TrimLeft(line, " \t")
	for path := range paths {
		if !strings.HasPrefix(line, path) {
			continue
		}
		rest := line[len(path):]
		if rest == "" || strings.HasPrefix(rest, "  ") {
			return true
		}
		if rest[0] == ' ' {
			rest = strings.TrimLeft(rest, " ")
			if rest == "" || strings.ContainsRune("<[-", rune(rest[0])) {
				return true
			}
		}
	}
	return false
}

var phase4CommandPaths = map[string]bool{
	"source read":  true,
	"source check": true,
}

var phase5CommandPaths = map[string]bool{
	"note create": true,
	"note update": true,
}

var prototextCommandPaths = map[string]bool{
	"source read": true,
	"read-source": true,
}

// phase6OwnershipCommandPaths is the exact commit-1 inventory of surfaces
// whose help changes when stable --json or --yes flags become command-owned.
var phase6OwnershipCommandPaths = map[string]bool{
	"account":           true,
	"analytics":         true,
	"artifact delete":   true,
	"artifact list":     true,
	"artifacts":         true,
	"audio create":      true,
	"audio delete":      true,
	"audio list":        true,
	"audio-list":        true,
	"audio-rm":          true,
	"audio-suggestions": true,
	"autolabel":         true,
	"betool":            true,
	"chat":              true,
	"chat delete":       true,
	"chat list":         true,
	"chat-list":         true,
	"create-audio":      true,
	"delete-artifact":   true,
	"delete-chat":       true,
	"discover-sources":  true,
	"guidebooks":        true,
	"label create":      true,
	"label generate":    true,
	"label list":        true,
	"label relabel-all": true,
	"label unlabeled":   true,
	"label-create":      true,
	"label-generate":    true,
	"label-list":        true,
	"label-relabel-all": true,
	"label-unlabeled":   true,
	"labels":            true,
	"list-artifacts":    true,
	"list-featured":     true,
	"note delete":       true,
	"note list":         true,
	"note-rm":           true,
	"notebook delete":   true,
	"notebook featured": true,
	"notes":             true,
	"rm":                true,
	"rm-note":           true,
	"rm-source":         true,
	"source delete":     true,
	"source list":       true,
	"source-guide":      true,
	"source-rm":         true,
	"sources":           true,
}

// phase6UnknownAcceptedPaths is the exact set of surfaces whose synthetic
// --unknown case was accepted before shared unknown-flag rejection.
var phase6UnknownAcceptedPaths = map[string]bool{
	"account":               true,
	"analytics":             true,
	"artifact delete":       true,
	"artifact export":       true,
	"artifact get":          true,
	"artifact list":         true,
	"artifact read":         true,
	"artifacts":             true,
	"audio delete":          true,
	"audio download":        true,
	"audio get":             true,
	"audio list":            true,
	"audio share":           true,
	"audio-download":        true,
	"audio-get":             true,
	"audio-list":            true,
	"audio-rm":              true,
	"audio-share":           true,
	"audio-suggestions":     true,
	"auth":                  true,
	"autolabel":             true,
	"betool":                true,
	"chat":                  true,
	"chat delete":           true,
	"chat instructions get": true,
	"chat list":             true,
	"chat show":             true,
	"chat-list":             true,
	"chat-show":             true,
	"check-source":          true,
	"create":                true,
	"create-slides":         true,
	"deck create":           true,
	"delete-artifact":       true,
	"delete-chat":           true,
	"dump-load-source":      true,
	"export-flashcards":     true,
	"generate-guide":        true,
	"generate-report":       true,
	"get-artifact":          true,
	"get-instructions":      true,
	"guidebook":             true,
	"guidebook-details":     true,
	"guidebook-publish":     true,
	"guidebook-rm":          true,
	"guidebook-share":       true,
	"label generate":        true,
	"label list":            true,
	"label relabel-all":     true,
	"label unlabeled":       true,
	"label-generate":        true,
	"label-list":            true,
	"label-relabel-all":     true,
	"label-unlabeled":       true,
	"labels":                true,
	"list-artifacts":        true,
	"note list":             true,
	"notebook create":       true,
	"notebook delete":       true,
	"notebook description":  true,
	"notebook unrecent":     true,
	"notebook-description":  true,
	"notebook-notes":        true,
	"notebook-unrecent":     true,
	"notes":                 true,
	"read-artifact":         true,
	"read-source":           true,
	"refresh":               true,
	"report-suggestions":    true,
	"rm":                    true,
	"share":                 true,
	"share-details":         true,
	"share-private":         true,
	"source check":          true,
	"source list":           true,
	"source pack":           true,
	"source read":           true,
	"source sync":           true,
	"sources":               true,
	"sync":                  true,
	"sync-pack":             true,
}

var phase6IgnoredArgumentPaths = map[string]bool{
	"auth":        true,
	"chat config": true,
	"chat-config": true,
	"refresh":     true,
}

func normalizePhase6UnknownCase(t *testing.T, got *commandParityCommand, want commandParityCommand) {
	t.Helper()
	got.Cases = slices.Clone(got.Cases)
	for i := range got.Cases {
		if !slices.Equal(got.Cases[i].Args, []string{"--unknown"}) {
			continue
		}
		got.Cases[i] = commandParityCaseForArgs(t, want, []string{"--unknown"})
		return
	}
	t.Fatalf("%s missing parity case %v", got.Path, []string{"--unknown"})
}

func normalizePhase6IgnoredArgumentCases(got *commandParityCommand, want commandParityCommand) {
	if phase6IgnoredArgumentPaths[got.Path] {
		got.Cases = want.Cases
	}
}

func commandParityCaseSemantics(cases []commandParityArgsCase) []commandParityArgsCase {
	var out []commandParityArgsCase
	for _, test := range cases {
		if slices.Equal(test.Args, []string{"--unknown"}) {
			continue
		}
		test.Stderr = ""
		out = append(out, test)
	}
	return out
}

func commandParityCaseForArgs(t *testing.T, command commandParityCommand, args []string) commandParityArgsCase {
	t.Helper()
	test, ok := findCommandParityCase(command, args)
	if !ok {
		t.Fatalf("%s missing parity case %v", command.Path, args)
	}
	return test
}

func findCommandParityCase(command commandParityCommand, args []string) (commandParityArgsCase, bool) {
	for _, test := range command.Cases {
		if slices.Equal(test.Args, args) {
			return test, true
		}
	}
	return commandParityArgsCase{}, false
}

// inventoryCommandPaths expands the 24 Phase 2 inventory rows across every
// stable and compatibility surface for the same command behavior.
var inventoryCommandPaths = map[string]bool{
	"notebook delete":     true,
	"rm":                  true,
	"source add":          true,
	"add":                 true,
	"source sync":         true,
	"sync":                true,
	"source pack":         true,
	"sync-pack":           true,
	"label attach":        true,
	"label-attach":        true,
	"app create":          true,
	"app-create":          true,
	"mindmap create":      true,
	"mindmap-create":      true,
	"audio create":        true,
	"create-audio":        true,
	"video create":        true,
	"create-video":        true,
	"deck create":         true,
	"create-slides":       true,
	"deck download":       true,
	"deck-download":       true,
	"download slide-deck": true,
	"artifact export":     true,
	"export-flashcards":   true,
	"artifact update":     true,
	"update-artifact":     true,
	"source-guide":        true,
	"generate-chat":       true,
	"create-report":       true,
	"generate-report":     true,
	"chat":                true,
	"chat show":           true,
	"chat-show":           true,
	"chat config":         true,
	"chat-config":         true,
	"research":            true,
	"auth":                true,
	"betool":              true,
	"refresh":             true,
}

// authorizedDetailedHelpPaths are the four read paths whose detailed first
// lines were explicitly authorized to match their already-correct inline-flag
// root synopses. No other parity field may change for these paths.
var authorizedDetailedHelpPaths = map[string]bool{
	"source read": true,
	"read-source": true,
	"note read":   true,
	"read-note":   true,
}

func collectCommandParity(t *testing.T, frozenCases map[string][][]string) commandParityGolden {
	t.Helper()
	golden := commandParityGolden{
		RootHelp: captureCommandStderr(t, printUsage),
	}
	for _, section := range helpSections {
		section := section
		golden.SectionHelp = append(golden.SectionHelp, commandParitySection{
			Name: section,
			Help: captureCommandStderr(t, func() {
				printSectionUsage(section)
			}),
		})
	}
	for i := range commands {
		cmd := &commands[i]
		paths := append([]string{cmd.name}, cmd.aliases...)
		for _, path := range paths {
			path := path
			entry := commandParityCommand{
				Path:      path,
				Name:      cmd.name,
				Surface:   cmd.surface,
				Section:   cmd.section,
				Summary:   cmd.usage,
				ArgsUsage: cmd.argsUsage,
				Hidden:    cmd.hidden,
				Help: captureCommandStderr(t, func() {
					warnCompatibilityCommand(path, cmd)
					printCommandHelp(path, cmd)
				}),
			}
			cases := frozenCases[path]
			if cases == nil {
				cases = commandParityArgs(cmd)
			}
			for _, args := range cases {
				args := slices.Clone(args)
				var validationErr error
				stderr := captureCommandStderr(t, func() {
					validationErr = validateCommandArgs(cmd, path, args, globalOptions{})
				})
				entry.Cases = append(entry.Cases, commandParityArgsCase{
					Args:       args,
					Accepted:   validationErr == nil,
					Error:      errorText(validationErr),
					UsageError: errors.Is(validationErr, errBadArgs),
					Stderr:     stderr,
				})
			}
			golden.Commands = append(golden.Commands, entry)
		}
	}
	return golden
}

func frozenCommandParityCases(golden commandParityGolden) map[string][][]string {
	cases := make(map[string][][]string, len(golden.Commands))
	for _, command := range golden.Commands {
		for _, test := range command.Cases {
			cases[command.Path] = append(cases[command.Path], slices.Clone(test.Args))
		}
	}
	return cases
}

func commandParityArgs(cmd *command) [][]string {
	var cases [][]string
	for n := 0; n <= 6; n++ {
		args := make([]string, n)
		for i := range args {
			args[i] = "arg"
		}
		cases = append(cases, args)
	}
	cases = append(cases,
		[]string{"--unknown"},
		[]string{"-"},
		[]string{"--"},
	)
	cases = append(cases, commandParityRequiredFlagCases[cmd.name]...)
	return uniqueStringSlices(cases)
}

var commandParityRequiredFlagCases = map[string][][]string{
	"app-create":          {{"--type", "prototype", "notebook"}},
	"deck-download":       {{"notebook", "--id", "artifact"}},
	"download slide-deck": {{"notebook", "--id", "artifact"}},
	"export-flashcards":   {{"artifact", "--format", "json"}},
	"mindmap-create":      {{"notebook"}},
	"read-note":           {{"notebook", "note"}},
	"read-source":         {{"source"}},
	"research":            {{"notebook", "query"}},
	"source read":         {{"source"}},
	"source sync":         {{"notebook"}},
	"sync":                {{"notebook"}},
	"sync-pack":           {{}},
	"deck download":       {{"notebook", "--id", "artifact"}},
	"artifact export":     {{"artifact", "--format", "json"}},
	"note read":           {{"notebook", "note"}},
	"app create":          {{"--type", "prototype", "notebook"}},
	"mindmap create":      {{"notebook"}},
	"source pack":         {{}},
}

func uniqueStringSlices(in [][]string) [][]string {
	seen := make(map[string]bool, len(in))
	out := make([][]string, 0, len(in))
	for _, args := range in {
		data, _ := json.Marshal(args)
		key := string(data)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, args)
	}
	return out
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func captureCommandStderr(t *testing.T, fn func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = write
	done := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(read)
		done <- data
	}()
	fn()
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = old
	data := <-done
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	return string(data)
}
