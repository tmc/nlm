package notebooklm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/gen/service"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	intmethod "github.com/tmc/nlm/internal/method"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
)

// Notebook is a NotebookLM project.
type Notebook = pb.Project

// Note is a notebook note.
//
// Rich and Grounding preserve the structured document returned for rich notes.
// RichText remains the flat text returned for plain notes.
type Note struct {
	*pb.Note
	Rich      *pb.RichDocument
	Grounding []*pb.Grounding
}

// AppArtifactKind identifies the kind of generated app artifact.
type AppArtifactKind int32

const (
	// AppArtifactKindFlashcards identifies a flashcards artifact.
	AppArtifactKindFlashcards AppArtifactKind = 1
	// AppArtifactKindQuiz identifies a quiz artifact.
	AppArtifactKindQuiz AppArtifactKind = 2
	// AppArtifactKindPrototype identifies an interactive prototype artifact.
	AppArtifactKindPrototype AppArtifactKind = 3
	// AppArtifactKindMindmap identifies a mind-map artifact.
	AppArtifactKindMindmap AppArtifactKind = 4
	// AppArtifactKindCanvas identifies a canvas artifact.
	AppArtifactKindCanvas AppArtifactKind = 5
)

// ParseAppArtifactKind parses an app artifact kind name.
func ParseAppArtifactKind(s string) (AppArtifactKind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "flashcards", "flashcard":
		return AppArtifactKindFlashcards, nil
	case "quiz":
		return AppArtifactKindQuiz, nil
	case "prototype", "notebook-app", "notebook_app":
		return AppArtifactKindPrototype, nil
	case "mindmap", "mindmap_app", "mind-map":
		return AppArtifactKindMindmap, nil
	case "canvas":
		return AppArtifactKindCanvas, nil
	default:
		return 0, fmt.Errorf("unknown app artifact type %q", s)
	}
}

// String returns the app artifact kind name.
func (k AppArtifactKind) String() string {
	switch k {
	case AppArtifactKindFlashcards:
		return "flashcards"
	case AppArtifactKindQuiz:
		return "quiz"
	case AppArtifactKindPrototype:
		return "prototype"
	case AppArtifactKindMindmap:
		return "mindmap"
	case AppArtifactKindCanvas:
		return "canvas"
	default:
		return fmt.Sprintf("AppArtifactKind(%d)", k)
	}
}

func (k AppArtifactKind) valid() bool {
	switch k {
	case AppArtifactKindFlashcards, AppArtifactKindQuiz,
		AppArtifactKindPrototype, AppArtifactKindMindmap, AppArtifactKindCanvas:
		return true
	default:
		return false
	}
}

// CreateAudioOverviewOptions configures audio overview generation.
type CreateAudioOverviewOptions struct {
	Instructions string
	AudioType    pb.AudioType
	Length       pb.AudioLength
	Language     string
	SourceIDs    []string
}

func (opts CreateAudioOverviewOptions) withDefaults() CreateAudioOverviewOptions {
	if opts.AudioType == pb.AudioType_AUDIO_TYPE_UNSPECIFIED {
		opts.AudioType = pb.AudioType_AUDIO_TYPE_DEEP_DIVE
	}
	if opts.Length == pb.AudioLength_AUDIO_LENGTH_UNSPECIFIED {
		opts.Length = pb.AudioLength_AUDIO_LENGTH_DEFAULT
	}
	if opts.Language == "" {
		opts.Language = "en"
	}
	return opts
}

// CreateVideoOverviewOptions configures video overview generation.
type CreateVideoOverviewOptions struct {
	Instructions string
	AudioType    pb.AudioType
	VideoStyle   pb.VideoStyle
	Language     string
	SourceIDs    []string
}

func (opts CreateVideoOverviewOptions) withDefaults() CreateVideoOverviewOptions {
	if opts.AudioType == pb.AudioType_AUDIO_TYPE_UNSPECIFIED {
		opts.AudioType = pb.AudioType_AUDIO_TYPE_BRIEF
	}
	if opts.Language == "" {
		opts.Language = "en"
	}
	return opts
}

