//go:build windows

package auth

import (
    "fmt"
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
    return filepath.Join(localAppData, "Google", "Chrome", "User Data", "Default")
}

func getChromePath() string {
    programFiles := os.Getenv("PROGRAMFILES")
    if programFiles == "" {
        programFiles = "C:\\Program Files"
    }
    return filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe")
}
