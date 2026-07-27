package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	intmethod "github.com/tmc/nlm/internal/method"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
)

// ListArtifacts returns artifacts for a project using direct RPC
func (c *Client) ListArtifacts(ctx context.Context, projectID string) ([]*pb.Artifact, error) {
	req := &pb.ListArtifactsRequest{
		Context:   universalArtifactRequestContext(),
		ProjectId: projectID,
		Filter:    `NOT artifact.status = "ARTIFACT_STATUS_SUGGESTED"`,
	}
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCListArtifacts,
		Args:       method.EncodeListArtifactsArgs(req),
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("list artifacts RPC: %w", err)
	}

	return artifactsFromProtoResponseWithOptions(resp, c.unmarshalOptions())
}

// artifactsFromProtoResponse decodes the common gArtLc response through the
// generated message while retaining the legacy READY rule for rendered
// artifacts. The positional state value is ambiguous for slide artifacts:
// a contribution.usercontent.google.com download URL proves readiness even
// when the state enum is 3 (the generated FAILED value).
func artifactsFromProtoResponse(raw []byte) ([]*pb.Artifact, error) {
	return artifactsFromProtoResponseWithOptions(raw, beprotojson.UnmarshalOptions{DiscardUnknown: true})
}

func artifactsFromProtoResponseWithOptions(raw []byte, options beprotojson.UnmarshalOptions) ([]*pb.Artifact, error) {
	var response pb.ListArtifactsResponse
	if err := options.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode artifact response: %w", err)
	}
	var wire interface{}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("inspect artifact response: %w", err)
	}
	artifacts := make([]*pb.Artifact, 0, len(response.GetArtifacts()))
	for _, generated := range response.GetArtifacts() {
		if generated == nil {
			continue
		}
		// Keep the public projection of parseArtifactsResponse stable while
		// letting the generated decoder own all positional field handling.
		artifact := &pb.Artifact{
			ArtifactId: generated.GetArtifactId(),
			Title:      generated.GetTitle(),
			Type:       generated.GetType(),
			State:      generated.GetState(),
			Sources:    generated.GetSources(),
			Note:       generated.GetNote(),
		}
		if artifactHasDownloadURL(wire, artifact.GetArtifactId()) {
			artifact.State = pb.ArtifactState_ARTIFACT_STATE_READY
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

// artifactHasDownloadURL reports whether one nested payload contains both
// the artifact ID and a rendered download URL. It deliberately avoids
// depending on positional indexes; the generated message remains the source
// of typed fields and this walk only preserves the documented state override.
func artifactHasDownloadURL(value interface{}, artifactID string) bool {
	var walk func(interface{}) (bool, bool, bool)
	walk = func(value interface{}) (hasID, hasURL, matched bool) {
		switch value := value.(type) {
		case string:
			return value == artifactID, strings.HasPrefix(value, artifactDownloadURLPrefix), false
		case []interface{}:
			for _, child := range value {
				childID, childURL, childMatched := walk(child)
				if childMatched {
					return true, true, true
				}
				// Only an ID directly in this array identifies the artifact
				// row. Do not combine an ID from one sibling row with a URL
				// from another row at an outer wrapper.
				if _, isString := child.(string); isString {
					hasID = hasID || childID
				}
				hasURL = hasURL || childURL
			}
			return hasID, hasURL, hasID && hasURL
		default:
			return false, false, false
		}
	}
	_, _, matched := walk(value)
	return matched
}

// GetArtifact returns a single artifact by ID.
//
// First tries the capture-verified v9rmvd RPC (one-shot direct read;
// arg_format = "[%artifact_id%, %context%]"). If that fails for any
// reason, it falls back to scanning ListRecentlyViewedProjects and
// ListArtifacts (gArtLc).
//
// The fallback path is unconditional on parse failure, so the worst
// case is the same scan-and-filter that callers got before this
// commit. The fast path is exercised first because, when it works,
// it cuts a fan-out call to N notebooks down to a single RPC.
func (c *Client) GetArtifact(ctx context.Context, artifactID string) (*pb.Artifact, error) {
	if artifact, err := c.getArtifactDirect(ctx, artifactID); err == nil && artifact != nil {
		return artifact, nil
	}
	projects, listErr := c.ListRecentlyViewedProjects(ctx)
	if listErr != nil {
		return nil, fmt.Errorf("list projects for artifact lookup: %w", listErr)
	}
	for _, project := range projects {
		artifacts, listArtifactsErr := c.ListArtifacts(ctx, project.GetProjectId())
		if listArtifactsErr != nil {
			continue
		}
		for _, artifact := range artifacts {
			if artifact.GetArtifactId() == artifactID {
				return artifact, nil
			}
		}
	}
	return nil, fmt.Errorf("artifact %q: %w", artifactID, ErrArtifactNotFound)
}

// getArtifactDirect tries the capture-verified v9rmvd RPC. Callers should
// fall back to the gArtLc list scan when this returns an error or nil artifact.
func (c *Client) getArtifactDirect(ctx context.Context, artifactID string) (*pb.Artifact, error) {
	artifact, err := c.orchestrationService.GetArtifact(ctx, &pb.GetArtifactRequest{
		ArtifactId: artifactID,
		Context:    universalArtifactRequestContext(),
	})
	if err != nil {
		return nil, err
	}
	if artifact.GetArtifactId() == "" {
		return nil, fmt.Errorf("empty response")
	}
	return artifact, nil
}

// GetArtifactDownloadURLs returns the rendered-output download URLs for an
// artifact (e.g. a slide deck's .pdf and .pptx links), or nil when the
// artifact has none yet (still generating) or the direct RPC is unavailable on
// this account. It uses the v9rmvd direct fetch, whose payload carries the
// links; the gArtLc list scan does not include them.
func (c *Client) GetArtifactDownloadURLs(ctx context.Context, artifactID string) ([]string, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:   rpc.RPCGetArtifact,
		Args: []interface{}{artifactID},
	})
	if err != nil {
		return nil, err
	}
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, err
	}
	if len(responseData) == 0 {
		return nil, nil
	}
	return extractArtifactDownloadURLs(responseData[0]), nil
}

