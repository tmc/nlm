package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
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
