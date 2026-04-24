package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// TestFriendlyError verifies that raw gRPC error codes never reach the user
// message and that outer fmt.Errorf context is preserved in front of the
// friendly description.
func TestFriendlyError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "bare batchexecute InvalidArgument",
			err: &batchexecute.APIError{
				ErrorCode: &batchexecute.ErrorCode{
					Code:        3,
					Type:        batchexecute.ErrorTypeInvalidInput,
					Message:     "Invalid argument",
					Description: "One or more arguments are invalid.",
				},
			},
			wantContains:   []string{"One or more arguments are invalid"},
			wantNotContain: []string{"API error", "code 3", "(InvalidInput)"},
		},
		{
			name: "wrapped batchexecute error keeps outer context",
			err: fmt.Errorf("get project: %w", &batchexecute.APIError{
				ErrorCode: &batchexecute.ErrorCode{
					Code:        16,
					Type:        batchexecute.ErrorTypeAuthentication,
					Message:     "Unauthenticated",
					Description: "The request does not have valid authentication credentials.",
				},
			}),
			wantContains:   []string{"get project", "valid authentication credentials"},
			wantNotContain: []string{"API error 16", "error 16:"},
		},
		{
			name: "source cap sentinel hides wrapper noise",
			err: fmt.Errorf("add source from URL: %w: %w",
				api.ErrSourceCapReached,
				&batchexecute.APIError{
					ErrorCode: &batchexecute.ErrorCode{
						Code:        9,
						Type:        batchexecute.ErrorTypeInvalidInput,
						Message:     "Failed precondition",
						Description: "Operation was rejected for a state reason.",
					},
				},
			),
			wantContains:   []string{"add source from URL", "source limit"},
			wantNotContain: []string{"notebook source cap reached", "API error 9"},
		},
		{
			name: "source too large sentinel replaces wrapper noise",
			err: fmt.Errorf("add text source %q (%d bytes > %d limit): %w",
				"big.jsonl", 13_815_499, 10*1024*1024, api.ErrSourceTooLarge),
			// The sentinel's own literal message is stripped; the friendly
			// rewrite must not double-surface it or leak the cap-reached
			// label users mistook size failures for.
			wantContains:   []string{"add text source", "per-request size limit", "nlm sync"},
			wantNotContain: []string{"notebook source cap reached", "source exceeds per-request size limit:"},
		},
		{
			name:         "plain error passes through unchanged",
			err:          errors.New("open file: no such file"),
			wantContains: []string{"open file: no such file"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := friendlyError(tt.err)
			for _, s := range tt.wantContains {
				if !strings.Contains(got, s) {
					t.Errorf("friendlyError() = %q, want contains %q", got, s)
				}
			}
			for _, s := range tt.wantNotContain {
				if strings.Contains(got, s) {
					t.Errorf("friendlyError() = %q, want NOT contain %q", got, s)
				}
			}
		})
	}
}
