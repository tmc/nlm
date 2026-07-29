package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/notebooklm"
)

// friendlyError rewrites the "API error <N> (<Type>): <msg>" format produced
// by *batchexecute.APIError into something a user can act on. It keeps the
// wrapping context (e.g. "get project: ...") so callers still see which
// operation failed. If err is not a batchexecute APIError the return value
// is err.Error() unchanged.
//
// errors.Join'd error trees (which sync produces when multiple chunk uploads
// fail in parallel) are formatted per-branch so each independent failure
// gets its own friendly rewrite, joined by newlines.
func friendlyError(err error) string {
	// Distinguish errors.Join (children are siblings, no shared prefix) from
	// fmt.Errorf with double-%w (children share a wrapping prefix and the
	// outer Error() is one line). errors.Join formats as child1\nchild2\n…;
	// matching that exactly is the only reliable signal — both shapes
	// implement Unwrap() []error.
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) > 1 && isJoinShape(err, children) {
			parts := make([]string, 0, len(children))
			for _, c := range children {
				parts = append(parts, friendlyError(c))
			}
			return strings.Join(parts, "\n")
		}
	}
	if errors.Is(err, notebooklm.ErrAuthExpired) {
		return friendlyTypedError(err, notebooklm.ErrAuthExpired, "authentication expired; run 'nlm auth' to re-authenticate")
	}
	if errors.Is(err, notebooklm.ErrSourceCapReached) {
		return friendlyTypedError(err, notebooklm.ErrSourceCapReached, "notebook is at the source limit; remove unused sources before adding more")
	}
	if errors.Is(err, notebooklm.ErrSourceTooLarge) {
		return friendlyTypedError(err, notebooklm.ErrSourceTooLarge, "source exceeds the per-request size limit; split it, or use `nlm sync` / `nlm sync-pack` which chunk automatically")
	}
	if errors.Is(err, notebooklm.ErrNotebookCapReached) {
		msg := "account is at the notebook limit; delete unused notebooks before creating more"
		var capErr *notebooklm.NotebookCapError
		if errors.As(err, &capErr) && capErr.Limit > 0 && capErr.Count >= 0 {
			msg = fmt.Sprintf("account is at the notebook limit (%d/%d); delete unused notebooks before creating more", capErr.Count, capErr.Limit)
		}
		return friendlyTypedError(err, notebooklm.ErrNotebookCapReached, msg)
	}
	if errors.Is(err, notebooklm.ErrNotebookNotAccessible) {
		msg := "notebook not found or not accessible by the current account"
		var accessErr *notebooklm.NotebookAccessError
		if errors.As(err, &accessErr) && accessErr.NotebookID != "" {
			msg = fmt.Sprintf("notebook %s not found or not accessible by the current account", accessErr.NotebookID)
		}
		return friendlyTypedError(err, notebooklm.ErrNotebookNotAccessible, msg)
	}
	var apiErr *batchexecute.APIError
	if !errors.As(err, &apiErr) {
		if isAuthenticationError(err) {
			return friendlyAuthenticationError(err)
		}
		return err.Error()
	}
	// Strip the "API error <N> (<Type>): <msg>" suffix from the wrapped
	// error chain so the user sees "<outer context>: <friendly message>".
	full := err.Error()
	suffix := apiErr.Error()
	prefix := strings.TrimSuffix(full, suffix)
	prefix = strings.TrimRight(prefix, ": ")

	msg := friendlyAPIMessage(apiErr)
	if prefix == "" {
		return msg
	}
	return prefix + ": " + msg
}

// isJoinShape reports whether err is the result of errors.Join (children
// surfaced as parallel siblings) rather than fmt.Errorf with multiple %w
// (children wrapped under a shared prefix). errors.Join produces an Error()
// equal to children joined by "\n"; fmt.Errorf produces a single line.
func isJoinShape(err error, children []error) bool {
	want := make([]string, 0, len(children))
	for _, c := range children {
		want = append(want, c.Error())
	}
	return err.Error() == strings.Join(want, "\n")
}

func friendlyTypedError(err, target error, msg string) string {
	full := err.Error()

	var apiErr *batchexecute.APIError
	if errors.As(err, &apiErr) {
		full = strings.TrimSuffix(full, apiErr.Error())
		full = strings.TrimRight(full, ": ")
	}

	if i := strings.Index(full, target.Error()); i >= 0 {
		full = full[:i]
	} else {
		full = strings.TrimSuffix(full, target.Error())
	}
	full = strings.TrimRight(full, ": ")

	if full == "" {
		return msg
	}
	return full + ": " + msg
}

// friendlyAPIMessage returns a human-readable description for an APIError.
// Prefers ErrorCode.Description (from the dictionary) over raw Message, and
// never surfaces the numeric code to the user.
func friendlyAPIMessage(apiErr *batchexecute.APIError) string {
	if apiErr.ErrorCode != nil {
		switch apiErr.ErrorCode.Type {
		case batchexecute.ErrorTypeAuthentication:
			return "authentication expired or invalid; run `nlm auth` to refresh, or re-export NLM_AUTH_TOKEN / NLM_COOKIES"
		case batchexecute.ErrorTypeAuthorization,
			batchexecute.ErrorTypePermissionDenied:
			return "resource not found or not accessible by the current account"
		}
	}
	switch apiErr.HTTPStatus {
	case 401:
		return "authentication expired or invalid; run `nlm auth` to refresh, or re-export NLM_AUTH_TOKEN / NLM_COOKIES"
	case 403:
		return "resource not found or not accessible by the current account"
	}
	// Code 9 ("Failed precondition") arrives bare for AddSource* — no
	// diagnostic text. The dictionary description ("Operation was rejected
	// for a state reason.") is too vague to act on. Replace with a list of
	// the actually-observed causes so users know what to check.
	if apiErr.ErrorCode != nil && apiErr.ErrorCode.Code == 9 {
		return "server rejected the request (code 9). Common causes: source content too large for one upload (split with `nlm sync` or `--chunk`), notebook at the 300-source cap (check with `nlm list-sources`), or transient server policy. The server does not return a diagnostic for this code."
	}
	if apiErr.ErrorCode != nil && apiErr.ErrorCode.Description != "" {
		return apiErr.ErrorCode.Description
	}
	if apiErr.ErrorCode != nil && apiErr.ErrorCode.Message != "" {
		return apiErr.ErrorCode.Message
	}
	if apiErr.Message != "" {
		return apiErr.Message
	}
	return "request failed"
}

func friendlyAuthenticationError(err error) string {
	const msg = "authentication expired or invalid; run `nlm auth` to refresh, or re-export NLM_AUTH_TOKEN / NLM_COOKIES"
	full := err.Error()
	lower := strings.ToLower(full)
	if strings.Contains(lower, "authentication required") {
		return "authentication required; run `nlm auth` first, or export NLM_AUTH_TOKEN and NLM_COOKIES"
	}
	for _, marker := range []string{": batchexecute error", ": http error", ": unauthorized"} {
		if i := strings.Index(lower, marker); i > 0 {
			return full[:i] + ": " + msg
		}
	}
	return msg
}
