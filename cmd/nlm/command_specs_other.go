package main

import (
	"context"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type mcpArgs struct{}

type betoolArgs struct {
	Values []string
	JSON   bool
}

type refreshArgs struct {
	Values []string
	Debug  bool
}

type accountArgs struct {
	Action    string
	ActionSet bool
	Key       string
	KeySet    bool
	Value     string
	ValueSet  bool
	JSON      bool
}

type heartbeatArgs struct{}

func configureOtherCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpec(specs["mcp"], commandFormOf(), decodeMCP)
	configureTypedCommandSpec(specs["betool"],
		commandFormOf(
			withUsage(remainingOperand("values"), "<mode>"),
			virtualOperand("[flags]"),
			virtualOperand("[file...]"),
		),
		decodeBetool,
	)
	configureTypedCommandSpec(specs["refresh"],
		commandFormOf(hiddenOperand(remainingOperand("values"))),
		decodeRefresh,
	)
	configureTypedCommandSpec(specs["account"],
		commandFormOf(
			withUsage(optionalOperand("action"), "[set <key> <value>]"),
			hiddenOperand(optionalOperand("key")),
			hiddenOperand(optionalOperand("value")),
		),
		decodeAccount,
	)
	configureTypedCommandSpec(specs["hb"], commandFormOf(), decodeHeartbeat)
}

func decodeMCP(parsedCommand) (commandCall, error) {
	_ = mcpArgs{}
	return func(_ context.Context, client *api.Client) error {
		return runMCP(client)
	}, nil
}

func decodeBetool(parsed parsedCommand) (commandCall, error) {
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return nil, err
	}
	args := betoolArgs{
		Values: append([]string(nil), parsed.Args["values"]...),
		JSON:   jsonOutput,
	}
	return func(context.Context, *api.Client) error {
		return runBetool(args)
	}, nil
}

func decodeRefresh(parsed parsedCommand) (commandCall, error) {
	args := refreshArgs{
		Values: append([]string(nil), parsed.Args["values"]...),
		Debug:  parsed.globals.debug,
	}
	return func(context.Context, *api.Client) error {
		return refreshCredentials(args.Debug)
	}, nil
}

func decodeAccount(parsed parsedCommand) (commandCall, error) {
	action, actionSet, err := parsedOptionalArgument(parsed, "action")
	if err != nil {
		return nil, err
	}
	key, keySet, err := parsedOptionalArgument(parsed, "key")
	if err != nil {
		return nil, err
	}
	value, valueSet, err := parsedOptionalArgument(parsed, "value")
	if err != nil {
		return nil, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return nil, err
	}
	args := accountArgs{
		Action:    action,
		ActionSet: actionSet,
		Key:       key,
		KeySet:    keySet,
		Value:     value,
		ValueSet:  valueSet,
		JSON:      jsonOutput,
	}
	return func(_ context.Context, client *api.Client) error {
		return runAccount(client, args)
	}, nil
}

func decodeHeartbeat(parsedCommand) (commandCall, error) {
	_ = heartbeatArgs{}
	return func(_ context.Context, client *api.Client) error {
		return heartbeat(client)
	}, nil
}
