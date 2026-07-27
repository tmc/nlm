package nlmmcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

const (
	defaultResearchPollInterval = 2 * time.Second
	defaultResearchMaxWait      = 10 * time.Minute
)

type deepResearchPoller interface {
	PollDeepResearch(context.Context, string, string) (*api.DeepResearchResult, error)
}

type researchWatchProgress struct {
	Value   float64
	Message string
}

func watchDeepResearch(
	ctx context.Context,
	poller deepResearchPoller,
	input watchDeepResearchInput,
	notify func(researchWatchProgress) error,
) (*api.DeepResearchResult, error) {
	if input.NotebookID == "" {
		return nil, fmt.Errorf("notebook_id is required")
	}
	if input.ResearchID == "" {
		return nil, fmt.Errorf("research_id is required")
	}
	if input.PollIntervalMS < 0 {
		return nil, fmt.Errorf("poll_interval_ms must be non-negative")
	}
	if input.MaxWaitSeconds < 0 {
		return nil, fmt.Errorf("max_wait_seconds must be non-negative")
	}

	interval := defaultResearchPollInterval
	if input.PollIntervalMS != 0 {
		interval = time.Duration(input.PollIntervalMS) * time.Millisecond
	}
	maxWait := defaultResearchMaxWait
	if input.MaxWaitSeconds != 0 {
		maxWait = time.Duration(input.MaxWaitSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	for attempt := 1; ; attempt++ {
		result, err := poller.PollDeepResearch(ctx, input.NotebookID, input.ResearchID)
		if err == nil && result != nil && result.Done {
			if notify != nil {
				if err := notify(researchWatchProgress{
					Value:   float64(attempt),
					Message: "deep research complete",
				}); err != nil {
					return nil, fmt.Errorf("notify completion: %w", err)
				}
			}
			return result, nil
		}
		if err != nil && !errors.Is(err, api.ErrResearchPolling) {
			return nil, fmt.Errorf("poll deep research: %w", err)
		}
		if notify != nil {
			if err := notify(researchWatchProgress{
				Value:   float64(attempt),
				Message: "deep research still running",
			}); err != nil {
				return nil, fmt.Errorf("notify progress: %w", err)
			}
		}

		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("wait for deep research: %w", ctx.Err())
		}
	}
}
