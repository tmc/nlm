package api

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/rpc"
)

func (c *Client) AddSourceFromText(projectID string, content, title string) (string, error) {
	resp, err := c.be.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					nil,
					[]string{
						title,
						content,
					},
					nil,
					2, // text source type
				},
			},
			projectID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("add text source: %w", err)
	}

	var response pb.AddSourceResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return response.SourceId, nil
}

func (c *Client) AddSourceFromBase64(projectID string, content, filename, contentType string) (string, error) {
	resp, err := c.be.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					content,
					filename,
					contentType,
					"base64",
				},
			},
			projectID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("add binary source: %w", err)
	}

	var response pb.AddSourceResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return response.SourceId, nil
}

func (c *Client) AddSourceFromReader(projectID string, r io.Reader, filename string) (string, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	contentType := http.DetectContentType(content)
	if strings.HasPrefix(contentType, "text/") {
		return c.AddSourceFromText(projectID, string(content), filename)
	}

	encoded := base64.StdEncoding.EncodeToString(content)
	return c.AddSourceFromBase64(projectID, encoded, filename, contentType)
}

func (c *Client) AddSourceFromFile(projectID string, filepath string) (string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	return c.AddSourceFromReader(projectID, f, filepath)
}

func (c *Client) AddSourceFromURL(projectID string, url string) (string, error) {
	resp, err := c.be.Do(rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{
					nil,
					nil,
					[]string{url},
				},
			},
			projectID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("add source from URL: %w", err)
	}

	var response pb.AddSourceResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return response.SourceId, nil
}

func (c *Client) DeleteSources(projectID string, sourceIDs []string) error {
	_, err := c.be.Do(rpc.Call{
		ID: rpc.RPCDeleteSources,
		Args: []interface{}{
			[][][]string{{sourceIDs}},
		},
		NotebookID: projectID,
	})
	if err != nil {
		return fmt.Errorf("delete sources: %w", err)
	}
	return nil
}

func (c *Client) MutateSource(sourceID string, updates *Source) (*Source, error) {
	resp, err := c.be.Do(rpc.Call{
		ID:   rpc.RPCMutateSource,
		Args: []interface{}{sourceID, updates},
	})
	if err != nil {
		return nil, fmt.Errorf("mutate source: %w", err)
	}

	var source Source
	if err := beprotojson.Unmarshal(resp, &source); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &source, nil
}

func (c *Client) RefreshSource(sourceID string) (*Source, error) {
	resp, err := c.be.Do(rpc.Call{
		ID:   rpc.RPCRefreshSource,
		Args: []interface{}{sourceID},
	})
	if err != nil {
		return nil, fmt.Errorf("refresh source: %w", err)
	}

	var source Source
	if err := beprotojson.Unmarshal(resp, &source); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &source, nil
}
