package main

import (
	"errors"
	"testing"

	"github.com/tmc/nlm/internal/auth"
)

func TestShouldRetryAuthVisibly(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "headless authentication rejected", err: errors.New("redirected to authentication page - not logged in"), want: true},
		{name: "chrome launch failed", err: errors.New("failed to load page: chrome failed to start"), want: true},
		{name: "profile scan failed", err: errors.New("scan profiles: permission denied"), want: false},
		{name: "profile copy failed", err: errors.New("copy profile: permission denied"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryAuthVisibly(tt.err); got != tt.want {
				t.Fatalf("shouldRetryAuthVisibly(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestVisibleBrowserOption(t *testing.T) {
	opts := &auth.Options{}
	for _, apply := range visibleAuthRetryOptions(nil, 0) {
		apply(opts)
	}
	if !opts.VisibleBrowser {
		t.Fatal("visible auth retry did not enable a headed browser")
	}
	if opts.KeepOpenSeconds != 30 {
		t.Fatalf("visible auth retry keep-open = %d, want 30", opts.KeepOpenSeconds)
	}

	opts = &auth.Options{}
	for _, apply := range visibleAuthRetryOptions([]auth.Option{auth.WithKeepOpenSeconds(12)}, 12) {
		apply(opts)
	}
	if opts.KeepOpenSeconds != 12 {
		t.Fatalf("visible auth retry replaced explicit keep-open with %d", opts.KeepOpenSeconds)
	}
}
