package rpc

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/tmc/nlm/internal/batchexecute"
)

// ServiceConfig defines configuration for a generated BatchExecute service client.
type ServiceConfig struct {
	Host      string
	App       string
	URLParams map[string]string
}

// Canonical rpc_id → service-method bindings.
//
// The boq_labs-tailwind-frontend JS bundle is the source of truth for
// what each rpc_id is officially called. Many of the Go-side names
// below describe the higher-level api.Client feature that uses an
// RPC ("StartFastResearch", "DeleteDeepResearch") rather than the
// raw service contract ("DiscoverSourcesManifold",
// "FinishDiscoverSourcesRun"); both are correct in their own frame.
// The constant comments call out the JS-bundle name when it differs.
//
// Re-derive the table by extracting "rpc_id" → "/LabsTailwind*Service.Method"
// pairs from www.gstatic.com.jsonl in any HAR capture (the bundle
// embeds these mappings as JSPB descriptors). 71 orchestration+sharing
// bindings observed in the 2026-04-23 bundle (Guidebook IDs come from
// a separate bundle slice). Every bundle binding has Go-side wiring
// (constant + proto rpc method).
// The recurring divergences are:
//
//   J7Gthc -> DeleteChatTurns           (we call it RateConversationTurn)
//   Ljjv0c -> DiscoverSourcesManifold   (we call it StartFastResearch)
//   QA9ei  -> DiscoverSourcesAsync      (we call it StartDeepResearch)
//   e3bVqc -> ListDiscoverSourcesJob    (we call it DeleteChatHistory / GetDeepResearchSessions)
//   LBwxtb -> FinishDiscoverSourcesRun  (we call it DeleteDeepResearch / BulkImportFromResearch)
//   KmcKPe -> DeriveArtifact            (we call it ReviseArtifact)
//   eyWvXc -> SendSdpOffer              (we call it SDPExchange)
//   Of0kDd -> GetIceConfig              (we call it FetchInteractivityToken)
//   o4cbdc -> AddTentativeSources       (we call it AddFileSource)
//   otmP3b -> GeneratePromptSuggestions (we call it AudioTopicSuggestions)
//   sqTeoe -> GetArtifactCustomizationChoices (we call it GetAudioFormats)
//
// These divergences are intentional — the api.Client API surfaces
// reflect the user-facing feature, not the wire endpoint name.
// Future readers grepping for a wire id should consult the JS bundle
// (www.gstatic.com .jsonl in any capture) for the canonical name.

