package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestReadLinesFromReader(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single line", "abc\n", []string{"abc"}},
		{"trailing newline omitted", "abc", []string{"abc"}},
		{"multiple lines", "a\nb\nc\n", []string{"a", "b", "c"}},
		{"blank lines skipped", "a\n\n\nb\n", []string{"a", "b"}},
		{"comments skipped", "a\n# comment\nb\n", []string{"a", "b"}},
		{"leading whitespace trimmed", "  a\n\tb\n", []string{"a", "b"}},
		{"column splits on first whitespace", "id1 Title One\nid2\tType\n", []string{"id1", "id2"}},
		{"mixed with blanks and comments", "\n# header\nuuid-1\n\n  # more\nuuid-2  extra\n", []string{"uuid-1", "uuid-2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readLinesFromReader(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveIDList(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want []string
	}{
		{"empty", "", nil},
		{"single id", "abc", []string{"abc"}},
		{"comma list", "a,b,c", []string{"a", "b", "c"}},
		{"comma list with spaces", "a, b , c", []string{"a", "b", "c"}},
		{"comma list trims empties", "a,,b,", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveIDList(tt.arg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

// Note: resolveIDList("-") requires non-TTY stdin; exercised at integration level.
