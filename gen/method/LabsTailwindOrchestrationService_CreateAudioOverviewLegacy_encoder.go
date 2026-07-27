package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateAudioOverviewLegacyArgs encodes arguments for LabsTailwindOrchestrationService.CreateAudioOverviewLegacy
// RPC ID: AHyHrd
// Argument format: [%project_id%, %audio_type%, %instructions%]
func EncodeCreateAudioOverviewLegacyArgs(req *notebooklmv1alpha1.CreateAudioOverviewLegacyRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %audio_type%, %instructions%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