// ArtifactDownloadURLForFormat returns the artifact's signed download URL whose
// filename matches format ("pdf", "pptx", …). It returns ErrArtifactGenerating
// when the artifact has no rendered output yet, and an error naming the
// available formats when the requested one is absent. The URL is browser-usable
// (the contribution.usercontent.google.com host serves it to an authenticated
// browser session); see DownloadArtifactFile for the direct-fetch caveat.
func (c *Client) ArtifactDownloadURLForFormat(ctx context.Context, artifactID, format string) (string, error) {
	urls, err := c.GetArtifactDownloadURLs(ctx, artifactID)
	if err != nil {
		return "", fmt.Errorf("get artifact download URLs: %w", err)
	}
	if len(urls) == 0 {
		return "", ErrArtifactGenerating
	}

	want := "." + strings.ToLower(strings.TrimPrefix(format, "."))
	var available []string
	for _, u := range urls {
		ext := artifactDownloadExtension(u)
		if ext != "" {
			available = append(available, strings.TrimPrefix(ext, "."))
		}
		if ext == want {
			return u, nil
		}
	}
	return "", fmt.Errorf("no %s download for artifact %s (available: %s)",
		strings.TrimPrefix(want, "."), artifactID, strings.Join(available, ", "))
}

// DownloadArtifactFile fetches an artifact's rendered output (e.g. a slide
// deck's .pdf or .pptx) to filename, selecting the file by format. It can fail
// with a 403 even for a valid session: the contribution.usercontent.google.com
// host that serves these files requires a full browser auth context that the
// CLI's cookies do not satisfy (the same limitation audio/video CDN downloads
// hit). Callers should fall back to ArtifactDownloadURLForFormat and hand the
// URL to a browser when this returns a download/permission error.
func (c *Client) DownloadArtifactFile(ctx context.Context, artifactID, format, filename string) error {
	chosen, err := c.ArtifactDownloadURLForFormat(ctx, artifactID, format)
	if err != nil {
		return err
	}
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()
	return c.downloadAuthed(ctx, chosen, file)
}

// ReadArtifactFile writes an artifact's rendered output to w, selecting the
// output by filename extension. It is useful for text artifacts that callers
// want to inspect without creating a local file.
func (c *Client) ReadArtifactFile(ctx context.Context, artifactID, format string, w io.Writer) error {
	chosen, err := c.ArtifactDownloadURLForFormat(ctx, artifactID, format)
	if err != nil {
		return err
	}
	return c.downloadAuthed(ctx, chosen, w)
}

