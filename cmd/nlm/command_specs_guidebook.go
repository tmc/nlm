package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

type guidebookListArgs struct {
	JSON bool
}

type guidebookIDArgs struct {
	GuidebookID string
}

type guidebookAskArgs struct {
	GuidebookID string
	Question    string
}

func configureGuidebookCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpec(specs["guidebooks"], commandFormOf(), decodeGuidebookList)
	idForm := commandFormOf(requiredOperand("guidebook"))
	configureTypedCommandSpec(specs["guidebook"], idForm, decodeGuidebookGet)
	configureTypedCommandSpec(specs["guidebook-details"], idForm, decodeGuidebookDetails)
	configureTypedCommandSpec(specs["guidebook-publish"], idForm, decodeGuidebookPublish)
	configureTypedCommandSpec(specs["guidebook-share"], idForm, decodeGuidebookShare)
	configureTypedCommandSpec(specs["guidebook-ask"],
		commandFormOf(
			requiredOperand("guidebook"),
			repeatedOperand("question"),
		),
		decodeGuidebookAsk,
	)
	configureTypedCommandSpec(specs["guidebook-rm"], idForm, decodeGuidebookDelete)
}

func decodeGuidebookList(parsed parsedCommand) (commandCall, error) {
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return nil, err
	}
	args := guidebookListArgs{JSON: jsonOutput}
	return func(ctx context.Context, client *notebooklm.Client) error {
		guidebooks, err := client.ListGuidebooks(ctx)
		if err != nil {
			return err
		}
		if args.JSON {
			enc := json.NewEncoder(os.Stdout)
			for _, guidebook := range guidebooks {
				rec := guidebookListRecord{
					GuidebookID: guidebook.GetGuidebookId(),
					Title:       guidebook.GetTitle(),
					Status:      guidebook.GetStatus().String(),
				}
				if err := enc.Encode(rec); err != nil {
					return err
				}
			}
			return nil
		}
		out, flush := newListWriter(os.Stdout)
		fmt.Fprintln(out, "ID\tTITLE\tSTATUS")
		for _, guidebook := range guidebooks {
			fmt.Fprintf(out, "%s\t%s\t%s\n", guidebook.GetGuidebookId(), guidebook.GetTitle(), guidebook.GetStatus().String())
		}
		return flush()
	}, nil
}

func decodeGuidebookGet(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGuidebookID(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *notebooklm.Client) error {
		guidebook, err := client.GetGuidebook(ctx, args.GuidebookID)
		if err != nil {
			return err
		}
		fmt.Printf("Guidebook: %s\n", guidebook.GetTitle())
		fmt.Printf("ID: %s\n", guidebook.GetGuidebookId())
		fmt.Printf("Status: %s\n", guidebook.GetStatus().String())
		if content := guidebook.GetContent(); content != "" {
			fmt.Printf("\n%s\n", content)
		}
		return nil
	}, nil
}

func decodeGuidebookDetails(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGuidebookID(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *notebooklm.Client) error {
		details, err := client.GetGuidebookDetails(ctx, args.GuidebookID)
		if err != nil {
			return err
		}
		if guidebook := details.GetGuidebook(); guidebook != nil {
			fmt.Printf("Guidebook: %s\n", guidebook.GetTitle())
			fmt.Printf("ID: %s\n", guidebook.GetGuidebookId())
			fmt.Printf("Status: %s\n", guidebook.GetStatus().String())
		}
		if sections := details.GetSections(); len(sections) > 0 {
			fmt.Printf("\nSections (%d):\n", len(sections))
			for i, section := range sections {
				fmt.Printf("  %d. %s\n", i+1, section.GetTitle())
			}
		}
		if analytics := details.GetAnalytics(); analytics != nil {
			data, err := json.MarshalIndent(analytics, "", "  ")
			if err == nil {
				fmt.Printf("\nAnalytics:\n%s\n", string(data))
			}
		}
		return nil
	}, nil
}

func decodeGuidebookPublish(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGuidebookID(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *notebooklm.Client) error {
		_, err := client.PublishGuidebook(ctx, args.GuidebookID)
		if err == nil {
			fmt.Fprintln(os.Stderr, "Guidebook published.")
		}
		return err
	}, nil
}

func decodeGuidebookShare(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGuidebookID(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *notebooklm.Client) error {
		_, err := client.ShareGuidebook(ctx, args.GuidebookID)
		if err == nil {
			fmt.Fprintln(os.Stderr, "Guidebook shared.")
		}
		return err
	}, nil
}

func decodeGuidebookAsk(parsed parsedCommand) (commandCall, error) {
	guidebookID, err := parsedArgument(parsed, "guidebook")
	if err != nil {
		return nil, err
	}
	question, err := parsedArguments(parsed, "question")
	if err != nil {
		return nil, err
	}
	args := guidebookAskArgs{
		GuidebookID: guidebookID,
		Question:    strings.Join(question, " "),
	}
	return func(ctx context.Context, client *notebooklm.Client) error {
		response, err := client.GuidebookAsk(ctx, args.GuidebookID, args.Question)
		if err != nil {
			return err
		}
		fmt.Println(response.GetAnswer())
		return nil
	}, nil
}

func decodeGuidebookDelete(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGuidebookID(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *notebooklm.Client) error {
		err := client.DeleteGuidebook(ctx, args.GuidebookID)
		if err == nil {
			fmt.Fprintln(os.Stderr, "Guidebook deleted.")
		}
		return err
	}, nil
}

func decodeGuidebookID(parsed parsedCommand) (guidebookIDArgs, error) {
	guidebookID, err := parsedArgument(parsed, "guidebook")
	if err != nil {
		return guidebookIDArgs{}, err
	}
	return guidebookIDArgs{GuidebookID: guidebookID}, nil
}
