package main

import (
	"context"
	"fmt"

	"github.com/tmc/nlm/notebooklm"
)

type generationNotebookArgs struct {
	NotebookID string
	JSON       bool
}

func configureGenerationCommandSpecs(specs map[commandID]*commandSpec) {
	notebookForm := commandFormOf(requiredOperand("notebook"))
	configureTypedCommandSpec(specs["generate-guide"], notebookForm, decodeGenerateGuide)
	configureTypedCommandSpec(specs["report-suggestions"], notebookForm, decodeReportSuggestions)
	configureTypedCommandSpec(specs["audio-suggestions"], notebookForm, decodeAudioSuggestions)
}

func decodeGenerateGuide(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGenerationNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return generateNotebookGuide(client, args.NotebookID)
	}, nil
}

func decodeReportSuggestions(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGenerationNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *notebooklm.Client) error {
		response, err := client.GenerateReportSuggestions(ctx, args.NotebookID)
		if err != nil {
			return err
		}
		for i, suggestion := range response.GetSuggestions() {
			if i > 0 {
				fmt.Println()
			}
			fmt.Println(suggestion.GetTitle())
			if suggestion.GetDescription() != "" {
				fmt.Printf("  %s\n", suggestion.GetDescription())
			}
			if suggestion.GetPrompt() != "" {
				fmt.Printf("  Prompt: %s\n", suggestion.GetPrompt())
			}
		}
		return nil
	}, nil
}

func decodeAudioSuggestions(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGenerationNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return audioSuggestions(client, args.NotebookID, args.JSON)
	}, nil
}

func decodeGenerationNotebook(parsed parsedCommand) (generationNotebookArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return generationNotebookArgs{}, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return generationNotebookArgs{}, err
	}
	return generationNotebookArgs{NotebookID: notebookID, JSON: jsonOutput}, nil
}
