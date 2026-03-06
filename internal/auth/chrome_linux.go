//go:build linux

package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func detectChrome(debug bool) Browser {
	// Try standard Chrome first
	if path, err := exec.LookPath("google-chrome"); err == nil {
		version := getChromeVersion(path)
		return Browser{
			Type:    BrowserChrome,
			Path:    path,
			Name:    "Google Chrome",
			Version: version,
		}
	}

	// Try Chromium as fallback
	if path, err := exec.LookPath("chromium"); err == nil {
		version := getChromeVersion(path)
		return Browser{
			Type:    BrowserChrome,
			Path:    path,
			Name:    "Chromium",
			Version: version,
		}
	}

	return Browser{Type: BrowserUnknown}
}

func getChromeVersion(path string) string {
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "google-chrome")
}

func getChromePath() string {
	for _, name := range []string{"google-chrome", "chrome", "chromium"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// getBrowserPathForProfile 在 Linux 中查找浏览器可执行文件
func getBrowserPathForProfile(browserName string) string {
	var binaryName string

	switch browserName {
	case "Brave":
		binaryName = "brave-browser"
	case "Chrome Canary":
		// Linux 上通常没有官方的 "Canary" 版本，对应的是 "google-chrome-unstable"
		binaryName = "google-chrome-unstable"
	default:
		// 默认回退到标准 chrome 或 chromium
		return getChromePath()
	}

	// 在 Linux 中，最好的做法是使用 exec.LookPath 在 $PATH 中查找
	if path, err := exec.LookPath(binaryName); err == nil {
		return path
	}

	// 备选方案：检查常见的硬编码路径（如 /usr/bin）
	commonPaths := []string{
		filepath.Join("/usr/bin", binaryName),
		filepath.Join("/usr/local/bin", binaryName),
		filepath.Join("/snap/bin", binaryName), // 支持 Snap 安装
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// 获取配置文件的基准目录（遵循 XDG 规范）
func getConfigDir() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return xdgConfig
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func getCanaryProfilePath() string {
	// 对应 google-chrome-unstable 的配置路径
	return filepath.Join(getConfigDir(), "google-chrome-unstable")
}

func getBraveProfilePath() string {
	// Brave 在 Linux 下的配置路径
	return filepath.Join(getConfigDir(), "BraveSoftware", "Brave-Browser")
}
