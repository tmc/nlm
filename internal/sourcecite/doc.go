// Package sourcecite resolves NotebookLM answer citations back to locations
// within a source's server-indexed text.
//
// A citation carries a character range into a source body. ResolveCitation
// maps that range to a file and offset, scanning txtar-archive sources for
// their member boundaries. NotebookLM flattens whitespace in the excerpt it
// returns, so resolution is validated against the excerpt and fails closed
// when no single source-text projection reproduces it, rather than emitting an
// unverified location.
package sourcecite
