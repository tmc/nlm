package nlmsync

import "testing"

func TestSplitChildName(t *testing.T) {
	for _, tt := range []struct {
		name  string
		index int
		want  string
	}{
		{"repo", 1, "repo (pt1) (a)"},
		{"repo", 2, "repo (pt1) (b)"},
		{"repo (pt3)", 2, "repo (pt3) (b)"},
		{"repo (pt3) (b)", 1, "repo (pt3) (ba)"},
		{"repo (pt1) (ab)", 2, "repo (pt1) (abb)"},
	} {
		if got := splitChildName(tt.name, tt.index); got != tt.want {
			t.Errorf("splitChildName(%q, %d) = %q, want %q", tt.name, tt.index, got, tt.want)
		}
	}
}

func TestTrimSplitSuffix(t *testing.T) {
	for _, tt := range []struct {
		title   string
		want    string
		trimmed bool
	}{
		{"repo (pt1) (a)", "repo", true},
		{"repo (pt3) (ba)", "repo (pt3) (b)", true},
		{"repo (pt3)", "repo (pt3)", false},
		{"repo", "repo", false},
		{"repo (split1) (split2)", "repo (split1)", true},
		{"repo (draft)", "repo (draft)", false},
	} {
		got, trimmed := trimSplitSuffix(tt.title)
		if got != tt.want || trimmed != tt.trimmed {
			t.Errorf("trimSplitSuffix(%q) = %q, %v; want %q, %v", tt.title, got, trimmed, tt.want, tt.trimmed)
		}
	}
}

func TestIsPartOfSplit(t *testing.T) {
	for _, tt := range []struct {
		title string
		want  bool
	}{
		{"repo (pt1) (a)", true},
		{"repo (pt4) (bab)", true},
		{"repo (pt2)", true},
		{"repo (split1) (split2)", true},
		{"repo backup", false},
	} {
		if got := isPartOf(tt.title, "repo"); got != tt.want {
			t.Errorf("isPartOf(%q, \"repo\") = %v, want %v", tt.title, got, tt.want)
		}
	}
}

func TestSplitDescendant(t *testing.T) {
	for _, tt := range []struct {
		title  string
		parent string
		want   bool
	}{
		{"repo (pt1) (a)", "repo", true},
		{"repo (pt1) (ab)", "repo", true},
		{"repo (pt3) (a)", "repo (pt3)", true},
		{"repo (pt3)", "repo", false},
		{"repo (pt4) (a)", "repo (pt3)", false},
	} {
		if got := splitDescendant(tt.title, tt.parent); got != tt.want {
			t.Errorf("splitDescendant(%q, %q) = %v, want %v", tt.title, tt.parent, got, tt.want)
		}
	}
}
