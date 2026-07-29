package main

import (
	"strings"
)

type groupedCommandSurface struct {
	ID      commandID
	Path    string
	Section string
}

// groupedCommandSurfaces is the stable grouped surface order frozen by the
// Phase 1 parity golden. Each entry adapts to one shared command behavior.
var groupedCommandSurfaces = []groupedCommandSurface{
	{ID: "list", Path: "notebook list"},
	{ID: "create", Path: "notebook create"},
	{ID: "rm", Path: "notebook delete"},
	{ID: "rename-notebook", Path: "notebook rename"},
	{ID: "notebook-emoji", Path: "notebook emoji"},
	{ID: "notebook-description", Path: "notebook description"},
	{ID: "notebook-cover", Path: "notebook cover"},
	{ID: "notebook-cover-image", Path: "notebook cover-image"},
	{ID: "notebook-unrecent", Path: "notebook unrecent"},
	{ID: "list-featured", Path: "notebook featured"},

	{ID: "sources", Path: "source list"},
	{ID: "add", Path: "source add"},
	{ID: "sync", Path: "source sync"},
	{ID: "sync-pack", Path: "source pack"},
	{ID: "rm-source", Path: "source delete"},
	{ID: "rename-source", Path: "source rename"},
	{ID: "refresh-source", Path: "source refresh"},
	{ID: "check-source", Path: "source check"},
	{ID: "read-source", Path: "source read"},

	{ID: "notes", Path: "note list"},
	{ID: "read-note", Path: "note read"},
	{ID: "new-note", Path: "note create"},
	{ID: "update-note", Path: "note update"},
	{ID: "rm-note", Path: "note delete"},

	{ID: "label-list", Path: "label list"},
	{ID: "label-generate", Path: "label generate"},
	{ID: "label-create", Path: "label create"},
	{ID: "label-rename", Path: "label rename"},
	{ID: "label-emoji", Path: "label emoji"},
	{ID: "label-delete", Path: "label delete"},
	{ID: "label-unlabeled", Path: "label unlabeled"},
	{ID: "label-relabel-all", Path: "label relabel-all"},
	{ID: "label-attach", Path: "label attach"},

	{ID: "artifacts", Path: "artifact list"},
	{ID: "get-artifact", Path: "artifact get"},
	{ID: "read-artifact", Path: "artifact read"},
	{ID: "export-flashcards", Path: "artifact export"},
	{ID: "update-artifact", Path: "artifact update"},
	{ID: "delete-artifact", Path: "artifact delete"},

	{ID: "chat-list", Path: "chat list"},
	{ID: "chat-history", Path: "chat history"},
	{ID: "chat-show", Path: "chat show"},
	{ID: "delete-chat", Path: "chat delete"},
	{ID: "chat-config", Path: "chat config"},
	{ID: "set-instructions", Path: "chat instructions set"},
	{ID: "get-instructions", Path: "chat instructions get"},

	{ID: "audio-list", Path: "audio list"},
	{ID: "create-audio", Path: "audio create", Section: "Audio"},
	{ID: "audio-get", Path: "audio get"},
	{ID: "audio-download", Path: "audio download"},
	{ID: "audio-rm", Path: "audio delete"},
	{ID: "audio-share", Path: "audio share"},

	{ID: "create-video", Path: "video create", Section: "Video"},

	{ID: "create-slides", Path: "deck create", Section: "Deck"},
	{ID: "deck-download", Path: "deck download"},

	{ID: "app-create", Path: "app create"},
	{ID: "mindmap-create", Path: "mindmap create"},
}

var commands []command
var commandSpecs []*commandSpec

