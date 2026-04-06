package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateAudioOverviewArgs encodes arguments for creating an audio overview
// via the CreateArtifact RPC (R7cb6c).
//
// HAR-verified wire format:
//
//	[ProjectContext, "notebook-id", artifactSpec]
//
// ProjectContext: [2, null, null, [1,null*9,[1]], [[1,4,2,3,6,5]]]
//
// artifactSpec: [null, null, 1, sourceRefs, null, null, [null, [null, audioType, null, sourceRefsFlat, "en", instructions, audioLength]]]
//
//	pos 2: artifact type = 1 (audio)
//	pos 3: sourceRefs = [[["id1"]], [["id2"]]] (triple-nested)
//	pos 6: [null, Tv] where Tv = audio metadata
//
// Tv audio metadata:
//
//	pos 0: null (not title)
//	pos 1: audioType (1=deep_dive, 2=brief, 3=critique, 4=debate, 5=podcast, 6=lecture, 7=talk_show)
//	pos 3: sourceRefsFlat [["src1"],["src2"]] (double-nested)
//	pos 4: "en" (language)
//	pos 5: instructions or null
//	pos 6: audioLength (1=short, 2=medium/default)
func EncodeCreateAudioOverviewArgs(req *notebooklmv1alpha1.CreateAudioOverviewRequest) []interface{} {
	// Build source references: triple-nested for artifact spec pos 3
	var sourceRefsTriple []interface{}
	// Build source references: double-nested for Tv pos 3
	var sourceRefsDouble []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRefsTriple = append(sourceRefsTriple, []interface{}{[]interface{}{id}})
		sourceRefsDouble = append(sourceRefsDouble, []interface{}{id})
	}

	audioType := int(req.GetAudioType())
	if audioType == 0 {
		audioType = 1 // default to deep_dive
	}
	audioLength := int(req.GetLength())
	if audioLength == 0 {
		audioLength = 2 // default medium
	}

	// Tv audio metadata
	var instructions interface{}
	if req.GetCustomInstructions() != "" {
		instructions = req.GetCustomInstructions()
	}
	tv := []interface{}{
		nil,              // pos 0: null
		audioType,        // pos 1: audio type enum
		nil,              // pos 2: gap
		sourceRefsDouble, // pos 3: source refs (double-nested)
		"en",             // pos 4: language
		instructions,     // pos 5: custom instructions or null
		audioLength,      // pos 6: audio length
	}

	// Rv audio wrapper: [null, Tv]
	rv := []interface{}{nil, tv}

	// Artifact spec
	artifactSpec := []interface{}{
		nil,               // pos 0
		nil,               // pos 1
		1,                 // pos 2: artifact type = 1 (audio)
		sourceRefsTriple,  // pos 3: source refs (triple-nested)
		nil,               // pos 4
		nil,               // pos 5
		rv,                // pos 6: [null, Tv] audio metadata
	}

	// ProjectContext with full capabilities
	projectContext := []interface{}{
		2, nil, nil,
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
		[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
	}

	return []interface{}{
		projectContext,      // field 1: ProjectContext
		req.GetProjectId(),  // field 2: project ID
		artifactSpec,        // field 3: artifact spec
	}
}
