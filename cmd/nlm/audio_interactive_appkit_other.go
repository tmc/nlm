//go:build !darwin

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type interactiveAudioOptions struct {
	AudioID        string
	TranscriptOnly bool
	NoMic          bool
	MicApp         bool
	Speaker        string
	Mic            string
	Timeout        time.Duration
	Help           bool
}

var errInteractiveAudioHelp = errors.New("interactive audio help shown")

func validateInteractiveAudioArgs(args []string) error {
	_, _, err := parseInteractiveAudioArgs(args)
	return err
}

func parseInteractiveAudioArgs(args []string) (interactiveAudioOptions, string, error) {
	if len(args) < 1 {
		return interactiveAudioOptions{}, "", fmt.Errorf("usage: nlm audio-interactive <notebook-id>")
	}
	return interactiveAudioOptions{}, args[0], nil
}

func runInteractiveAudioCommand(client *api.Client, notebookID string, opts interactiveAudioOptions) error {
	_, _, _ = client, notebookID, opts
	return fmt.Errorf("audio-interactive is only supported on macOS")
}

func runInteractiveAudioWithMicApp(client *api.Client, notebookID string, opts interactiveAudioOptions) error {
	_, _, _ = client, notebookID, opts
	return fmt.Errorf("--mic-app is only supported on darwin")
}

func lockInteractiveAudioAppThreadIfNeeded(args []string) {
	_ = args
}
