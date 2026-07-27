package api

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	intmethod "github.com/tmc/nlm/internal/method"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
)

func uploadURL(authUser string) string {
	const base = "https://notebook.google.com/upload/_/"
	if authUser == "" {
		return base
	}
	return base + "?authuser=" + url.QueryEscape(authUser)
}

func setAuthUserHeader(header http.Header, authUser string) {
	if authUser != "" {
		header.Set("X-Goog-AuthUser", authUser)
	}
}

// UploadProjectCoverImage uploads a custom cover image and associates it with
// the notebook. The flow is HAR-verified (2026-04-25 nb-images):
//
//  1. Client generates an image UUID.
//  2. Start a resumable upload to /upload/_/ with UPLOAD_TYPE=CUSTOMIZATION
//     metadata, IMAGE_UUID set to the client-generated value, and
//     X-Goog-Upload-Header-Content-Length matching the image bytes.
//  3. POST the bytes to the upload URL returned in X-Goog-Upload-Url.
//  4. Send s0tc2d MutateProject to associate the IMAGE_UUID.
//
// imageBytes is consumed in full; the caller should pass the full file
// contents (Scotty's resumable protocol is used in single-chunk mode here).
// displayName surfaces in the upload metadata (browser sends the original
// filename); pass any short label.
func (c *Client) UploadProjectCoverImage(ctx context.Context, projectID, displayName string, imageBytes []byte) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	if len(imageBytes) == 0 {
		return fmt.Errorf("image bytes are empty")
	}
	imageUUID := strings.ToUpper(uuid.New().String())

	uploadURL, err := c.startCustomizationUpload(ctx, projectID, displayName, imageUUID, len(imageBytes))
	if err != nil {
		return fmt.Errorf("start cover upload: %w", err)
	}
	if err := c.uploadFileBytes(ctx, uploadURL, imageBytes); err != nil {
		return fmt.Errorf("upload cover bytes: %w", err)
	}

	if _, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCMutateProject,
		NotebookID: projectID,
		Args:       intmethod.MutateProjectCustomImageArgs(projectID, imageUUID),
	}); err != nil {
		return fmt.Errorf("associate cover image: %w", err)
	}
	return nil
}

// startCustomizationUpload initiates the CUSTOMIZATION-flavored resumable
// upload used for notebook cover images. Unlike source uploads, the metadata
// body is sent as raw JSON (not base64) and includes UPLOAD_TYPE,
// IMAGE_TYPE, IMAGE_UUID, and DISPLAY_NAME instead of SOURCE_ID.
func (c *Client) startCustomizationUpload(ctx context.Context, projectID, displayName, imageUUID string, contentLength int) (string, error) {
	metadata := struct {
		ProjectID   string `json:"PROJECT_ID"`
		UploadType  string `json:"UPLOAD_TYPE"`
		ImageType   int    `json:"IMAGE_TYPE"`
		ImageUUID   string `json:"IMAGE_UUID"`
		DisplayName string `json:"DISPLAY_NAME"`
	}{
		ProjectID:   projectID,
		UploadType:  "CUSTOMIZATION",
		ImageType:   1,
		ImageUUID:   imageUUID,
		DisplayName: displayName,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	uploadInitURL := uploadURL(c.config.AuthUser)
	req, err := http.NewRequestWithContext(ctx, "POST", uploadInitURL, bytes.NewReader(metadataJSON))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("X-Goog-Upload-Command", "start")
	req.Header.Set("X-Goog-Upload-Protocol", "resumable")
	req.Header.Set("X-Goog-Upload-Header-Content-Length", fmt.Sprintf("%d", contentLength))
	setAuthUserHeader(req.Header, c.config.AuthUser)
	if cookies := c.rpc.Config.Cookies; cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("Referer", "https://notebook.google.com/")
	setChromeClientHints(req.Header)

	client := httpClientWithTimeout(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload init request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			if status := resp.Header.Get("X-Goog-Upload-Status"); status != "" {
				msg = "X-Goog-Upload-Status=" + status
			}
		}
		if msg == "" {
			msg = "(empty body)"
		}
		return "", fmt.Errorf("upload init failed (status %d): %s", resp.StatusCode, msg)
	}

	uploadURL := resp.Header.Get("X-Goog-Upload-Url")
	if uploadURL == "" {
		uploadURL = resp.Header.Get("x-goog-upload-url")
	}
	if uploadURL == "" {
		return "", fmt.Errorf("no upload URL in response headers")
	}
	return uploadURL, nil
}

