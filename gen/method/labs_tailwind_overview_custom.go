package method

import notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"

func encodeOverviewSourceRefs(sourceIDs []string) []interface{} {
	refs := make([]interface{}, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		refs = append(refs, []interface{}{[]interface{}{id}})
	}
	return refs
}

// EncodeCreateAudioOverviewArgs encodes the observed R7cb6c audio-overview payload.
func EncodeCreateAudioOverviewArgs(req *notebooklmv1alpha1.CreateAudioOverviewRequest) []interface{} {
	// Wire format verified against HAR capture — do not regenerate.
	sourceRefs := encodeOverviewSourceRefs(req.GetSourceIds())
	return []interface{}{
		[]interface{}{int32(req.GetAudioType())},
		req.GetProjectId(),
		[]interface{}{
			nil,
			nil,
			1,
			sourceRefs,
			nil,
			nil,
			[]interface{}{
				nil,
				[]interface{}{
					req.GetCustomInstructions(),
					int32(req.GetLength()),
					nil,
					sourceRefs,
					req.GetLanguage(),
					true,
					1,
				},
			},
		},
	}
}

// EncodeCreateSlideDeckArgs encodes the observed R7cb6c slide-deck payload.
func EncodeCreateSlideDeckArgs(projectID string, sourceIDs []string, instructions, language string) []interface{} {
	// Wire format verified against HAR capture — do not regenerate.
	sourceRefs := encodeOverviewSourceRefs(sourceIDs)
	return []interface{}{
		[]interface{}{
			2, nil, nil,
			[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
			[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
		},
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

// EncodeCreateVideoOverviewArgs encodes the observed R7cb6c video-overview payload.
func EncodeCreateVideoOverviewArgs(req *notebooklmv1alpha1.CreateVideoOverviewRequest) []interface{} {
	// Wire format verified against HAR capture — do not regenerate.
	sourceRefs := encodeOverviewSourceRefs(req.GetSourceIds())
	return []interface{}{
		[]interface{}{int32(req.GetAudioType())},
		req.GetProjectId(),
		[]interface{}{
			nil,
			nil,
			3,
			sourceRefs,
			nil,
			nil,
			nil,
			nil,
			[]interface{}{
				nil,
				nil,
				[]interface{}{
					sourceRefs,
					req.GetLanguage(),
					req.GetCustomInstructions(),
					nil,
					1,
					int32(req.GetVideoStyle()),
				},
			},
		},
	}
}
