package main

import (
	"slices"
	"testing"
)

func TestCommandFlagOwners(t *testing.T) {
	tests := []struct {
		flag string
		want []commandID
	}{
		{
			flag: "direct-rpc",
			want: []commandID{
				"audio-download",
				"audio-get",
				"audio-list",
				"audio-rm",
				"audio-share",
				"create-audio",
				"create-video",
			},
		},
		{
			flag: "skip-sources",
			want: []commandID{
				"chat",
				"discover-sources",
				"generate-chat",
				"generate-report",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.flag, func(t *testing.T) {
			var got []commandID
			for _, spec := range commandSpecs {
				if findFlagSpec(spec.Flags, test.flag) != nil {
					got = append(got, spec.ID)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, test.want) {
				t.Fatalf("%s owners = %v, want %v", test.flag, got, test.want)
			}
		})
	}
}

func TestCommandClientOptionsDecodeOwnedFlags(t *testing.T) {
	tests := []struct {
		path string
		args []string
		want commandClientOptions
	}{
		{
			path: "source list",
			args: []string{"--chunked", "notebook"},
			want: commandClientOptions{Chunked: true},
		},
		{
			path: "audio create",
			args: []string{"--direct-rpc", "notebook", "instructions"},
			want: commandClientOptions{DirectRPC: true},
		},
		{
			path: "chat",
			args: []string{"--skip-sources", "notebook"},
			want: commandClientOptions{SkipSources: true},
		},
		{
			path: "audio create",
			args: []string{"notebook", "instructions"},
			want: commandClientOptions{DirectRPC: true},
		},
		{
			path: "source list",
			args: []string{"notebook"},
			want: commandClientOptions{},
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			command, ok := lookupCommand(test.path)
			if !ok {
				t.Fatalf("missing command %q", test.path)
			}
			globals := globalOptions{}
			if test.path == "audio create" && len(test.args) == 2 {
				globals.useDirectRPC = true
			}
			if test.path == "source list" && len(test.args) == 1 {
				globals.useDirectRPC = true
			}
			parsed, err := parseBoundCommand(command, test.path, test.args, globals)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeCommandClientOptions(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("client options = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestChunkedOwnedByClientCommands(t *testing.T) {
	for _, spec := range commandSpecs {
		want := !spec.noClient || spec.ID == "chat-list" || spec.ID == "chat-show"
		got := findFlagSpec(spec.Flags, "chunked") != nil
		if got != want {
			t.Errorf("%s owns --chunked = %v, want %v", spec.ID, got, want)
		}
	}
}
