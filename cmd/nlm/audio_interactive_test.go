package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tmc/nlm/internal/interactiveaudio"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestParseInteractiveAudioArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantID   string
		wantOpts interactiveAudioOptions
	}{
		{
			name:   "id before flags",
			args:   []string{"notebook-123", "--transcript-only", "--no-mic", "--speaker", "Built-in Output", "--mic=USB Mic", "--timeout", "45m"},
			wantID: "notebook-123",
			wantOpts: interactiveAudioOptions{
				TranscriptOnly: true,
				NoMic:          true,
				Speaker:        "Built-in Output",
				Mic:            "USB Mic",
				Timeout:        45 * time.Minute,
			},
		},
		{
			name:   "flags before id",
			args:   []string{"--no-mic", "--timeout=15m", "notebook-456"},
			wantID: "notebook-456",
			wantOpts: interactiveAudioOptions{
				NoMic:   true,
				Timeout: 15 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotOpts, gotID, err := parseInteractiveAudioArgs(tt.args)
			if err != nil {
				t.Fatalf("parseInteractiveAudioArgs() error = %v", err)
			}
			if gotID != tt.wantID {
				t.Fatalf("notebook ID = %q, want %q", gotID, tt.wantID)
			}
			if gotOpts.TranscriptOnly != tt.wantOpts.TranscriptOnly {
				t.Fatalf("TranscriptOnly = %v, want %v", gotOpts.TranscriptOnly, tt.wantOpts.TranscriptOnly)
			}
			if gotOpts.NoMic != tt.wantOpts.NoMic {
				t.Fatalf("NoMic = %v, want %v", gotOpts.NoMic, tt.wantOpts.NoMic)
			}
			if gotOpts.Speaker != tt.wantOpts.Speaker {
				t.Fatalf("Speaker = %q, want %q", gotOpts.Speaker, tt.wantOpts.Speaker)
			}
			if gotOpts.Mic != tt.wantOpts.Mic {
				t.Fatalf("Mic = %q, want %q", gotOpts.Mic, tt.wantOpts.Mic)
			}
			if gotOpts.Timeout != tt.wantOpts.Timeout {
				t.Fatalf("Timeout = %v, want %v", gotOpts.Timeout, tt.wantOpts.Timeout)
			}
		})
	}
}

func TestParseInteractiveAudioArgsHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := parseInteractiveAudioArgs(args)
			if !errors.Is(err, errInteractiveAudioHelp) {
				t.Fatalf("error = %v, want errInteractiveAudioHelp", err)
			}
		})
	}
}

func TestParseInteractiveAudioArgsRejectsUnknownFlag(t *testing.T) {
	_, _, err := parseInteractiveAudioArgs([]string{"notebook-123", "--wat"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %v, want unknown flag error", err)
	}
}

func TestRunInteractiveAudioRefreshesPageStateBeforeStartingSession(t *testing.T) {
	origRefresh := refreshInteractiveAudioPageState
	origSignalerAuth := refreshInteractiveAudioSignalerAuth
	origGetOverview := getInteractiveAudioOverview
	origRun := runInteractiveAudioSession
	origAuthToken := authToken
	origCookies := cookies
	origDebug := debug
	t.Cleanup(func() {
		refreshInteractiveAudioPageState = origRefresh
		refreshInteractiveAudioSignalerAuth = origSignalerAuth
		getInteractiveAudioOverview = origGetOverview
		runInteractiveAudioSession = origRun
		authToken = origAuthToken
		cookies = origCookies
		debug = origDebug
	})

	authToken = "token-a"
	cookies = "cookie-a"
	debug = false

	var calls []string
	refreshInteractiveAudioPageState = func(bool) error {
		calls = append(calls, "refresh")
		return nil
	}
	getInteractiveAudioOverview = func(_ *api.Client, notebookID string) (*api.AudioOverviewResult, error) {
		calls = append(calls, "overview")
		if notebookID != "notebook-123" {
			t.Fatalf("notebookID = %q, want notebook-123", notebookID)
		}
		return &api.AudioOverviewResult{
			ProjectID: notebookID,
			AudioID:   "audio-123",
			IsReady:   true,
		}, nil
	}
	refreshInteractiveAudioSignalerAuth = func(bool) (string, error) {
		calls = append(calls, "signaler")
		return "Bearer signaler-token", nil
	}
	runInteractiveAudioSession = func(_ context.Context, _, _, _ string, opts interactiveaudio.Options) error {
		calls = append(calls, "run")
		if !opts.Config.TranscriptOnly {
			t.Fatalf("TranscriptOnly = false, want true")
		}
		if opts.AudioOverviewID != "audio-123" {
			t.Fatalf("AudioOverviewID = %q, want audio-123", opts.AudioOverviewID)
		}
		if opts.SignalerAuthorization != "Bearer signaler-token" {
			t.Fatalf("SignalerAuthorization = %q, want Bearer signaler-token", opts.SignalerAuthorization)
		}
		return nil
	}

	err := runInteractiveAudio(nil, "notebook-123", interactiveAudioOptions{
		TranscriptOnly: true,
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("runInteractiveAudio() error = %v", err)
	}
	if got, want := strings.Join(calls, ","), "refresh,overview,signaler,run"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
}
