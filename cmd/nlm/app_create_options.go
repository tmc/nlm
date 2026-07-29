package main

import (
	"fmt"
	"os"
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

func printAppCreateUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [instructions]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --type <type>            App type: prototype, mindmap, or canvas")
	fmt.Fprintln(os.Stderr, "  --instructions <text>    Generation instructions")
	printSelectorFlags()
}

func printAudioCreateUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <instructions>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --length <value>         Audio length: default, short, or long")
	fmt.Fprintln(os.Stderr, "  --language <code>        Language code (default en)")
	fmt.Fprintln(os.Stderr, "  --audio-type <value>     Audio style: deep-dive, brief, critique, or debate")
}

func printVideoCreateUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <instructions>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --style <value>          Video style: auto, classic, or whiteboard")
	fmt.Fprintln(os.Stderr, "  --language <code>        Language code (default en)")
	fmt.Fprintln(os.Stderr, "  --audio-type <value>     Content style: brief, deep-dive, critique, or debate")
}

func printSlidesCreateUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [instructions]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --format, -f <value>     Deck format: detailed (default) or presenter")
	fmt.Fprintln(os.Stderr, "                           presenter is experimental (wire values not yet HAR-verified)")
	printSelectorFlags()
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "When no source selector is given, every source in the notebook is used.")
}

func printSelectorFlags() {
	fmt.Fprintln(os.Stderr, "  --source-ids <ids>       Focus on these source IDs ('a,b,c' or '-' for stdin)")
	fmt.Fprintln(os.Stderr, "  --source-match <regex>   Focus on sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --source-exclude <regex> Exclude sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-ids <ids>        Include sources tagged with any of these label IDs")
	fmt.Fprintln(os.Stderr, "  --label-match <regex>    Include sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-exclude <regex>  Exclude sources tagged with any label whose name matches the regex")
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
