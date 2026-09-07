package nlmsync

import "testing"

func TestLiteralFamilyIdentity(t *testing.T) {
	for _, tt := range []struct {
		base, title, want string
		old, owned        bool
	}{
		{"notes (draft)", "notes (draft)", "notes (draft)", false, true},
		{"notes (draft)", "notes (draft) (pt2)", "notes (draft) (pt2)", false, true},
		{"notes (pt2)", "notes (pt2) (pt1) (ab)", "notes (pt2) (pt1) (ab)", false, true},
		{"base", "base (pt1)", "base", false, true},
		{"base", "base (pt2) (split1) (split2)", "base (pt2) (ab)", false, true},
		{"base", "base (split1) (split2)", "base (pt1) (ab)", false, true},
		{"base", "base (pt2) (ab)", "base (pt2) (ab)", false, true},
		{"base", "base (pt2) [old]", "base (pt2)", true, true},
		{"base [old]", "base [old]", "base [old]", false, true},
		{"base", "base (pt0)", "", false, false},
		{"base", "base (pt01)", "", false, false},
		{"base", "base (pt1) (z)", "", false, false},
		{"base", "base (split3)", "", false, false},
		{"base", "base (draft)", "", false, false},
		{"base", "baseball", "", false, false},
		{"base", "[old] base", "", false, false},
	} {
		t.Run(tt.title, func(t *testing.T) {
			p, old, ok := parsePart(tt.title, tt.base)
			if ok != tt.owned || old != tt.old || (ok && p.title(tt.base) != tt.want) {
				t.Fatalf("got %+v, old=%v owned=%v", p, old, ok)
			}
			if isPartOf(tt.title, tt.base) != tt.owned {
				t.Fatal("ownership mismatch")
			}
		})
	}
	if got := partChild("notes (pt2)", "notes (pt2)", 1); got != "notes (pt2) (pt1) (a)" {
		t.Fatalf("child=%s", got)
	}
	for _, sources := range [][]Source{
		{{ID: "a", Title: "base"}, {ID: "b", Title: "base (pt1)"}},
		{{ID: "a", Title: "base (split1)"}, {ID: "b", Title: "base (pt1) (a)"}},
	} {
		if _, err := checkSourceFamily(sources, "base"); err == nil {
			t.Fatal("duplicate canonical identity accepted")
		}
	}
	if _, err := checkSourceFamily([]Source{{ID: "a", Title: "base"}, {ID: "b", Title: "base [old]"}}, "base"); err != nil {
		t.Fatal(err)
	}
}
