//go:build linux
package auth

import (
    "os/exec"
    "path/filepath"
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