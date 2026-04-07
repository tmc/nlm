package interactiveaudio

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestRunValidatesOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts Options
		want string
	}{
		{
			name: "speaker selection not wired",
			opts: Options{
				Config: Config{
					TranscriptOnly: true,
					Speaker:        "Built-in Output",
				},
			},
			want: "speaker selection is not wired yet",
		},
		{
			name: "microphone selection not wired",
			opts: Options{
				Config: Config{
					TranscriptOnly: true,
					Mic:            "Built-in Microphone",
				},
			},
			want: "microphone selection is not wired yet",
		},
		{
			name: "local playback not wired",
			opts: Options{
				Config: Config{},
			},
			want: "local audio playback is not wired yet",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Run(context.Background(), "auth", "cookies", "notebook", Options{
				Config: tt.opts.Config,
				Stdout: io.Discard,
				Stderr: io.Discard,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRunRequiresAuthentication(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), "", "", "notebook", Options{
		Config: Config{TranscriptOnly: true},
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "requires authentication") {
		t.Fatalf("Run() error = %v, want authentication error", err)
	}
}

func TestRunRequiresNotebookID(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), "auth", "cookies", "", Options{
		Config: Config{TranscriptOnly: true},
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "missing notebook id") {
		t.Fatalf("Run() error = %v, want missing notebook id error", err)
	}
}
