package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

type appCreateArgs struct {
	NotebookID string
	Options    appCreateOptions
}

type audioCreateArgs struct {
	NotebookID   string
	Instructions string
	Options      audioCreateOptions
}

type videoCreateArgs struct {
	NotebookID   string
	Instructions string
	Options      videoCreateOptions
}

type slidesCreateArgs struct {
	NotebookID   string
	Instructions string
	Options      slidesCreateOptions
}

func configureCreateCommandSpecs(specs map[commandID]*commandSpec) {
	appFlags := append(selectorFlagSpecs(),
		flagSpec{Name: "type", Value: "type", Description: "app type"},
		flagSpec{Name: "instructions", Value: "text", Description: "generation instructions"},
	)
	appForm := []commandForm{{
		Parts: []operandSpec{withUsage(remainingOperand("positionals"), "<notebook-id> <instructions...>")},
		Constraints: []constraint{
			constraintFunc(func(parsed parsedCommand) error {
				_, err := decodeAppCreateArgs(parsed, "")
				return err
			}),
		},
	}}
	appSpec := specs["app-create"]
	appSpec.Flags = appFlags
	configureTypedCommandSpecWithUsage(appSpec, appForm, decodeAppCreate, printAppCreateErrorUsage)

	mindmapSpec := specs["mindmap-create"]
	mindmapSpec.Flags = append([]flagSpec(nil), appFlags...)
	mindmapForm := []commandForm{{
		Parts: []operandSpec{withUsage(remainingOperand("positionals"), "<notebook-id> <instructions...>")},
		Constraints: []constraint{
			constraintFunc(func(parsed parsedCommand) error {
				_, err := decodeAppCreateArgs(parsed, "mindmap")
				return err
			}),
		},
	}}
	configureTypedCommandSpecWithUsage(mindmapSpec, mindmapForm, decodeMindmapCreate, printAppCreateErrorUsage)

	audioSpec := specs["create-audio"]
	audioSpec.Flags = []flagSpec{
		{Name: "length", Value: "value", Description: "audio length"},
		{Name: "language", Value: "code", Description: "language code"},
		{Name: "audio-type", Value: "value", Description: "audio style"},
	}
	audioForm := []commandForm{{
		Parts: []operandSpec{withUsage(remainingOperand("positionals"), "<notebook-id> <instructions...>")},
		Constraints: []constraint{
			constraintFunc(func(parsed parsedCommand) error {
				_, err := decodeAudioCreateArgs(parsed)
				return err
			}),
		},
	}}
	configureTypedCommandSpecWithUsage(audioSpec, audioForm, decodeAudioCreate, printCreateMediaErrorUsage)

	videoSpec := specs["create-video"]
	videoSpec.Flags = []flagSpec{
		{Name: "style", Value: "value", Description: "video style"},
		{Name: "language", Value: "code", Description: "language code"},
		{Name: "audio-type", Value: "value", Description: "content style"},
	}
	videoForm := []commandForm{{
		Parts: []operandSpec{withUsage(remainingOperand("positionals"), "<notebook-id> <instructions...>")},
		Constraints: []constraint{
			constraintFunc(func(parsed parsedCommand) error {
				_, err := decodeVideoCreateArgs(parsed)
				return err
			}),
		},
	}}
	configureTypedCommandSpecWithUsage(videoSpec, videoForm, decodeVideoCreate, printCreateMediaErrorUsage)

	slidesSpec := specs["create-slides"]
	slidesSpec.Flags = append(selectorFlagSpecs(),
		flagSpec{Name: "format", Aliases: []string{"f"}, Value: "value", Description: "deck format"},
	)
	slidesForm := []commandForm{{
		Parts: []operandSpec{withUsage(remainingOperand("positionals"), "<notebook-id> [instructions...]")},
		Constraints: []constraint{
			constraintFunc(func(parsed parsedCommand) error {
				_, err := decodeSlidesCreateArgs(parsed)
				return err
			}),
		},
	}}
	configureTypedCommandSpecWithUsage(slidesSpec, slidesForm, decodeSlidesCreate, printSlidesCreateErrorUsage)
}

func printAppCreateErrorUsage(path string) {
	printCommandUsageForPath(path)
}

func printCreateMediaErrorUsage(path string) {
	printCommandUsageForPath(path)
}

func printSlidesCreateErrorUsage(path string) {
	printCommandUsageForPath(path)
}

func decodeAppCreate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeAppCreateArgs(parsed, "")
	if err != nil {
		return nil, err
	}
	return appCreateCall(args), nil
}

func decodeMindmapCreate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeAppCreateArgs(parsed, "mindmap")
	if err != nil {
		return nil, err
	}
	return appCreateCall(args), nil
}

