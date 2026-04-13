//go:build !darwin

package main

import "fmt"

func prepareInteractiveAudioRuntimePlatform(opts interactiveAudioOptions) error {
	if opts.MicApp {
		return fmt.Errorf("--mic-app is only supported on darwin")
	}
	return nil
}
