package main

import (
	"testing"
)

// TestMainFunction is deprecated - use scripttest framework instead
// The scripttest files in testdata/ provide better coverage of the CLI
// For example: testdata/comprehensive_auth.txt tests command parsing
func TestMainFunction(t *testing.T) {
	t.Skip("Deprecated - use scripttest framework (see TestCLICommands and TestComprehensiveScripts)")
}

// TestAuthCommand tests that noAuth is set correctly in the command table.
func TestAuthCommand(t *testing.T) {
	tests := []struct {
		cmd      string
		wantAuth bool // true means command requires auth
	}{
		{"auth", false},
		{"refresh", false},
		{"chat-list", false},
		{"list", true},
		{"create", true},
		{"rm", true},
		{"sources", true},
		{"add", true},
		{"rm-source", true},
		{"create-audio", true},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			cmd, ok := lookupCommand(tt.cmd)
			if !ok {
				t.Fatalf("command %q not found in table", tt.cmd)
			}
			needsAuth := !cmd.noAuth
			if needsAuth != tt.wantAuth {
				t.Errorf("command %q: needsAuth=%v, want %v", tt.cmd, needsAuth, tt.wantAuth)
			}
		})
	}
}

// TestCommandTable checks every entry has required fields.
func TestCommandTable(t *testing.T) {
	for _, cmd := range commandTableEntries() {
		t.Run(cmd.name, func(t *testing.T) {
			if cmd.run == nil {
				t.Errorf("command %q has nil run func", cmd.name)
			}
			if cmd.usage == "" {
				t.Errorf("command %q has empty usage", cmd.name)
			}
			if cmd.section == "" {
				t.Errorf("command %q has empty section", cmd.name)
			}
			if cmd.maxArgs >= 0 && cmd.minArgs > cmd.maxArgs {
				t.Errorf("command %q: minArgs (%d) > maxArgs (%d)", cmd.name, cmd.minArgs, cmd.maxArgs)
			}
		})
	}
}

// TestCommandAliasesResolve checks all aliases are reachable.
func TestCommandAliasesResolve(t *testing.T) {
	for _, cmd := range commandTableEntries() {
		for _, alias := range cmd.aliases {
			t.Run(alias, func(t *testing.T) {
				entry, ok := lookupCommand(alias)
				if !ok {
					t.Fatalf("alias %q not found", alias)
				}
				if entry.name != cmd.name {
					t.Errorf("alias %q resolves to %q, want %q", alias, entry.name, cmd.name)
				}
			})
		}
	}
}
