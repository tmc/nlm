package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tmc/nlm/internal/interactiveaudio"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

var errInteractiveAudioHelp = errors.New("interactive audio help shown")
var refreshInteractiveAudioPageState = refreshNotebookLMPageState
var refreshInteractiveAudioSignalerAuth = refreshNotebookLMSignalerAuthorization
var runInteractiveAudioSession = interactiveaudio.Run
var listInteractiveAudioOverviews = func(client *api.Client, notebookID string) ([]*api.AudioOverviewResult, error) {
	if client == nil {
		return nil, fmt.Errorf("interactive audio requires api client")
	}
	return client.ListAudioOverviews(notebookID)
}
var getInteractiveAudioOverview = func(client *api.Client, notebookID string) (*api.AudioOverviewResult, error) {
	if client == nil {
		return nil, fmt.Errorf("interactive audio requires api client")
	}
	return client.GetAudioOverview(notebookID)
}

type interactiveAudioOptions struct {
	AudioID        string
	TranscriptOnly bool
	NoMic          bool
	Speaker        string
	Mic            string
	Timeout        time.Duration
	Help           bool
}

func validateInteractiveAudioArgs(args []string) error {
	_, _, err := parseInteractiveAudioArgs(args)
	return err
}

func parseInteractiveAudioArgs(args []string) (interactiveAudioOptions, string, error) {
	const defaultTimeout = 30 * time.Minute

	opts := interactiveAudioOptions{Timeout: defaultTimeout}
	flags := flag.NewFlagSet("audio-interactive", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&opts.AudioID, "audio-id", "", "specific audio overview id to launch")
	flags.BoolVar(&opts.TranscriptOnly, "transcript-only", false, "skip audio playback, print transcript only")
	flags.BoolVar(&opts.NoMic, "no-mic", false, "listen-only mode (no microphone input)")
	flags.StringVar(&opts.Speaker, "speaker", "", "audio output device (default: system default)")
	flags.StringVar(&opts.Mic, "mic", "", "audio input device (default: system default)")
	flags.DurationVar(&opts.Timeout, "timeout", defaultTimeout, "session timeout")
	flags.BoolVar(&opts.Help, "help", false, "show help for audio-interactive")
	flags.BoolVar(&opts.Help, "h", false, "show help for audio-interactive")
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: nlm audio-interactive <notebook-id> [flags]\n\n")
		fmt.Fprintln(os.Stderr, "Flags:")
		fmt.Fprintln(os.Stderr, "  --audio-id <id>     Specific audio overview to launch")
		fmt.Fprintln(os.Stderr, "  --transcript-only   Skip audio playback, print transcript only")
		fmt.Fprintln(os.Stderr, "  --no-mic            Listen-only mode (no microphone input)")
		fmt.Fprintln(os.Stderr, "  --speaker <device>  Audio output device (default: system default)")
		fmt.Fprintln(os.Stderr, "  --mic <device>      Audio input device (default: system default)")
		fmt.Fprintln(os.Stderr, "  --timeout <dur>     Session timeout (default: 30m)")
		fmt.Fprintln(os.Stderr, "  --help              Show help for audio-interactive")
	}

	flagArgs, notebookID, err := splitInteractiveAudioArgs(args)
	if err != nil {
		flags.Usage()
		return opts, "", err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, "", err
	}
	if opts.Help {
		flags.Usage()
		return opts, "", errInteractiveAudioHelp
	}
	if notebookID == "" {
		flags.Usage()
		return opts, "", fmt.Errorf("missing notebook id")
	}
	return opts, notebookID, nil
}

func splitInteractiveAudioArgs(args []string) ([]string, string, error) {
	flagArgs := make([]string, 0, len(args))
	var notebookID string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			for j := i + 1; j < len(args); j++ {
				if notebookID == "" {
					notebookID = args[j]
					continue
				}
				return nil, "", fmt.Errorf("unexpected argument: %s", args[j])
			}
			break
		}

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			if notebookID == "" {
				notebookID = arg
				continue
			}
			return nil, "", fmt.Errorf("unexpected argument: %s", arg)
		}

		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "transcript-only", "no-mic", "help", "h":
			flagArgs = append(flagArgs, arg)
		case "audio-id", "speaker", "mic", "timeout":
			if hasValue {
				flagArgs = append(flagArgs, arg)
				continue
			}
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("flag needs an argument: %s", arg)
			}
			flagArgs = append(flagArgs, arg, args[i+1])
			i++
		default:
			return nil, "", fmt.Errorf("unknown flag: %s", arg)
		}

	}

	return flagArgs, notebookID, nil
}