// SetProjectCover selects a built-in cover image for the notebook by preset
// ID. Wire format is HAR-verified (2026-04-25); the captured request used
// preset 4. Other valid IDs have not been catalogued.
func (c *Client) SetProjectCover(ctx context.Context, projectID string, coverID int) error {
	_, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCMutateProject,
		NotebookID: projectID,
		Args:       intmethod.MutateProjectCoverArgs(projectID, coverID),
	})
	if err != nil {
		return fmt.Errorf("set project cover: %w", err)
	}
	return nil
}

// RemoveRecentlyViewedProject removes a notebook from the recent list.
func (c *Client) RemoveRecentlyViewedProject(ctx context.Context, projectID string) error {
	req := &pb.RemoveRecentlyViewedProjectRequest{
		ProjectId: projectID,
	}

	_, err := c.orchestrationService.RemoveRecentlyViewedProject(ctx, req)
	return err
}

// Source operations

// AddSources dispatches the izAoDd AddSources RPC with a bulk []SourceInput
// envelope. NOT exercised by any CLI caller today — `nlm add` iterates
// per-type RPCs (AddSourceFromText/FromBase64/uploadFileSource) so a failure
// on one item doesn't mask the rest. The izAoDd bulk wire envelope is
// unverified: do not dispatch bulk through this method without HAR
// evidence that the current argument layout matches what the web UI emits.
func (c *Client) AddSources(ctx context.Context, projectID string, sources []*pb.SourceInput) (*pb.AddSourcesResponse, error) {
	req := &pb.AddSourceRequest{
		Sources:   sources,
		ProjectId: projectID,
	}
	resp, err := c.orchestrationService.AddSources(ctx, req)
	if err != nil {
		return nil, wrapSourceAddError("add sources", err)
	}
	return resp, nil
}

// deleteSourcesBatchSize is the largest batch size known to work reliably
// against the tGMBJ DeleteSources RPC.
const deleteSourcesBatchSize = 10

// DeleteSources deletes the specified sources in bounded batches.
func (c *Client) DeleteSources(ctx context.Context, projectID string, sourceIDs []string) error {
	for start := 0; start < len(sourceIDs); start += deleteSourcesBatchSize {
		end := start + deleteSourcesBatchSize
		if end > len(sourceIDs) {
			end = len(sourceIDs)
		}
		if err := c.deleteSourcesBatch(ctx, projectID, sourceIDs[start:end]); err != nil {
			if len(sourceIDs) <= deleteSourcesBatchSize {
				return err
			}
			return fmt.Errorf("delete sources %d-%d of %d: %w", start+1, end, len(sourceIDs), err)
		}
	}
	return nil
}

func (c *Client) deleteSourcesBatch(ctx context.Context, projectID string, sourceIDs []string) error {
	// Wire format: [repeated_source_ids, project_context]. Keep the request
	// typed and let the generated encoder preserve the captured positional
	// envelope.
	req := &pb.DeleteSourcesRequest{Context: &pb.RequestContext{Version: proto.Int32(2)}}
	for _, id := range sourceIDs {
		req.SourceIds = append(req.SourceIds, &pb.SourceIdList{SourceId: id})
	}
	_, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCDeleteSources,
		NotebookID: projectID,
		Args:       method.EncodeDeleteSourcesArgs(req),
	})
	if err != nil {
		return fmt.Errorf("delete sources: %w", err)
	}
	return nil
}

// MutateSource applies the populated source updates.
func (c *Client) MutateSource(ctx context.Context, sourceID string, updates *pb.Source) (*pb.Source, error) {
	req := &pb.MutateSourceRequest{
		SourceId: &pb.SourceIdList{SourceId: sourceID},
		Updates: &pb.MutateSourceUpdates{Update: &pb.MutateSourceUpdate{
			Title: &pb.MutateSourceTitle{Title: updates.GetTitle()},
		}},
	}
	// Bypass the service client: its generated encoder uses argbuilder and
	// produces the wrong wire format. Use the HAR-verified encoder from
	// internal/method.
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCMutateSource,
		NotebookID: rpc.NotebookIDFromMessage(req),
		Args:       intmethod.EncodeMutateSourceArgs(req),
	})
	if err != nil {
		return nil, fmt.Errorf("mutate source: %w", err)
	}
	var source pb.Source
	if err := c.unmarshal(resp, &source); err != nil {
		return nil, fmt.Errorf("mutate source: unmarshal response: %w", err)
	}
	return &source, nil
}

