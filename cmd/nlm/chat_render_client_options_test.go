package main

import "testing"

// TestChatRenderOptionsCarryClientOptions pins the wiring streaming chat
// depends on: --skip-sources must reach chatRenderOptions, since that is where
// streamChatResponse reads it to decide whether to list the notebook's
// sources.
func TestChatRenderOptionsCarryClientOptions(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want bool
	}{
		{args: []string{"nb", "hello"}, want: false},
		{args: []string{"nb", "hello", "--skip-sources"}, want: true},
	} {
		command, ok := lookupCommand("chat")
		if !ok {
			t.Fatal("command \"chat\" not found")
		}
		parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, tt.args, globalOptions{})
		if err != nil {
			t.Fatalf("parse chat %q: %v", tt.args, err)
		}
		options, err := decodeChatRenderOptions(parsed)
		if err != nil {
			t.Fatalf("decode chat %q: %v", tt.args, err)
		}
		if options.Client.SkipSources != tt.want {
			t.Fatalf("chat %q SkipSources = %v, want %v", tt.args, options.Client.SkipSources, tt.want)
		}
	}
}
