package method

import notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"

// GENERATION_BEHAVIOR: append

// RPC ID: rc3d8d
func EncodeRenameArtifactArgs(req *notebooklmv1alpha1.RenameArtifactRequest) []interface{} {
	return []interface{}{
		[]interface{}{req.GetArtifactId(), req.GetNewTitle()},
		[]interface{}{[]interface{}{"title"}},
	}
}