// RefreshSource refreshes a source from its upstream content.
func (c *Client) RefreshSource(ctx context.Context, projectID, sourceID string) (*pb.Source, error) {
	req := &pb.RefreshSourceRequest{
		Source:    &pb.SourceIdList{SourceId: sourceID},
		Context:   conversationRequestContext(),
		ProjectId: projectID,
	}
	source, err := c.orchestrationService.RefreshSource(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("refresh source: %w", err)
	}
	return source, nil
}

// DiscoverSources dispatches the Es3dTe RPC. arg_format is
// [%project_id%, %query%] per the proto. Returns the suggested
// sources the server thinks are relevant to the query.
//
// Distinct from Ljjv0c (StartFastResearch) — the JS bundle binds
// Es3dTe to a discovery job that returns concrete source candidates,
// while Ljjv0c kicks off a research session that streams a synthesis.
// Earlier commits routed the CLI's discover-sources subcommand
// through fast-research as a workaround; this method gives callers
// the actual Es3dTe path.
func (c *Client) DiscoverSources(ctx context.Context, projectID, query string) (*pb.DiscoverSourcesResponse, error) {
	req := &pb.DiscoverSourcesRequest{
		ProjectId: projectID,
		Query:     query,
	}
	resp, err := c.orchestrationService.DiscoverSources(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("discover sources: %w", err)
	}
	return resp, nil
}

// LoadSource returns a source by ID.
func (c *Client) LoadSource(ctx context.Context, sourceID string) (*pb.Source, error) {
	req := &pb.LoadSourceRequest{
		Source: &pb.SourceIdList{SourceId: sourceID},
	}
	resp, err := c.orchestrationService.LoadSource(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("load source: %w", err)
	}
	return resp.GetSource(), nil
}

// LoadSourceRaw calls the LoadSource RPC (hizoJc) and returns the raw JSON
// wire response. The generated pb.Source struct does not model every field
// the server returns — most notably the indexed full-text body — so callers
// that need to inspect or parse the full payload can read it directly.
//
// The observed wire shape (HAR-verified against the web UI) is:
//
//	f.req=[[["hizoJc","[[\"SOURCE_ID\"],[2],[2]]",null,"generic"]]]
//
// i.e. args = [[source_id], [2], [2]]. The trailing [2] arrays appear to be
// view/mode enums; they are required — single-arg forms return
// "One or more arguments are invalid".
//
// notebookID is optional but is forwarded in the `source-path` URL param
// (`/notebook/<project_id>`) when provided, matching the web UI.
func (c *Client) LoadSourceRaw(ctx context.Context, sourceID, notebookID string) (json.RawMessage, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCLoadSource,
		NotebookID: notebookID,
		Args: []interface{}{
			[]interface{}{sourceID},
			[]interface{}{2},
			[]interface{}{2},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("load source raw: %w", err)
	}
	return resp, nil
}

// CheckSourceFreshness reports whether a source has changed upstream.
func (c *Client) CheckSourceFreshness(ctx context.Context, sourceID string) (*pb.CheckSourceFreshnessResponse, error) {
	req := &pb.CheckSourceFreshnessRequest{
		Source:  &pb.SourceIdList{SourceId: sourceID},
		Context: conversationRequestContext(),
	}
	result, err := c.orchestrationService.CheckSourceFreshness(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("check source freshness: %w", err)
	}
	return result, nil
}

// GetOrCreateAccount dispatches the ZwVcOc RPC. Returns the
// authenticated user's NotebookLM account record. The request carries the
// context envelope observed in the web client. Doubles as a "can the CLI
// talk to the server" sanity check.
func (c *Client) GetOrCreateAccount(ctx context.Context) (*pb.Account, error) {
	resp, err := c.orchestrationService.GetOrCreateAccount(ctx, accountRequest())
	if err != nil {
		return nil, fmt.Errorf("get or create account: %w", err)
	}
	return resp, nil
}

// ActOnSources preserves the unverified legacy action-verb request shape.
//
// The generated yyryJe model describes a distinct observed chat-query shape.
// No captured request establishes how these action verbs map to that shape.
func (c *Client) ActOnSources(ctx context.Context, projectID string, action string, sourceIDs []string) (string, error) {
	call := rpc.Call{
		ID:         "yyryJe",
		NotebookID: projectID,
		Args:       legacyActOnSourcesArgs(projectID, action, sourceIDs),
	}
	resp, err := c.rpc.Do(ctx, call)
	if err != nil {
		return "", fmt.Errorf("act on sources: %w", err)
	}
	return extractTextContent(resp), nil
}

