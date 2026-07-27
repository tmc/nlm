package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

type flashcard struct {
	Front string `json:"f"`
	Back  string `json:"b"`
}

type flashcardTopics struct {
	Covered  []string `json:"covered,omitempty"`
	FollowUp []string `json:"followUp,omitempty"`
}

type flashcardData struct {
	Flashcards []flashcard     `json:"flashcards"`
	Topics     flashcardTopics `json:"topics,omitempty"`
}

type flashcardDeck struct {
	ArtifactID string
	Title      string
	HTML       string
	Data       flashcardData
}

type artifactExportOptions struct {
	Format string
	Output string
}

func printArtifactExportUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <artifact-id> [--format format] [--output file]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Exports a READY artifact using its server-rendered download.")
	fmt.Fprintln(os.Stderr, "Type-4 flashcard apps additionally support md, json, tsv, and html.")
	fmt.Fprintln(os.Stderr, "Output is written to stdout by default.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --format, -f <format>  Server file extension or flashcard format (default md)")
	fmt.Fprintln(os.Stderr, "  --output, -o <file>    Write to a file instead of stdout")
}

func validateArtifactExportArgsWithOptions(cmdName string, args []string, _ globalOptions) error {
	if _, _, err := parseArtifactExportArgs(args); err != nil {
		fmt.Fprintf(os.Stderr, "nlm: %s: %v\n\n", cmdName, err)
		printArtifactExportUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func parseArtifactExportArgs(args []string) (artifactExportOptions, string, error) {
	opts := artifactExportOptions{Format: "md"}
	flags := flag.NewFlagSet("artifact-export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Format, "format", opts.Format, "")
	flags.StringVar(&opts.Format, "f", opts.Format, "")
	flags.StringVar(&opts.Output, "output", "", "")
	flags.StringVar(&opts.Output, "o", "", "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"format": true,
		"f":      true,
		"output": true,
		"o":      true,
	}, nil)
	if err != nil {
		return opts, "", err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, "", err
	}
	if len(positional) != 1 {
		return opts, "", fmt.Errorf("requires exactly one artifact id")
	}
	opts.Format = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(opts.Format), "."))
	if opts.Format == "" {
		return opts, "", fmt.Errorf("format is empty")
	}
	for _, r := range opts.Format {
		if r < 'a' || r > 'z' {
			if r < '0' || r > '9' {
				return opts, "", fmt.Errorf("invalid format %q", opts.Format)
			}
		}
	}
	return opts, positional[0], nil
}

func runArtifactExport(c *api.Client, args []string) error {
	opts, artifactID, err := parseArtifactExportArgs(args)
	if err != nil {
		return err
	}
	artifact, err := c.GetArtifact(context.Background(), artifactID)
	if err != nil {
		return fmt.Errorf("get artifact: %w", err)
	}
	write, err := artifactExportWriter(c, artifact, opts.Format)
	if err != nil {
		return err
	}

	w := io.Writer(os.Stdout)
	var output *os.File
	if opts.Output != "" {
		output, err = os.Create(opts.Output)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		w = output
	}
	writeErr := write(w)
	if output != nil {
		if err := output.Close(); writeErr == nil {
			writeErr = err
		}
	}
	if writeErr != nil {
		return fmt.Errorf("write artifact: %w", writeErr)
	}
	return nil
}

type artifactFileReader interface {
	ReadArtifactFile(context.Context, string, string, io.Writer) error
}

func artifactExportWriter(c artifactFileReader, artifact *pb.Artifact, format string) (func(io.Writer) error, error) {
	if artifact == nil {
		return nil, fmt.Errorf("artifact is empty")
	}
	if artifact.GetType() == pb.ArtifactType_ARTIFACT_TYPE_REPORT &&
		artifact.GetTailoredReport().GetMindMapDataJson() != "" {
		deck, err := flashcardDeckFromArtifact(artifact)
		if err != nil {
			return nil, err
		}
		return func(w io.Writer) error {
			return writeFlashcardDeck(w, deck, format)
		}, nil
	}
	if artifact.GetType() == pb.ArtifactType_ARTIFACT_TYPE_9 {
		return nil, fmt.Errorf("native type-9 artifact export is not supported")
	}
	if artifact.GetState() != pb.ArtifactState_ARTIFACT_STATE_READY {
		return nil, fmt.Errorf("artifact %s is not READY (state %s)",
			artifact.GetArtifactId(), artifact.GetState())
	}
	return func(w io.Writer) error {
		return c.ReadArtifactFile(context.Background(), artifact.GetArtifactId(), format, w)
	}, nil
}

func flashcardDeckFromArtifact(artifact *pb.Artifact) (*flashcardDeck, error) {
	if artifact == nil {
		return nil, fmt.Errorf("flashcard artifact is empty")
	}
	if artifact.GetType() != pb.ArtifactType_ARTIFACT_TYPE_REPORT {
		return nil, fmt.Errorf("artifact %s is type %s, not a type-4 flashcard app",
			artifact.GetArtifactId(), artifactTypeName(artifact.GetType()))
	}
	app := artifact.GetTailoredReport()
	if app == nil || app.GetMindMapDataJson() == "" {
		return nil, fmt.Errorf("artifact %s has no flashcard app data", artifact.GetArtifactId())
	}

	var data flashcardData
	if err := json.Unmarshal([]byte(app.GetMindMapDataJson()), &data); err != nil {
		return nil, fmt.Errorf("decode flashcard app data: %w", err)
	}
	if len(data.Flashcards) == 0 {
		return nil, fmt.Errorf("artifact %s has no flashcards", artifact.GetArtifactId())
	}
	for i, card := range data.Flashcards {
		if strings.TrimSpace(card.Front) == "" {
			return nil, fmt.Errorf("flashcard %d has an empty front", i+1)
		}
		if strings.TrimSpace(card.Back) == "" {
			return nil, fmt.Errorf("flashcard %d has an empty back", i+1)
		}
	}
	return &flashcardDeck{
		ArtifactID: artifact.GetArtifactId(),
		Title:      artifact.GetTitle(),
		HTML:       app.GetLeadingText(),
		Data:       data,
	}, nil
}

func writeFlashcardDeck(w io.Writer, deck *flashcardDeck, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(deck.Data)
	case "tsv":
		records := csv.NewWriter(w)
		records.Comma = '\t'
		for _, card := range deck.Data.Flashcards {
			if err := records.Write([]string{card.Front, card.Back}); err != nil {
				return err
			}
		}
		records.Flush()
		return records.Error()
	case "html":
		if deck.HTML == "" {
			return fmt.Errorf("artifact %s has no HTML app", deck.ArtifactID)
		}
		_, err := io.WriteString(w, deck.HTML)
		return err
	case "md":
		title := strings.TrimSpace(strings.ReplaceAll(deck.Title, "\n", " "))
		if title == "" {
			title = "Flashcards"
		}
		if _, err := fmt.Fprintf(w, "# %s\n\n", title); err != nil {
			return err
		}
		for i, card := range deck.Data.Flashcards {
			if _, err := fmt.Fprintf(w, "## %d. %s\n\n%s\n\n", i+1, card.Front, card.Back); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}
