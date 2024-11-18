//go:build darwin

package auth

import (
	"os"
	"path/filepath"
)

func getProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default")
}

func getChromePath() string {
	return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
}