func runInteractiveAudioCommand(client *api.Client, notebookID string, opts interactiveAudioOptions) error {
	return runInteractiveAudio(client, notebookID, opts)
}

func runInteractiveAudio(client *api.Client, notebookID string, opts interactiveAudioOptions) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	if err := refreshInteractiveAudioPageState(debug); err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "nlm: interactive audio page-state refresh failed: %v\n", err)
		}
	}

	audioOverview, source, err := resolveInteractiveAudioOverview(client, notebookID, opts)
	if err != nil {
		return fmt.Errorf("resolve audio overview: %w", err)
	}
	if audioOverview == nil || strings.TrimSpace(audioOverview.AudioID) == "" {
		return fmt.Errorf("interactive audio requires an audio overview")
	}
	if !audioOverview.IsReady {
		return fmt.Errorf("audio overview is not ready yet")
	}
	if debug {
		title := strings.TrimSpace(audioOverview.Title)
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(os.Stderr, "nlm: selected audio overview: id=%s title=%q ready=%v source=%s\n",
			audioOverview.AudioID, title, audioOverview.IsReady, source)
	}

	signalerAuthorization, signalerErr := refreshInteractiveAudioSignalerAuth(debug)
	if signalerErr != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "nlm: interactive audio signaler auth refresh failed: %v\n", signalerErr)
		}
	}

	err = runInteractiveAudioSession(ctx, authToken, cookies, notebookID, interactiveaudio.Options{
		Config: interactiveaudio.Config{
			TranscriptOnly: opts.TranscriptOnly,
			NoMic:          opts.NoMic,
			Speaker:        opts.Speaker,
			Mic:            opts.Mic,
		},
		AudioOverviewID:       audioOverview.AudioID,
		Debug:                 debug,
		Stdout:                os.Stdout,
		Stderr:                os.Stderr,
		TTY:                   isTerminal(os.Stdout),
		SignalerAuthorization: signalerAuthorization,
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func resolveInteractiveAudioOverview(client *api.Client, notebookID string, opts interactiveAudioOptions) (*api.AudioOverviewResult, string, error) {
	audioID := strings.TrimSpace(opts.AudioID)
	overviews, err := listInteractiveAudioOverviews(client, notebookID)
	if err == nil {
		if audioID != "" {
			for _, overview := range overviews {
				if overview != nil && strings.TrimSpace(overview.AudioID) == audioID {
					return overview, "audio-list override", nil
				}
			}
			if len(overviews) > 0 {
				return nil, "", fmt.Errorf("audio overview %q not found", audioID)
			}
		}
		if overview := firstReadyAudioOverview(overviews); overview != nil {
			return overview, "audio-list ready", nil
		}
		if len(overviews) > 0 && overviews[0] != nil {
			return overviews[0], "audio-list fallback", nil
		}
	} else if debug {
		fmt.Fprintf(os.Stderr, "nlm: list audio overviews failed: %v\n", err)
	}

	if audioID != "" {
		return &api.AudioOverviewResult{
			ProjectID: notebookID,
			AudioID:   audioID,
			IsReady:   true,
		}, "audio-id override", nil
	}

	overview, err := getInteractiveAudioOverview(client, notebookID)
	if err != nil {
		return nil, "", err
	}
	return overview, "get-audio-overview", nil
}

func firstReadyAudioOverview(overviews []*api.AudioOverviewResult) *api.AudioOverviewResult {
	for _, overview := range overviews {
		if overview != nil && strings.TrimSpace(overview.AudioID) != "" && overview.IsReady {
			return overview
		}
	}
	return nil
}
