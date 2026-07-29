package main

import "github.com/tmc/nlm/internal/betool"

func runBetool(args betoolArgs) error {
	return betool.Run(args.Values, betool.Options{
		JSONOutput:        args.JSON,
		DebugParsing:      debugParsing,
		DebugFieldMapping: debugFieldMapping,
	})
}