// artifactDownloadExtension returns the lowercase ".ext" carried in a download
// URL's filename query parameter (e.g. "Deck.pptx" -> ".pptx"), or "" if none.
func artifactDownloadExtension(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name := u.Query().Get("filename")
	if name == "" {
		return ""
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		return strings.ToLower(name[i:])
	}
	return ""
}

// downloadAuthed GETs fileURL with the client's session cookies and writes the
// body to w. It is used for contribution.usercontent.google.com artifact
// downloads, which are gated on the NotebookLM session cookie. The header set
// and optional authuser query parameter mirror DownloadVideoWithAuth, which
// downloads from the same usercontent host.
func (c *Client) downloadAuthed(ctx context.Context, fileURL string, w io.Writer) error {
	if c.config.AuthUser != "" && !strings.Contains(fileURL, "authuser=") {
		sep := "?"
		if strings.Contains(fileURL, "?") {
			sep = "&"
		}
		fileURL += sep + "authuser=" + c.config.AuthUser
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.6")
	req.Header.Set("Range", "bytes=0-")
	req.Header.Set("Referer", "https://notebook.google.com/")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	cookies := c.rpc.Config.Cookies
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	// The usercontent host 302-redirects to a storage backend. Go drops the
	// Cookie header on cross-origin hops, which the backend answers with 403,
	// so re-attach it on every redirect (the same approach downloadAudioFromURL
	// uses for these CDN URLs).
	client := httpClientWithTimeout(300 * time.Second)
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if cookies != "" {
			r.Header.Set("Cookie", cookies)
		}
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("download failed with status %s (check authentication)", resp.Status)
	}
	// An HTML body means the server redirected to a login/consent page rather
	// than serving the file — surface that as an auth problem, not a saved file.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return fmt.Errorf("download returned HTML, not a file (authentication may have expired)")
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("write download: %w", err)
	}
	return nil
}

// DeleteArtifact deletes an artifact by ID using the V5N4be RPC.
//
// Wire format verified against HAR capture 2026-04-07 — see
// internal/method/LabsTailwindOrchestrationService_DeleteArtifact_encoder.go.
func (c *Client) DeleteArtifact(ctx context.Context, artifactID string) error {
	_, err := c.rpc.Do(ctx, rpc.Call{
		ID: rpc.RPCDeleteArtifact,
		Args: intmethod.EncodeDeleteArtifactArgs(&pb.DeleteArtifactRequest{
			ArtifactId: artifactID,
		}),
	})
	if err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}
	return nil
}

// RenameArtifact renames an artifact using the rc3d8d RPC.
//
// Wire format: see
// internal/method/LabsTailwindOrchestrationService_RenameArtifact_encoder.go.
func (c *Client) RenameArtifact(ctx context.Context, artifactID, newTitle string) (*pb.Artifact, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID: rpc.RPCRenameArtifact,
		Args: intmethod.EncodeRenameArtifactArgs(&pb.RenameArtifactRequest{
			ArtifactId: artifactID,
			NewTitle:   newTitle,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("rename artifact: %w", err)
	}
	return c.parseRenameArtifactResponse(resp, artifactID)
}

func (c *Client) parseRenameArtifactResponse(resp []byte, artifactID string) (*pb.Artifact, error) {
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse rename response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("Rename artifact response: %+v\n", responseData)
	}

	if len(responseData) > 0 {
		if artifact := c.parseArtifactFromResponse(responseData[0]); artifact != nil {
			return artifact, nil
		}
	}

	// Rename succeeds even when the RPC only returns a status marker.
	return &pb.Artifact{ArtifactId: artifactID}, nil
}

// ReviseArtifact re-runs an artifact generator with a free-form
// revision instruction. It dispatches the KmcKPe RPC (JS bundle:
// "DeriveArtifact"). The response carries the revised artifact;
// non-trivial responses are decoded via parseArtifactFromResponse.
//
// TODO(har): The wire body for KmcKPe is unverified. The encoding
// here mirrors the in-file "[%context%, %artifact_id%, %instructions%]"
// convention used by sibling RPCs (CreateArtifact, GenerateReportSuggestions).
// Capture HAR by clicking "Revise" on a generated artifact and
// confirm before promoting this off best-effort.
func (c *Client) ReviseArtifact(ctx context.Context, artifactID, instructions string) (*pb.Artifact, error) {
	projectContext := []interface{}{
		2, nil, nil,
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
		[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
	}
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:   rpc.RPCReviseArtifact,
		Args: []interface{}{projectContext, artifactID, instructions},
	})
	if err != nil {
		return nil, fmt.Errorf("revise artifact: %w", err)
	}
	var responseData []interface{}
	if jsonErr := json.Unmarshal(resp, &responseData); jsonErr == nil && len(responseData) > 0 {
		if artifact := c.parseArtifactFromResponse(responseData[0]); artifact != nil {
			return artifact, nil
		}
	}
	// Fall back: report success with the existing artifact_id so callers
	// that just want a "did it run" signal can use this method until the
	// response shape is locked down.
	return &pb.Artifact{ArtifactId: artifactID}, nil
}

