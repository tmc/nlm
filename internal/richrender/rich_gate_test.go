package richrender

import "testing"

// a minimal non-nil tree so the gate's other conditions are what's under test.
func gateTree() *RichDocument {
	return &RichDocument{Blocks: []richSpan{{Start: "0", End: "1", Leaf: &richLeaf{Text: "x"}}}}
}

func TestShouldReflowFromTree(t *testing.T) {
	tree := gateTree()
	cases := []struct {
		name    string
		rich    *RichDocument
		content string
		want    bool
	}{
		{"no tree", nil, "run together answer", false},
		{"newline-free prose reflows", tree, "one two three four", true},
		{"newline present, left alone", tree, "para one\n\npara two", false},
		{"json object left alone", tree, `{"topic_name":"a","notes":"line1\nline2"}`, false},
		{"json array left alone", tree, `[{"k":"v"},{"k":"w"}]`, false},
		{"prose starting with brace but not json", tree, "{not json, just prose with a brace", true},
		{"empty content", tree, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldReflowFromTree(tc.rich, tc.content); got != tc.want {
				t.Errorf("shouldReflowFromTree(%q) = %v, want %v", tc.content, got, tc.want)
			}
		})
	}
}

func TestLooksLikeJSON(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`{"a":1}`, true},
		{`[1,2,3]`, true},
		{`  {"a":1}  `, true}, // leading/trailing space tolerated
		{`{"a":1`, false},     // truncated
		{"plain prose", false},
		{"{not json", false},
		{"", false},
		{"[", false},
	}
	for _, tc := range cases {
		if got := looksLikeJSON(tc.in); got != tc.want {
			t.Errorf("looksLikeJSON(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
