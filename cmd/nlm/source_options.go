package main

// sourceAddOptions controls source ingestion after command decoding.
type sourceAddOptions struct {
	Name            string
	MIMEType        string
	ReplaceSourceID string
	// PreProcess is a shell command run for each non-URL source. The source
	// content is piped to its stdin; stdout replaces what gets uploaded. A
	// non-zero exit status aborts the batch.
	PreProcess string
	// Chunk, if > 0, splits each non-URL source into parts of at most this
	// many bytes each. The first part keeps the source name; subsequent
	// parts are named "<name> (pt2)", "<name> (pt3)", ... Matches the
	// naming scheme `nlm sync` uses for bundle chunks.
	Chunk int
}

type syncOptions struct {
	Name             string
	Force            bool
	DryRun           bool
	MaxBytes         int
	JSON             bool
	Exclude          []string
	IncludeUntracked bool
	Parallel         int
	PreProcess       string
}

type syncPackOptions struct {
	Name       string
	MaxBytes   int
	Chunk      int
	Exclude    []string
	PreProcess string
}
