// Command designreview verifies prose citations, resolves native NotebookLM
// citations, and renders chat JSONL back to Markdown.
//
// Usage:
//
//	designreview verify --repo /path/to/repo [--format jsonl|grep|sarif|github] < report.md
//	designreview resolve --notebook <id> [--format jsonl|grep|sarif|github] < citations.jsonl
//	designreview render < chat.jsonl
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tmc/nlm/internal/designreview"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "verify":
		if err := runVerify(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "designreview:", err)
			os.Exit(1)
		}
	case "resolve":
		if err := runResolve(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "designreview:", err)
			os.Exit(1)
		}
	case "render":
		if err := runRender(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "designreview:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "designreview: unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  designreview verify --repo <path> [--format jsonl|grep|sarif|github] [--skip-dir name ...] < report.md")
	fmt.Fprintln(os.Stderr, "  designreview resolve --notebook <id> [--format jsonl|grep|sarif|github] < citations.jsonl")
	fmt.Fprintln(os.Stderr, "  designreview render < chat.jsonl")
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	repoRoot := fs.String("repo", "", "path to the reviewed repo (required)")
	formatFlag := fs.String("format", "jsonl", "output format: jsonl | grep | sarif | github")
	var skipDirs stringSliceFlag
	fs.Var(&skipDirs, "skip-dir", "directory basename to skip during scan (repeatable; defaults below)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repoRoot == "" {
		return fmt.Errorf("--repo is required")
	}
	format, err := designreview.ParseFormat(*formatFlag)
	if err != nil {
		return err
	}

	defaultSkips := map[string]bool{
		".git": true, "vendor": true, "node_modules": true,
		"testdata": true, ".build": true, "dist": true, "build": true,
	}
	for _, d := range skipDirs {
		defaultSkips[d] = true
	}
	skip := func(name string) bool { return defaultSkips[name] }

	repo := &designreview.Repo{}
	if err := repo.Scan(*repoRoot, skip); err != nil {
		return fmt.Errorf("scan %s: %w", *repoRoot, err)
	}

	cites := designreview.Extract(os.Stdin)
	cites = designreview.Verify(repo, cites)

	return designreview.Write(os.Stdout, format, repo.Root, cites)
}

func runResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	notebookID := fs.String("notebook", "", "NotebookLM notebook ID (required)")
	formatFlag := fs.String("format", "jsonl", "output format: jsonl | grep | sarif | github")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *notebookID == "" {
		return fmt.Errorf("--notebook is required")
	}
	format, err := designreview.ParseFormat(*formatFlag)
	if err != nil {
		return err
	}
	authToken, cookies, err := loadAuth()
	if err != nil {
		return err
	}
	cites, err := designreview.ReadNativeCitations(os.Stdin)
	if err != nil {
		return err
	}
	client := api.New(authToken, cookies)
	if authUser := os.Getenv("NLM_AUTHUSER"); authUser != "" {
		client.SetAuthUser(authUser)
	}
	resolved, err := designreview.ResolveAll(func(sourceID string) (api.LoadSourceText, error) {
		return client.LoadSourceText(sourceID, *notebookID)
	}, cites)
	if err != nil {
		return err
	}
	out := make([]designreview.Citation, 0, len(resolved))
	for _, r := range resolved {
		out = append(out, designreview.ResolvedAsCitation(r, ""))
	}
	return designreview.Write(os.Stdout, format, "", out)
}

func runRender(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("render takes no arguments")
	}
	return designreview.RenderChatAnswer(os.Stdout, os.Stdin)
}

func loadAuth() (string, string, error) {
	loadStoredEnv()
	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")
	if authToken == "" || cookies == "" {
		return "", "", fmt.Errorf("authentication required; run 'nlm auth' first, or export NLM_AUTH_TOKEN and NLM_COOKIES")
	}
	return authToken, cookies, nil
}

func loadStoredEnv() {
	for key, value := range readStoredEnv() {
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		os.Setenv(key, value)
	}
}

func readStoredEnv() map[string]string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".nlm", "env"))
	if err != nil {
		return nil
	}
	values := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values[key] = value
	}
	return values
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string     { return fmt.Sprint(*s) }
func (s *stringSliceFlag) Set(v string) error { *s = append(*s, v); return nil }
