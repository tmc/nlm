package main

import "fmt"

type commandClientOptions struct {
	Chunked     bool
	DirectRPC   bool
	SkipSources bool
}

var jsonFlag = flagSpec{
	Name:        "json",
	Description: "emit JSON",
}

var yesFlag = flagSpec{
	Name:        "yes",
	Aliases:     []string{"y"},
	Description: "skip confirmation prompts",
}

var chunkedFlag = flagSpec{
	Name:        "chunked",
	Description: "use chunked response format",
	Visibility:  flagHidden,
}

var directRPCFlag = flagSpec{
	Name:        "direct-rpc",
	Description: "use direct RPC calls",
	Visibility:  flagHidden,
}

var skipSourcesFlag = flagSpec{
	Name:        "skip-sources",
	Description: "skip fetching sources for chat",
	Visibility:  flagHidden,
}

func configureCommandFlagOwnership(specs map[commandID]*commandSpec) {
	addOwnedFlag(specs, jsonFlag,
		"sources",
		"notes",
		"label-list",
		"label-generate",
		"label-create",
		"label-unlabeled",
		"label-relabel-all",
		"artifacts",
		"guidebooks",
		"audio-suggestions",
		"chat-list",
		"account",
		"audio-list",
		"analytics",
		"list-featured",
		"source-guide",
		"discover-sources",
		"betool",
	)
	addOwnedFlag(specs, yesFlag,
		"rm",
		"rm-source",
		"rm-note",
		"audio-rm",
		"create-audio",
		"delete-artifact",
		"delete-chat",
		"chat",
	)

	// Source read retains its old boolean format switches for one release.
	// They are command-owned compatibility aliases, not global output modes.
	addOwnedFlag(specs, flagSpec{
		Name:        "json",
		Description: "deprecated alias for --format=json",
		Visibility:  flagDeprecated,
	}, "read-source")
	addOwnedFlag(specs, flagSpec{
		Name:        "markdown",
		Description: "deprecated alias for --format=markdown",
		Visibility:  flagDeprecated,
	}, "read-source")
	addOwnedFlag(specs, flagSpec{
		Name:        "html",
		Description: "deprecated alias for --format=html",
		Visibility:  flagDeprecated,
	}, "read-source")
	addOwnedFlag(specs, flagSpec{
		Name:        "profile",
		Value:       "name",
		Description: "Chrome profile to use for image extraction",
		Visibility:  flagHidden,
	}, "read-source")
	addOwnedFlag(specs, flagSpec{
		Name:        "cdp-url",
		Value:       "url",
		Description: "remote CDP WebSocket URL",
		Visibility:  flagHidden,
	}, "read-artifact")
	addOwnedFlag(specs, flagSpec{
		Name:        "force",
		Description: "refresh cached source guides",
	}, "source-guide")

	for _, spec := range specs {
		if spec.noClient {
			continue
		}
		addFlagToSpec(spec, chunkedFlag)
	}
	addOwnedFlag(specs, chunkedFlag, "chat-list", "chat-show")
	addOwnedFlag(specs, directRPCFlag,
		"create-audio",
		"create-video",
		"audio-list",
		"audio-get",
		"audio-download",
		"audio-rm",
		"audio-share",
	)
	addOwnedFlag(specs, skipSourcesFlag,
		"chat",
		"generate-chat",
		"generate-report",
		"discover-sources",
	)
}

func addOwnedFlag(specs map[commandID]*commandSpec, flag flagSpec, ids ...commandID) {
	for _, id := range ids {
		spec := specs[id]
		if spec == nil {
			panic("missing command flag owner: " + id)
		}
		addFlagToSpec(spec, flag)
	}
}

func addFlagToSpec(spec *commandSpec, flag flagSpec) {
	if len(spec.Flags) == 0 {
		spec.BareDoubleDashArg = true
	}
	spec.Flags = appendFlagIfMissing(spec.Flags, flag)
	for i := range spec.Surfaces {
		if spec.Surfaces[i].Flags != nil {
			if len(spec.Surfaces[i].Flags) == 0 {
				spec.Surfaces[i].BareDoubleDashArg = true
			}
			spec.Surfaces[i].Flags = appendFlagIfMissing(spec.Surfaces[i].Flags, flag)
		}
	}
}

func appendFlagIfMissing(flags []flagSpec, flag flagSpec) []flagSpec {
	for _, existing := range flags {
		if existing.Name == flag.Name {
			return flags
		}
	}
	return append(flags, flag)
}

func decodeCommandClientOptions(parsed parsedCommand) (commandClientOptions, error) {
	chunked, err := parsedBoolFlag(parsed, "chunked", ownedGlobalBool(parsed, "chunked", parsed.globals.chunkedResponse))
	if err != nil {
		return commandClientOptions{}, fmt.Errorf("decode --chunked: %w", err)
	}
	directRPC, err := parsedBoolFlag(parsed, "direct-rpc", ownedGlobalBool(parsed, "direct-rpc", parsed.globals.useDirectRPC))
	if err != nil {
		return commandClientOptions{}, fmt.Errorf("decode --direct-rpc: %w", err)
	}
	skipSources, err := parsedBoolFlag(parsed, "skip-sources", ownedGlobalBool(parsed, "skip-sources", parsed.globals.skipSources))
	if err != nil {
		return commandClientOptions{}, fmt.Errorf("decode --skip-sources: %w", err)
	}
	return commandClientOptions{
		Chunked:     chunked,
		DirectRPC:   directRPC,
		SkipSources: skipSources,
	}, nil
}

func ownedGlobalBool(parsed parsedCommand, name string, value bool) bool {
	if !value {
		return false
	}
	command, ok := lookupCommand(parsed.path)
	return ok && findFlagSpec(commandFlagsForSurface(command.spec, command.surfaceSpec), name) != nil
}

func findFlagSpec(flags []flagSpec, name string) *flagSpec {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}
