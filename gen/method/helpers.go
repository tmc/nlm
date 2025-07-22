package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
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
	// Return a map with only the fields that are set
	result := make(map[string]interface{})
	if updates.GetTitle() != "" {
		result["title"] = updates.GetTitle()
	}
	if updates.GetEmoji() != "" {
		result["emoji"] = updates.GetEmoji()
	}
	return result
}

// encodeSourceUpdates encodes source updates for the batchexecute format
func encodeSourceUpdates(updates *notebooklmv1alpha1.Source) interface{} {
	// Return a map with only the fields that are set
	result := make(map[string]interface{})
	if updates.GetTitle() != "" {
		result["title"] = updates.GetTitle()
	}
	return result
}

// encodeContext encodes context for the batchexecute format
func encodeContext(ctx *notebooklmv1alpha1.Context) interface{} {
	if ctx == nil {
		return nil
	}
	return map[string]interface{}{
		"project_id": ctx.GetProjectId(),
		"source_ids": ctx.GetSourceIds(),
	}
}

// encodeArtifact encodes an artifact for the batchexecute format
func encodeArtifact(artifact *notebooklmv1alpha1.Artifact) interface{} {
	if artifact == nil {
		return nil
	}
	result := make(map[string]interface{})
	if artifact.GetArtifactId() != "" {
		result["artifact_id"] = artifact.GetArtifactId()
	}
	if artifact.GetProjectId() != "" {
		result["project_id"] = artifact.GetProjectId()
	}
	if artifact.GetType() != notebooklmv1alpha1.ArtifactType_ARTIFACT_TYPE_UNSPECIFIED {
		result["type"] = int32(artifact.GetType())
	}
	if artifact.GetState() != notebooklmv1alpha1.ArtifactState_ARTIFACT_STATE_UNSPECIFIED {
		result["state"] = int32(artifact.GetState())
	}
	// Add sources if present
	if len(artifact.GetSources()) > 0 {
		var sources []interface{}
		for _, src := range artifact.GetSources() {
			sources = append(sources, encodeArtifactSource(src))
		}
		result["sources"] = sources
	}
	return result
}

// encodeArtifactSource encodes an artifact source for the batchexecute format
func encodeArtifactSource(src *notebooklmv1alpha1.ArtifactSource) interface{} {
	if src == nil {
		return nil
	}
	result := make(map[string]interface{})
	if src.GetSourceId() != nil {
		result["source_id"] = src.GetSourceId().GetSourceId()
	}
	// Add text fragments if present
	if len(src.GetTextFragments()) > 0 {
		var fragments []interface{}
		for _, frag := range src.GetTextFragments() {
			fragments = append(fragments, map[string]interface{}{
				"text":         frag.GetText(),
				"start_offset": frag.GetStartOffset(),
				"end_offset":   frag.GetEndOffset(),
			})
		}
		result["text_fragments"] = fragments
	}
	return result
}

// encodeFieldMask encodes a field mask for the batchexecute format
func encodeFieldMask(mask *fieldmaskpb.FieldMask) interface{} {
	if mask == nil {
		return nil
	}
	return mask.GetPaths()
}