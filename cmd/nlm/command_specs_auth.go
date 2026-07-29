package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/tmc/nlm/internal/authuser"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

type authArgs struct {
	Options   authOptions
	Remaining []string
	Globals   globalOptions
	Raw       []string
	FlagError error
}

func configureAuthCommandSpec(specs map[commandID]*commandSpec) {
	spec := specs["auth"]
	spec.Flags = authFlagSpecs()
	spec.FlagGroup = "options"
	spec.FlagGroupAfter = 1
	spec.DeferFlagErrors = true
	spec.DeferFlagValidation = true
	configureTypedCommandSpec(
		spec,
		authCommandForms(),
		decodeAuth,
	)
}

func authCommandForms() []commandForm {
	return []commandForm{
		{
			Parts: []operandSpec{
				operandSpec{Literal: "login"},
				optionalOperand("profile"),
			},
			Hidden: true,
		},
		{
			Parts: []operandSpec{
				virtualOperand("[login]"),
				withUsage(optionalOperand("profile"), "[profile-name]"),
			},
		},
		{
			Parts: []operandSpec{hiddenOperand(remainingOperand("positionals"))},
			Constraints: []constraint{
				constraintFunc(rejectExtraAuthArguments),
			},
			Hidden: true,
		},
	}
}

func rejectExtraAuthArguments(parsed parsedCommand) error {
	args := parsed.Args["positionals"]
	extra := 1
	if args[0] == "login" {
		extra = 2
	}
	return badArgsf("unexpected argument %q for %q", args[extra], parsed.path)
}

func authFlagSpecs() []flagSpec {
	return []flagSpec{
		{Name: "all", Aliases: []string{"a"}, Description: "try all profiles"},
		{Name: "profile", Aliases: []string{"p"}, Value: "string", Description: "browser profile"},
		{Name: "url", Aliases: []string{"u"}, Value: "string", Description: "target URL"},
		{Name: "notebooks", Aliases: []string{"n"}, Description: "check notebooks"},
		{Name: "debug", Aliases: []string{"d"}, Description: "enable debug output"},
		{Name: "help", Aliases: []string{"h"}, Description: "show help"},
		{Name: "print-env", Description: "print environment"},
		{Name: "keep-open", Aliases: []string{"k"}, Value: "int", Description: "keep browser open"},
		{Name: "cdp-url", Aliases: []string{"c"}, Value: "string", Description: "remote CDP URL"},
		{Name: "authuser", Aliases: []string{"au"}, Value: "string", Description: "Google account index"},
	}
}

func decodeAuth(parsed parsedCommand) (commandCall, error) {
	args := decodeAuthArgs(parsed)
	return func(_ context.Context, _ *api.Client) error {
		_, _, err := handleDecodedAuth(args)
		return err
	}, nil
}

func decodeAuthArgs(parsed parsedCommand) authArgs {
	options := authOptions{
		ProfileName: parsed.globals.chromeProfile,
		TargetURL:   "https://notebook.google.com",
		Debug:       parsed.globals.debug,
	}
	if parsed.globals.authUserSet {
		options.AuthUser = authuser.Normalize(parsed.globals.authUser)
	}

	flagError := parsed.flagError
	if flagError == nil {
		for _, flag := range parsed.flagOccurrences {
			if err := setAuthOption(&options, flag); err != nil {
				flagError = err
				break
			}
		}
	}

	remaining := append([]string(nil), parsed.Args["profile"]...)
	if !options.TryAllProfiles && options.ProfileName == "" && len(remaining) > 0 {
		options.ProfileName = remaining[0]
		remaining = remaining[1:]
	}
	if !options.TryAllProfiles && options.ProfileName == "" {
		options.ProfileName = "Default"
		if profile := os.Getenv("NLM_BROWSER_PROFILE"); profile != "" {
			options.ProfileName = profile
		}
	}
	if options.RemoteCDPURL == "" {
		options.RemoteCDPURL = os.Getenv("NLM_CDP_URL")
	}

	return authArgs{
		Options:   options,
		Remaining: remaining,
		Globals:   parsed.globals,
		Raw:       append([]string(nil), parsed.raw...),
		FlagError: flagError,
	}
}

func setAuthOption(options *authOptions, flag parsedFlag) error {
	switch flag.Name {
	case "all":
		return setAuthBool(flag, &options.TryAllProfiles)
	case "profile":
		options.ProfileName = flag.Value
	case "url":
		options.TargetURL = flag.Value
	case "notebooks":
		return setAuthBool(flag, &options.CheckNotebooks)
	case "debug":
		return setAuthBool(flag, &options.Debug)
	case "help":
		return setAuthBool(flag, &options.Help)
	case "print-env":
		return setAuthBool(flag, &options.PrintEnv)
	case "keep-open":
		value, err := strconv.ParseInt(flag.Value, 0, strconv.IntSize)
		if err != nil {
			return fmt.Errorf("invalid value %q for flag -%s: parse error", flag.Value, flag.InputName)
		}
		options.KeepOpenSeconds = int(value)
	case "cdp-url":
		options.RemoteCDPURL = flag.Value
	case "authuser":
		options.AuthUser = flag.Value
	}
	return nil
}

func setAuthBool(flag parsedFlag, dst *bool) error {
	value, err := strconv.ParseBool(flag.Value)
	if err != nil {
		return fmt.Errorf("invalid boolean value %q for -%s: parse error", flag.Value, flag.InputName)
	}
	*dst = value
	return nil
}
