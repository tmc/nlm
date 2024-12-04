//go:build linux

package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func getProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "google-chrome", "Default")
}

func getChromePath() string {
	if path, err := exec.LookPath("google-chrome"); err == nil {
		return path
	}
	if path, err := exec.LookPath("chromium"); err == nil {
		return path
	}
	return ""
}
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
