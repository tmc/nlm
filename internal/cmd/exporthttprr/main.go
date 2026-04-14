package main

import (
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/httprr"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <httprr-file> [output-txtar-file]\n", os.Args[0])
		os.Exit(1)
	}

	httprFile := os.Args[1]
	txtarFile := httprFile[:len(httprFile)-len(".httprr")] + ".txtar"
	if len(os.Args) >= 3 {
		txtarFile = os.Args[2]
	}

	// Create RecordReplay instance
	rr := &httprr.RecordReplay{}
	rr.SetFile(httprFile)

	// Export to txtar (without secrets by default)
	includeSecrets := os.Getenv("INCLUDE_SECRETS") == "true"
	if err := rr.ExportToTxtar(txtarFile, includeSecrets); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Exported %s -> %s\n", httprFile, txtarFile)
}
