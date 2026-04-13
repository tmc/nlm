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

// EncodeCreateVideoOverviewArgs encodes the observed R7cb6c video-overview payload.
func EncodeCreateVideoOverviewArgs(req *notebooklmv1alpha1.CreateVideoOverviewRequest) []interface{} {
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
