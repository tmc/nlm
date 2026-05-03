package method

import notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"

// artifactTypeDescriptor is the constant arg[0] for R7cb6c calls.
// Verified against HAR captures for audio, video, and slides — identical across all types.
var artifactTypeDescriptor = []interface{}{
	2, nil, nil,
	[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
	[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
}

// infographicArtifactTypeDescriptor is the observed descriptor for infographic
// R7cb6c calls. Unlike audio/video/slides, the UI omits the final type 5 flag.
var infographicArtifactTypeDescriptor = []interface{}{
	2, nil, nil,
	[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
	[]interface{}{[]interface{}{1, 4, 2, 3, 6}},
}

// encodeOverviewSourceRefs returns 3-level nesting: [[[id1]], [[id2]], ...]
// Used for the outer source refs at arg[2][3].
func encodeOverviewSourceRefs(sourceIDs []string) []interface{} {
	refs := make([]interface{}, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		refs = append(refs, []interface{}{[]interface{}{id}})
	}
	return refs
}

// encodeInnerSourceRefs returns 2-level nesting: [[id1], [id2], ...]
// Used for source refs inside audioConfig/videoConfig inner arrays.
func encodeInnerSourceRefs(sourceIDs []string) []interface{} {
	refs := make([]interface{}, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		refs = append(refs, []interface{}{id})
	}
	return refs
}

// EncodeCreateAudioOverviewArgs encodes the observed R7cb6c audio-overview payload.
func EncodeCreateAudioOverviewArgs(req *notebooklmv1alpha1.CreateAudioOverviewRequest) []interface{} {
	// Wire format verified against HAR capture (2026-04-14) — do not regenerate.
	sourceRefs := encodeOverviewSourceRefs(req.GetSourceIds())
	innerSourceRefs := encodeInnerSourceRefs(req.GetSourceIds())
	var instructions interface{}
	if req.GetCustomInstructions() != "" {
		instructions = req.GetCustomInstructions()
	}
	return []interface{}{
		artifactTypeDescriptor,
		req.GetProjectId(),
		[]interface{}{
			nil,
			nil,
			1, // artifact type = audio
			sourceRefs,
			nil,
			nil,
			[]interface{}{
				nil,
				[]interface{}{
					instructions,              // [0] custom instructions or nil
					2,                         // [1] constant
					nil,                       // [2]
					innerSourceRefs,           // [3] 2-level nesting
					req.GetLanguage(),         // [4] language
					nil,                       // [5] nil (not true)
					int32(req.GetAudioType()), // [6] audio style enum
				},
			},
		},
	}
}

// EncodeCreateSlideDeckArgs encodes the observed R7cb6c slide-deck payload.
func EncodeCreateSlideDeckArgs(projectID string, sourceIDs []string, instructions, language string) []interface{} {
	// Wire format verified against HAR capture (2026-04-14) — do not regenerate.
	sourceRefs := encodeOverviewSourceRefs(sourceIDs)
	return []interface{}{
		artifactTypeDescriptor,
		projectID,
		[]interface{}{
			nil,
			nil,
			8, // artifact type 8 = slide deck
			sourceRefs,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			[]interface{}{[]interface{}{instructions, language, 1, 3}},
		},
	}
}

// EncodeCreateInfographicArgs encodes the R7cb6c infographic payload.
func EncodeCreateInfographicArgs(projectID string, sourceIDs []string, instructions, language string) []interface{} {
	_ = language
	sourceRefs := encodeOverviewSourceRefs(sourceIDs)
	var customInstructions interface{}
	if instructions != "" {
		customInstructions = instructions
	}
	return []interface{}{
		infographicArtifactTypeDescriptor,
		projectID,
		[]interface{}{
			nil,
			nil,
			7, // artifact type 7 = infographic
			sourceRefs,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			[]interface{}{[]interface{}{customInstructions, nil, nil, 1, 2}},
		},
	}
}

// EncodeCreateFlashcardsArgs encodes the observed R7cb6c flashcards payload.
func EncodeCreateFlashcardsArgs(projectID string, sourceIDs []string) []interface{} {
	sourceRefs := encodeOverviewSourceRefs(sourceIDs)
	return []interface{}{
		infographicArtifactTypeDescriptor,
		projectID,
		[]interface{}{
			nil,
			nil,
			4, // artifact type 4 covers generated report-style artifacts, including flashcards
			sourceRefs,
			nil, nil, nil, nil, nil,
			[]interface{}{
				nil,
				[]interface{}{1, nil, nil, nil, nil, nil, []interface{}{2, 2}},
			},
		},
	}
}

// EncodeCreateVideoOverviewArgs encodes the observed R7cb6c video-overview payload.
func EncodeCreateVideoOverviewArgs(req *notebooklmv1alpha1.CreateVideoOverviewRequest) []interface{} {
	// Wire format verified against HAR capture (2026-04-14) — do not regenerate.
	sourceRefs := encodeOverviewSourceRefs(req.GetSourceIds())
	innerSourceRefs := encodeInnerSourceRefs(req.GetSourceIds())
	return []interface{}{
		artifactTypeDescriptor,
		req.GetProjectId(),
		[]interface{}{
			nil,
			nil,
			3, // artifact type = video
			sourceRefs,
			nil, nil, nil, nil,
			[]interface{}{
				nil,
				nil,
				[]interface{}{
					innerSourceRefs,            // [0] 2-level nesting
					nil,                        // [1]
					nil,                        // [2]
					nil,                        // [3]
					int32(req.GetVideoStyle()), // [4] video style enum
				},
			},
		},
	}
}
