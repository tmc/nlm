package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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

func appCreateCommandLocalFlags() map[string]bool {
	flags := selectorCommandLocalFlags()
	flags["type"] = true
	flags["instructions"] = true
	return flags
}

func audioCreateCommandLocalFlags() map[string]bool {
	return map[string]bool{
		"length": true, "language": true, "audio-type": true,
	}
}

func videoCreateCommandLocalFlags() map[string]bool {
	return map[string]bool{
		"style": true, "language": true, "audio-type": true,
	}
}

func slidesCreateCommandLocalFlags() map[string]bool {
	flags := selectorCommandLocalFlags()
	flags["format"] = true
	flags["f"] = true
	return flags
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

func validateAppCreateArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, _, err := parseAppCreateArgsWithOptions(args, globals)
	if err != nil {
		fmt.Fprintf(os.Stderr, "usage: nlm %s --type <prototype|mindmap|canvas> <notebook-id> [instructions]\n", cmdName)
		return errBadArgs
	}
	return nil
}

func parseAppCreateArgsWithOptions(args []string, globals globalOptions) (appCreateOptions, []string, error) {
	opts := appCreateOptions{Selectors: selectorOptionsFromGlobals(globals)}
	flags := flag.NewFlagSet("app-create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Type, "type", "", "")
	flags.StringVar(&opts.Instructions, "instructions", "", "")
	flags.StringVar(&opts.Selectors.SourceIDs, "source-ids", opts.Selectors.SourceIDs, "")
	flags.StringVar(&opts.Selectors.SourceMatch, "source-match", opts.Selectors.SourceMatch, "")
	flags.StringVar(&opts.Selectors.SourceExclude, "source-exclude", opts.Selectors.SourceExclude, "")
	flags.StringVar(&opts.Selectors.LabelIDs, "label-ids", opts.Selectors.LabelIDs, "")
	flags.StringVar(&opts.Selectors.LabelMatch, "label-match", opts.Selectors.LabelMatch, "")
	flags.StringVar(&opts.Selectors.LabelExclude, "label-exclude", opts.Selectors.LabelExclude, "")

	flagArgs, positional, err := splitCommandFlags(args, appCreateCommandLocalFlags(), nil)
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if opts.Type == "" {
		return opts, nil, fmt.Errorf("--type is required")
	}
	if len(positional) == 0 {
		return opts, nil, fmt.Errorf("missing notebook id")
	}
	if opts.Instructions == "" && len(positional) > 1 {
		opts.Instructions = strings.Join(positional[1:], " ")
	}
	if opts.Instructions == "" {
		return opts, nil, fmt.Errorf("missing instructions")
	}
	return opts, positional[:1], nil
}

func runAppCreateWithOptions(c *api.Client, args []string, globals globalOptions) error {
	opts, positional, err := parseAppCreateArgsWithOptions(args, globals)
	if err != nil {
		return err
	}
	kind, err := api.ParseAppArtifactKind(opts.Type)
	if err != nil {
		return err
	}
	notebookID := positional[0]
	var sourceIDs []string
	if !opts.Selectors.empty() {
		sourceIDs, err = resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "Creating %s app artifact for notebook %s...\n", kind.String(), notebookID)
	artifactID, err := c.CreateAppArtifact(context.Background(), notebookID, kind, opts.Instructions, sourceIDs)
	if err != nil {
		return err
	}
	fmt.Println(artifactID)
	fmt.Fprintf(os.Stderr, "Created %s app artifact. Use 'nlm artifact get %s' to check status.\n", kind.String(), artifactID)
	return nil
}

func runMindmapCreateWithOptions(c *api.Client, args []string, globals globalOptions) error {
	args = append([]string{"--type", "mindmap"}, args...)
	return runAppCreateWithOptions(c, args, globals)
}

func validateAudioCreateArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, _, err := parseAudioCreateArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <instructions>\n", cmdName)
		return errBadArgs
	}
	return nil
}

