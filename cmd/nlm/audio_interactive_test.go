package main

import (
	"bytes"
	"context"
	"errors"
	"io"
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
			args:   []string{"notebook-123", "--audio-id", "audio-789", "--transcript-only", "--no-mic", "--speaker", "Built-in Output", "--mic=USB Mic", "--timeout", "45m"},
			wantID: "notebook-123",
			wantOpts: interactiveAudioOptions{
				AudioID:        "audio-789",
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
		{
			name:   "mic app flag",
			args:   []string{"--mic-app", "notebook-789"},
			wantID: "notebook-789",
			wantOpts: interactiveAudioOptions{
				MicApp:  true,
				Timeout: 30 * time.Minute,
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
			if gotOpts.MicApp != tt.wantOpts.MicApp {
				t.Fatalf("MicApp = %v, want %v", gotOpts.MicApp, tt.wantOpts.MicApp)
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

func TestParseInteractiveAudioArgsRejectsMicAppConflicts(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "transcript only",
			args: []string{"notebook-123", "--mic-app", "--transcript-only"},
			want: "--mic-app cannot be used with --transcript-only",
		},
		{
			name: "no mic",
			args: []string{"notebook-123", "--mic-app", "--no-mic"},
			want: "--mic-app cannot be used with --no-mic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseInteractiveAudioArgs(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDescribeInteractiveAudioMicMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts interactiveAudioOptions
		want string
	}{
		{
			name: "transcript only",
			opts: interactiveAudioOptions{TranscriptOnly: true},
			want: "Mic: off. Transcript-only mode disables audio playback and microphone input.",
		},
		{
			name: "listen only",
			opts: interactiveAudioOptions{NoMic: true},
			want: "Mic: off. Running in listen-only mode. Rerun without --no-mic to enable the mic toggle.",
		},
		{
			name: "mic toggled in session",
			opts: interactiveAudioOptions{},
			want: "Mic: off by default. Press 'm' during the session to turn it on or off.",
		},
		{
			name: "mic app",
			opts: interactiveAudioOptions{MicApp: true},
			want: "Mic: off by default. Press 'm' in the terminal or use the mic window to turn it on or off.",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := describeInteractiveAudioMicMode(tt.opts); got != tt.want {
				t.Fatalf("describeInteractiveAudioMicMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunInteractiveAudioRefreshesPageStateBeforeStartingSession(t *testing.T) {
	origRefresh := refreshInteractiveAudioPageState
	origSignalerAuth := refreshInteractiveAudioSignalerAuth
	origListOverview := listInteractiveAudioOverviews
	origGetOverview := getInteractiveAudioOverview
	origRun := runInteractiveAudioSession
	origAuthToken := authToken
	origCookies := cookies
	origDebug := debug
	t.Cleanup(func() {
		refreshInteractiveAudioPageState = origRefresh
		refreshInteractiveAudioSignalerAuth = origSignalerAuth
		listInteractiveAudioOverviews = origListOverview
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
	listInteractiveAudioOverviews = func(_ *api.Client, notebookID string) ([]*api.AudioOverviewResult, error) {
		calls = append(calls, "list")
		if notebookID != "notebook-123" {
			t.Fatalf("notebookID = %q, want notebook-123", notebookID)
		}
		return []*api.AudioOverviewResult{{
			ProjectID: notebookID,
			AudioID:   "audio-123",
			Title:     "Ready audio",
			IsReady:   true,
		}}, nil
	}
	getInteractiveAudioOverview = func(_ *api.Client, notebookID string) (*api.AudioOverviewResult, error) {
		t.Fatal("getInteractiveAudioOverview should not be called when list returns a ready overview")
		return nil, nil
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
	if got, want := strings.Join(calls, ","), "refresh,list,signaler,run"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
}

func TestRunInteractiveAudioUsesAudioIDOverride(t *testing.T) {
	origRefresh := refreshInteractiveAudioPageState
	origSignalerAuth := refreshInteractiveAudioSignalerAuth
	origListOverview := listInteractiveAudioOverviews
	origGetOverview := getInteractiveAudioOverview
	origRun := runInteractiveAudioSession
	origAuthToken := authToken
	origCookies := cookies
	origDebug := debug
	t.Cleanup(func() {
		refreshInteractiveAudioPageState = origRefresh
		refreshInteractiveAudioSignalerAuth = origSignalerAuth
		listInteractiveAudioOverviews = origListOverview
		getInteractiveAudioOverview = origGetOverview
		runInteractiveAudioSession = origRun
		authToken = origAuthToken
		cookies = origCookies
		debug = origDebug
	})

	authToken = "token-a"
	cookies = "cookie-a"
	debug = false

	refreshInteractiveAudioPageState = func(bool) error { return nil }
	listInteractiveAudioOverviews = func(_ *api.Client, notebookID string) ([]*api.AudioOverviewResult, error) {
		return []*api.AudioOverviewResult{
			{ProjectID: notebookID, AudioID: "audio-123", Title: "Old", IsReady: true},
			{ProjectID: notebookID, AudioID: "audio-456", Title: "Chosen", IsReady: true},
		}, nil
	}
	getInteractiveAudioOverview = func(_ *api.Client, notebookID string) (*api.AudioOverviewResult, error) {
		t.Fatal("getInteractiveAudioOverview should not be called when override resolves from list")
		return nil, nil
	}
	refreshInteractiveAudioSignalerAuth = func(bool) (string, error) { return "", nil }
	runInteractiveAudioSession = func(_ context.Context, _, _, _ string, opts interactiveaudio.Options) error {
		if opts.AudioOverviewID != "audio-456" {
			t.Fatalf("AudioOverviewID = %q, want audio-456", opts.AudioOverviewID)
		}
		return nil
	}

	err := runInteractiveAudio(nil, "notebook-123", interactiveAudioOptions{
		AudioID:        "audio-456",
		TranscriptOnly: true,
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("runInteractiveAudio() error = %v", err)
	}
}

func TestRunInteractiveAudioUsesAudioIDOverrideWhenListIsEmpty(t *testing.T) {
	origRefresh := refreshInteractiveAudioPageState
	origSignalerAuth := refreshInteractiveAudioSignalerAuth
	origListOverview := listInteractiveAudioOverviews
	origGetOverview := getInteractiveAudioOverview
	origRun := runInteractiveAudioSession
	origAuthToken := authToken
	origCookies := cookies
	origDebug := debug
	t.Cleanup(func() {
		refreshInteractiveAudioPageState = origRefresh
		refreshInteractiveAudioSignalerAuth = origSignalerAuth
		listInteractiveAudioOverviews = origListOverview
		getInteractiveAudioOverview = origGetOverview
		runInteractiveAudioSession = origRun
		authToken = origAuthToken
		cookies = origCookies
		debug = origDebug
	})

	authToken = "token-a"
	cookies = "cookie-a"
	debug = false

	refreshInteractiveAudioPageState = func(bool) error { return nil }
	listInteractiveAudioOverviews = func(_ *api.Client, notebookID string) ([]*api.AudioOverviewResult, error) {
		return nil, nil
	}
	getInteractiveAudioOverview = func(_ *api.Client, notebookID string) (*api.AudioOverviewResult, error) {
		t.Fatal("getInteractiveAudioOverview should not be called when audio-id override is set")
		return nil, nil
	}
	refreshInteractiveAudioSignalerAuth = func(bool) (string, error) { return "", nil }
	runInteractiveAudioSession = func(_ context.Context, _, _, _ string, opts interactiveaudio.Options) error {
		if opts.AudioOverviewID != "audio-override" {
			t.Fatalf("AudioOverviewID = %q, want audio-override", opts.AudioOverviewID)
		}
		return nil
	}

	err := runInteractiveAudio(nil, "notebook-123", interactiveAudioOptions{
		AudioID:        "audio-override",
		TranscriptOnly: true,
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("runInteractiveAudio() error = %v", err)
	}
}

func TestRunInteractiveAudioTreatsCanceledSessionAsCleanExit(t *testing.T) {
	origRefresh := refreshInteractiveAudioPageState
	origSignalerAuth := refreshInteractiveAudioSignalerAuth
	origListOverview := listInteractiveAudioOverviews
	origGetOverview := getInteractiveAudioOverview
	origRun := runInteractiveAudioSession
	origAuthToken := authToken
	origCookies := cookies
	origDebug := debug
	t.Cleanup(func() {
		refreshInteractiveAudioPageState = origRefresh
		refreshInteractiveAudioSignalerAuth = origSignalerAuth
		listInteractiveAudioOverviews = origListOverview
		getInteractiveAudioOverview = origGetOverview
		runInteractiveAudioSession = origRun
		authToken = origAuthToken
		cookies = origCookies
		debug = origDebug
	})

	authToken = "token-a"
	cookies = "cookie-a"
	debug = false

	refreshInteractiveAudioPageState = func(bool) error { return nil }
	listInteractiveAudioOverviews = func(_ *api.Client, notebookID string) ([]*api.AudioOverviewResult, error) {
		return []*api.AudioOverviewResult{{
			ProjectID: notebookID,
			AudioID:   "audio-123",
			IsReady:   true,
		}}, nil
	}
	getInteractiveAudioOverview = func(_ *api.Client, notebookID string) (*api.AudioOverviewResult, error) {
		t.Fatal("getInteractiveAudioOverview should not be called when list returns a ready overview")
		return nil, nil
	}
	refreshInteractiveAudioSignalerAuth = func(bool) (string, error) { return "", nil }
	runInteractiveAudioSession = func(_ context.Context, _, _, _ string, _ interactiveaudio.Options) error {
		return context.Canceled
	}

	err := runInteractiveAudio(nil, "notebook-123", interactiveAudioOptions{
		TranscriptOnly: true,
		Timeout:        time.Second,
	})
	if err != nil {
		t.Fatalf("runInteractiveAudio() error = %v, want nil", err)
	}
}

func TestRunInteractiveAudioReportsMicMode(t *testing.T) {
	origRefresh := refreshInteractiveAudioPageState
	origSignalerAuth := refreshInteractiveAudioSignalerAuth
	origListOverview := listInteractiveAudioOverviews
	origGetOverview := getInteractiveAudioOverview
	origRun := runInteractiveAudioSession
	origStatusWriter := interactiveAudioStatusWriter
	origAuthToken := authToken
	origCookies := cookies
	origDebug := debug
	t.Cleanup(func() {
		refreshInteractiveAudioPageState = origRefresh
		refreshInteractiveAudioSignalerAuth = origSignalerAuth
		listInteractiveAudioOverviews = origListOverview
		getInteractiveAudioOverview = origGetOverview
		runInteractiveAudioSession = origRun
		interactiveAudioStatusWriter = origStatusWriter
		authToken = origAuthToken
		cookies = origCookies
		debug = origDebug
	})

	authToken = "token-a"
	cookies = "cookie-a"
	debug = false

	refreshInteractiveAudioPageState = func(bool) error { return nil }
	listInteractiveAudioOverviews = func(_ *api.Client, notebookID string) ([]*api.AudioOverviewResult, error) {
		return []*api.AudioOverviewResult{{
			ProjectID: notebookID,
			AudioID:   "audio-123",
			IsReady:   true,
		}}, nil
	}
	getInteractiveAudioOverview = func(_ *api.Client, notebookID string) (*api.AudioOverviewResult, error) {
		t.Fatal("getInteractiveAudioOverview should not be called when list returns a ready overview")
		return nil, nil
	}
	refreshInteractiveAudioSignalerAuth = func(bool) (string, error) { return "", nil }
	runInteractiveAudioSession = func(_ context.Context, _, _, _ string, _ interactiveaudio.Options) error {
		return context.Canceled
	}

	var status bytes.Buffer
	interactiveAudioStatusWriter = func() io.Writer { return &status }

	err := runInteractiveAudio(nil, "notebook-123", interactiveAudioOptions{
		NoMic:   true,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("runInteractiveAudio() error = %v, want nil", err)
	}
	if got := status.String(); !strings.Contains(got, "Mic: off. Running in listen-only mode.") {
		t.Fatalf("status = %q, want mic mode message", got)
	}
}

func TestRunInteractiveAudioReportsDefaultMicToggleMode(t *testing.T) {
	origRefresh := refreshInteractiveAudioPageState
	origSignalerAuth := refreshInteractiveAudioSignalerAuth
	origListOverview := listInteractiveAudioOverviews
	origGetOverview := getInteractiveAudioOverview
	origRun := runInteractiveAudioSession
	origStatusWriter := interactiveAudioStatusWriter
	origAuthToken := authToken
	origCookies := cookies
	origDebug := debug
	t.Cleanup(func() {
		refreshInteractiveAudioPageState = origRefresh
		refreshInteractiveAudioSignalerAuth = origSignalerAuth
		listInteractiveAudioOverviews = origListOverview
		getInteractiveAudioOverview = origGetOverview
		runInteractiveAudioSession = origRun
		interactiveAudioStatusWriter = origStatusWriter
		authToken = origAuthToken
		cookies = origCookies
		debug = origDebug
	})

	authToken = "token-a"
	cookies = "cookie-a"
	debug = false

	refreshInteractiveAudioPageState = func(bool) error { return nil }
	listInteractiveAudioOverviews = func(_ *api.Client, notebookID string) ([]*api.AudioOverviewResult, error) {
		return []*api.AudioOverviewResult{{
			ProjectID: notebookID,
			AudioID:   "audio-123",
			IsReady:   true,
		}}, nil
	}
	getInteractiveAudioOverview = func(_ *api.Client, notebookID string) (*api.AudioOverviewResult, error) {
		t.Fatal("getInteractiveAudioOverview should not be called when list returns a ready overview")
		return nil, nil
	}
	refreshInteractiveAudioSignalerAuth = func(bool) (string, error) { return "", nil }
	runInteractiveAudioSession = func(_ context.Context, _, _, _ string, _ interactiveaudio.Options) error {
		return context.Canceled
	}

	var status bytes.Buffer
	interactiveAudioStatusWriter = func() io.Writer { return &status }

	err := runInteractiveAudio(nil, "notebook-123", interactiveAudioOptions{
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("runInteractiveAudio() error = %v, want nil", err)
	}
	if got := status.String(); !strings.Contains(got, "Mic: off by default. Press 'm' during the session to turn it on or off.") {
		t.Fatalf("status = %q, want default mic toggle message", got)
	}
}