// ReportContent submits an abuse/safety report against an artifact.
// Dispatches the OmVMXc RPC (JS-bundle-canonical).
//
// TODO(har): wire shape unverified. Encoding mirrors the in-file
// "[%context%, %artifact_id%, %reason%, %detail%]" convention used by
// sibling artifact RPCs. Capture HAR by opening an artifact's kebab
// menu and submitting a "Report" before promoting this off
// best-effort. The response is not parsed beyond success/failure.
func (c *Client) ReportContent(ctx context.Context, artifactID, reason, detail string) error {
	projectContext := []interface{}{
		2, nil, nil,
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
		[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
	}
	_, err := c.rpc.Do(ctx, rpc.Call{
		ID:   rpc.RPCReportContent,
		Args: []interface{}{projectContext, artifactID, reason, detail},
	})
	if err != nil {
		return fmt.Errorf("report content: %w", err)
	}
	return nil
}

func (c *Client) parseArtifactsResponse(resp []byte) ([]*pb.Artifact, error) {
	var responseData []interface{}
	if err := json.Unmarshal(resp, &responseData); err != nil {
		return nil, fmt.Errorf("parse artifacts response: %w", err)
	}

	if c.config.Debug {
		fmt.Printf("Artifacts response: %+v\n", responseData)
	}

	items := responseData
	if wrapped, ok := interfaceSliceAt(responseData, 0); ok {
		if len(wrapped) == 0 {
			items = wrapped
		} else if _, ok := wrapped[0].([]interface{}); ok {
			items = wrapped
		}
	}

	artifacts := make([]*pb.Artifact, 0, len(items))
	for _, item := range items {
		artifact := c.parseArtifactFromResponse(item)
		if artifact != nil {
			artifacts = append(artifacts, artifact)
		}
	}
	return artifacts, nil
}

// parseArtifactFromResponse parses an artifact from RPC response data
func (c *Client) parseArtifactFromResponse(data interface{}) *pb.Artifact {
	artifactData, ok := data.([]interface{})
	if !ok || len(artifactData) == 0 {
		return nil
	}

	artifactID := stringAt(artifactData, 0)
	if artifactID == "" {
		return nil
	}

	artifact := &pb.Artifact{
		ArtifactId: artifactID,
	}

	// Observed gArtLc artifact shape:
	//   [artifact_id, title, type_code, source_refs, state_code, ...]
	if typeCode, ok := int32At(artifactData, 2); ok {
		artifact.Type = pb.ArtifactType(typeCode)
	}
	if stateCode, ok := int32At(artifactData, 4); ok {
		artifact.State = pb.ArtifactState(stateCode)
	}
	for _, source := range parseArtifactSources(artifactData) {
		artifact.Sources = append(artifact.Sources, source)
	}

	// A finished artifact carries its rendered output as download URLs deep in
	// the payload (e.g. a slide deck's .pdf / .pptx links). Their presence is
	// authoritative proof the artifact is READY, independent of the state code
	// at [4] — which for slide decks is observed to hold 3 (the same value our
	// ArtifactState enum assigns to FAILED) on fully-rendered decks. Trust the
	// output over the ambiguous code so a completed deck is never reported as
	// failed. HAR-verified against a v9rmvd slide-deck payload (2026-06-06).
	if hasArtifactDownloadURL(artifactData) {
		artifact.State = pb.ArtifactState_ARTIFACT_STATE_READY
	}

	return artifact
}

// artifactDownloadURLPrefix is the host+path that fronts every rendered
// artifact download (slide-deck .pdf/.pptx, etc.).
const artifactDownloadURLPrefix = "https://contribution.usercontent.google.com/download"

// extractArtifactDownloadURLs walks an artifact's decoded payload and returns
// every rendered-output download URL it contains, in document order. Used both
// to prove completion and to surface the file links to the user.
func extractArtifactDownloadURLs(data interface{}) []string {
	var out []string
	seen := make(map[string]bool)
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case string:
			if strings.HasPrefix(t, artifactDownloadURLPrefix) && !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		case []interface{}:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(data)
	return out
}

// hasArtifactDownloadURL reports whether the payload contains at least one
// rendered-output download URL.
func hasArtifactDownloadURL(data interface{}) bool {
	return len(extractArtifactDownloadURLs(data)) > 0
}

func parseArtifactSources(artifactData []interface{}) []*pb.ArtifactSource {
	if len(artifactData) <= 3 {
		return nil
	}

	sourcesData, ok := artifactData[3].([]interface{})
	if !ok {
		return nil
	}

	var sources []*pb.ArtifactSource
	seen := make(map[string]bool)
	appendArtifactSources(sourcesData, seen, &sources)
	return sources
}

func appendArtifactSources(values []interface{}, seen map[string]bool, out *[]*pb.ArtifactSource) {
	if len(values) >= 3 {
		if id, ok := artifactSourceID(values[0]); ok && id != "" && !seen[id] {
			seen[id] = true
			source := &pb.ArtifactSource{SourceId: &pb.SourceId{SourceId: id}}
			if unknown3, ok := int32At(values, 2); ok {
				source.Unknown_3 = unknown3
			}
			*out = append(*out, source)
			return
		}
	}
	if len(values) == 1 {
		if id, ok := values[0].(string); ok && id != "" && !seen[id] {
			seen[id] = true
			*out = append(*out, &pb.ArtifactSource{SourceId: &pb.SourceId{SourceId: id}})
			return
		}
	}
	for _, value := range values {
		nested, ok := value.([]interface{})
		if !ok {
			continue
		}
		appendArtifactSources(nested, seen, out)
	}
}

func artifactSourceID(value interface{}) (string, bool) {
	values, ok := value.([]interface{})
	if !ok || len(values) != 1 {
		return "", false
	}
	id, ok := values[0].(string)
	return id, ok
}

// Guidebook operations

// ListGuidebooks lists recently viewed guidebooks.
func (c *Client) ListGuidebooks(ctx context.Context) ([]*pb.Guidebook, error) {
	req := &pb.ListRecentlyViewedGuidebooksRequest{}
	resp, err := c.guidebooksService.ListRecentlyViewedGuidebooks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list guidebooks: %w", err)
	}
	return resp.Guidebooks, nil
}

