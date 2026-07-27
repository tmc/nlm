package main

import "github.com/tmc/nlm/internal/betool"

func runBetool(args []string, jsonOutput bool) error {
	return betool.Run(args, betool.Options{
		JSONOutput:        jsonOutput,
		DebugParsing:      debugParsing,
		DebugFieldMapping: debugFieldMapping,
	})
}

func printBetoolUsage() {
	betool.PrintUsage()
}
