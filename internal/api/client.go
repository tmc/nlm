// Package api provides the NotebookLM API client.
package api

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/rpc"
)

type Notebook = pb.Project

// Client handles NotebookLM API interactions.
type Client struct {
	rpc *rpc.Client
}

// New creates a new NotebookLM API client.
func New(authToken, cookies string, opts ...batchexecute.Option) *Client {
	return &Client{
		rpc: rpc.New(authToken, cookies, opts...),
	}
}

// Notebook operations

func (c *Client) ListNotebooks() ([]*Notebook, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCListNotebooks,
		Args: []interface{}{nil, 1},
	})
	if err != nil {
		return nil, fmt.Errorf("list notebooks failed: %w", err)
	}

	var response pb.ListRecentlyViewedProjectsResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return response.Projects, nil
}

func (c *Client) CreateNotebook(title string) (*Notebook, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCCreateNotebook,
		Args: []interface{}{title, "📔"},
	})
	if err != nil {
		return nil, fmt.Errorf("rpc call: %w", err)
	}

	var project pb.Project
	if err := beprotojson.Unmarshal(resp, &project); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &project, nil
}

func (c *Client) DeleteNotebook(id string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCSync,
		Args:       []interface{}{id},
		NotebookID: id,
	})
	if err != nil {
		return fmt.Errorf("rpc call: %w", err)
	}
	return nil
}

// Source operations

func (c *Client) ListSources(notebookID string) ([]*pb.Source, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCLoadNotebook,
		Args:       []interface{}{notebookID},
		NotebookID: notebookID,
	})
	if err != nil {
		return nil, fmt.Errorf("load notebook: %w", err)
	}

	var notebook pb.Project
	if err := beprotojson.Unmarshal(resp, &notebook); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return notebook.Sources, nil
}

func (c *Client) AddSourceFromURL(notebookID, url string) error {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCInsertNote, // Note: RPC name remains same despite functionality
		NotebookID: notebookID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					nil,
					nil,
					[]string{url},
				},
			},
			notebookID,
		},
	})
	if err != nil {
		return fmt.Errorf("add source failed: %w", err)
	}
	fmt.Println(string(resp))
	return nil
}

func (c *Client) AddSourceFromReader(notebookID string, r io.Reader, filename string) error {
	content, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}

	contentType := http.DetectContentType(content)

	if strings.HasPrefix(contentType, "text/") {
		return c.AddSourceFromText(notebookID, string(content), filename)
	}

	encoded := base64.StdEncoding.EncodeToString(content)
	return c.AddSourceFromBase64(notebookID, encoded, filename, contentType)
}

func (c *Client) AddSourceFromText(notebookID, content, title string) error {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCInsertNote,
		NotebookID: notebookID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					nil,
					[]string{
						title,
						content,
					},
					nil,
					2, // This seems to be the type for text sources
				},
			},
			notebookID,
		},
	})
	if err != nil {
		return fmt.Errorf("add text source failed: %w", err)
	}
	fmt.Println(resp)
	return nil
}

func (c *Client) AddSourceFromBase64(notebookID, content, filename, contentType string) error {
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCInsertNote,
		NotebookID: notebookID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					content,
					filename,
					contentType,
					"base64",
				},
			},
			notebookID,
		},
	})
	if err != nil {
		return fmt.Errorf("add binary source failed: %w", err)
	}
	fmt.Println(resp)
	return nil
}

func (c *Client) AddSourceFromFile(notebookID, filepath string) error {
	f, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	return c.AddSourceFromReader(notebookID, f, filepath)
}

func (c *Client) RemoveSource(notebookID, sourceID string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCRemoveSource,
		Args: []interface{}{
			notebookID,
			sourceID,
		},
		NotebookID: notebookID,
	})
	if err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	return nil
}

func (c *Client) RenameSource(sourceID, newName string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCRenameSource,
		Args: []interface{}{
			nil,
			[]string{sourceID},
			[][][]string{{{newName}}},
		},
	})
	if err != nil {
		return fmt.Errorf("rename source: %w", err)
	}
	return nil
}

// Note operations

func (c *Client) CreateNote(notebookID string, title string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCCreateNote,
		Args: []interface{}{
			notebookID,
			"",       // empty content initially
			[]int{1}, // note type
			nil,
			title,
		},
		NotebookID: notebookID,
	})
	return err
}

func (c *Client) UpdateNote(notebookID, noteID string, content, title string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCUpdateNote,
		Args: []interface{}{
			notebookID,
			noteID,
			[][][]interface{}{{
				{content, title, []interface{}{}},
			}},
		},
		NotebookID: notebookID,
	})
	return err
}

func (c *Client) RemoveNote(noteID string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCRemoveNote,
		Args: []interface{}{
			[][][]string{{{noteID}}},
		},
	})
	if err != nil {
		return fmt.Errorf("remove note: %w", err)
	}
	return nil
}

// Audio overview operations

type AudioOverviewOptions struct {
	Instructions string
}

func (c *Client) CreateAudioOverview(notebookID string, opts AudioOverviewOptions) error {
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCAudioOverview,
		Args: []interface{}{
			notebookID,
			0, // Fixed value in the protocol
			[]string{opts.Instructions},
		},
		NotebookID: notebookID,
	})
	if err != nil {
		return fmt.Errorf("create audio overview: %w", err)
	}
	return nil
}

// Sync operations

func (c *Client) SyncNotebook(notebookID string, timestamp [2]int64) error {
	_, err := c.rpc.Do(rpc.Call{
		ID: rpc.RPCSync,
		Args: []interface{}{
			notebookID,
			nil,
			timestamp,
		},
		NotebookID: notebookID,
	})
	return err
}