// GetGuidebook returns a guidebook by ID.
func (c *Client) GetGuidebook(ctx context.Context, guidebookID string) (*pb.Guidebook, error) {
	req := &pb.GetGuidebookRequest{GuidebookId: guidebookID}
	resp, err := c.guidebooksService.GetGuidebook(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get guidebook: %w", err)
	}
	return resp, nil
}

// DeleteGuidebook deletes a guidebook by ID.
func (c *Client) DeleteGuidebook(ctx context.Context, guidebookID string) error {
	req := &pb.DeleteGuidebookRequest{GuidebookId: guidebookID}
	_, err := c.guidebooksService.DeleteGuidebook(ctx, req)
	if err != nil {
		return fmt.Errorf("delete guidebook: %w", err)
	}
	return nil
}

// PublishGuidebook publishes a guidebook.
func (c *Client) PublishGuidebook(ctx context.Context, guidebookID string) (*pb.PublishGuidebookResponse, error) {
	req := &pb.PublishGuidebookRequest{GuidebookId: guidebookID}
	resp, err := c.guidebooksService.PublishGuidebook(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("publish guidebook: %w", err)
	}
	return resp, nil
}

// ShareGuidebook creates sharing details for a guidebook.
func (c *Client) ShareGuidebook(ctx context.Context, guidebookID string) (*pb.ShareGuidebookResponse, error) {
	req := &pb.ShareGuidebookRequest{GuidebookId: guidebookID}
	resp, err := c.guidebooksService.ShareGuidebook(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("share guidebook: %w", err)
	}
	return resp, nil
}

// GetGuidebookDetails returns sharing and metadata details for a guidebook.
func (c *Client) GetGuidebookDetails(ctx context.Context, guidebookID string) (*pb.GuidebookDetails, error) {
	req := &pb.GetGuidebookDetailsRequest{GuidebookId: guidebookID}
	resp, err := c.guidebooksService.GetGuidebookDetails(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get guidebook details: %w", err)
	}
	return resp, nil
}