func legacyActOnSourcesArgs(projectID, action string, sourceIDs []string) []interface{} {
	return []interface{}{projectID, action, sourceIDs}
}

// extractTextContent walks a raw JSON response looking for the first non-empty string.
// ActOnSources responses typically nest the content at varying depths.
func extractTextContent(raw json.RawMessage) string {
	var data interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return string(raw)
	}
	if s := findFirstString(data); s != "" {
		return s
	}
	return ""
}

// findFirstString does a depth-first search for the first non-empty string in a JSON value.
func findFirstString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		for _, item := range val {
			if s := findFirstString(item); s != "" {
				return s
			}
		}
	}
	return ""
}

// Source upload utility methods

// detectMIMEType attempts to determine the MIME type of content using multiple methods:
// 1. Use provided contentType if specified
// 2. Use http.DetectContentType for binary detection
// 3. Use file extension as fallback
// 4. Default to application/octet-stream if all else fails
func detectMIMEType(content []byte, filename string, providedType string) string {
	// Use explicitly provided type if available
	if providedType != "" {
		return providedType
	}

	// Try content-based detection first
	detectedType := http.DetectContentType(content)

	// Special case for JSON files - check content
	if bytes.HasPrefix(bytes.TrimSpace(content), []byte("{")) ||
		bytes.HasPrefix(bytes.TrimSpace(content), []byte("[")) {
		// This looks like JSON content
		return "application/json"
	}

	if detectedType != "application/octet-stream" && !strings.HasPrefix(detectedType, "text/plain") {
		return detectedType
	}

	// Try extension-based detection
	ext := filepath.Ext(filename)
	if ext != "" {
		if mimeType := mime.TypeByExtension(ext); mimeType != "" {
			return mimeType
		}
	}

	// If we detected text/plain but have a known extension, trust the extension
	if strings.HasPrefix(detectedType, "text/plain") && ext != "" {
		if mimeType := mime.TypeByExtension(ext); mimeType != "" {
			return mimeType
		}
	}

	return detectedType
}

// AddSourceFromReader adds reader content as a text or uploaded source.
func (c *Client) AddSourceFromReader(ctx context.Context, projectID string, r io.Reader, filename string, contentType ...string) (string, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	// Get provided content type if available
	var providedType string
	if len(contentType) > 0 {
		providedType = contentType[0]
	}

	detectedType := detectMIMEType(content, filename, providedType)

	// Treat plain text or JSON content as text source
	if strings.HasPrefix(detectedType, "text/") ||
		detectedType == "application/json" ||
		strings.HasSuffix(filename, ".json") {
		if c.config.Debug && (strings.HasSuffix(filename, ".json") || detectedType == "application/json") {
			fmt.Fprintf(os.Stderr, "Handling JSON file as text: %s (MIME: %s)\n", filename, detectedType)
		}
		return c.AddSourceFromText(ctx, projectID, string(content), filename)
	}

	// Use resumable upload for binary files (PDF, etc.)
	return c.uploadFileSource(ctx, projectID, filepath.Base(filename), content)
}

// MaxTextSourceBytes is the client-side ceiling for AddSourceFromText
// payloads. The server accepts text sources well under 3MB and rejects
// payloads ≥13MB with a generic "failed precondition" code that carries
// no diagnostic text — indistinguishable on the wire from the source-cap
// rejection. Failing fast at 10MB keeps headroom above the safe band
// while staying below the known-fail band; callers with larger content
// should split it or use `nlm sync` / `nlm sync-pack`, which chunk
// automatically at 5MB boundaries.
const MaxTextSourceBytes = 10 * 1024 * 1024

