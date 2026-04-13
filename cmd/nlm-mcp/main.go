// Command nlm-mcp provides an MCP server for NotebookLM.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/nlmmcp"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

var (
	authToken       string
	cookies         string
	chunkedResponse bool
	useDirectRPC    bool
)

func init() {
	flag.StringVar(&authToken, "auth", os.Getenv("NLM_AUTH_TOKEN"), "auth token (or set NLM_AUTH_TOKEN)")
	flag.StringVar(&cookies, "cookies", os.Getenv("NLM_COOKIES"), "cookies (or set NLM_COOKIES)")
	flag.BoolVar(&chunkedResponse, "chunked", false, "use chunked response format (rt=c)")
	flag.BoolVar(&useDirectRPC, "direct-rpc", false, "use direct RPC calls for audio/video operations")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: nlm-mcp [flags]\n\n")
		fmt.Fprintf(os.Stderr, "The server communicates over stdin/stdout using MCP.\n")
		fmt.Fprintf(os.Stderr, "Authentication is loaded from ~/.nlm/env, NLM_AUTH_TOKEN, and NLM_COOKIES.\n")
		flag.PrintDefaults()
	}
}

func main() {
	loadStoredEnv()
	flag.Parse()

	if authToken == "" {
		authToken = os.Getenv("NLM_AUTH_TOKEN")
	}
	if cookies == "" {
		cookies = os.Getenv("NLM_COOKIES")
	}
	if authToken == "" || cookies == "" {
		fmt.Fprintln(os.Stderr, "nlm-mcp: authentication required; run `nlm auth` first")
		os.Exit(1)
	}

	var opts []batchexecute.Option
	if chunkedResponse {
		opts = append(opts, batchexecute.WithURLParams(map[string]string{
			"rt": "c",
		}))
	}

	client := api.New(authToken, cookies, opts...)
	if useDirectRPC {
		client.SetUseDirectRPC(true)
	}

	impl := &mcp.Implementation{
		Name:    "nlm-mcp",
		Version: buildVersion(),
	}
	if err := nlmmcp.Run(context.Background(), client, impl); err != nil {
		fmt.Fprintf(os.Stderr, "nlm-mcp: %v\n", err)
		os.Exit(1)
	}
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "devel"
	}
	return info.Main.Version
}

func loadStoredEnv() {
	for key, value := range readStoredEnv() {
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		_ = os.Setenv(key, value)
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
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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
