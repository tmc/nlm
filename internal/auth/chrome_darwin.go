//go:build darwin

package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func getProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default")
}

func getChromePath() string {
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chrome.app/Contents/MacOS/Chrome",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Try finding Chrome via mdfind
	if path := findChromeViaMDFind(); path != "" {
		return path
	}

	return ""
}

func findChromeViaMDFind() string {
	cmd := exec.Command("mdfind", "kMDItemCFBundleIdentifier == 'com.google.Chrome'")
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		paths := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(paths) > 0 {
			return filepath.Join(paths[0], "Contents/MacOS/Google Chrome")
		}
	}
	return ""
}

func getChromeVersion(path string) string {
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(strings.TrimPrefix(string(out), "Google Chrome "))
}

func removeQuarantine(path string) error {
	cmd := exec.Command("xattr", "-d", "com.apple.quarantine", path)
	return cmd.Run()
}
func detectChrome(debug bool) Browser {
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chrome.app/Contents/MacOS/Chrome",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			version := getChromeVersion(path)
			return Browser{
				Type:    BrowserChrome,
				Path:    path,
				Name:    "Google Chrome",
				Version: version,
			}
		}
	}

	// Try finding Chrome via mdfind
	if path := findChromeViaMDFind(); path != "" {
		version := getChromeVersion(path)
		return Browser{
			Type:    BrowserChrome,
			Path:    path,
			Name:    "Google Chrome",
			Version: version,
		}
	}

	return Browser{Type: BrowserUnknown}
}