func decodeAppCreateArgs(parsed parsedCommand, defaultType string) (appCreateArgs, error) {
	positionals := parsed.Args["positionals"]
	opts := appCreateOptions{
		Type:         parsedStringFlag(parsed, "type", defaultType),
		Instructions: parsedStringFlag(parsed, "instructions", ""),
		Selectors:    decodeSelectorOptions(parsed),
	}
	if opts.Type == "" {
		return appCreateArgs{}, fmt.Errorf("--type is required")
	}
	if len(positionals) == 0 {
		return appCreateArgs{}, fmt.Errorf("missing notebook id")
	}
	if opts.Instructions == "" && len(positionals) > 1 {
		opts.Instructions = strings.Join(positionals[1:], " ")
	}
	if opts.Instructions == "" {
		return appCreateArgs{}, fmt.Errorf("missing instructions")
	}
	return appCreateArgs{NotebookID: positionals[0], Options: opts}, nil
}

func appCreateCall(args appCreateArgs) commandCall {
	return func(_ context.Context, client *notebooklm.Client) error {
		kind, err := notebooklm.ParseAppArtifactKind(args.Options.Type)
		if err != nil {
			return err
		}
		var sourceIDs []string
		if !args.Options.Selectors.empty() {
			sourceIDs, err = resolveSourceSelectorsWithOptions(client, args.NotebookID, args.Options.Selectors)
			if err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "Creating %s app artifact for notebook %s...\n", kind.String(), args.NotebookID)
		artifactID, err := client.CreateAppArtifact(
			context.Background(),
			args.NotebookID,
			kind,
			args.Options.Instructions,
			sourceIDs,
		)
		if err != nil {
			return err
		}
		fmt.Println(artifactID)
		fmt.Fprintf(os.Stderr, "Created %s app artifact. Use 'nlm artifact get %s' to check status.\n", kind.String(), artifactID)
		return nil
	}
}

func decodeAudioCreate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeAudioCreateArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return createAudioOverviewWithOptions(client, args.NotebookID, args.Instructions, args.Options)
	}, nil
}

func decodeAudioCreateArgs(parsed parsedCommand) (audioCreateArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) < 2 {
		return audioCreateArgs{}, fmt.Errorf("missing notebook id or instructions")
	}
	yes, err := parsedBoolFlag(parsed, "yes", parsed.globals.yes)
	if err != nil {
		return audioCreateArgs{}, err
	}
	return audioCreateArgs{
		NotebookID:   positionals[0],
		Instructions: strings.Join(positionals[1:], " "),
		Options: audioCreateOptions{
			Length:    parsedStringFlag(parsed, "length", "default"),
			Language:  parsedStringFlag(parsed, "language", "en"),
			AudioType: parsedStringFlag(parsed, "audio-type", ""),
			Yes:       yes,
		},
	}, nil
}

func decodeVideoCreate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeVideoCreateArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return createVideoOverviewWithOptions(client, args.NotebookID, args.Instructions, args.Options)
	}, nil
}

func decodeVideoCreateArgs(parsed parsedCommand) (videoCreateArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) < 2 {
		return videoCreateArgs{}, fmt.Errorf("missing notebook id or instructions")
	}
	return videoCreateArgs{
		NotebookID:   positionals[0],
		Instructions: strings.Join(positionals[1:], " "),
		Options: videoCreateOptions{
			Style:     parsedStringFlag(parsed, "style", ""),
			Language:  parsedStringFlag(parsed, "language", "en"),
			AudioType: parsedStringFlag(parsed, "audio-type", ""),
		},
	}, nil
}

func decodeSlidesCreate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeSlidesCreateArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		var sourceIDs []string
		var err error
		if !args.Options.Selectors.empty() {
			sourceIDs, err = resolveSourceSelectorsWithOptions(client, args.NotebookID, args.Options.Selectors)
			if err != nil {
				return err
			}
		}
		artifactID, err := client.CreateSlideDeckWithOptions(
			context.Background(),
			args.NotebookID,
			args.Instructions,
			sourceIDs,
			args.Options.DeckFormat,
		)
		if err != nil {
			return err
		}
		fmt.Println(artifactID)
		fmt.Fprintf(os.Stderr, "Created slide deck. Use 'nlm artifact get %s' to check status.\n", artifactID)
		return nil
	}, nil
}

func decodeSlidesCreateArgs(parsed parsedCommand) (slidesCreateArgs, error) {
	format := parsedStringFlag(parsed, "format", "")
	deckFormat, err := parseSlideDeckFormat(format)
	if err != nil {
		return slidesCreateArgs{}, err
	}
	positionals := parsed.Args["positionals"]
	if len(positionals) == 0 {
		return slidesCreateArgs{}, fmt.Errorf("missing notebook id")
	}
	return slidesCreateArgs{
		NotebookID:   positionals[0],
		Instructions: strings.Join(positionals[1:], " "),
		Options: slidesCreateOptions{
			Format:     format,
			DeckFormat: deckFormat,
			Selectors:  decodeSelectorOptions(parsed),
		},
	}, nil
}
