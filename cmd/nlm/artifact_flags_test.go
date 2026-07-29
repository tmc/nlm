package main

import "testing"

func TestParseUpdateArtifactArgsWithOptions(t *testing.T) {
	command, ok := lookupCommand("artifact update")
	if !ok {
		t.Fatal("artifact update command not found")
	}
	parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, []string{"art-1", "--name", "New"}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	args, err := decodeArtifactUpdateArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if args.ArtifactID != "art-1" || args.Options.Name != "New" {
		t.Fatalf("arguments = %+v; want art-1 New", args)
	}

	parsed, err = parseCommandSpec(command.spec, command.surfaceSpec, []string{"art-2"}, globalOptions{sourceName: "FromGlobal"})
	if err != nil {
		t.Fatal(err)
	}
	args, err = decodeArtifactUpdateArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if args.ArtifactID != "art-2" || args.Options.Name != "FromGlobal" {
		t.Fatalf("arguments = %+v; want art-2 FromGlobal", args)
	}
}
