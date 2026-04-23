//go:build darwin

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
var interactiveAudioStatusWriter = func() io.Writer { return os.Stderr }
var prepareInteractiveAudioRuntime = prepareInteractiveAudioRuntimePlatform
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
	MicApp         bool
	Speaker        string
	Mic            string
	Timeout        time.Duration
	Help           bool
}

func validateInteractiveAudioArgs(args []string) error {
	_, _, err := parseInteractiveAudioArgs(args)
	return err
}

func printInteractiveAudioUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> [flags]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --audio-id <id>     Specific audio overview to launch")
	fmt.Fprintln(os.Stderr, "  --transcript-only   Print transcript only; audio playback and microphone stay off")
	fmt.Fprintln(os.Stderr, "  --no-mic            Disable microphone input entirely for this session")
	fmt.Fprintln(os.Stderr, "  --mic-app           Show a small AppKit mic controller window")
	fmt.Fprintln(os.Stderr, "  --speaker <device>  Audio output device (default: system default)")
	fmt.Fprintln(os.Stderr, "  --mic <device>      Audio input device (default: system default)")
	fmt.Fprintln(os.Stderr, "  --timeout <dur>     Session timeout (default: 30m)")
	fmt.Fprintln(os.Stderr, "  --help              Show help for audio-interactive")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  nlm audio-interactive <notebook-id>           Start with mic muted; press 'm' to toggle it")
	fmt.Fprintln(os.Stderr, "  nlm audio-interactive <notebook-id> --no-mic  Start in listen-only mode with no mic control")
	fmt.Fprintln(os.Stderr, "  nlm audio-interactive <notebook-id> --mic-app Start with the AppKit mic controller window")
}

func parseInteractiveAudioArgs(args []string) (interactiveAudioOptions, string, error) {
	const defaultTimeout = 30 * time.Minute

	opts := interactiveAudioOptions{Timeout: defaultTimeout}
	flags := flag.NewFlagSet("audio-interactive", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&opts.AudioID, "audio-id", "", "specific audio overview id to launch")
	flags.BoolVar(&opts.TranscriptOnly, "transcript-only", false, "skip audio playback, print transcript only")
	flags.BoolVar(&opts.NoMic, "no-mic", false, "listen-only mode (no microphone input)")
	flags.BoolVar(&opts.MicApp, "mic-app", false, "show a small AppKit mic controller window")
	flags.StringVar(&opts.Speaker, "speaker", "", "audio output device (default: system default)")
	flags.StringVar(&opts.Mic, "mic", "", "audio input device (default: system default)")
	flags.DurationVar(&opts.Timeout, "timeout", defaultTimeout, "session timeout")
	flags.BoolVar(&opts.Help, "help", false, "show help for audio-interactive")
	flags.BoolVar(&opts.Help, "h", false, "show help for audio-interactive")
	flags.Usage = func() { printInteractiveAudioUsage("audio-interactive") }

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
	if opts.MicApp && opts.TranscriptOnly {
		return opts, "", fmt.Errorf("--mic-app cannot be used with --transcript-only")
	}
	if opts.MicApp && opts.NoMic {
		return opts, "", fmt.Errorf("--mic-app cannot be used with --no-mic")
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
		case "transcript-only", "no-mic", "mic-app", "help", "h":
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
	if err := prepareInteractiveAudioRuntime(opts); err != nil {
		return err
	}
	if opts.MicApp {
		return runInteractiveAudioWithMicApp(client, notebookID, opts)
	}
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

	return runInteractiveAudioWithControllerContext(ctx, client, notebookID, opts, nil)
}

type interactiveAudioMicController interface {
	ToggleEvents() <-chan struct{}
	SetEnabled(bool)
	Close()
}

func runInteractiveAudioWithControllerContext(
	ctx context.Context,
	client *api.Client,
	notebookID string,
	opts interactiveAudioOptions,
	controller interactiveAudioMicController,
) error {
	if controller != nil {
		defer controller.Close()
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
	fmt.Fprintln(interactiveAudioStatusWriter(), describeInteractiveAudioMicMode(opts))

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
		MicToggle:             toggleEvents(controller),
		MicSetState:           setMicControllerState(controller),
		MicClose:              closeMicController(controller),
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func toggleEvents(controller interactiveAudioMicController) <-chan struct{} {
	if controller == nil {
		return nil
	}
	return controller.ToggleEvents()
}

func setMicControllerState(controller interactiveAudioMicController) func(bool) {
	if controller == nil {
		return nil
	}
	return controller.SetEnabled
}

func closeMicController(controller interactiveAudioMicController) func() {
	if controller == nil {
		return nil
	}
	return controller.Close
}

func describeInteractiveAudioMicMode(opts interactiveAudioOptions) string {
	switch {
	case opts.TranscriptOnly:
		return "Mic: off. Transcript-only mode disables audio playback and microphone input."
	case opts.NoMic:
		return "Mic: off. Running in listen-only mode. Rerun without --no-mic to enable the mic toggle."
	case opts.MicApp:
		return "Mic: off by default. Press 'm' in the terminal or use the mic window to turn it on or off."
	default:
		return "Mic: off by default. Press 'm' during the session to turn it on or off."
	}
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