// RPC endpoint IDs for NotebookLM services
const (
	// NotebookLM service - Project/Notebook operations
	RPCListRecentlyViewedProjects = "wXbhsf" // ListRecentlyViewedProjects
	RPCCreateProject              = "CCqFvf" // CreateProject
	RPCGetProject                 = "rLM1Ne" // GetProject
	RPCDeleteProjects             = "WWINqb" // DeleteProjects
	RPCMutateProject              = "s0tc2d" // MutateProject
	RPCRemoveRecentlyViewed       = "fejl7e" // RemoveRecentlyViewedProject

	// NotebookLM service - Source operations
	RPCAddSources           = "izAoDd" // AddSources
	RPCAddFileSource        = "o4cbdc" // AddFileSource (register uploaded file). JS bundle: AddTentativeSources.
	RPCDeleteSources        = "tGMBJ"  // DeleteSources
	RPCMutateSource         = "b7Wfje" // MutateSource
	RPCRefreshSource        = "FLmJqe" // RefreshSource
	RPCLoadSource           = "hizoJc" // LoadSource
	RPCCheckSourceFreshness = "yR9Yof" // CheckSourceFreshness
	RPCActOnSources         = "yyryJe" // ActOnSources
	// RPCDiscoverSources: the JS bundle binds Es3dTe to
	// /LabsTailwindOrchestrationService.DiscoverSources. The previous
	// "qXyaNe" value was a speculative inference that is not present
	// in any JS bundle or HAR capture. No HAR evidence yet for the
	// wire shape; TODO(har): capture by triggering "Discover sources"
	// from the source-add UI.
	RPCDiscoverSources = "Es3dTe"

	// NotebookLM service - Note operations
	RPCCreateNote  = "CYK0Xb" // CreateNote
	RPCMutateNote  = "cYAfTb" // MutateNote
	RPCDeleteNotes = "AH0mwd" // DeleteNotes
	RPCGetNotes    = "cFji9"  // GetNotes

	// NotebookLM service - Audio operations
	RPCCreateAudioOverview     = "AHyHrd" // CreateAudioOverview
	RPCGetAudioOverview        = "VUsiyb" // GetAudioOverview
	RPCDeleteAudioOverview     = "sJDbic" // DeleteAudioOverview
	RPCFetchInteractivityToken = "Of0kDd" // FetchInteractivityToken - voice session auth + ICE config
	RPCSDPExchange             = "eyWvXc" // SDP offer/answer exchange for WebRTC negotiation

	// NotebookLM service - Video/Artifact operations
	RPCCreateUniversalArtifact = "R7cb6c" // Universal artifact creator (audio, video, reports, flashcards, slides, infographics)
	RPCCreateVideoOverview     = "R7cb6c" // Alias for backward compatibility

	// NotebookLM service - Chat operations
	// NOTE: GenerateFreeFormStreamed does NOT use batchexecute. It uses a gRPC-Web
	// endpoint at /_/LabsTailwindUi/data/google.internal.labs.tailwind.orchestration.v1.LabsTailwindOrchestrationService/GenerateFreeFormStreamed
	// The "BD" ID was incorrectly assumed; kept only for reference.
	RPCGenerateFreeFormStreamed = "BD"     // DEPRECATED: chat uses gRPC-Web, not batchexecute
	RPCGetConversations         = "hPTbtc" // GetConversations - list conversation IDs for a notebook
	RPCGetConversationHistory   = "khqZz"  // GetConversationHistory - retrieve chat messages
	RPCDeleteChatHistory        = "e3bVqc" // DeleteChatHistory / PollDeepResearch - server routes by args
	RPCRateConversationTurn     = "J7Gthc" // RateConversationTurn - mark conversation turn (thumbs up/down?)

	// NotebookLM service - Research operations
	//
	// Note: the JS bundle gives a wholly-different naming for these
	// rpc_ids than what api.Client uses today:
	//   Ljjv0c → DiscoverSourcesManifold     (we call it StartFastResearch)
	//   QA9ei  → DiscoverSourcesAsync        (we call it StartDeepResearch)
	//   e3bVqc → ListDiscoverSourcesJob      (we call it DeleteChatHistory / GetDeepResearchSessions; polymorphic — verified)
	//   LBwxtb → FinishDiscoverSourcesRun    (we call it DeleteDeepResearch / BulkImportFromResearch)
	// The Go-side names reflect how api.Client uses these RPCs (the
	// "research" feature is built on top of the DiscoverSources job
	// system); the JS-bundle names reflect the raw service contract.
	// Both are correct in their own frame.
	RPCStartFastResearch       = "Ljjv0c"             // StartFastResearch (JS: DiscoverSourcesManifold) - HAR-verified 2026-04-17; response is [conversation_id]
	RPCFastResearch            = RPCStartFastResearch // DEPRECATED alias.
	RPCStartDeepResearch       = "QA9ei"              // StartDeepResearch (JS: DiscoverSourcesAsync) - initiate deep research; returns [research_id, conversation_id]
	RPCGetDeepResearchSessions = "e3bVqc"             // GetDeepResearchSessions (JS: ListDiscoverSourcesJob) - returns ALL deep/fast research sessions for a notebook
	RPCDeleteDeepResearch      = "LBwxtb"             // DeleteDeepResearch (JS: FinishDiscoverSourcesRun) - soft-delete a session
	RPCBulkImportFromResearch  = "LBwxtb"             // BulkImportFromResearch (5-position variant of LBwxtb)

	// Deprecated: e3bVqc is a polymorphic RPC that serves
	// DeleteChatHistory, GetDeepResearchSessions, AND (as of 2026-04-17)
	// the session list for fast-research. Use the explicit constants
	// above at call sites; this alias kept for legacy callers.
	RPCPollDeepResearch = RPCGetDeepResearchSessions

	// NotebookLM service - Generation operations
	//
	// Notes on the four "draft/section" RPCs (lCjAd, BeTrYd, exXvGf,
	// pGC7gf): none appear in the boq_labs-tailwind-frontend JS bundle
	// or in any HAR capture under docs/captures/. They were inferred
	// from earlier proto drafts and likely belonged to a now-removed
	// authoring surface; the live report-authoring workflow is
	// ciyUvf → R7cb6c. Constants are kept so any latent caller still
	// compiles, but they should be considered dead.
	RPCGenerateDocumentGuides    = "tr032e" // GenerateDocumentGuides
	RPCGenerateNotebookGuide     = "VfAZjd" // GenerateNotebookGuide
	RPCGenerateOutline           = "lCjAd"  // DEPRECATED: dead; not in JS bundle. Use ciyUvf → R7cb6c.
	RPCGenerateSection           = "BeTrYd" // DEPRECATED: dead; not in JS bundle. Use ciyUvf → R7cb6c.
	RPCStartDraft                = "exXvGf" // DEPRECATED: dead; not in JS bundle.
	RPCStartSection              = "pGC7gf" // DEPRECATED: dead; not in JS bundle.
	RPCGenerateReportSuggestions = "ciyUvf" // GenerateReportSuggestions (HAR-verified; was GHsKob)
	RPCGetAudioFormats           = "sqTeoe" // GetAudioFormats - returns available audio overview types
	RPCGenerateMagicView         = "uK8f7c" // GenerateMagicView - JS-bundle-verified; companion to RPCGetMagicView (rtY7md)

	// NotebookLM service - Account operations
	RPCGetOrCreateAccount = "ZwVcOc" // GetOrCreateAccount
	RPCMutateAccount      = "hT54vc" // MutateAccount

	// NotebookLM service - Analytics operations
	RPCGetProjectAnalytics = "AUrzMb" // GetProjectAnalytics
	RPCSubmitFeedback      = "uNyJKe" // SubmitFeedback
	// RPCLogEvent: misnamed. The wire RPC is actually a
	// promo/upsell-card placement lookup that returns the user's
	// NotebookLM tier (e.g. "NOTEBOOKLM_TIER_PRO_CONSUMER_USER") and a
	// "Manage subscription" CTA. HAR-verified across 12+ captures
	// (notebooklm.google.com.jsonl, 2026-04-19..2026-04-23). The web
	// UI fires it once per page load. No Go caller invokes it; the
	// constant name is preserved so renaming doesn't break callers
	// elsewhere, but treat it as GetPromoCampaign for analytical
	// purposes. See proto LogEventResponse for the surfaced field.
	RPCLogEvent = "ozz5Z"

	// NotebookLMSharing service operations
	RPCShareAudio = "RGP97b" // ShareAudio
	// RPCGetProjectDetails. JS bundle confirms the canonical name:
	// JFMDGd → /LabsTailwindSharingService.GetProjectDetails.
	//
	// Caveat: in every HAR capture surveyed (12+ captures spanning
	// 2026-04-06..2026-04-23), the response carries only the
	// collaborators slice — per-user [email, role, display_name,
	// avatar_url] entries plus permission flags — and not the
	// title/emoji/thumbnail metadata that the proto's ProjectDetails
	// message also models. api.Client.parseProjectDetailsResponse
	// extracts only OwnerName + IsPublic from this list. The fuller
	// project-metadata fields may populate under different request
	// modes (the trailing [2] arg looks like a mode enum); they
	// remain unobserved in our corpus.
	RPCGetProjectDetails = "JFMDGd"
	RPCShareProject      = "QDyure" // ShareProject

	// NotebookLMGuidebooks service operations
	RPCDeleteGuidebook              = "ARGkVc" // DeleteGuidebook
	RPCGetGuidebook                 = "EYqtU"  // GetGuidebook
	RPCListRecentlyViewedGuidebooks = "YJBpHc" // ListRecentlyViewedGuidebooks
	RPCPublishGuidebook             = "R6smae" // PublishGuidebook
	RPCGetGuidebookDetails          = "LJyzeb" // GetGuidebookDetails
	RPCShareGuidebook               = "OTl0K"  // ShareGuidebook
	RPCGuidebookGenerateAnswer      = "itA0pc" // GuidebookGenerateAnswer

	// LabsTailwindOrchestrationService - Artifact operations
	//
	// GetArtifact (v9rmvd) does exist per the JS bundle binding to
	// /LabsTailwindOrchestrationService.GetArtifact. The web UI does not
	// invoke it (the rendering path scans gArtLc and filters
	// client-side, which api.Client.GetArtifact mirrors). The earlier
	// BnLyuf/WxBZtb inferences returned 400 — different, unreachable
	// ids. DeleteArtifact (V5N4be) is HAR-verified 2026-04-07; see
	// a NotebookLM web UI batchexecute capture.
	//
	// CreateArtifact / UpdateArtifact: the JS bundle's canonical
	// bindings are R7cb6c → CreateArtifact and rc3d8d → UpdateArtifact.
	// The legacy xpWGLf / DJezBc ids below are kept for backward
	// compatibility but never appeared on the wire in our HAR corpus;
	// new callers should use RPCCreateUniversalArtifact and
	// RPCRenameArtifact respectively.
	RPCGetArtifact    = "v9rmvd" // GetArtifact — JS-bundle-verified; never observed on the wire (UI uses gArtLc scan instead)
	RPCCreateArtifact = "xpWGLf" // DEPRECATED: stale; bundle says R7cb6c. Use RPCCreateUniversalArtifact.
	RPCUpdateArtifact = "DJezBc" // DEPRECATED: stale; bundle says rc3d8d. Use RPCRenameArtifact.
	RPCRenameArtifact = "rc3d8d" // RenameArtifact / UpdateArtifact (JS-bundle name) - for title and field updates
	RPCDeleteArtifact = "V5N4be" // DeleteArtifact — HAR-verified 2026-04-07
	RPCListArtifacts  = "gArtLc" // ListArtifacts - get artifacts list

	// LabsTailwindOrchestrationService - Additional operations
	RPCListFeaturedProjects = "ub2Bae" // ListFeaturedProjects
	// ReportContent: the JS bundle binds the live ReportContent RPC to
	// OmVMXc. The historical rJKx8e id never appeared in any HAR
	// capture or in the JS bundle's binding table; it was an early
	// inference that survived as a stale constant. Use
	// RPCReportContent (OmVMXc) for new call sites; the deprecated
	// alias is kept so any existing reference continues to compile.
	RPCReportContent         = "OmVMXc" // ReportContent (JS-bundle canonical)
	RPCReportContentLegacy   = "rJKx8e" // DEPRECATED: stale id, never observed on the wire; use RPCReportContent.
	RPCReviseArtifact        = "KmcKPe" // ReviseArtifact / DeriveArtifact (JS bundle name)
	RPCListCollections       = "ub2Bae" // ListCollections - list notebook collections/folders
	RPCAudioTopicSuggestions = "otmP3b" // AudioTopicSuggestions / GeneratePromptSuggestions (JS bundle name)

	// LabsTailwindOrchestrationService - Labels (autolabel)
	//
	// RPCGetLabels returns the per-notebook labels (autolabel
	// clusters). Wire request: [[2], project_id]. Response: [] or
	// [[name, [[src_id], ...], label_id, ""], ...]. HAR-verified
	// across 8+ NotebookLM web UI captures (2026-04-23). The companion
	// mutation RPCs CreateLabel (agX4Bc),
	// MutateLabel (le8sX), and DeleteLabels (GyzE7e) confirm the
	// labels framing.
	RPCGetLabels = "I3xc3c"

	// RPCUpsertArtifactUserState (JS bundle:
	// /LabsTailwindOrchestrationService.UpsertArtifactUserState) is
	// the write side of the per-user artifact-state pair; the read
	// side is GetArtifactUserState (ulBSjf). No HAR observed yet —
	// the upsert path is fired by user actions on generated artifacts
	// (star/view/dismiss).
	// TODO(har): capture before treating as callable.
	RPCUpsertArtifactUserState = "Fxmvse"
	RPCGetArtifactUserState    = "ulBSjf" // GetArtifactUserState — read companion to RPCUpsertArtifactUserState. TODO(har).

	// LabsTailwindOrchestrationService - Bundle bindings not yet wired
	// from a Go caller. Each ID is JS-bundle-verified in the
	// boq_labs-tailwind-frontend bundle but has no HAR capture in our corpus. Wire shapes for
	// these are unverified; capture HAR before encoding by exercising
	// the corresponding UI flow.
	RPCCreateLabel                   = "agX4Bc" // CreateLabel (write companion to GetLabels). TODO(har).
	RPCMutateLabel                   = "le8sX"  // MutateLabel (rename / re-categorize). TODO(har).
	RPCDeleteLabels                  = "GyzE7e" // DeleteLabels (bulk delete by label_id). TODO(har).
	RPCGenerateArtifact              = "Rytqqe" // GenerateArtifact — distinct from CreateArtifact (R7cb6c). TODO(har).
	RPCCancelDiscoverSourcesJob      = "Zbrupe" // CancelDiscoverSourcesJob (cancels the in-flight Es3dTe/Ljjv0c job). TODO(har).
	RPCExportToDrive                 = "Krh3pd" // ExportToDrive (export notebook artifacts to user's Drive). TODO(har).
	RPCUpdateFeaturedNotebookStatus  = "DemIHe" // UpdateFeaturedNotebookStatus (admin/internal). TODO(har).
	RPCListModelOptions              = "EnujNd" // ListModelOptions (returns the available chat/generation models). TODO(har).
	RPCUpdateProjectUserState        = "LQhfEb" // UpdateProjectUserState (per-user notebook state — last-viewed, pinned, etc.). TODO(har).
	RPCExecuteWritingFunction        = "likKIe" // ExecuteWritingFunction (in-document writing assistant — rewrite/expand/summarize). TODO(har).
	RPCListExpertIntelligenceContent = "mVtEUb" // ListExpertIntelligenceContent (curated featured-content surface). TODO(har).
	RPCGenerateAccessToken           = "preRPe" // GenerateAccessToken (per-session token mint, possibly for embed widgets). TODO(har).
	RPCGetMagicView                  = "rtY7md" // GetMagicView (companion to uK8f7c GenerateMagicView). TODO(har).
	RPCCopyProject                   = "te3DCe" // CopyProject (duplicate a notebook). TODO(har).
	RPCStreamGenerateFreeForm        = "laWbsf" // GenerateFreeFormStreamed (chat path; the live UI uses gRPC-Web — not batchexecute — but the JS bundle still maps the rpc_id).
	RPCCreateAccessRequest           = "n3dkHd" // CreateAccessRequest (LabsTailwindSharingService — request access to a shared notebook). TODO(har).
)

