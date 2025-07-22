package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// encodeSourceInput encodes a source input for the batchexecute format
func encodeSourceInput(src *notebooklmv1alpha1.SourceInput) []interface{} {
	switch src.GetSourceType() {
	case notebooklmv1alpha1.SourceType_SOURCE_TYPE_GOOGLE_DOCS:
		return []interface{}{
			nil,
			nil,
			[]string{src.GetUrl()},
		}
	case notebooklmv1alpha1.SourceType_SOURCE_TYPE_YOUTUBE_VIDEO:
		return []interface{}{
			nil,
			nil,
			src.GetYoutubeVideoId(),
			nil,
			int(notebooklmv1alpha1.SourceType_SOURCE_TYPE_YOUTUBE_VIDEO),
		}
	default:
		// Text source
		return []interface{}{
			nil,
			[]string{
				src.GetTitle(),
				src.GetContent(),
			},
			nil,
			2, // text source type
		}
	}
}

// encodeProjectUpdates encodes project updates for the batchexecute format
func encodeProjectUpdates(updates *notebooklmv1alpha1.Project) interface{} {
	// TODO: Implement proper encoding based on which fields are set
	return updates
}

// encodeSourceUpdates encodes source updates for the batchexecute format
func encodeSourceUpdates(updates *notebooklmv1alpha1.Source) interface{} {
	// TODO: Implement proper encoding based on which fields are set
	return updates
}