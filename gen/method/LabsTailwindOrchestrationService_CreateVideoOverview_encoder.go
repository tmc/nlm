package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateVideoOverviewArgs encodes arguments for creating a video overview
// via the CreateArtifact RPC (R7cb6c).
//
// Wire format mirrors audio (type=1) but with type=3 for video:
//
//	[ProjectContext, "notebook-id", artifactSpec]
//
// artifactSpec: [null, null, 3, sourceRefs, null, null, null, null, videoWrapper]
//
//	pos 2: artifact type = 3 (video)
//	pos 3: sourceRefs = [[["id1"]], [["id2"]]] (triple-nested)
//	pos 8: [null, null, Vv] where Vv = video metadata
//
// Vv video metadata:
//
//	pos 0: sourceRefsFlat [["src1"],["src2"]] (double-nested)
//	pos 1: "en" (language)
//	pos 2: instructions or null
func EncodeCreateVideoOverviewArgs(req *notebooklmv1alpha1.CreateVideoOverviewRequest) []interface{} {
	var sourceRefsTriple []interface{}
	var sourceRefsDouble []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRefsTriple = append(sourceRefsTriple, []interface{}{[]interface{}{id}})
		sourceRefsDouble = append(sourceRefsDouble, []interface{}{id})
	}

	var instructions interface{}
	if req.GetCustomInstructions() != "" {
		instructions = req.GetCustomInstructions()
	}

	// Video metadata
	vv := []interface{}{
		sourceRefsDouble, // pos 0: source refs (double-nested)
		"en",             // pos 1: language
		instructions,     // pos 2: custom instructions or null
	}

	// Video wrapper: [null, null, Vv]
	videoWrapper := []interface{}{nil, nil, vv}

	// Artifact spec — type=3 for video, videoWrapper at pos 8
	artifactSpec := []interface{}{
		nil,              // pos 0
		nil,              // pos 1
		3,                // pos 2: artifact type = 3 (video)
		sourceRefsTriple, // pos 3: source refs (triple-nested)
		nil,              // pos 4
		nil,              // pos 5
		nil,              // pos 6 (audio uses this)
		nil,              // pos 7
		videoWrapper,     // pos 8: [null, null, Vv] video metadata
	}

	projectContext := []interface{}{
		2, nil, nil,
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
		[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
	}

	return []interface{}{
		projectContext,     // field 1: ProjectContext
		req.GetProjectId(), // field 2: project ID
		artifactSpec,       // field 3: artifact spec
	}
}
