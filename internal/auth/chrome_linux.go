//go:build linux

package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// linuxBrowsers defines the priority order for Chromium-based browser detection on Linux.
var linuxBrowsers = []struct {
	binaries []string
	name     string
}{
	{[]string{"google-chrome", "google-chrome-stable"}, "Google Chrome"},
	{[]string{"chromium", "chromium-browser"}, "Chromium"},
	{[]string{"google-chrome-unstable"}, "Chrome Canary"},
	{[]string{"microsoft-edge-stable", "microsoft-edge"}, "Microsoft Edge"},
	{[]string{"brave-browser", "brave"}, "Brave"},
}

func detectChrome(debug bool) Browser {
	for _, b := range linuxBrowsers {
		for _, bin := range b.binaries {
			if path, err := exec.LookPath(bin); err == nil {
				return Browser{
					Type:    BrowserChrome,
					Path:    path,
					Name:    b.name,
					Version: getChromeVersion(path),
				}
			}
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
	chromePath := filepath.Join(home, ".config", "google-chrome")
	if _, err := os.Stat(chromePath); os.IsNotExist(err) {
		if brave := getBraveProfilePath(); dirExists(brave) {
			return brave
		}
	}
	return chromePath
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func getChromePath() string {
	candidates := []string{
		"google-chrome", "google-chrome-stable", "chrome",
		"chromium", "chromium-browser",
		"microsoft-edge-stable", "microsoft-edge",
		"brave-browser", "brave",
	}
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func getBrowserPathForProfile(browserName string) string {
	switch browserName {
	case "Brave":
		for _, bin := range []string{"brave-browser", "brave"} {
			if path, err := exec.LookPath(bin); err == nil {
				return path
			}
		}
	case "Chrome Canary":
		if path, err := exec.LookPath("google-chrome-unstable"); err == nil {
			return path
		}
	case "Microsoft Edge":
		for _, bin := range []string{"microsoft-edge-stable", "microsoft-edge"} {
			if path, err := exec.LookPath(bin); err == nil {
				return path
			}
		}
	}
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