// AddSourceFromText adds a plain-text source to a notebook.
func (c *Client) AddSourceFromText(ctx context.Context, projectID string, content, title string) (string, error) {
	if n := len(content); n > MaxTextSourceBytes {
		return "", fmt.Errorf("add text source %q (%d bytes > %d limit): %w", title, n, MaxTextSourceBytes, ErrSourceTooLarge)
	}
	resp, err := c.rpc.Do(ctx, rpc.Call{
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
		return "", wrapSourceAddError("add text source", err)
	}

	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

// AddSourceFromBase64 adds a base64-encoded binary source.
func (c *Client) AddSourceFromBase64(ctx context.Context, projectID string, content, filename, contentType string) (string, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
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
		return "", wrapSourceAddError("add binary source", err)
	}

	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

// uploadFileSource uploads a binary file using Google's Resumable Upload Protocol.
//
// The protocol order, per a fresh Chrome HAR, is:
//  1. Register source via RPC o4cbdc; server returns the SOURCE_ID
//  2. Start upload: POST to /upload/_/ with that SOURCE_ID, get back an upload URL
//  3. Upload bytes: POST raw file bytes to the upload URL
//
// Doing (1) last (as earlier versions did) causes Scotty to reject (2) with
// 500 + X-Goog-Upload-Status: final, because the SOURCE_ID in the metadata is
// unknown to the server.
func (c *Client) uploadFileSource(ctx context.Context, projectID, filename string, content []byte) (string, error) {
	// Step 1: Register the source first so the server assigns a SOURCE_ID.
	sourceID, err := c.registerFileSource(ctx, projectID, filename)
	if err != nil {
		return "", fmt.Errorf("register file source: %w", err)
	}

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: uploading file %q (%d bytes) via resumable upload\n", filename, len(content))
		fmt.Fprintf(os.Stderr, "DEBUG: server-assigned source ID: %s\n", sourceID)
	}

	// Step 2: Start the resumable upload session with the server's SOURCE_ID.
	uploadURL, err := c.startResumableUpload(ctx, projectID, filename, sourceID, len(content))
	if err != nil {
		// Scotty sometimes returns 500 with X-Goog-Upload-Status: final even
		// though it already registered the source. The source was registered
		// in step 1, so reconcile: if it now appears in the project, the upload
		// effectively succeeded and reporting a failure would be misleading.
		if isUploadFinalizedError(err) && c.sourceExistsInProject(ctx, projectID, sourceID) {
			return sourceID, nil
		}
		if isUploadFinalizedError(err) {
			return "", fmt.Errorf("start upload: %w (the upload may have finalized anyway; run 'nlm source list %s' to check for %q)", err, projectID, filename)
		}
		return "", fmt.Errorf("start upload: %w", err)
	}

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: got upload URL: %s\n", uploadURL)
	}

	// Step 3: Upload the file bytes.
	if err := c.uploadFileBytes(ctx, uploadURL, content); err != nil {
		return "", fmt.Errorf("upload file bytes: %w", err)
	}

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: file bytes uploaded successfully\n")
	}

	return sourceID, nil
}

// isUploadFinalizedError reports whether err is the Scotty failure mode where
// the upload init returns a 5xx carrying X-Goog-Upload-Status: final — a state
// in which the source has often already been created server-side despite the
// error. startResumableUpload folds that header into the error message.
func isUploadFinalizedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "X-Goog-Upload-Status=final")
}

// sourceExistsInProject reports whether sourceID is present in the project's
// current source list. Used to reconcile an upload that errored but may have
// landed. A lookup error is treated as "not found" so the caller surfaces the
// original failure rather than masking it.
func (c *Client) sourceExistsInProject(ctx context.Context, projectID, sourceID string) bool {
	if sourceID == "" {
		return false
	}
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return false
	}
	for _, src := range project.Sources {
		if src.SourceId != nil && src.SourceId.SourceId == sourceID {
			return true
		}
	}
	return false
}

func buildSourceUploadMetadata(projectID, filename, sourceID string) ([]byte, error) {
	metadata := struct {
		ProjectID  string `json:"PROJECT_ID"`
		SourceName string `json:"SOURCE_NAME"`
		SourceID   string `json:"SOURCE_ID"`
	}{
		ProjectID:  projectID,
		SourceName: filename,
		SourceID:   sourceID,
	}
	return json.Marshal(metadata)
}

