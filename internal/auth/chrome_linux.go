//go:build linux

package auth

import (
	"os"
	"os/exec"
	"path/filepath"
)

func getProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "google-chrome")
}

func getChromePath() string {
	for _, name := range []string{"google-chrome", "chrome", "chromium"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func getBrowserPathForProfile(browserName string) string {
	switch browserName {
	case "Chrome Canary":
		// On Linux, Canary is usually google-chrome-unstable
		if path, err := exec.LookPath("google-chrome-unstable"); err == nil {
			return path
		}
	case "Brave":
		if path, err := exec.LookPath("brave-browser"); err == nil {
			return path
		}
	}
	// Default to Chrome/Chromium
	return getChromePath()
}

func getCanaryProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "google-chrome-unstable")
}

func getBraveProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser")
}
