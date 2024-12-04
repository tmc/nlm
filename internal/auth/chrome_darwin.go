//go:build darwin

package auth

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
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

    // Try finding Chrome via mdfind
    if path := findBrowserViaMDFind("com.google.Chrome"); path != "" {
        return filepath.Join(path, "Contents/MacOS/Google Chrome")
    }

    return ""
}

func detectChrome(debug bool) Browser {
    // First try standard paths
    for _, browser := range macOSBrowserPaths {
        if browser.Type != BrowserChrome {
            continue
        }
        if _, err := os.Stat(browser.Path); err == nil {
            version := getChromeVersion(browser.Path)
            if debug {
                fmt.Printf("Found %s at %s (version: %s)\n", browser.Name, browser.Path, version)
            }
            return Browser{
                Type:    browser.Type,
                Path:    browser.Path,
                Name:    browser.Name,
                Version: version,
            }
        }
    }

    // Try finding Chrome-based browsers via mdfind
    browserBundles := map[string]BrowserPriority{
        "com.google.Chrome":        {Name: "Google Chrome", Type: BrowserChrome},
        "com.google.Chrome.canary": {Name: "Chrome Canary", Type: BrowserChrome},
        "org.chromium.Chromium":    {Name: "Chromium", Type: BrowserChrome},
        "com.microsoft.edgemac":    {Name: "Microsoft Edge", Type: BrowserChrome},
        "com.brave.Browser":        {Name: "Brave", Type: BrowserChrome},
    }

    for bundleID, browser := range browserBundles {
        if path := findBrowserViaMDFind(bundleID); path != "" {
            execPath := filepath.Join(path, "Contents/MacOS", browser.Name)
            version := getChromeVersion(execPath)
            if debug {
                fmt.Printf("Found %s via mdfind at %s (version: %s)\n", browser.Name, execPath, version)
            }
            return Browser{
                Type:    browser.Type,
                Path:    execPath,
                Name:    browser.Name,
                Version: version,
            }
        }
    }

    if debug {
        fmt.Printf("No Chrome-based browsers found\n")
    }
    return Browser{Type: BrowserUnknown}
}

func findBrowserViaMDFind(bundleID string) string {
    cmd := exec.Command("mdfind", fmt.Sprintf("kMDItemCFBundleIdentifier == '%s'", bundleID))
    out, err := cmd.Output()
    if err == nil && len(out) > 0 {
        paths := strings.Split(strings.TrimSpace(string(out)), "\n")
        if len(paths) > 0 {
            return paths[0]
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

func getProfilePath() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default")
}

func checkBrowserInstallation() string {
    var messages []string
    var found bool

    for _, browser := range macOSBrowserPaths {
        if _, err := os.Stat(browser.Path); err == nil {
            found = true
            break
        }
    }

    if !found {
        messages = append(messages, "No supported browsers found. Please install one of:")
        messages = append(messages, "- Google Chrome (https://www.google.com/chrome/)")
        messages = append(messages, "- Chrome Canary (https://www.google.com/chrome/canary/)")
        messages = append(messages, "- Chromium (https://www.chromium.org/)")
        messages = append(messages, "- Microsoft Edge (https://www.microsoft.com/edge)")
        messages = append(messages, "- Brave Browser (https://brave.com/)")
        messages = append(messages, "Or use Safari (pre-installed)")
    }

    return strings.Join(messages, "\n")
}
