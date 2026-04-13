//go:build !darwin

package main

import (
	"fmt"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func runInteractiveAudioWithMicApp(client *api.Client, notebookID string, opts interactiveAudioOptions) error {
	_, _, _ = client, notebookID, opts
	return fmt.Errorf("--mic-app is only supported on darwin")
}

func lockInteractiveAudioAppThreadIfNeeded(args []string) {
	_ = args
}
