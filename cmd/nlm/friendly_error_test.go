package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

// TestFriendlyError verifies that raw gRPC error codes never reach the user
// message and that outer fmt.Errorf context is preserved in front of the
// friendly description.
func TestFriendlyError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantContains  []string
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
