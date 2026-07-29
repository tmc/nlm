package notebooklm

import (
	"errors"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

func TestClassifyGetProjectError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantAccess bool
	}{
		{
			name: "permission denied",
			err: &batchexecute.APIError{
				ErrorCode: &batchexecute.ErrorCode{
					Type: batchexecute.ErrorTypePermissionDenied,
				},
			},
			wantAccess: true,
		},
		{
			name:       "http forbidden",
			err:        &batchexecute.APIError{HTTPStatus: 403},
			wantAccess: true,
		},
		{
			name: "authentication stays auth",
			err: &batchexecute.APIError{
				ErrorCode: &batchexecute.ErrorCode{
					Type: batchexecute.ErrorTypeAuthentication,
				},
			},
		},
		{
			name: "http unauthorized stays auth",
			err:  &batchexecute.APIError{HTTPStatus: 401},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyGetProjectError("nb-1", tt.err)
			if errors.Is(got, ErrNotebookNotAccessible) != tt.wantAccess {
				t.Fatalf("access classification = %v, want %v; err=%v", errors.Is(got, ErrNotebookNotAccessible), tt.wantAccess, got)
			}
			var access *NotebookAccessError
			if tt.wantAccess {
				if !errors.As(got, &access) {
					t.Fatalf("got %T, want NotebookAccessError", got)
				}
				if access.NotebookID != "nb-1" {
					t.Fatalf("notebook id = %q, want nb-1", access.NotebookID)
				}
			}
		})
	}
}
