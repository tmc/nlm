package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxSourceImageBytes = 25 << 20

// DownloadSourceImage fetches a transient image URL returned by hizoJc.
// The URL must be an HTTPS googleusercontent URL. The returned content type
// is suitable for a data URI, with any parameters removed.
func (c *Client) DownloadSourceImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	u, err := url.Parse(imageURL)
	if err != nil {
		return nil, "", fmt.Errorf("parse source image URL: %w", err)
	}
	if u.Scheme != "https" || (u.Hostname() != "googleusercontent.com" && !strings.HasSuffix(u.Hostname(), ".googleusercontent.com")) {
		return nil, "", fmt.Errorf("source image URL must be an HTTPS googleusercontent URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create source image request: %w", err)
	}
	setChromeClientHints(req.Header)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	if cookies := c.rpc.Config.Cookies; cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	client := httpClientWithTimeout(60 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch source image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch source image: unexpected status %s", resp.Status)
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("fetch source image: unexpected content type %q", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read source image: %w", err)
	}
	if len(data) > maxSourceImageBytes {
		return nil, "", fmt.Errorf("source image exceeds %d bytes", maxSourceImageBytes)
	}
	return data, contentType, nil
}