// httpClientWithTimeout returns an IPv4-preferring HTTP client with the given timeout.
func httpClientWithTimeout(timeout time.Duration) *http.Client {
	c := batchexecute.NewIPv4HTTPClient()
	c.Timeout = timeout
	return c
}

// idleTimeoutReader wraps a reader and enforces a per-read idle timeout.
// Unlike http.Client.Timeout which limits the entire request/response,
// this resets the deadline on each successful read — suitable for
// long-running streaming responses where data arrives in chunks.
type idleTimeoutReader struct {
	r        io.ReadCloser
	timeout  time.Duration
	timer    *time.Timer
	done     chan struct{}
	close    sync.Once
	closeErr error
}

func newIdleTimeoutReader(r io.ReadCloser, timeout time.Duration) *idleTimeoutReader {
	return &idleTimeoutReader{
		r:       r,
		timeout: timeout,
		timer:   time.NewTimer(timeout),
		done:    make(chan struct{}),
	}
}

// Read reads from the underlying response body with an idle timeout.
func (r *idleTimeoutReader) Read(p []byte) (int, error) {
	// Reset the idle timer before each read.
	r.timer.Reset(r.timeout)
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		n, err := r.r.Read(p)
		ch <- result{n, err}
	}()
	select {
	case res := <-ch:
		return res.n, res.err
	case <-r.timer.C:
		return 0, fmt.Errorf("idle timeout: no data received for %s", r.timeout)
	case <-r.done:
		return 0, fmt.Errorf("reader closed")
	}
}

// Close closes the underlying response body.
func (r *idleTimeoutReader) Close() error {
	r.close.Do(func() {
		close(r.done)
		r.timer.Stop()
		r.closeErr = r.r.Close()
	})
	return r.closeErr
}

// Client handles NotebookLM API interactions.
type Client struct {
	rpc                  *rpc.Client
	orchestrationService *service.LabsTailwindOrchestrationServiceClient
	sharingService       *service.LabsTailwindSharingServiceClient
	guidebooksService    *service.LabsTailwindGuidebooksServiceClient
	config               clientConfig
}

// New creates a new NotebookLM API client.
func New(credentials Credentials, options ...Option) *Client {
	var config clientConfig
	for _, option := range options {
		option(&config)
	}

	batchOptions := append([]batchexecute.Option(nil), config.batchOptions...)
	batchOptions = append(batchOptions,
		batchexecute.WithDebug(config.Debug),
		batchexecute.WithProtoDebug(config.DebugParsing, config.DebugFieldMapping),
	)
	if config.AuthUser != "" {
		batchOptions = append(batchOptions,
			batchexecute.WithURLParams(map[string]string{"authuser": config.AuthUser}),
			batchexecute.WithHeaders(map[string]string{"x-goog-authuser": config.AuthUser}),
		)
	}

	client := &Client{
		rpc:                  rpc.New(credentials.AuthToken, credentials.Cookies, batchOptions...),
		orchestrationService: service.NewLabsTailwindOrchestrationServiceClient(credentials.AuthToken, credentials.Cookies, batchOptions...),
		sharingService:       service.NewLabsTailwindSharingServiceClient(credentials.AuthToken, credentials.Cookies, batchOptions...),
		guidebooksService:    service.NewLabsTailwindGuidebooksServiceClient(credentials.AuthToken, credentials.Cookies, batchOptions...),
		config:               config,
	}
	return client
}

func (c *Client) unmarshal(b []byte, m proto.Message) error {
	return c.unmarshalOptions().Unmarshal(b, m)
}

func (c *Client) unmarshalOptions() beprotojson.UnmarshalOptions {
	options := beprotojson.UnmarshalOptions{DiscardUnknown: true}
	if c == nil || c.rpc == nil {
		return options
	}
	config := c.rpc.Config
	options.DebugParsing = config.DebugParsing
	options.DebugFieldMapping = config.DebugFieldMapping
	return options
}

