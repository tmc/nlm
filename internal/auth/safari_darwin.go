//go:build darwin

package auth

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func detectSafari(debug bool) Browser {
	paths := []string{
		"/Applications/Safari.app/Contents/MacOS/Safari",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			version := getSafariVersion()
			return Browser{
				Type:    BrowserSafari,
				Path:    path,
				Name:    "Safari",
				Version: version,
			}
		}
	}

	return Browser{Type: BrowserUnknown}
}

func getSafariVersion() string {
	cmd := exec.Command("defaults", "read", "/Applications/Safari.app/Contents/Info.plist", "CFBundleShortVersionString")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

type SafariAutomation struct {
	debug  bool
	script string
}

func newSafariAutomation(debug bool) *SafariAutomation {
	return &SafariAutomation{
		debug: debug,
		script: `
tell application "Safari"
    activate
    make new document
    set URL of document 1 to "https://notebooklm.google.com"

    -- Wait for page load
    repeat until (do JavaScript "!!window.WIZ_global_data" in document 1) is true
        delay 1
    end repeat

    -- Get auth data
    set authToken to do JavaScript "window.WIZ_global_data.SNlM0e" in document 1
    set cookies to do JavaScript "document.cookie" in document 1

    -- Return results
    return authToken & "|" & cookies
end tell
`,
	}
}

func (sa *SafariAutomation) Execute() (token, cookies string, err error) {
	cmd := exec.Command("osascript", "-e", sa.script)
	out, err := cmd.Output()
	if err != nil {
		return "", "", err
	}

	parts := strings.Split(string(out), "|")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected Safari automation output")
	}

	return parts[0], parts[1], nil
}
