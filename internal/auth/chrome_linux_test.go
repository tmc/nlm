//go:build linux

package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetCanaryProfilePath(t *testing.T) {
	got := getCanaryProfilePath()
	want := filepath.Join(".config", "google-chrome-unstable")
	if !strings.HasSuffix(got, want) {
		t.Errorf("getCanaryProfilePath() = %q, want suffix %q", got, want)
	}
}

func TestGetBraveProfilePath(t *testing.T) {
	got := getBraveProfilePath()
	want := filepath.Join(".config", "BraveSoftware", "Brave-Browser")
	if !strings.HasSuffix(got, want) {
		t.Errorf("getBraveProfilePath() = %q, want suffix %q", got, want)
	}
}

func TestDirExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nlm-test-direxists-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile, err := os.CreateTemp("", "nlm-test-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"existing directory", tmpDir, true},
		{"non-existing path", filepath.Join(tmpDir, "nonexistent"), false},
		{"existing file (not a dir)", tmpFile.Name(), false},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dirExists(tt.path)
			if got != tt.want {
				t.Errorf("dirExists(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestGetProfilePath(t *testing.T) {
	origHome := os.Getenv("HOME")

	t.Run("returns chrome path when neither chrome nor brave dir exist", func(t *testing.T) {
		tmpHome, err := os.MkdirTemp("", "nlm-test-home-*")
		if err != nil {
			t.Fatalf("failed to create temp home: %v", err)
		}
		defer os.RemoveAll(tmpHome)
		os.Setenv("HOME", tmpHome)
		defer os.Setenv("HOME", origHome)

		got := getProfilePath()
		want := filepath.Join(tmpHome, ".config", "google-chrome")
		if got != want {
			t.Errorf("getProfilePath() = %q, want %q", got, want)
		}
	})

	t.Run("returns chrome path when chrome dir exists", func(t *testing.T) {
		tmpHome, err := os.MkdirTemp("", "nlm-test-home-*")
		if err != nil {
			t.Fatalf("failed to create temp home: %v", err)
		}
		defer os.RemoveAll(tmpHome)
		os.Setenv("HOME", tmpHome)
		defer os.Setenv("HOME", origHome)

		chromeDir := filepath.Join(tmpHome, ".config", "google-chrome")
		if err := os.MkdirAll(chromeDir, 0755); err != nil {
			t.Fatalf("failed to create chrome dir: %v", err)
		}

		got := getProfilePath()
		if got != chromeDir {
			t.Errorf("getProfilePath() = %q, want %q", got, chromeDir)
		}
	})

	t.Run("falls back to brave when chrome dir absent and brave dir exists", func(t *testing.T) {
		tmpHome, err := os.MkdirTemp("", "nlm-test-home-*")
		if err != nil {
			t.Fatalf("failed to create temp home: %v", err)
		}
		defer os.RemoveAll(tmpHome)
		os.Setenv("HOME", tmpHome)
		defer os.Setenv("HOME", origHome)

		braveDir := filepath.Join(tmpHome, ".config", "BraveSoftware", "Brave-Browser")
		if err := os.MkdirAll(braveDir, 0755); err != nil {
			t.Fatalf("failed to create brave dir: %v", err)
		}

		got := getProfilePath()
		if got != braveDir {
			t.Errorf("getProfilePath() = %q, want %q", got, braveDir)
		}
	})
}

func TestGetBrowserPathForProfile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nlm-test-browsers-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir)
	defer os.Setenv("PATH", origPath)

	fakeBrowsers := []struct {
		bin  string
		want string
	}{
		{"brave-browser", "Brave"},
		{"google-chrome-unstable", "Chrome Canary"},
		{"microsoft-edge-stable", "Microsoft Edge"},
	}
	for _, fb := range fakeBrowsers {
		path := filepath.Join(tmpDir, fb.bin)
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'fake browser'\n"), 0755); err != nil {
			t.Fatalf("failed to create fake browser %s: %v", fb.bin, err)
		}
	}

	tests := []struct {
		name        string
		browserName string
		wantSuffix  string
	}{
		{"Brave uses brave-browser binary", "Brave", "brave-browser"},
		{"Chrome Canary uses google-chrome-unstable", "Chrome Canary", "google-chrome-unstable"},
		{"Microsoft Edge uses microsoft-edge-stable", "Microsoft Edge", "microsoft-edge-stable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getBrowserPathForProfile(tt.browserName)
			if !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("getBrowserPathForProfile(%q) = %q, want suffix %q",
					tt.browserName, got, tt.wantSuffix)
			}
		})
	}

	t.Run("unknown browser name falls back to getChromePath result", func(t *testing.T) {
		// Isolate PATH so no browsers are found, ensuring fallback returns ""
		emptyDir, err := os.MkdirTemp("", "nlm-test-empty-*")
		if err != nil {
			t.Fatalf("failed to create empty dir: %v", err)
		}
		defer os.RemoveAll(emptyDir)

		os.Setenv("PATH", emptyDir)
		defer os.Setenv("PATH", origPath)

		got := getBrowserPathForProfile("Unknown Browser")
		if got != "" {
			t.Errorf("getBrowserPathForProfile(unknown) with empty PATH = %q, want %q", got, "")
		}
	})
}

func TestDetectChrome(t *testing.T) {
	t.Run("returns BrowserUnknown when no browsers in PATH", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "nlm-test-empty-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		origPath := os.Getenv("PATH")
		os.Setenv("PATH", tmpDir)
		defer os.Setenv("PATH", origPath)

		got := detectChrome(false)
		if got.Type != BrowserUnknown {
			t.Errorf("detectChrome() with empty PATH = type %v, want BrowserUnknown", got.Type)
		}
	})

	browsers := []struct {
		bin  string
		name string
	}{
		{"google-chrome", "Google Chrome"},
		{"chromium", "Chromium"},
		{"google-chrome-unstable", "Chrome Canary"},
		{"microsoft-edge-stable", "Microsoft Edge"},
		{"brave-browser", "Brave"},
	}

	for _, b := range browsers {
		b := b
		t.Run("detects "+b.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "nlm-test-browser-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			fakePath := filepath.Join(tmpDir, b.bin)
			script := "#!/bin/sh\necho '" + b.name + " 120.0.0.0'\n"
			if err := os.WriteFile(fakePath, []byte(script), 0755); err != nil {
				t.Fatalf("failed to create fake %s: %v", b.bin, err)
			}

			origPath := os.Getenv("PATH")
			os.Setenv("PATH", tmpDir)
			defer os.Setenv("PATH", origPath)

			got := detectChrome(false)
			if got.Type != BrowserChrome {
				t.Errorf("detectChrome() for %s = type %v, want BrowserChrome", b.name, got.Type)
			}
			if got.Name != b.name {
				t.Errorf("detectChrome() for %s = name %q, want %q", b.name, got.Name, b.name)
			}
			if got.Path != fakePath {
				t.Errorf("detectChrome() for %s = path %q, want %q", b.name, got.Path, fakePath)
			}
		})
	}
}