func buildCommandRegistry() {
	specs := make(map[commandID]*commandSpec, len(commandDefinitions))
	for i := range commandDefinitions {
		definition := &commandDefinitions[i]
		id := commandID(definition.name)
		if specs[id] != nil {
			panic("duplicate command ID: " + id)
		}
		spec := newCommandSpec(id, definition)
		specs[id] = spec
		commandSpecs = append(commandSpecs, spec)
	}

	for _, grouped := range groupedCommandSurfaces {
		spec := specs[grouped.ID]
		if spec == nil {
			panic("missing grouped command behavior: " + grouped.ID)
		}
		section := grouped.Section
		if section == "" {
			section = spec.Section
		}
		spec.Surfaces = append(spec.Surfaces, commandSurfaceSpec{
			Path:    strings.Fields(grouped.Path),
			Surface: surfaceStable,
			Section: section,
		})
	}
	for _, spec := range commandSpecs {
		definition := spec.definition
		surface := commandDefinitionSurface(definition.name)
		var aliases [][]string
		for _, alias := range definition.aliases {
			aliases = append(aliases, strings.Fields(alias))
		}
		replacement := compatibilityReplacements[definition.name]
		spec.Surfaces = append(spec.Surfaces, commandSurfaceSpec{
			Path:        strings.Fields(definition.name),
			Aliases:     aliases,
			Surface:     surface,
			Replacement: strings.Fields(replacement),
			Section:     definition.section,
		})
	}
	configureNotebookCommandSpecs(specs)
	configureSourceCommandSpecs(specs)
	configureNoteCommandSpecs(specs)
	configureLabelCommandSpecs(specs)
	configureAudioCommandSpecs(specs)
	configureArtifactCommandSpecs(specs)
	configureGuidebookCommandSpecs(specs)
	configureGenerationCommandSpecs(specs)
	configureSimpleChatCommandSpecs(specs)
	configureSharingCommandSpecs(specs)
	configureOtherCommandSpecs(specs)
	configureDeckCommandSpecs(specs)
	configureResearchCommandSpecs(specs)
	configureSelectorCommandSpecs(specs)
	configureCreateCommandSpecs(specs)
	configureChatCommandSpecs(specs)
	configureAuthCommandSpec(specs)

	for _, spec := range commandSpecs {
		if spec.Decode == nil || len(spec.Forms) == 0 {
			panic("incomplete command spec: " + spec.ID)
		}
	}
	for _, grouped := range groupedCommandSurfaces {
		spec := specs[grouped.ID]
		surface := findSpecSurface(spec, grouped.Path)
		commands = append(commands, bindCommandSurface(spec, surface))
	}
	for _, spec := range commandSpecs {
		definition := spec.definition
		surface := findSpecSurface(spec, definition.name)
		commands = append(commands, bindCommandSurface(spec, surface))
	}
}

func newCommandSpec(id commandID, definition *commandDefinition) *commandSpec {
	return &commandSpec{
		ID:         id,
		Section:    definition.section,
		Summary:    definition.usage,
		definition: definition,
	}
}

func commandDefinitionSurface(name string) commandSurface {
	switch {
	case experimentalCommands[name]:
		return surfaceExperimental
	case internalCommands[name]:
		return surfaceInternal
	case compatibilityCommands[name]:
		return surfaceCompatibility
	default:
		return surfaceStable
	}
}

func findSpecSurface(spec *commandSpec, path string) *commandSurfaceSpec {
	for i := range spec.Surfaces {
		if strings.Join(spec.Surfaces[i].Path, " ") == path {
			return &spec.Surfaces[i]
		}
	}
	panic("missing command surface: " + path)
}

func bindCommandSurface(spec *commandSpec, surface *commandSurfaceSpec) command {
	definition := spec.definition
	var aliases []string
	for _, alias := range surface.Aliases {
		aliases = append(aliases, strings.Join(alias, " "))
	}
	name := strings.Join(surface.Path, " ")
	hidden := definition.hidden
	if name != definition.name {
		hidden = false
	}
	return command{
		commandDefinition: definition,
		spec:              spec,
		surfaceSpec:       surface,
		name:              name,
		aliases:           aliases,
		section:           surface.Section,
		surface:           surface.Surface,
		hidden:            hidden,
	}
}

func parseBoundCommand(cmd *command, path string, args []string, globals globalOptions) (parsedCommand, error) {
	surface := *cmd.surfaceSpec
	surface.Path = strings.Fields(path)
	if cmd.spec.parse != nil {
		return cmd.spec.parse(&surface, args, globals)
	}
	return parseCommandSpec(cmd.spec, &surface, args, globals)
}