// Call represents a NotebookLM RPC call
type Call struct {
	ID         string        // RPC endpoint ID
	Args       []interface{} // Arguments for the call
	NotebookID string        // Optional notebook ID for context
}

// Client handles NotebookLM RPC communication
type Client struct {
	Config batchexecute.Config
	client *batchexecute.Client
}

// New creates a new NotebookLM RPC client
func New(authToken, cookies string, options ...batchexecute.Option) *Client {
	return NewWithConfig(authToken, cookies, ServiceConfig{
		Host: "notebooklm.google.com",
		App:  "LabsTailwindUi",
	}, options...)
}

// NewWithConfig creates a new NotebookLM RPC client with explicit service settings.
func NewWithConfig(authToken, cookies string, serviceConfig ServiceConfig, options ...batchexecute.Option) *Client {
	// Use session-specific parameters from env if available (set during auth)
	blParam := os.Getenv("NLM_BL_PARAM")
	if blParam == "" {
		blParam = "boq_labs-tailwind-frontend_20260406.14_p0"
	}
	sessionID := os.Getenv("NLM_SESSION_ID")
	if sessionID == "" {
		sessionID = "-3785608638908410209"
	}
	host := serviceConfig.Host
	if host == "" {
		host = "notebooklm.google.com"
	}
	app := serviceConfig.App
	if app == "" {
		app = "LabsTailwindUi"
	}
	urlParams := map[string]string{
		"bl":    blParam,
		"f.sid": sessionID,
		"hl":    "en",
		"rt":    "c",
	}
	for k, v := range serviceConfig.URLParams {
		if v == "" {
			continue
		}
		switch k {
		case "bl", "f.sid", "rt":
			// Prefer live session parameters over generated defaults.
			continue
		default:
			urlParams[k] = v
		}
	}

	config := batchexecute.Config{
		Host:      host,
		App:       app,
		AuthToken: authToken,
		Cookies:   cookies,
		Headers: map[string]string{
			"content-type":    "application/x-www-form-urlencoded;charset=UTF-8",
			"origin":          fmt.Sprintf("https://%s", host),
			"referer":         fmt.Sprintf("https://%s/", host),
			"x-same-domain":   "1",
			"accept":          "*/*",
			"accept-language": "en-US,en;q=0.9",
			"cache-control":   "no-cache",
			"pragma":          "no-cache",
		},
		URLParams: urlParams,
	}
	return &Client{
		Config: config,
		client: batchexecute.NewClient(config, options...),
	}
}

