package render

import (
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/richrender"
	"github.com/tmc/nlm/notebooklm"
)

// CitationTitle returns the best available notebook-source title for a
// citation. resolveTitle maps a notebook-source ID to its title; it is
// consulted first for the citation's ParentSourceID (the source that owns
// the cited passage) and then for its SourceID.
//
// The notebooklm client never fills [notebooklm.Citation.Title]: a citation
// grounds a chunk, so its SourceID is a passage handle absent from the
// project source list, and only ParentSourceID resolves to a title. Callers
// that hold the project sources can pass [SourceTitleResolver].
func CitationTitle(citation notebooklm.Citation, resolveTitle func(string) string) string {
	return richrender.CitationTitle(citation, resolveTitle)
}

// SourceTitleResolver returns a lookup from source ID to title over a
// project's sources, suitable for [CitationTitle]. It returns the empty
// string for unknown IDs.
func SourceTitleResolver(sources []*pb.Source) func(string) string {
	titles := make(map[string]string, len(sources))
	for _, source := range sources {
		if id := source.GetSourceId().GetSourceId(); id != "" {
			titles[id] = source.GetTitle()
		}
	}
	return func(id string) string { return titles[id] }
}