// GuidebookAsk asks a question against a guidebook.
func (c *Client) GuidebookAsk(ctx context.Context, guidebookID, question string) (*pb.GuidebookGenerateAnswerResponse, error) {
	req := &pb.GuidebookGenerateAnswerRequest{
		GuidebookId: guidebookID,
		Question:    question,
	}
	resp, err := c.guidebooksService.GuidebookGenerateAnswer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("guidebook ask: %w", err)
	}
	return resp, nil
}

// Slide deck operations

// SlideDeckFormat selects the deck layout NotebookLM generates. The
// presenter format is experimental; see intmethod.SlideDeckFormat.
type SlideDeckFormat = intmethod.SlideDeckFormat

const (
	// SlideDeckFormatDetailed generates a detailed slide deck.
	SlideDeckFormatDetailed = intmethod.SlideDeckFormatDetailed
	// SlideDeckFormatPresenter generates an experimental presenter deck.
	SlideDeckFormatPresenter = intmethod.SlideDeckFormatPresenter
)

// CreateSlideDeck generates a detailed slide deck from every source in the
// notebook. It is a convenience wrapper over CreateSlideDeckWithOptions.
func (c *Client) CreateSlideDeck(ctx context.Context, projectID, instructions string) (string, error) {
	return c.CreateSlideDeckWithOptions(ctx, projectID, instructions, nil, SlideDeckFormatDetailed)
}

// CreateSlideDeckWithOptions generates a slide deck in the given format. When
// sourceIDs is empty, every source in the notebook is used; otherwise only the
// listed sources are included. An empty sourceIDs slice after expansion (a
// notebook with no sources) is an error.
func (c *Client) CreateSlideDeckWithOptions(ctx context.Context, projectID, instructions string, sourceIDs []string, format SlideDeckFormat) (string, error) {
	if len(sourceIDs) == 0 {
		project, err := c.GetProject(ctx, projectID)
		if err != nil {
			return "", fmt.Errorf("get project sources: %w", err)
		}
		for _, src := range project.Sources {
			if src.SourceId != nil {
				sourceIDs = append(sourceIDs, src.SourceId.SourceId)
			}
		}
	}
	if len(sourceIDs) == 0 {
		return "", fmt.Errorf("notebook has no sources")
	}
	if instructions == "" && format == SlideDeckFormatDetailed {
		artifact, err := c.orchestrationService.CreateUniversalArtifact(ctx, &pb.CreateUniversalArtifactRequest{
			Context:   universalArtifactRequestContext(),
			ProjectId: projectID,
			Options: &pb.UniversalArtifactOptions{
				Kind:         8,
				SourceGroups: universalArtifactSourceGroups(sourceIDs),
				Slides:       []*pb.UniversalSlideOptions{{Language: "en", Format: 2, Style: 4}},
			},
		})
		if err != nil {
			return "", fmt.Errorf("create slide deck: %w", err)
		}
		if artifact.GetArtifactId() == "" {
			return "", fmt.Errorf("create returned no artifact id (the server may have rejected it, e.g. quota exhausted); check 'nlm artifact list'")
		}
		return artifact.GetArtifactId(), nil
	}

	args := intmethod.EncodeCreateSlideDeckArgs(projectID, sourceIDs, instructions, "en", format)
	call := rpc.Call{
		ID:         "R7cb6c",
		NotebookID: projectID,
		Args:       args,
	}
	resp, err := c.rpc.Do(ctx, call)
	if err != nil {
		return "", fmt.Errorf("create slide deck: %w", err)
	}
	return createdArtifactIDFromProtoWithOptions(resp, c.unmarshalOptions())
}

// createdArtifactIDFromProto projects the nested Artifact response returned
// by legacy R7cb6c create paths. The custom request encoders remain in place;
// only response decoding moves to the generated Artifact message.
func createdArtifactIDFromProto(resp []byte) (string, error) {
	return createdArtifactIDFromProtoWithOptions(resp, beprotojson.UnmarshalOptions{DiscardUnknown: true})
}

func createdArtifactIDFromProtoWithOptions(resp []byte, options beprotojson.UnmarshalOptions) (string, error) {
	var outer []json.RawMessage
	if err := json.Unmarshal(resp, &outer); err != nil {
		return "", fmt.Errorf("decode create response: %w", err)
	}
	if len(outer) > 0 {
		var artifact pb.Artifact
		if err := options.Unmarshal(outer[0], &artifact); err == nil && artifact.GetArtifactId() != "" {
			return artifact.GetArtifactId(), nil
		}
	}
	return "", fmt.Errorf("create returned no artifact id (the server may have rejected it, e.g. quota exhausted); check 'nlm artifact list'")
}

