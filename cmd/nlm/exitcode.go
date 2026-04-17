package main

import (
	"errors"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// Exit codes classify failure modes so shell scripts can branch on them.
// Keep these classes stable; scripts may branch on them.
//
//	0 success
//	1 generic error (default for unclassified failures)
//	2 bad arguments (flag parser, malformed input)
//	3 auth required / auth failed
//	4 not found (notebook, source, artifact)
//	5 permanent precondition (source-cap reached, quota exhausted, deleted)
//	6 transient server / network / 5xx / rate limit
//	7 resource busy / still generating (poll-in-progress)
const (
	exitSuccess          = 0
	exitGeneric          = 1
	exitBadArgs          = 2
	exitAuth             = 3
	exitNotFound         = 4
	exitPrecondition     = 5
	exitTransient        = 6
	exitBusy             = 7
)

// exitCodeName returns a short, stable, machine-parseable name for a
// non-success exit code, or "" for codes without a distinct class (including
// exitSuccess and the catch-all exitGeneric). The stderr message wrapper
// only emits the `exit-class=<name>` line when this returns non-empty.
func exitCodeName(code int) string {
	switch code {
	case exitBadArgs:
		return "bad-args"
	case exitAuth:
		return "auth"
	case exitNotFound:
		return "not-found"
	case exitPrecondition:
		return "precondition"
	case exitTransient:
		return "transient"
	case exitBusy:
		return "busy"
	default:
		return ""
	}
}

// exitCodeFor maps a run() error to an exit code per the taxonomy above.
// Order of checks matters: the typed api sentinels are more specific than
// the batchexecute.APIError classification, and isAuthenticationError runs
// first because it folds in legacy string-matching cases that predate the
// structured ErrorType classification.
func exitCodeFor(err error) int {
	if err == nil {
		return exitSuccess
	}

	// Cmd-layer sentinels take precedence over any underlying classifier,
	// since they capture intent ("user gave bad args") that a downstream
	// batchexecute error wouldn't carry.
	if errors.Is(err, errBadArgs) {
		return exitBadArgs
	}

	if isAuthenticationError(err) {
		return exitAuth
	}

	// Typed api-layer sentinels for states batchexecute cannot disambiguate.
	switch {
	case errors.Is(err, api.ErrSourceCapReached):
		return exitPrecondition
	case errors.Is(err, api.ErrArtifactGenerating),
		errors.Is(err, api.ErrResearchPolling):
		return exitBusy
	}

	// Structured batchexecute.APIError classification.
	var apiErr *batchexecute.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode != nil {
			switch apiErr.ErrorCode.Type {
			case batchexecute.ErrorTypeAuthentication,
				batchexecute.ErrorTypeAuthorization,
				batchexecute.ErrorTypePermissionDenied:
				return exitAuth
			case batchexecute.ErrorTypeNotFound:
				return exitNotFound
			case batchexecute.ErrorTypeResourceExhausted:
				return exitPrecondition
			case batchexecute.ErrorTypeRateLimit,
				batchexecute.ErrorTypeServerError,
				batchexecute.ErrorTypeUnavailable,
				batchexecute.ErrorTypeNetworkError:
				return exitTransient
			case batchexecute.ErrorTypeInvalidInput:
				return exitBadArgs
			case batchexecute.ErrorTypeUnknown:
				return exitGeneric
			}
		}
		// HTTPStatus fallback for APIErrors without a parsed ErrorCode.
		switch {
		case apiErr.HTTPStatus == 401, apiErr.HTTPStatus == 403:
			return exitAuth
		case apiErr.HTTPStatus == 404:
			return exitNotFound
		case apiErr.HTTPStatus == 429:
			return exitTransient
		case apiErr.HTTPStatus >= 500 && apiErr.HTTPStatus <= 599:
			return exitTransient
		case apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus <= 499:
			return exitBadArgs
		}
	}

	return exitGeneric
}
