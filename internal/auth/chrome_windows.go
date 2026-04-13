//go:build windows

package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func detectChrome(debug bool) Browser {
	path := getChromePath()
	if path == "" {
		return Browser{Type: BrowserUnknown}
	}

	version := getChromeVersion(path)
	return Browser{
		Type:    BrowserChrome,
		Path:    path,
		Name:    "Google Chrome",
		Version: version,
	}
}

func getChromeVersion(path string) string {
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(strings.TrimPrefix(string(out), "Google Chrome "))
}

func getProfilePath() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "Google", "Chrome", "User Data")
}

func getChromePath() string {
	paths := []string{
		filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
	}
	if os.Getenv("PROGRAMFILES") == "" {
		paths = append(paths, filepath.Join("C:\\Program Files", "Google", "Chrome", "Application", "chrome.exe"))
	}
	if os.Getenv("PROGRAMFILES(X86)") == "" {
		paths = append(paths, filepath.Join("C:\\Program Files (x86)", "Google", "Chrome", "Application", "chrome.exe"))
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func getBrowserPathForProfile(browserName string) string {
	switch browserName {
	case "Brave":
		bravePaths := []string{
			filepath.Join(os.Getenv("PROGRAMFILES"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		}
		for _, path := range bravePaths {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	case "Chrome Canary":
		canaryPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome SxS", "Application", "chrome.exe")
		if _, err := os.Stat(canaryPath); err == nil {
			return canaryPath
		}
	}
	return getChromePath()
}

func getCanaryProfilePath() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "Google", "Chrome SxS", "User Data")
}

func getBraveProfilePath() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "User Data")
}
