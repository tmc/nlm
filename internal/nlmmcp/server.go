package nlmmcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

var (
	readOnlyAnnotations = &mcp.ToolAnnotations{
		ReadOnlyHint:  true,
		OpenWorldHint: boolPtr(false),
	}
	mutatingAnnotations = &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
	}
	destructiveAnnotations = &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: boolPtr(true),
		OpenWorldHint:   boolPtr(false),
	}
)

func boolPtr(v bool) *bool { return &v }

// New returns an MCP server for NotebookLM operations.
func New(client *api.Client, impl *mcp.Implementation) *mcp.Server {
	if impl == nil {
		impl = &mcp.Implementation{
			Name:    "nlm-mcp",
			Version: "devel",
		}
	}

	server := mcp.NewServer(impl, &mcp.ServerOptions{
		Instructions: `NotebookLM MCP server.

Use list_notebooks to discover notebook IDs.
Most tools require a notebook_id argument.
Mutating tools change NotebookLM state directly.`,
	})
	registerTools(server, client)
	return server
}

// Run serves the NotebookLM MCP server on stdio.
func Run(ctx context.Context, client *api.Client, impl *mcp.Implementation) error {
	return New(client, impl).Run(ctx, &mcp.StdioTransport{})
}
