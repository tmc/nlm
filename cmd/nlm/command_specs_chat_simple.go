package main

import (
	"context"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type chatListArgs struct {
	NotebookID    string
	NotebookIDSet bool
}

type chatHistoryArgs struct {
	NotebookID     string
	ConversationID string
}

type chatNotebookArgs struct {
	NotebookID string
}

type chatConfigArgs struct {
	NotebookID string
	Setting    string
	Mode       string
	ModeSet    bool
	Values     []string
}

type chatInstructionsArgs struct {
	NotebookID string
	Prompt     string
}

func configureSimpleChatCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpec(specs["chat-list"],
		commandFormOf(optionalOperand("notebook")),
		decodeChatList,
	)
	configureTypedCommandSpec(specs["chat-history"],
		commandFormOf(
			requiredOperand("notebook"),
			requiredOperand("conversation"),
		),
		decodeChatHistory,
	)
	configureTypedCommandSpec(specs["delete-chat"],
		commandFormOf(requiredOperand("notebook")),
		decodeChatDelete,
	)
	configureTypedCommandSpec(specs["chat-config"],
		chatConfigForms(),
		decodeChatConfig,
	)
	configureTypedCommandSpec(specs["set-instructions"],
		commandFormOf(
			requiredOperand("notebook"),
			withUsage(repeatedOperand("prompt"), "\"prompt\""),
		),
		decodeChatInstructionsSet,
	)
	configureTypedCommandSpec(specs["get-instructions"],
		commandFormOf(requiredOperand("notebook")),
		decodeChatInstructionsGet,
	)
}

func chatConfigForms() []commandForm {
	form := func(usage string, valid func(parsedCommand) bool) commandForm {
		return commandForm{
			Parts: []operandSpec{
				withUsage(requiredOperand("notebook"), usage),
				hiddenOperand(requiredOperand("setting")),
				hiddenOperand(optionalOperand("mode")),
				hiddenOperand(remainingOperand("values")),
			},
			Constraints: []constraint{
				constraintFunc(func(parsed parsedCommand) error {
					if valid(parsed) {
						return nil
					}
					return errBadArgs
				}),
			},
		}
	}
	return []commandForm{
		form("<notebook-id> goal default", func(parsed parsedCommand) bool {
			if len(parsed.Args["mode"]) == 0 {
				return false
			}
			return parsed.Args["setting"][0] == "goal" &&
				parsed.Args["mode"][0] == "default" &&
				len(parsed.Args["values"]) == 0
		}),
		form("<notebook-id> goal custom <prompt...>", func(parsed parsedCommand) bool {
			if len(parsed.Args["mode"]) == 0 {
				return false
			}
			return parsed.Args["setting"][0] == "goal" &&
				parsed.Args["mode"][0] == "custom" &&
				len(parsed.Args["values"]) > 0
		}),
		form("<notebook-id> length <default|longer|shorter>", func(parsed parsedCommand) bool {
			if len(parsed.Args["mode"]) == 0 ||
				parsed.Args["setting"][0] != "length" ||
				len(parsed.Args["values"]) != 0 {
				return false
			}
			switch parsed.Args["mode"][0] {
			case "default", "longer", "shorter":
				return true
			default:
				return false
			}
		}),
		{
			Parts: []operandSpec{
				requiredOperand("notebook"),
				requiredOperand("setting"),
				optionalOperand("mode"),
				remainingOperand("values"),
			},
			Hidden: true,
		},
	}
}

func decodeChatList(parsed parsedCommand) (commandCall, error) {
	notebookID, notebookIDSet, err := parsedOptionalArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	args := chatListArgs{NotebookID: notebookID, NotebookIDSet: notebookIDSet}
	return func(context.Context, *api.Client) error {
		if args.NotebookIDSet {
			return listChatConversationsWithAuth(args.NotebookID)
		}
		return listChatSessions()
	}, nil
}

func decodeChatHistory(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	conversationID, err := parsedArgument(parsed, "conversation")
	if err != nil {
		return nil, err
	}
	args := chatHistoryArgs{NotebookID: notebookID, ConversationID: conversationID}
	return func(_ context.Context, client *api.Client) error {
		return printChatHistory(client, args.NotebookID, args.ConversationID)
	}, nil
}

func decodeChatDelete(parsed parsedCommand) (commandCall, error) {
	args, err := decodeChatNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return deleteChatHistory(client, args.NotebookID)
	}, nil
}

func decodeChatConfig(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	setting, err := parsedArgument(parsed, "setting")
	if err != nil {
		return nil, err
	}
	mode, modeSet, err := parsedOptionalArgument(parsed, "mode")
	if err != nil {
		return nil, err
	}
	args := chatConfigArgs{
		NotebookID: notebookID,
		Setting:    setting,
		Mode:       mode,
		ModeSet:    modeSet,
		Values:     append([]string(nil), parsed.Args["values"]...),
	}
	return func(_ context.Context, client *api.Client) error {
		return setChatConfig(client, args)
	}, nil
}

func decodeChatInstructionsSet(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	prompt, err := parsedArguments(parsed, "prompt")
	if err != nil {
		return nil, err
	}
	args := chatInstructionsArgs{
		NotebookID: notebookID,
		Prompt:     strings.Join(prompt, " "),
	}
	return func(_ context.Context, client *api.Client) error {
		return setInstructions(client, args.NotebookID, args.Prompt)
	}, nil
}

func decodeChatInstructionsGet(parsed parsedCommand) (commandCall, error) {
	args, err := decodeChatNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return getInstructions(client, args.NotebookID)
	}, nil
}

func decodeChatNotebook(parsed parsedCommand) (chatNotebookArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return chatNotebookArgs{}, err
	}
	return chatNotebookArgs{NotebookID: notebookID}, nil
}
