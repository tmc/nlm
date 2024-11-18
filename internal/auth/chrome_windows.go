//go:build windows
package auth

import (
    "os"
    "path/filepath"
)

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
        programFiles = `C:\Program Files`
    }
    return filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe")
}
