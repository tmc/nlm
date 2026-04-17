package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// Research flags (set in init() via flag.StringVar).
var (
	researchMode  string // "fast" or "deep"; default "deep"
	researchMD    bool   // emit markdown report on stdout instead of JSONL events
	researchPollMs int   // override polling interval for deep research; 0 = default 5s
)

// researchEvent is one JSON-lines record emitted on stdout.
//
// Wire shape is stable across fast and deep modes so downstream scripts can
// consume both with the same jq expression. The "type" discriminates:
//
//	progress         — partial delta during deep-research polling
//	source_discovered — one source pulled out of the deep-research response
//	report_chunk     — a chunk of the markdown report (deep only)
//	complete         — terminal event with the final report and source list
type researchEvent struct {
	Type    string           `json:"type"`
	Mode    string           `json:"mode,omitempty"`
	Query   string           `json:"query,omitempty"`
	Report  string           `json:"report,omitempty"`
	Delta   string           `json:"report_delta,omitempty"`
	Sources []api.ResearchSource `json:"sources,omitempty"`
	// Deep-only: surface the researchID so a caller that needs to resume a
	// poll later can persist it.
	ResearchID string `json:"research_id,omitempty"`
}

// runResearch is the nlm research <topic> command entry point. Emits
// JSON-lines on stdout by default; --md switches to raw markdown.
//
// Current implementation is scaffolding: both fast and deep modes call the
// same api.Client methods, but their encoders remain argbuilder stubs until
// the request and poll response shapes are captured. Exit-code wiring
// via ErrResearchPolling is in place so the taxonomy (exit 7) applies the
// moment the polling shape is known.
func runResearch(c *api.Client, notebookID, query string) error {
	mode := strings.ToLower(strings.TrimSpace(researchMode))
	if mode == "" {
		mode = "deep"
	}
	switch mode {
	case "fast":
		return runFastResearch(c, notebookID, query)
	case "deep":
		return runDeepResearch(c, notebookID, query)
	default:
		return fmt.Errorf("--mode=%q: want fast or deep", researchMode)
	}
}

func runFastResearch(c *api.Client, notebookID, query string) error {
	fmt.Fprintf(os.Stderr, "Fast research: %s\n", query)

	result, err := c.FastResearch(notebookID, query)
	if err != nil {
		return fmt.Errorf("fast research: %w", err)
	}

	if researchMD {
		fmt.Print(result.Report)
		if !strings.HasSuffix(result.Report, "\n") {
			fmt.Println()
		}
		return nil
	}

	return emitResearchEvent(researchEvent{
		Type:    "complete",
		Mode:    "fast",
		Query:   query,
		Report:  result.Report,
		Sources: result.Sources,
	})
}

func runDeepResearch(c *api.Client, notebookID, query string) error {
	project, err := c.GetProject(notebookID)
	if err != nil {
		return fmt.Errorf("look up notebook: %w", err)
	}
	if len(project.Sources) == 0 {
		return fmt.Errorf("notebook has no sources; add at least one with 'nlm add %s <path-or-url>' before running research", notebookID)
	}

	fmt.Fprintf(os.Stderr, "Deep research: %s\n", query)

	start, err := c.StartDeepResearch(notebookID, query)
	if err != nil {
		return fmt.Errorf("start deep research: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Research ID: %s\n", start.ResearchID)

	pollInterval := 5 * time.Second
	if researchPollMs > 0 {
		pollInterval = time.Duration(researchPollMs) * time.Millisecond
	}
	const maxPolls = 120 // cap at ~10min default

	for i := 0; i < maxPolls; i++ {
		time.Sleep(pollInterval)

		result, err := c.PollDeepResearch(notebookID, start.ResearchID)
		if err != nil {
			// Still polling — emit a progress event and keep going. The
			// classifier will map the wrapped ErrResearchPolling to exit 7
			// only if it leaks out of this loop.
			if result != nil {
				_ = emitResearchEvent(researchEvent{
					Type:       "progress",
					Mode:       "deep",
					Query:      query,
					ResearchID: start.ResearchID,
				})
				continue
			}
			return fmt.Errorf("poll deep research: %w", err)
		}
		if result.Done {
			report, sources := splitDeepResearchContent(result.Content)
			if researchMD {
				fmt.Print(report)
				if !strings.HasSuffix(report, "\n") {
					fmt.Println()
				}
				return nil
			}
			return emitResearchEvent(researchEvent{
				Type:       "complete",
				Mode:       "deep",
				Query:      query,
				ResearchID: start.ResearchID,
				Report:     report,
				Sources:    sources,
			})
		}
	}

	// Loop exhausted without a done signal; surface the busy sentinel so
	// scripts can retry via polling instead of treating this as a fatal error.
	return fmt.Errorf("deep research polling exhausted after %d attempts: %w", maxPolls, api.ErrResearchPolling)
}

// splitDeepResearchContent separates the markdown report from the discovered
// sources embedded in a deep-research response.
//
// BLOCKED on HAR capture of a completed QA9ei → e3bVqc poll (P0.6). The
// current implementation treats the entire content blob as the report and
// returns no structured sources. When the HAR lands, parse sources out and
// return them alongside the report.
func splitDeepResearchContent(content string) (string, []api.ResearchSource) {
	return content, nil
}

// emitResearchEvent writes one JSON-lines record to stdout.
func emitResearchEvent(ev researchEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode research event: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