// CreateReport creates a report artifact via R7cb6c (mode 4).
// reportType and reportDescription typically come from GenerateReportSuggestions.
// instructions is an optional custom user prompt.
// Returns the artifact ID. The report is generated asynchronously;
// poll with ListArtifacts to check completion status.
// CreateReport creates a report artifact via R7cb6c.
// If targetSourceIDs is non-empty, only those sources are used; otherwise all project sources are included.
func (c *Client) CreateReport(ctx context.Context, projectID, reportType, reportDescription, instructions string, targetSourceIDs ...string) (string, error) {
	var sourceIDs []string
	if len(targetSourceIDs) > 0 && len(targetSourceIDs[0]) > 0 {
		sourceIDs = targetSourceIDs
	} else {
		project, err := c.GetProject(ctx, projectID)
		if err != nil {
			return "", fmt.Errorf("get project sources: %w", err)
		}
		for _, src := range project.Sources {
			if src.SourceId != nil {
				sourceIDs = append(sourceIDs, src.SourceId.SourceId)
			}
		}
	}
	if len(sourceIDs) == 0 {
		return "", fmt.Errorf("notebook has no sources")
	}

	args := intmethod.EncodeCreateReportArgs(projectID, sourceIDs, reportType, reportDescription, instructions)
	call := rpc.Call{
		ID:         "R7cb6c",
		NotebookID: projectID,
		Args:       args,
	}
	resp, err := c.rpc.Do(ctx, call)
	if err != nil {
		return "", fmt.Errorf("create report: %w", err)
	}

	var raw []interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return "", fmt.Errorf("parse report response: %w", err)
	}
	if len(raw) > 0 {
		if id, ok := raw[0].(string); ok {
			return id, nil
		}
		if inner, ok := raw[0].([]interface{}); ok && len(inner) > 0 {
			if id, ok := inner[0].(string); ok {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("unexpected report response format")
}

// ArtifactSuggestion is one blueprint returned by GenerateArtifactSuggestions.
// Title and Description are AI-authored; pass Description (optionally edited)
// to CreateAudioOverview as the instructions argument.
type ArtifactSuggestion struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// GenerateArtifactSuggestions calls the otmP3b RPC to fetch AI-generated
// topic blueprints for a notebook's sources. The UI uses these as the
// starting point for audio/video/slides creation; users can select or
// edit one and feed it as the instructions argument to the matching
// CreateX method.
//
// Only kind=ArtifactSuggestionKindAudio is HAR-verified today. Other
// kinds will likely work but are not attested.
//
// variation controls which of several suggestion sets the server
// returns. The UI increments this each time the user clicks "refresh";
// 1 is a reasonable default.
func (c *Client) GenerateArtifactSuggestions(ctx context.Context, projectID string, kind int, variation int) ([]ArtifactSuggestion, error) {
	project, err := c.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project sources: %w", err)
	}
	var sourceIDs []string
	for _, src := range project.Sources {
		if src.SourceId != nil {
			sourceIDs = append(sourceIDs, src.SourceId.SourceId)
		}
	}
	if len(sourceIDs) == 0 {
		return nil, fmt.Errorf("notebook has no sources")
	}

	args := intmethod.EncodeGenerateArtifactSuggestionsArgs(kind, projectID, sourceIDs, variation)
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCAudioTopicSuggestions,
		NotebookID: projectID,
		Args:       args,
	})
	if err != nil {
		return nil, fmt.Errorf("generate artifact suggestions: %w", err)
	}

	// Response shape: [[[title, description], [title, description], ...]]
	// The outer wrapper carries the suggestion list at outer[0].
	var outer []interface{}
	if err := json.Unmarshal(resp, &outer); err != nil {
		return nil, fmt.Errorf("parse suggestions response: %w", err)
	}
	if len(outer) == 0 {
		return nil, nil
	}
	items, ok := outer[0].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected suggestions response shape")
	}
	var suggestions []ArtifactSuggestion
	for _, item := range items {
		pair, ok := item.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}
		title, _ := pair[0].(string)
		description, _ := pair[1].(string)
		if title == "" && description == "" {
			continue
		}
		suggestions = append(suggestions, ArtifactSuggestion{
			Title:       title,
			Description: description,
		})
	}
	return suggestions, nil
}