// setChromeClientHints sets User-Agent and the sec-ch-ua-* client hint headers,
// plus the Sec-Fetch-* trio and Origin. Scotty's /upload/_/ endpoint rejects
// requests without browser-style headers with 500 + X-Goog-Upload-Status: final.
// Values mirror a current Brave/Chromium build.
func setChromeClientHints(h http.Header) {
	h.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36")
	h.Set("sec-ch-ua", `"Brave";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	h.Set("sec-ch-ua-arch", `"arm"`)
	h.Set("sec-ch-ua-bitness", `"64"`)
	h.Set("sec-ch-ua-full-version-list", `"Brave";v="147.0.0.0", "Not.A/Brand";v="8.0.0.0", "Chromium";v="147.0.0.0"`)
	h.Set("sec-ch-ua-mobile", "?0")
	h.Set("sec-ch-ua-model", `""`)
	h.Set("sec-ch-ua-platform", `"macOS"`)
	h.Set("sec-ch-ua-platform-version", `"26.4.1"`)
	h.Set("sec-ch-ua-wow64", "?0")
	h.Set("Origin", "https://notebook.google.com")
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Dest", "empty")
}

// Project/Notebook operations

// ListRecentlyViewedProjects lists the account's recently viewed notebooks.
func (c *Client) ListRecentlyViewedProjects(ctx context.Context) ([]*Notebook, error) {
	req := &pb.ListRecentlyViewedProjectsRequest{}

	response, err := c.orchestrationService.ListRecentlyViewedProjects(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	return response.Projects, nil
}

// CreateProject creates a notebook with the given title and emoji.
func (c *Client) CreateProject(ctx context.Context, title string, emoji string) (*Notebook, error) {
	req := &pb.CreateProjectRequest{
		Title: title,
		Emoji: emoji,
	}

	project, err := c.orchestrationService.CreateProject(ctx, req)
	if err != nil {
		count, limit := -1, -1
		if status, statusErr := c.GetAccountStatus(ctx); statusErr == nil {
			limit = status.NotebookLimit
		}
		if projects, listErr := c.ListRecentlyViewedProjects(ctx); listErr == nil {
			count = len(projects)
		}
		return nil, classifyCreateProjectError(err, count, limit)
	}
	return project, nil
}

// nearNotebookCapTolerance is the gap below NotebookLimit at which a generic
// CreateProject failure (Invalid argument / Failed precondition with no
// machine-readable details) is reclassified as ErrNotebookCapReached.
//
// ListRecentlyViewedProjects underreports the server's true notebook count —
// it omits archived, shared-from, and otherwise-not-recently-touched
// notebooks. An account that displays "492 / 500" in `nlm account` can still
// be at the server-side cap. The web UI surfaces an explicit "limit reached"
// banner; batchexecute returns a bare code-3 with no diagnostic. The
// tolerance trades a small false-positive risk (a malformed title at 481/500
// gets blamed on quota) for catching the much more common true-positive
// (creates fail near the cap because the cap is full).
const nearNotebookCapTolerance = 20

// classifyCreateProjectError wraps a CreateProject failure with
// ErrNotebookCapReached when the surrounding evidence — current notebook
// count and account limit — points at quota exhaustion. The exact wire
// signal is unreliable (see nearNotebookCapTolerance) so we combine the
// bare error code with an account-state heuristic.
//
// count and limit are -1 when unavailable (e.g. fetching account status
// failed); in that case we leave the error unwrapped rather than guess.
func classifyCreateProjectError(err error, count, limit int) error {
	if err == nil {
		return nil
	}
	wrapCap := func() error {
		return fmt.Errorf("create project: %w", &NotebookCapError{
			Count: count,
			Limit: limit,
			Err:   err,
		})
	}
	if limit > 0 && count >= 0 {
		// Strict cap: the client-side count is already at or above the
		// reported limit. ListRecentlyViewedProjects underreports, so this
		// path is rare in practice but unambiguous when it triggers.
		if count >= limit {
			return wrapCap()
		}
		// Heuristic cap: the client-side count is within a small tolerance
		// of the limit AND the underlying error is the polysemic "bad args
		// / failed precondition" code that the server returns for quota
		// exhaustion (alongside genuinely-malformed input). The web UI
		// distinguishes these via UI state we don't have access to here.
		if count >= limit-nearNotebookCapTolerance && isLikelyQuotaError(err) {
			return wrapCap()
		}
	}
	return fmt.Errorf("create project: %w", err)
}

// isLikelyQuotaError reports whether err looks like the bare batchexecute
// error the server returns for CreateProject when the account is at the
// notebook cap. Code 3 ("Invalid argument") is the actually-observed code;
// code 9 ("Failed precondition") is included because the wire layer maps
// some quota states there as well. Either code can also indicate genuine
// bad input — the caller must combine this with out-of-band evidence
// (account count) before classifying.
func isLikelyQuotaError(err error) bool {
	var apiErr *batchexecute.APIError
	if !errors.As(err, &apiErr) || apiErr.ErrorCode == nil {
		return false
	}
	switch apiErr.ErrorCode.Code {
	case 3, 9:
		return true
	}
	return false
}

func classifyGetProjectError(projectID string, err error) error {
	var apiErr *batchexecute.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.ErrorCode != nil {
		switch apiErr.ErrorCode.Type {
		case batchexecute.ErrorTypeAuthorization,
			batchexecute.ErrorTypePermissionDenied,
			batchexecute.ErrorTypeNotFound:
			return &NotebookAccessError{NotebookID: projectID, Err: err}
		}
	}
	switch apiErr.HTTPStatus {
	case http.StatusForbidden, http.StatusNotFound:
		return &NotebookAccessError{NotebookID: projectID, Err: err}
	}
	return err
}

// GetProject returns a notebook and its sources.
func (c *Client) GetProject(ctx context.Context, projectID string) (*Notebook, error) {
	req := &pb.GetProjectRequest{
		ProjectId: projectID,
	}

	project, err := c.orchestrationService.GetProject(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", classifyGetProjectError(projectID, err))
	}

	if c.config.Debug && project.Sources != nil {
		fmt.Fprintf(os.Stderr, "DEBUG: Successfully parsed project with %d sources\n", len(project.Sources))
	}
	return project, nil
}

// DeleteProjects deletes the specified notebooks.
func (c *Client) DeleteProjects(ctx context.Context, projectIDs []string) error {
	req := &pb.DeleteProjectsRequest{
		ProjectIds: projectIDs,
	}

	_, err := c.orchestrationService.DeleteProjects(ctx, req)
	if err != nil {
		return fmt.Errorf("delete projects: %w", err)
	}
	return nil
}

// MutateProject applies the populated fields in updates to a notebook.
func (c *Client) MutateProject(ctx context.Context, projectID string, updates *pb.Project) (*Notebook, error) {
	req := &pb.MutateProjectRequest{
		ProjectId: projectID,
		Updates:   updates,
	}
	// Bypass the service client: its generated argbuilder encoder serializes
	// the Project submessage as a JSON object, which the server rejects. Use
	// the HAR-verified positional encoder from internal/method.
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCMutateProject,
		NotebookID: projectID,
		Args:       intmethod.EncodeMutateProjectArgs(req),
	})
	if err != nil {
		return nil, fmt.Errorf("mutate project: %w", err)
	}
	var project pb.Project
	if err := c.unmarshal(resp, &project); err != nil {
		return nil, fmt.Errorf("mutate project: unmarshal response: %w", err)
	}
	return &project, nil
}

// SetProjectDescription updates the notebook "creator notes" / description
// via the s0tc2d MutateProject RPC. Wire format is HAR-verified
// (2026-04-25); see internal/method/LabsTailwindOrchestrationService_MutateProject_encoder.go.
func (c *Client) SetProjectDescription(ctx context.Context, projectID, description string) error {
	_, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCMutateProject,
		NotebookID: projectID,
		Args:       intmethod.MutateProjectDescriptionArgs(projectID, description),
	})
	if err != nil {
		return fmt.Errorf("set project description: %w", err)
	}
	return nil
}
