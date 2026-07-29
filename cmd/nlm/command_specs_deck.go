package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tmc/nlm/notebooklm"
)

type deckDownloadArgs struct {
	NotebookID string
	Options    deckDownloadOptions
}

func configureDeckCommandSpecs(specs map[commandID]*commandSpec) {
	for _, id := range []commandID{"deck-download", "download slide-deck"} {
		spec := specs[id]
		spec.Flags = []flagSpec{
			{Name: "id", Aliases: []string{"artifact-id"}, Value: "artifact-id", Description: "artifact ID"},
			{Name: "format", Aliases: []string{"f"}, Value: "format", Description: "export format"},
			{Name: "output", Aliases: []string{"o"}, Value: "file", Description: "output file"},
		}
		configureTypedCommandSpecWithErrorUsage(spec,
			[]commandForm{{
				Parts: []operandSpec{withUsage(remainingOperand("notebooks"), "<notebook-id>")},
				Constraints: []constraint{
					constraintFunc(validateDeckDownloadCommand),
				},
			}},
			decodeDeckDownload,
			func(path string, err error) {
				fmt.Fprintf(os.Stderr, "nlm: %s: %v\n\n", path, err)
				printCommandHelpForPath(path)
			},
		)
	}
}

func validateDeckDownloadCommand(parsed parsedCommand) error {
	_, err := decodeDeckDownloadArgs(parsed)
	return err
}

func decodeDeckDownload(parsed parsedCommand) (commandCall, error) {
	args, err := decodeDeckDownloadArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return runDeckDownload(client, args)
	}, nil
}

func decodeDeckDownloadArgs(parsed parsedCommand) (deckDownloadArgs, error) {
	notebooks := parsed.Args["notebooks"]
	if len(notebooks) != 1 {
		return deckDownloadArgs{}, fmt.Errorf("requires exactly one notebook id")
	}
	opts := deckDownloadOptions{
		ArtifactID: parsedStringFlag(parsed, "id", ""),
		Format:     parsedStringFlag(parsed, "format", "pdf"),
		Output:     parsedStringFlag(parsed, "output", ""),
	}
	if opts.ArtifactID == "" {
		return deckDownloadArgs{}, fmt.Errorf("missing --id <artifact-id>")
	}
	switch opts.Format {
	case "pdf", "pptx":
	default:
		return deckDownloadArgs{}, fmt.Errorf("unsupported format %q (want pdf or pptx)", opts.Format)
	}
	return deckDownloadArgs{NotebookID: notebooks[0], Options: opts}, nil
}
