package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeGenerateNotebookGuideArgs encodes arguments for
// LabsTailwindOrchestrationService.GenerateNotebookGuide.
// RPC ID: VfAZjd
//
// Wire format, verified against captured NotebookLM requests:
//
//	[%project_id%, %request_context%]
func EncodeGenerateNotebookGuideArgs(req *notebooklmv1alpha1.GenerateNotebookGuideRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		[]interface{}{
			2,
			nil,
			[]interface{}{1},
			[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1, 3}},
		},
	}
}
