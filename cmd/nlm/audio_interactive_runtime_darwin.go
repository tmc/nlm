//go:build darwin

package main

import (
	"fmt"
	"os"

	"github.com/tmc/macgo"
	"golang.org/x/term"
)

var startInteractiveAudioMacgo = func(cfg *macgo.Config) error {
	return macgo.Start(cfg)
}

var interactiveAudioTTY = term.IsTerminal

func prepareInteractiveAudioRuntimePlatform(opts interactiveAudioOptions) error {
	if opts.TranscriptOnly || opts.NoMic {
		return nil
	}
	if interactiveAudioTTY(int(os.Stdin.Fd())) {
		_ = os.Setenv("MACGO_TTY_PASSTHROUGH", "1")
	}

	cfg := macgo.NewConfig().
		WithAppName("nlm audio").
		WithBundleID("com.tmc.nlm.audiointeractive").
		WithPermissions(macgo.Microphone).
		WithMicrophoneUsage("Capture microphone audio for NotebookLM interactive audio sessions.").
		WithDevMode().
		WithAdHocSign()
	if opts.MicApp {
		cfg.WithUIMode(macgo.UIModeAccessory)
	} else {
		cfg.WithUIMode(macgo.UIModeBackground)
	}
	if debug {
		cfg.WithDebug()
	}
	if err := startInteractiveAudioMacgo(cfg); err != nil {
		return fmt.Errorf("prepare interactive audio bundle: %w", err)
	}
	return nil
}
