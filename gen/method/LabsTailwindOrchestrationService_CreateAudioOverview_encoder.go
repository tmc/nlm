package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateAudioOverviewArgs encodes arguments for creating an audio overview
// via the CreateArtifact RPC (R7cb6c).
//
// Audio overviews are a type of artifact. The request goes through CreateArtifact
// with the audio-specific Uv artifact spec at field 3.
//
// Wire format: [jC_ProjectContext, "projectId", Uv_artifact_spec]
//
// k_a request fields:
//   Field 1: bC ProjectContext (jC variant)
//   Field 2: string project ID
//   Field 3: Uv artifact spec
//
// Uv fields:
//   Field 2: string title (optional)
//   Field 3: int artifact type
//   Field 4: repeated Ru source references
//   Field 7: oneof Rv audio message {field 2: Tv audio metadata}
//
// Tv audio metadata fields:
//   Field 1: string title/name
//   Field 2: int audio type (1=deep_dive, 2=brief, 3=critique, 4=debate)
//   Field 4: repeated Ru source references
//   Field 5: string custom instructions
//   Field 7: int audio style/length
func EncodeCreateAudioOverviewArgs(req *notebooklmv1alpha1.CreateAudioOverviewRequest) []interface{} {
	// Build source references
	var sourceRefs []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRefs = append(sourceRefs, []interface{}{id})
	}

	// Build Tv audio metadata
	// [title, audioType, null, [sourceRefs], instructions, null, audioLength]
	audioType := int(req.GetAudioType())
	if audioType == 0 {
		audioType = 1 // default to deep_dive
	}
	audioLength := int(req.GetLength())
	if audioLength == 0 {
		audioLength = 2 // default length
	}

	tv := []interface{}{
		req.GetCustomInstructions(), // field 1: title/name (using instructions as name)
		audioType,                   // field 2: audio type enum
		nil,                         // field 3: gap
		sourceRefs,                  // field 4: source references
		req.GetCustomInstructions(), // field 5: custom instructions
		nil,                         // field 6: gap
		audioLength,                 // field 7: audio style/length
	}

	// Build Rv audio wrapper: {field 2: Tv}
	rv := []interface{}{nil, tv}

	// Build Uv artifact spec
	// Field 3 = type (for audio, check what value to use)
	// Field 4 = source refs
	// Field 7 = Rv audio (oneof)
	artifactSpec := []interface{}{
		nil,        // field 1: gap
		nil,        // field 2: title
		7,          // field 3: artifact type (audio = 7 based on Qv oneof position)
		sourceRefs, // field 4: source references
		nil,        // field 5: gap
		nil,        // field 6: gap
		rv,         // field 7: Rv audio (oneof)
	}

	// ProjectContext jC variant: [2, null, null, [1, null, null, null, null, null, null, null, null, null, [1]], [[1]]]
	projectContext := []interface{}{
		2, nil, nil,
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
		[]interface{}{[]interface{}{1}},
	}

	return []interface{}{
		projectContext,        // field 1: ProjectContext (jC variant)
		req.GetProjectId(),   // field 2: project ID
		artifactSpec,         // field 3: Uv artifact spec
	}
}