// startResumableUpload initiates a resumable upload session and returns the upload URL.
func (c *Client) startResumableUpload(ctx context.Context, projectID, filename, sourceID string, contentLength int) (string, error) {
	// Build metadata payload. Field order matches Chrome's upload
	// (PROJECT_ID, SOURCE_NAME, SOURCE_ID); Go's map marshaling sorts keys
	// alphabetically, which Scotty rejects with 500.
	metadataJSON, err := buildSourceUploadMetadata(projectID, filename, sourceID)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	uploadInitURL := uploadURL(c.config.AuthUser)
	req, err := http.NewRequestWithContext(ctx, "POST", uploadInitURL, bytes.NewReader(metadataJSON))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// Required headers for resumable upload initiation per Scotty's protocol.
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("X-Goog-Upload-Command", "start")
	req.Header.Set("X-Goog-Upload-Protocol", "resumable")
	req.Header.Set("X-Goog-Upload-Header-Content-Length", fmt.Sprintf("%d", contentLength))
	setAuthUserHeader(req.Header, c.config.AuthUser)

	// Upload uses cookies only; no Authorization or X-Same-Domain headers.
	// The current web upload init includes Origin and Referer.
	if cookies := c.rpc.Config.Cookies; cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("Origin", "https://notebook.google.com")
	req.Header.Set("Referer", "https://notebook.google.com/")
	setChromeClientHints(req.Header)

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: upload init URL: %s\n", uploadInitURL)
		fmt.Fprintf(os.Stderr, "DEBUG: upload init body: %s\n", string(metadataJSON))
		for k, v := range req.Header {
			if k != "Cookie" { // Don't dump cookies
				fmt.Fprintf(os.Stderr, "DEBUG: upload init header %s: %v\n", k, v)
			}
		}
	}

	client := httpClientWithTimeout(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload init request: %w", err)
	}
	defer resp.Body.Close()

	if c.config.Debug {
		fmt.Fprintf(os.Stderr, "DEBUG: upload init response status: %s\n", resp.Status)
		for k, v := range resp.Header {
			fmt.Fprintf(os.Stderr, "DEBUG: upload init response header %s: %v\n", k, v)
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			// Scotty frequently returns 5xx with an empty body; the real signal
			// lives in response headers (X-Goog-Upload-Status, upload id, etc.).
			if status := resp.Header.Get("X-Goog-Upload-Status"); status != "" {
				msg = "X-Goog-Upload-Status=" + status
			}
			if id := resp.Header.Get("X-Guploader-Uploadid"); id != "" {
				if msg != "" {
					msg += " "
				}
				msg += "X-Guploader-Uploadid=" + id
			}
			if msg == "" {
				msg = "(empty body)"
			}
		}
		return "", fmt.Errorf("upload init failed (status %d): %s", resp.StatusCode, msg)
	}

	// The upload URL is returned in the X-Goog-Upload-URL header
	uploadURL := resp.Header.Get("X-Goog-Upload-Url")
	if uploadURL == "" {
		// Try lowercase
		uploadURL = resp.Header.Get("x-goog-upload-url")
	}
	if uploadURL == "" {
		return "", fmt.Errorf("no upload URL in response headers")
	}

	return uploadURL, nil
}

// uploadFileBytes uploads the raw file bytes to the resumable upload URL.
func (c *Client) uploadFileBytes(ctx context.Context, uploadURL string, content []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}

	// Per HAR: Content-Type is form-urlencoded even for binary data
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	req.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	req.Header.Set("X-Goog-Upload-Offset", "0")
	setAuthUserHeader(req.Header, c.config.AuthUser)

	// Upload uses cookies only — no Authorization header
	if cookies := c.rpc.Config.Cookies; cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("Referer", "https://notebook.google.com/")
	setChromeClientHints(req.Header)

	client := httpClientWithTimeout(5 * time.Minute)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			if status := resp.Header.Get("X-Goog-Upload-Status"); status != "" {
				msg = "X-Goog-Upload-Status=" + status
			}
			if id := resp.Header.Get("X-Guploader-Uploadid"); id != "" {
				if msg != "" {
					msg += " "
				}
				msg += "X-Guploader-Uploadid=" + id
			}
			if msg == "" {
				msg = "(empty body)"
			}
		}
		return fmt.Errorf("upload failed (status %d): %s", resp.StatusCode, msg)
	}

	return nil
}

// registerFileSource registers a file as a notebook source via RPC o4cbdc and
// returns the server-assigned SOURCE_ID. Called before the Scotty upload so the
// upload init can reference a SOURCE_ID Scotty knows about.
func (c *Client) registerFileSource(ctx context.Context, projectID, filename string) (string, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCAddFileSource,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{
				[]interface{}{filename},
			},
			projectID,
			[]interface{}{2}, // source type: file upload
			[]interface{}{
				1, nil, nil, nil, nil, nil, nil, nil, nil, nil,
				[]interface{}{1},
			},
		},
	})
	if err != nil {
		return "", wrapSourceAddError("register file source RPC", err)
	}

	registeredID, err := extractSourceID(resp)
	if err != nil {
		if c.config.Debug {
			fmt.Fprintf(os.Stderr, "DEBUG: register response: %s\n", string(resp))
		}
		return "", fmt.Errorf("extract source ID from register response: %w", err)
	}
	return registeredID, nil
}

