package sourcecite

// Status is the outcome of resolving a native citation against a source's
// server-indexed text.
type Status string

const (
	StatusOK         Status = "ok"          // citation resolved to a source location.
	StatusHeaderSpan Status = "header_span" // citation overlaps a txtar header, not a file body.
	StatusOffsetMiss Status = "offset_miss" // citation points outside the indexed source text.
)
