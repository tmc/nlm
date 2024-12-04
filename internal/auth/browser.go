package auth

import (
    "fmt"
)

type BrowserType int

const (
    BrowserUnknown BrowserType = iota
    BrowserChrome
    BrowserSafari
)

type Browser struct {
    Type    BrowserType
    Path    string
    Name    string
    Version string
}

func (b Browser) String() string {
    return fmt.Sprintf("%s (%s)", b.Name, b.Version)
}

func detectBrowsers(debug bool) []Browser {
    var browsers []Browser

    if chrome := detectChrome(debug); chrome.Path != "" {
        browsers = append(browsers, chrome)
    }

    if safari := detectSafari(debug); safari.Path != "" {
        browsers = append(browsers, safari)
    }

    return browsers
}
