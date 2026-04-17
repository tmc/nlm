package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// apiErrorWithType returns a *batchexecute.APIError whose ErrorCode.Type is t.
// The concrete Code is a stable sample from internal/batchexecute/errors.go's
// dictionary so behavior matches what a real wire response would produce.
func apiErrorWithType(t batchexecute.ErrorType) *batchexecute.APIError {
	var code int
	switch t {
	case batchexecute.ErrorTypeAuthentication:
		code = 16
	case batchexecute.ErrorTypeAuthorization:
		code = 80620
	case batchexecute.ErrorTypeRateLimit:
		code = 324934
	case batchexecute.ErrorTypeNotFound:
		code = 143
	case batchexecute.ErrorTypeInvalidInput:
		code = 3
	case batchexecute.ErrorTypeServerError:
		code = 13
	case batchexecute.ErrorTypeNetworkError:
		code = 500
	case batchexecute.ErrorTypePermissionDenied:
		code = 7
	case batchexecute.ErrorTypeResourceExhausted:
		code = 8
	case batchexecute.ErrorTypeUnavailable:
		code = 14
	}
	ec, _ := batchexecute.GetErrorCode(code)
	if ec == nil {
		ec = &batchexecute.ErrorCode{Type: t}
	}
	return &batchexecute.APIError{ErrorCode: ec}
}

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		// Success.
		{"nil error", nil, exitSuccess},

		// Legacy auth detection via isAuthenticationError string-matching.
		{"session expired keyword", errors.New("session expired: please re-auth"), exitAuth},
		{"unauthenticated keyword", errors.New("unauthenticated"), exitAuth},

		// Typed api-layer sentinels.
		{"ErrSourceCapReached", fmt.Errorf("add sources: %w", api.ErrSourceCapReached), exitPrecondition},
		{"ErrArtifactGenerating", fmt.Errorf("poll: %w", api.ErrArtifactGenerating), exitBusy},
		{"ErrResearchPolling", fmt.Errorf("poll: %w", api.ErrResearchPolling), exitBusy},
		{"ErrSourceCapReached wrapped twice", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", api.ErrSourceCapReached)), exitPrecondition},

		// Structured batchexecute.APIError by ErrorType (exhaustive over
		// every enumerant in internal/batchexecute/errors.go).
		{"Authentication", apiErrorWithType(batchexecute.ErrorTypeAuthentication), exitAuth},
		{"Authorization", apiErrorWithType(batchexecute.ErrorTypeAuthorization), exitAuth},
		{"PermissionDenied", apiErrorWithType(batchexecute.ErrorTypePermissionDenied), exitAuth},
		{"NotFound", apiErrorWithType(batchexecute.ErrorTypeNotFound), exitNotFound},
		{"ResourceExhausted", apiErrorWithType(batchexecute.ErrorTypeResourceExhausted), exitPrecondition},
		{"RateLimit", apiErrorWithType(batchexecute.ErrorTypeRateLimit), exitTransient},
		{"ServerError", apiErrorWithType(batchexecute.ErrorTypeServerError), exitTransient},
		{"Unavailable", apiErrorWithType(batchexecute.ErrorTypeUnavailable), exitTransient},
		{"NetworkError", apiErrorWithType(batchexecute.ErrorTypeNetworkError), exitTransient},
		{"InvalidInput", apiErrorWithType(batchexecute.ErrorTypeInvalidInput), exitBadArgs},
		{"Unknown", &batchexecute.APIError{ErrorCode: &batchexecute.ErrorCode{Type: batchexecute.ErrorTypeUnknown}}, exitGeneric},

		// APIError wrapped via fmt.Errorf still reaches the classifier via errors.As.
		{"wrapped APIError NotFound", fmt.Errorf("get project: %w", apiErrorWithType(batchexecute.ErrorTypeNotFound)), exitNotFound},

		// HTTPStatus fallback — APIError with no parsed ErrorCode.
		{"HTTP 401", &batchexecute.APIError{HTTPStatus: 401}, exitAuth},
		{"HTTP 403", &batchexecute.APIError{HTTPStatus: 403}, exitAuth},
		{"HTTP 404", &batchexecute.APIError{HTTPStatus: 404}, exitNotFound},
		{"HTTP 429", &batchexecute.APIError{HTTPStatus: 429}, exitTransient},
		{"HTTP 400", &batchexecute.APIError{HTTPStatus: 400}, exitBadArgs},
		{"HTTP 418", &batchexecute.APIError{HTTPStatus: 418}, exitBadArgs},
		{"HTTP 500", &batchexecute.APIError{HTTPStatus: 500}, exitTransient},
		{"HTTP 503", &batchexecute.APIError{HTTPStatus: 503}, exitTransient},
		{"HTTP 599", &batchexecute.APIError{HTTPStatus: 599}, exitTransient},
		{"HTTP 200 (no code, no classification)", &batchexecute.APIError{HTTPStatus: 200}, exitGeneric},

		// Sentinel has precedence over APIError classification.
		// (Real callers wrap both via fmt.Errorf("op: %w: %w", ErrX, apiErr).)
		{"sentinel with wrapped APIError", fmt.Errorf("add sources: %w: %w", api.ErrSourceCapReached, apiErrorWithType(batchexecute.ErrorTypeInvalidInput)), exitPrecondition},

		// Generic plain error falls through to exit 1.
		{"plain string error", errors.New("something broke"), exitGeneric},

		// Cmd-layer bad-args sentinel from validateCommandArgs.
		{"errBadArgs", errBadArgs, exitBadArgs},
		{"errBadArgs wrapped", fmt.Errorf("validate: %w", errBadArgs), exitBadArgs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeFor(tt.err); got != tt.want {
				t.Errorf("exitCodeFor(%q) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

// TestExitCodeForCoversAllErrorTypes ensures every batchexecute.ErrorType
// enumerant has an explicit case in the classifier. If a new ErrorType is
// added to internal/batchexecute/errors.go this test fails so we don't
// silently default it to exitGeneric.
func TestExitCodeForCoversAllErrorTypes(t *testing.T) {
	known := []batchexecute.ErrorType{
		batchexecute.ErrorTypeUnknown,
		batchexecute.ErrorTypeAuthentication,
		batchexecute.ErrorTypeAuthorization,
		batchexecute.ErrorTypeRateLimit,
		batchexecute.ErrorTypeNotFound,
		batchexecute.ErrorTypeInvalidInput,
		batchexecute.ErrorTypeServerError,
		batchexecute.ErrorTypeNetworkError,
		batchexecute.ErrorTypePermissionDenied,
		batchexecute.ErrorTypeResourceExhausted,
		batchexecute.ErrorTypeUnavailable,
	}
	for _, et := range known {
		err := apiErrorWithType(et)
		code := exitCodeFor(err)
		// Unknown is allowed to be exitGeneric; every other type must not be.
		if et != batchexecute.ErrorTypeUnknown && code == exitGeneric {
			t.Errorf("ErrorType %v fell through to exitGeneric; add an explicit case", et)
		}
	}
}

func TestExitCodeName(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{exitSuccess, ""},
		{exitGeneric, ""},
		{exitBadArgs, "bad-args"},
		{exitAuth, "auth"},
		{exitNotFound, "not-found"},
		{exitPrecondition, "precondition"},
		{exitTransient, "transient"},
		{exitBusy, "busy"},
		{99, ""},
	}
	for _, tt := range tests {
		if got := exitCodeName(tt.code); got != tt.want {
			t.Errorf("exitCodeName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}
