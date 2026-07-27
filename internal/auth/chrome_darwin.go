//go:build darwin

package auth

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BrowserType distinguishes the browser families listed in macOSBrowserPaths.
type BrowserType int

const (
	BrowserChrome BrowserType = iota
	BrowserSafari
)

type BrowserPriority struct {
	Path    string
	Name    string
	Type    BrowserType
	Version string
}

var macOSBrowserPaths = []BrowserPriority{
	{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "Google Chrome", BrowserChrome, ""},
	{"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary", "Chrome Canary", BrowserChrome, ""},
	{"/Applications/Chromium.app/Contents/MacOS/Chromium", "Chromium", BrowserChrome, ""},
	{"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "Microsoft Edge", BrowserChrome, ""},
	{"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser", "Brave", BrowserChrome, ""},
	{"/Applications/Safari.app/Contents/MacOS/Safari", "Safari", BrowserSafari, ""},
}

func getChromePath() string {
	// First try standard paths
	for _, browser := range macOSBrowserPaths {
		if browser.Type == BrowserChrome {
			if _, err := os.Stat(browser.Path); err == nil {
				return browser.Path
			}
		}
	}

	// Try finding browsers via mdfind
	browserPaths := map[string]string{
		"com.google.Chrome": "Contents/MacOS/Google Chrome",
		"com.brave.Browser": "Contents/MacOS/Brave Browser",
	}

	for bundleID, execPath := range browserPaths {
		if path := findBrowserViaMDFind(bundleID); path != "" {
			return filepath.Join(path, execPath)
		}
	}

	return ""
}

// getBrowserPathForProfile returns the appropriate browser executable for a given browser type
func getBrowserPathForProfile(browserName string) string {
	switch browserName {
	case "Brave":
		// Try Brave paths first
		bravePaths := []string{
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
		for _, path := range bravePaths {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		// Try finding via mdfind
		if path := findBrowserViaMDFind("com.brave.Browser"); path != "" {
			return filepath.Join(path, "Contents/MacOS/Brave Browser")
		}
	case "Chrome Canary":
		canaryPaths := []string{
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
		}
		for _, path := range canaryPaths {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		if path := findBrowserViaMDFind("com.google.Chrome.canary"); path != "" {
			return filepath.Join(path, "Contents/MacOS/Google Chrome Canary")
		}
	}

	// Fallback to any Chrome-based browser
	return getChromePath()
}

func findBrowserViaMDFind(bundleID string) string {
	cmd := exec.Command("mdfind", fmt.Sprintf("kMDItemCFBundleIdentifier == '%s'", bundleID))
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		paths := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(paths) > 0 {
			// If there are multiple instances, prioritize by most recently modified
			if len(paths) > 1 {
				return getMostRecentPath(paths)
			}
			return paths[0]
		}
	}
	return ""
}

func getMostRecentPath(paths []string) string {
	var mostRecent string
	var mostRecentTime time.Time

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		modTime := info.ModTime()
		if mostRecent == "" || modTime.After(mostRecentTime) {
			mostRecent = path
			mostRecentTime = modTime
		}
	}

	return mostRecent
}

func getProfilePath() string {
	home, _ := os.UserHomeDir()
	chromePath := filepath.Join(home, "Library", "Application Support", "Google", "Chrome")

	// Check if Chrome directory exists, if not, try Brave
	if _, err := os.Stat(chromePath); os.IsNotExist(err) {
		// Try Brave instead
		bravePath := getBraveProfilePath()
		if _, err := os.Stat(bravePath); err == nil {
			return bravePath
		}
	}

	return chromePath
}

func getCanaryProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Google", "Chrome Canary")
}

func getBraveProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser")
}