func parseAudioCreateArgs(args []string) (audioCreateOptions, []string, error) {
	opts := audioCreateOptions{Length: "default", Language: "en"}
	flags := flag.NewFlagSet("audio-create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Length, "length", opts.Length, "")
	flags.StringVar(&opts.Language, "language", opts.Language, "")
	flags.StringVar(&opts.AudioType, "audio-type", opts.AudioType, "")
	flagArgs, positional, err := splitCommandFlags(args, audioCreateCommandLocalFlags(), nil)
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) < 2 {
		return opts, nil, fmt.Errorf("missing notebook id or instructions")
	}
	return opts, positional, nil
}

func validateVideoCreateArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, _, err := parseVideoCreateArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <instructions>\n", cmdName)
		return errBadArgs
	}
	return nil
}

func parseVideoCreateArgs(args []string) (videoCreateOptions, []string, error) {
	opts := videoCreateOptions{Language: "en"}
	flags := flag.NewFlagSet("video-create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Style, "style", opts.Style, "")
	flags.StringVar(&opts.Language, "language", opts.Language, "")
	flags.StringVar(&opts.AudioType, "audio-type", opts.AudioType, "")
	flagArgs, positional, err := splitCommandFlags(args, videoCreateCommandLocalFlags(), nil)
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) < 2 {
		return opts, nil, fmt.Errorf("missing notebook id or instructions")
	}
	return opts, positional, nil
}

func validateSlidesCreateArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, _, err := parseSlidesCreateArgs(args, globals)
	if err != nil {
		fmt.Fprintf(os.Stderr, "usage: nlm %s [--format detailed|presenter] [selectors] <notebook-id> [instructions]\n", cmdName)
		return errBadArgs
	}
	return nil
}

// parseSlidesCreateArgs parses create-slides / deck create flags. It returns
// the create options and the positional args (notebook-id first, then any
// instruction words). Instructions are optional — a deck can be generated from
// sources alone.
func parseSlidesCreateArgs(args []string, globals globalOptions) (slidesCreateOptions, []string, error) {
	opts := slidesCreateOptions{Selectors: selectorOptionsFromGlobals(globals)}
	flags := flag.NewFlagSet("slides-create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Format, "format", opts.Format, "")
	flags.StringVar(&opts.Format, "f", opts.Format, "")
	flags.StringVar(&opts.Selectors.SourceIDs, "source-ids", opts.Selectors.SourceIDs, "")
	flags.StringVar(&opts.Selectors.SourceMatch, "source-match", opts.Selectors.SourceMatch, "")
	flags.StringVar(&opts.Selectors.SourceExclude, "source-exclude", opts.Selectors.SourceExclude, "")
	flags.StringVar(&opts.Selectors.LabelIDs, "label-ids", opts.Selectors.LabelIDs, "")
	flags.StringVar(&opts.Selectors.LabelMatch, "label-match", opts.Selectors.LabelMatch, "")
	flags.StringVar(&opts.Selectors.LabelExclude, "label-exclude", opts.Selectors.LabelExclude, "")

	flagArgs, positional, err := splitCommandFlags(args, slidesCreateCommandLocalFlags(), nil)
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	opts.DeckFormat, err = parseSlideDeckFormat(opts.Format)
	if err != nil {
		return opts, nil, err
	}
	if len(positional) == 0 {
		return opts, nil, fmt.Errorf("missing notebook id")
	}
	return opts, positional, nil
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

func runSlidesCreateWithOptions(c *api.Client, args []string, globals globalOptions) error {
	opts, positional, err := parseSlidesCreateArgs(args, globals)
	if err != nil {
		return err
	}
	notebookID := positional[0]
	instructions := strings.Join(positional[1:], " ")

	var sourceIDs []string
	if !opts.Selectors.empty() {
		sourceIDs, err = resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
		if err != nil {
			return err
		}
	}

	artifactID, err := c.CreateSlideDeckWithOptions(context.Background(), notebookID, instructions, sourceIDs, opts.DeckFormat)
	if err != nil {
		return err
	}
	fmt.Println(artifactID)
	fmt.Fprintf(os.Stderr, "Created slide deck. Use 'nlm artifact get %s' to check status.\n", artifactID)
	return nil
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