// Generation operations

// SourceGuide is the per-source summary + key-topic chips the web UI shows
// next to each source. The frontend JS fires a `keyTopicAsked` event when a
// chip is clicked, so we call them key topics rather than keywords or
// prompts. The wire response is positional JSON; no proto round-trips.
type SourceGuide struct {
	Summary   string   `json:"summary"`
	KeyTopics []string `json:"key_topics"`
}

// GenerateSourceGuide returns the per-source guide (auto-summary + key-topic
// chips) that the web UI shows next to each source. The wire call is the
// same tr032e RPC that post-upload processing fires, but keyed by source_id
// with the 4-level nested shape [[[["source_id"]]]]. The response shape is
// [[[null, ["summary"], [["topic", "topic", ...]], []]]].
func (c *Client) GenerateSourceGuide(ctx context.Context, sourceID string) (*SourceGuide, error) {
	resp, err := c.orchestrationService.GenerateDocumentGuides(ctx, &pb.GenerateDocumentGuidesRequest{
		Sources: &pb.GenerateDocumentGuideSources{Source: &pb.GenerateDocumentGuideSource{
			Source: &pb.SourceIdList{SourceId: sourceID},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("generate source guide: %w", err)
	}
	return sourceGuideFromProto(resp), nil
}

func sourceGuideFromProto(resp *pb.GenerateDocumentGuidesResponse) *SourceGuide {
	g := &SourceGuide{}
	if resp == nil || len(resp.GetGuides()) == 0 {
		return g
	}
	guide := resp.GetGuides()[0]
	g.Summary = guide.GetSummary().GetText()
	g.KeyTopics = append(g.KeyTopics, guide.GetTopics().GetTopics()...)
	return g
}

// GenerateNotebookGuide generates the notebook-level guide.
func (c *Client) GenerateNotebookGuide(ctx context.Context, projectID string) (*pb.GenerateNotebookGuideResponse, error) {
	guide, err := c.orchestrationService.GenerateNotebookGuide(ctx, &pb.GenerateNotebookGuideRequest{
		ProjectId: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate notebook guide: %w", err)
	}
	return guide, nil
}

// GenerateMagicView generates a magic-view artifact for a notebook.
func (c *Client) GenerateMagicView(ctx context.Context, projectID string, sourceIDs []string) (*pb.GenerateMagicViewResponse, error) {
	_ = sourceIDs // The captured uK8f7c request carries only context and project ID.
	req := &pb.GenerateMagicViewRequest{
		Context: &pb.MagicViewRequestContext{
			Version: proto.Int32(2),
			Surface: &pb.MagicViewRequestSurface{Value: proto.Int32(1)},
			Caps:    &pb.MagicViewRequestCaps{Version: proto.Int32(1), CapabilityCodes: []int32{1, 3}},
		},
		ProjectId: projectID,
	}
	magicView, err := c.orchestrationService.GenerateMagicView(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate magic view: %w", err)
	}
	return magicView, nil
}

// GenerateOutline generates an outline for a notebook.
func (c *Client) GenerateOutline(ctx context.Context, projectID string) (*pb.GenerateOutlineResponse, error) {
	req := &pb.GenerateOutlineRequest{
		ProjectId: projectID,
	}
	outline, err := c.orchestrationService.GenerateOutline(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate outline: %w", err)
	}
	return outline, nil
}

// GenerateSection generates the next notebook section.
func (c *Client) GenerateSection(ctx context.Context, projectID string) (*pb.GenerateSectionResponse, error) {
	req := &pb.GenerateSectionRequest{
		ProjectId: projectID,
	}
	section, err := c.orchestrationService.GenerateSection(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate section: %w", err)
	}
	return section, nil
}

// StartDraft starts a notebook draft.
func (c *Client) StartDraft(ctx context.Context, projectID string) (*pb.StartDraftResponse, error) {
	req := &pb.StartDraftRequest{
		ProjectId: projectID,
	}
	draft, err := c.orchestrationService.StartDraft(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("start draft: %w", err)
	}
	return draft, nil
}

// StartSection starts a notebook section.
func (c *Client) StartSection(ctx context.Context, projectID string) (*pb.StartSectionResponse, error) {
	req := &pb.StartSectionRequest{
		ProjectId: projectID,
	}
	section, err := c.orchestrationService.StartSection(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("start section: %w", err)
	}
	return section, nil
}
