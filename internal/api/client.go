package api

import (
    "github.com/tmc/nlm/internal/rpc"
    "github.com/tmc/nlm/internal/batchexecute"
)

// Client handles NotebookLM API interactions
type Client struct {
    be *rpc.Client

    // Configuration
    host      string
    app       string
    buildID   string
    language  string
    headers   map[string]string
    urlParams map[string]string
}

// New creates a new NotebookLM API client
func New(authToken, cookies string, opts ...Option) *Client {
    c := &Client{
        host:      defaultHost,
        app:       defaultApp,
        buildID:   defaultBuildID,
        language:  defaultLang,
        headers: map[string]string{
            "content-type":    "application/x-www-form-urlencoded;charset=UTF-8",
            "origin":          "https://notebooklm.google.com",
            "referer":         "https://notebooklm.google.com/",
            "x-same-domain":   "1",
            "accept":          "*/*",
            "accept-language": "en-US,en;q=0.9",
            "cache-control":   "no-cache",
            "pragma":          "no-cache",
        },
        urlParams: make(map[string]string),
    }

    // Apply user options
    for _, opt := range opts {
        opt(c)
    }

    // Convert to batchexecute options
    beOpts := []batchexecute.Option{
        func(bc *batchexecute.Client) {
            cfg := bc.Config()
            cfg.Host = c.host
            cfg.App = c.app
            cfg.Headers = c.headers
            cfg.AuthToken = authToken
            cfg.Cookies = cookies

            // Set URL params
            cfg.URLParams = map[string]string{
                "bl": c.buildID,
                "hl": c.language,
            }
            // Add any additional URL params
            for k, v := range c.urlParams {
                cfg.URLParams[k] = v
            }
        },
    }

    c.be = rpc.New(authToken, cookies, beOpts...)
    return c
}