// setAuthHeaders adds authentication headers to an HTTP request.
func (c *Client) setAuthHeaders(req *http.Request) {
	cookies := c.rpc.Config.Cookies
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	// Add SAPISIDHASH authorization
	if sapisid := extractSAPISID(cookies); sapisid != "" {
		origin := "https://notebook.google.com"
		req.Header.Set("Authorization", generateSAPISIDHASH(sapisid, origin))
	}

	req.Header.Set("Origin", "https://notebook.google.com")
	req.Header.Set("Referer", "https://notebook.google.com/")
	req.Header.Set("X-Same-Domain", "1")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
}

// extractSAPISID extracts the SAPISID cookie value from a cookie string.
func extractSAPISID(cookies string) string {
	for _, part := range strings.Split(cookies, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "SAPISID=") {
			return strings.TrimPrefix(part, "SAPISID=")
		}
	}
	return ""
}

// generateSAPISIDHASH generates the SAPISIDHASH authorization header value.
func generateSAPISIDHASH(sapisid, origin string) string {
	timestamp := time.Now().Unix()
	data := fmt.Sprintf("%d %s %s", timestamp, sapisid, origin)
	hash := sha1.Sum([]byte(data))
	return fmt.Sprintf("SAPISIDHASH %d_%x", timestamp, hash)
}

// AddSourceFromFile adds a local file as a notebook source.
func (c *Client) AddSourceFromFile(ctx context.Context, projectID string, filepath string, contentType ...string) (string, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var providedType string
	if len(contentType) > 0 {
		providedType = contentType[0]
	}
	return c.AddSourceFromReader(ctx, projectID, f, filepath, providedType)
}

// AddSourceFromURL adds a web or YouTube URL as a notebook source.
func (c *Client) AddSourceFromURL(ctx context.Context, projectID string, url string) (string, error) {
	// Check if it's a YouTube URL first
	if isYouTubeURL(url) {
		if _, err := extractYouTubeVideoID(url); err != nil {
			return "", fmt.Errorf("invalid YouTube URL: %w", err)
		}
		// Use dedicated YouTube method
		return c.AddYouTubeSource(ctx, projectID, url)
	}

	// Regular URL handling
	resp, err := c.rpc.Do(ctx, rpc.Call{
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
		return "", wrapSourceAddError("add source from URL", err)
	}

	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

// AddYouTubeSource adds a YouTube video as a notebook source.
func (c *Client) AddYouTubeSource(ctx context.Context, projectID, youtubeURL string) (string, error) {
	sourceURL, err := normalizeYouTubeSourceURL(youtubeURL)
	if err != nil {
		return "", err
	}

	if c.rpc.Config.Debug {
		fmt.Printf("=== AddYouTubeSource ===\n")
		fmt.Printf("Project ID: %s\n", projectID)
		fmt.Printf("YouTube URL: %s\n", sourceURL)
	}

	payload := buildYouTubeSourcePayload(projectID, sourceURL)

	if c.rpc.Config.Debug {
		fmt.Printf("\nPayload Structure:\n")
	}

	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCAddSources,
		NotebookID: projectID,
		Args:       payload,
	})
	if err != nil {
		return "", wrapSourceAddError("add YouTube source", err)
	}

	if c.rpc.Config.Debug {
		fmt.Printf("\nRaw Response:\n%s\n", string(resp))
	}

	if len(resp) == 0 {
		return "", fmt.Errorf("empty response from server (check debug output for request details)")
	}

	sourceID, err := extractSourceID(resp)
	if err != nil {
		return "", fmt.Errorf("extract source ID: %w", err)
	}
	return sourceID, nil
}

func buildYouTubeSourcePayload(projectID, youtubeURL string) []interface{} {
	return []interface{}{
		[]interface{}{
			[]interface{}{
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				[]string{youtubeURL},
				nil,
				nil,
				1,
			},
		},
		projectID,
		[]int{2},
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []int{1}},
	}
}

// wrapSourceAddError wraps an AddSource* failure with the operation name. It
// does not auto-classify any wire error: code-9 ("Failed precondition") from
// the server arrives with no discriminator, so we can't distinguish source-
// cap from oversize/malformed/server-policy at this layer. Callers with
// out-of-band evidence (e.g. a fresh ListSources count) wrap with the
// ErrSourceCapReached sentinel themselves.
func wrapSourceAddError(op string, err error) error {
	return fmt.Errorf("%s: %w", op, err)
}

