package method

// EncodeGenerateArtifactSuggestionsArgs encodes arguments for otmP3b
// (the undocumented GenerateArtifactSuggestions RPC). The UI calls this
// before R7cb6c (CreateUniversalArtifact) to get a list of AI-generated
// topic blueprints; the user picks or edits one and passes it as
// instructions to R7cb6c.
//
// Wire format verified against HAR capture (create-audio, 2026-04-14):
//
//	[[kind], projectID, [[src1], [src2], ...], variation]
//
// Only audio (kind=2) is HAR-verified. Video and slides kinds are not
// covered by this encoder pending their own captures.
//
// variation is a small integer (1, 2, 5, 6 observed) that re-rolls the
// suggestions; the backend returns three different topic/description
// pairs per variation.
func EncodeGenerateArtifactSuggestionsArgs(kind int, projectID string, sourceIDs []string, variation int) []interface{} {
	sourceRefs := make([]interface{}, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		sourceRefs = append(sourceRefs, []interface{}{id})
	}
	return []interface{}{
		[]interface{}{kind},
		projectID,
		sourceRefs,
		variation,
	}
}

// ArtifactSuggestionKindAudio is the kind argument for audio-overview
// blueprints. The wire enum distinct from R7cb6c artifact types.
const ArtifactSuggestionKindAudio = 2
