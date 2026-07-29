package main

import (
	"fmt"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

type appCreateOptions struct {
	Type         string
	Instructions string
	Selectors    selectorOptions
}

type audioCreateOptions struct {
	Length    string
	Language  string
	AudioType string
	Yes       bool
}

type videoCreateOptions struct {
	Style     string
	Language  string
	AudioType string
}

type slidesCreateOptions struct {
	Format     string
	DeckFormat api.SlideDeckFormat
	Selectors  selectorOptions
}

func parseSlideDeckFormat(s string) (api.SlideDeckFormat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "detailed", "detail", "handout":
		return api.SlideDeckFormatDetailed, nil
	case "presenter", "present", "sparse":
		return api.SlideDeckFormatPresenter, nil
	default:
		return 0, fmt.Errorf("unknown slide deck format %q (want detailed or presenter)", s)
	}
}

func parseAudioLength(s string) (pb.AudioLength, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "default":
		return pb.AudioLength_AUDIO_LENGTH_DEFAULT, nil
	case "short", "shorter":
		return pb.AudioLength_AUDIO_LENGTH_SHORT, nil
	case "long", "longer":
		return pb.AudioLength_AUDIO_LENGTH_LONG, nil
	default:
		return 0, fmt.Errorf("unknown audio length %q", s)
	}
}

func parseAudioType(s string, def pb.AudioType) (pb.AudioType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return def, nil
	case "deep-dive", "deep_dive", "deep":
		return pb.AudioType_AUDIO_TYPE_DEEP_DIVE, nil
	case "brief":
		return pb.AudioType_AUDIO_TYPE_BRIEF, nil
	case "critique":
		return pb.AudioType_AUDIO_TYPE_CRITIQUE, nil
	case "debate":
		return pb.AudioType_AUDIO_TYPE_DEBATE, nil
	default:
		return 0, fmt.Errorf("unknown audio type %q", s)
	}
}

func parseVideoStyle(s string) (pb.VideoStyle, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto", "autoselect", "auto-select":
		return pb.VideoStyle_VIDEO_STYLE_AUTOSELECT, nil
	case "classic":
		return pb.VideoStyle_VIDEO_STYLE_CLASSIC, nil
	case "whiteboard", "white-board":
		return pb.VideoStyle_VIDEO_STYLE_WHITEBOARD, nil
	default:
		return 0, fmt.Errorf("unknown video style %q", s)
	}
}