// Do executes a NotebookLM RPC call
func (c *Client) Do(call Call) (json.RawMessage, error) {
	if c.Config.Debug {
		fmt.Printf("\n=== RPC Call ===\n")
		fmt.Printf("ID: %s\n", call.ID)
		fmt.Printf("NotebookID: %s\n", call.NotebookID)
		fmt.Printf("Args:\n")
		spew.Dump(call.Args)
	}

	// Create request-specific URL parameters
	urlParams := make(map[string]string)
	for k, v := range c.Config.URLParams {
		urlParams[k] = v
	}

	if call.NotebookID != "" {
		urlParams["source-path"] = "/notebook/" + call.NotebookID
	} else {
		urlParams["source-path"] = "/"
	}

	rpc := batchexecute.RPC{
		ID:        call.ID,
		Args:      call.Args,
		Index:     "generic",
		URLParams: urlParams,
	}

	if c.Config.Debug {
		fmt.Printf("\nRPC Request:\n")
		spew.Dump(rpc)
	}

	resp, err := c.client.Do(rpc)
	if err != nil {
		return nil, fmt.Errorf("execute rpc: %w", err)
	}

	if c.Config.Debug {
		fmt.Printf("\nRPC Response:\n")
		spew.Dump(resp)
	}

	return resp.Data, nil
}

// Heartbeat sends a heartbeat to keep the session alive
func (c *Client) Heartbeat() error {
	return nil
}

// ListNotebooks returns all notebooks
func (c *Client) ListNotebooks() (json.RawMessage, error) {
	return c.Do(Call{
		ID: RPCListRecentlyViewedProjects,
	})
}

// CreateNotebook creates a new notebook with the given title
func (c *Client) CreateNotebook(title string) (json.RawMessage, error) {
	return c.Do(Call{
		ID: RPCCreateProject,
		Args: []interface{}{
			title,
			"",
		},
	})
}

// DeleteNotebook deletes a notebook by ID
func (c *Client) DeleteNotebook(id string) error {
	_, err := c.Do(Call{
		ID: RPCDeleteProjects,
		Args: []interface{}{
			[]interface{}{id},
		},
	})
	if err != nil {
		return fmt.Errorf("delete notebook: %w", err)
	}
	return nil
}
