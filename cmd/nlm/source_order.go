package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

var errSourceRelationNotFound = errors.New("neither argument order identifies a source in a notebook")

type sourceMembershipClient interface {
	GetProject(context.Context, string) (*api.Notebook, error)
}

type sourceCommandTarget struct {
	Path       string
	SourceID   string
	NotebookID string
	Resolve    bool
	Grace      bool
}

type sourceCommandResolution struct {
	SourceID   string
	NotebookID string
	Member     *pb.Source
	Reversed   bool
}

func decodeSourceCommandTarget(parsed parsedCommand, stablePath string) (sourceCommandTarget, error) {
	sourceID, err := parsedArgument(parsed, "source")
	if err != nil {
		return sourceCommandTarget{}, err
	}
	notebookID, notebookSet, err := parsedOptionalArgument(parsed, "notebook")
	if err != nil {
		return sourceCommandTarget{}, err
	}
	target := sourceCommandTarget{
		Path:       parsed.path,
		SourceID:   sourceID,
		NotebookID: notebookID,
	}
	if parsed.path == stablePath {
		target.Resolve = notebookSet
		target.Grace = !notebookSet
	}
	return target, nil
}

func runSourceCommand(
	ctx context.Context,
	client sourceMembershipClient,
	stderr io.Writer,
	target sourceCommandTarget,
	run func(sourceCommandResolution) error,
) error {
	if target.Grace {
		warnSingleSourceArgument(stderr, target)
		return run(sourceCommandResolution{SourceID: target.SourceID})
	}
	if !target.Resolve {
		return run(sourceCommandResolution{
			SourceID:   target.SourceID,
			NotebookID: target.NotebookID,
		})
	}

	resolved, err := resolveSourceCommand(ctx, client, target.NotebookID, target.SourceID)
	if err != nil {
		if errors.Is(err, errSourceRelationNotFound) {
			printCommandUsage(stderr, target.Path)
			return fmt.Errorf("%w: %v", errBadArgs, err)
		}
		return err
	}
	if resolved.Reversed {
		warnReversedSourceArguments(stderr, target)
	}
	return run(resolved)
}

func resolveSourceCommand(
	ctx context.Context,
	client sourceMembershipClient,
	first, second string,
) (sourceCommandResolution, error) {
	member, found, err := sourceNotebookRelation(ctx, client, first, second)
	if err != nil {
		return sourceCommandResolution{}, err
	}
	if found {
		// Notebook and source IDs occupy disjoint namespaces. A valid
		// documented relation cannot also be a valid reversed relation, so
		// do not add a second lookup to the common path.
		return sourceCommandResolution{
			NotebookID: first,
			SourceID:   second,
			Member:     member,
		}, nil
	}

	member, found, err = sourceNotebookRelation(ctx, client, second, first)
	if err != nil {
		return sourceCommandResolution{}, err
	}
	if !found {
		return sourceCommandResolution{}, errSourceRelationNotFound
	}
	return sourceCommandResolution{
		NotebookID: second,
		SourceID:   first,
		Member:     member,
		Reversed:   true,
	}, nil
}

func sourceNotebookRelation(
	ctx context.Context,
	client sourceMembershipClient,
	notebookID, sourceID string,
) (*pb.Source, bool, error) {
	notebook, err := client.GetProject(ctx, notebookID)
	if err != nil {
		if notebookLookupNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("list sources in notebook %s: %w", notebookID, err)
	}
	for _, source := range notebook.GetSources() {
		if source.GetSourceId().GetSourceId() == sourceID {
			return source, true, nil
		}
	}
	return nil, false, nil
}

func notebookLookupNotFound(err error) bool {
	// Only a structured not-found response permits trying the reversed
	// relation. Authentication, permission, transport, and server failures
	// remain hard errors.
	var apiErr *batchexecute.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.ErrorCode != nil {
		if apiErr.ErrorCode.Type == batchexecute.ErrorTypeNotFound {
			return true
		}
	}
	return apiErr.HTTPStatus == http.StatusNotFound
}

func warnSingleSourceArgument(w io.Writer, target sourceCommandTarget) {
	legacy := "read-source"
	if target.Path == "source check" {
		legacy = "check-source"
	}
	fmt.Fprintf(w, "nlm: '%s %s' is deprecated; use '%s %s'\n",
		target.Path, target.SourceID, legacy, target.SourceID)
}

func warnReversedSourceArguments(w io.Writer, target sourceCommandTarget) {
	fmt.Fprintf(w, "nlm: '%s %s %s' uses deprecated SOURCE NOTEBOOK order; use '%s %s %s'\n",
		target.Path, target.NotebookID, target.SourceID,
		target.Path, target.SourceID, target.NotebookID)
}

func printCommandUsage(w io.Writer, path string) {
	cmd, ok := lookupCommand(path)
	if !ok {
		panic("missing command usage path: " + path)
	}
	fmt.Fprintf(w, "usage: nlm %s %s\n", path, cmd.argsUsage)
}