// Helper function to extract source ID with better error handling
func extractSourceID(resp json.RawMessage) (string, error) {
	if len(resp) == 0 {
		return "", fmt.Errorf("empty response")
	}

	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		// JSON parsing failed — likely due to double-escaped batchexecute response.
		// Fall back to scanning raw bytes for a UUID pattern.
		if id := findUUIDInBytes(resp); id != "" {
			return id, nil
		}
		return "", fmt.Errorf("parse response JSON: %w", err)
	}

	// Try different response formats
	// Format 1: [[[["id",...]]]]
	// Format 2: [[["id",...]]]
	// Format 3: [["id",...]]
	for _, format := range []func([]interface{}) (string, bool){
		// Format 1
		func(d []interface{}) (string, bool) {
			if len(d) > 0 {
				if d0, ok := d[0].([]interface{}); ok && len(d0) > 0 {
					if d1, ok := d0[0].([]interface{}); ok && len(d1) > 0 {
						if d2, ok := d1[0].([]interface{}); ok && len(d2) > 0 {
							if id, ok := d2[0].(string); ok {
								return id, true
							}
						}
					}
				}
			}
			return "", false
		},
		// Format 2
		func(d []interface{}) (string, bool) {
			if len(d) > 0 {
				if d0, ok := d[0].([]interface{}); ok && len(d0) > 0 {
					if d1, ok := d0[0].([]interface{}); ok && len(d1) > 0 {
						if id, ok := d1[0].(string); ok {
							return id, true
						}
					}
				}
			}
			return "", false
		},
		// Format 3
		func(d []interface{}) (string, bool) {
			if len(d) > 0 {
				if d0, ok := d[0].([]interface{}); ok && len(d0) > 0 {
					if id, ok := d0[0].(string); ok {
						return id, true
					}
				}
			}
			return "", false
		},
	} {
		if id, ok := format(data); ok {
			return id, nil
		}
	}

	// Last resort: scan raw bytes for UUID
	if id := findUUIDInBytes(resp); id != "" {
		return id, nil
	}

	return "", fmt.Errorf("could not find source ID in response structure: %v", data)
}

// findUUIDInBytes scans raw bytes for a UUID v4 pattern (8-4-4-4-12 hex).
// Used as a fallback when JSON parsing fails due to double-escaped responses.
func findUUIDInBytes(b []byte) string {
	s := string(b)
	for i := 0; i <= len(s)-36; i++ {
		candidate := s[i : i+36]
		if isUUID(candidate) {
			return candidate
		}
	}
	return ""
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// Helper functions to identify and extract YouTube video IDs
func isYouTubeURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return isYouTubeHost(u.Hostname())
}

func isYouTubeHost(host string) bool {
	host = strings.ToLower(host)
	return host == "youtu.be" || host == "youtube.com" || strings.HasSuffix(host, ".youtube.com")
}

func extractYouTubeVideoID(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	host := strings.ToLower(u.Hostname())
	if host == "youtu.be" {
		id := strings.TrimPrefix(u.Path, "/")
		if strings.Contains(id, "/") {
			return "", fmt.Errorf("unsupported YouTube URL format")
		}
		return id, nil
	}

	if (host == "youtube.com" || strings.HasSuffix(host, ".youtube.com")) && u.Path == "/watch" {
		return u.Query().Get("v"), nil
	}

	return "", fmt.Errorf("unsupported YouTube URL format")
}

func normalizeYouTubeSourceURL(input string) (string, error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		videoID, err := extractYouTubeVideoID(input)
		if err != nil {
			return "", fmt.Errorf("invalid YouTube URL: %w", err)
		}
		sourceURL, err := canonicalYouTubeWatchURL(videoID)
		if err != nil {
			return "", fmt.Errorf("invalid YouTube URL: %w", err)
		}
		return sourceURL, nil
	}

	return canonicalYouTubeWatchURL(input)
}

func canonicalYouTubeWatchURL(videoID string) (string, error) {
	if videoID == "" {
		return "", fmt.Errorf("invalid YouTube video ID: empty")
	}
	if strings.ContainsAny(videoID, "/?&= ") {
		return "", fmt.Errorf("invalid YouTube video ID: %q", videoID)
	}
	return "https://www.youtube.com/watch?v=" + videoID, nil
}
