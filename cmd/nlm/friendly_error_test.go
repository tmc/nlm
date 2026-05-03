package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// getCode13 returns the dictionary entry for gRPC INTERNAL so the test
// exercises the same Description text users see in the wild instead of a
// hand-rolled stub that could drift.
func getCode13() *batchexecute.ErrorCode {
	if ec, ok := batchexecute.GetErrorCode(13); ok {
		return ec
	}
	return nil
}

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
		{
			// Regression: code 13 used to surface the verbatim dictionary
			// string "Internal errors that shouldn't be exposed to clients."
			// via friendlyAPIMessage — useless to a user trying to act on
			// it. The replacement names the operation, hints at retry, and
			// points at the chunking workaround.
			name: "internal error rewrites unhelpful stock description",
			err: fmt.Errorf("upload %q: add text source: execute rpc: %w",
				"docs",
				&batchexecute.APIError{
					ErrorCode: getCode13(),
				}),
			wantContains:   []string{"upload \"docs\"", "internal error", "retry", "nlm sync"},
			wantNotContain: []string{"shouldn't be exposed to clients"},
		},
		{
			// Code-9 ("Failed precondition") used to be auto-classified as
			// ErrSourceCapReached, which lied when the actual failure was an
			// oversize payload or server policy. The replacement enumerates
			// the actually-observed causes and explicitly disclaims the
			// dictionary description ("Operation was rejected for a state
			// reason.") so the user sees actionable text.
			name: "code 9 rewrites to actionable list of causes",
			err: fmt.Errorf("add text source: %w",
				&batchexecute.APIError{
					ErrorCode: &batchexecute.ErrorCode{
						Code:        9,
						Type:        batchexecute.ErrorTypeInvalidInput,
						Message:     "Failed precondition",
						Description: "Operation was rejected for a state reason.",
					},
				}),
			wantContains:   []string{"add text source", "code 9", "300-source cap", "nlm sync"},
			wantNotContain: []string{"notebook source cap reached", "Operation was rejected"},
		},
		{
			// Regression: errors.Join'd parallel-upload failures used to get
			// only one friendly rewrite — the others surfaced raw. Each
			// branch of a join must be rewritten independently, joined with
			// newlines so the user sees every distinct failure.
			name: "errors.Join applies friendly rewrite per branch",
			err: errors.Join(
				fmt.Errorf("upload %q: %w", "docs (pt7)", &batchexecute.APIError{
					ErrorCode: &batchexecute.ErrorCode{Code: 9, Type: batchexecute.ErrorTypeInvalidInput, Message: "Failed precondition"},
				}),
				fmt.Errorf("upload %q: %w", "docs (pt8)", &batchexecute.APIError{
					ErrorCode: &batchexecute.ErrorCode{Code: 9, Type: batchexecute.ErrorTypeInvalidInput, Message: "Failed precondition"},
				}),
			),
			wantContains:   []string{"docs (pt7)", "docs (pt8)", "code 9"},
			wantNotContain: []string{"API error 9"},
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
