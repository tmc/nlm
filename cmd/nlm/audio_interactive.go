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

type interactiveAudioOptions struct {
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
		case "speaker", "mic", "timeout":
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
	_ = client

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	err := interactiveaudio.Run(ctx, authToken, cookies, notebookID, interactiveaudio.Options{
		Config: interactiveaudio.Config{
			TranscriptOnly: opts.TranscriptOnly,
			NoMic:          opts.NoMic,
			Speaker:        opts.Speaker,
			Mic:            opts.Mic,
		},
		Debug:  debug,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		TTY:    isTerminal(os.Stdout),
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}
