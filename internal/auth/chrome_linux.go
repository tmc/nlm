//go:build linux

package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func detectChrome(debug bool) Browser {
	// Try standard Chrome first
	if path, err := exec.LookPath("google-chrome"); err == nil {
		version := getChromeVersion(path)
		return Browser{
			Type:    BrowserChrome,
			Path:    path,
			Name:    "Google Chrome",
			Version: version,
		}
	}

	// Try Chromium as fallback
	if path, err := exec.LookPath("chromium"); err == nil {
		version := getChromeVersion(path)
		return Browser{
			Type:    BrowserChrome,
			Path:    path,
			Name:    "Chromium",
			Version: version,
		}
	}

	return Browser{Type: BrowserUnknown}
}

func getChromeVersion(path string) string {
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

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

// getBrowserPathForProfile returns the appropriate browser executable for a given browser type
func getBrowserPathForProfile(browserName string) string {
	switch browserName {
	case "Brave":
		// Try Brave paths
		bravePaths := []string{"brave-browser", "brave"}
		for _, name := range bravePaths {
			if path, err := exec.LookPath(name); err == nil {
				return path
			}
		}
	case "Chrome Canary":
		// Chrome Canary is typically not available on Linux
		// Fall back to regular Chrome
		return getChromePath()
	}

	// Fallback to any Chrome-based browser
	return getChromePath()
}

func getCanaryProfilePath() string {
	// Chrome Canary is not typically available on Linux
	// Return an empty string or fall back to regular Chrome profile path
	return getProfilePath()
}

func getBraveProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser")
}
