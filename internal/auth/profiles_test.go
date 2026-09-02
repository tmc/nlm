package auth

import (
	"bytes"
	"testing"
	"time"
)

func testProfiles() []ProfileInfo {
	at := func(day int) time.Time {
		return time.Date(2026, 9, day, 10, 0, 0, 0, time.UTC)
	}
	return []ProfileInfo{
		{Name: "Profile 3", Path: "/p/3", Browser: "Brave", LastUsed: at(1), Files: []string{"Cookies"}, Size: 1 << 20},
		{Name: "Default", Path: "/p/default", Browser: "Brave", LastUsed: at(3), Files: []string{"Cookies", "History"}, Size: 2 << 20, HasTargetCookies: true},
		{Name: "Profile 7", Path: "/p/7", Browser: "Chrome", LastUsed: at(2), Files: []string{"Cookies"}, Size: 3 << 20},
	}
}

// TestPrintProfileTable pins the defect the inventory used to have: every row
// was numbered 1. Rows are numbered by index, ordered most recently used
// first, and the profile that will be used is marked.
func TestPrintProfileTable(t *testing.T) {
	profiles := testProfiles()
	sortProfilesByLastUsed(profiles)
	selected := selectProfile(profiles, "Profile 7")

	var buf bytes.Buffer
	printProfileTable(&buf, profiles, "notebook.google.com", selected, false)

	want := "nlm: browser profiles:\n" +
		"nlm:   1. Default [Brave] - last used 2026-09-03 10:00:00 (2 files, 2.0 MB) [notebook.google.com cookies]\n" +
		"nlm: * 2. Profile 7 [Chrome] - last used 2026-09-02 10:00:00 (1 files, 3.0 MB) [no notebook.google.com cookies]\n" +
		"nlm:   3. Profile 3 [Brave] - last used 2026-09-01 10:00:00 (1 files, 1.0 MB) [no notebook.google.com cookies]\n" +
		"nlm: using profile Profile 7 [Chrome]\n"
	if got := buf.String(); got != want {
		t.Errorf("printProfileTable wrote:\n%s\nwant:\n%s", got, want)
	}
}

// TestPrintProfileTableTryAll marks no row when every profile will be tried.
func TestPrintProfileTableTryAll(t *testing.T) {
	profiles := testProfiles()
	sortProfilesByLastUsed(profiles)

	var buf bytes.Buffer
	printProfileTable(&buf, profiles, "", selectProfile(profiles, "Default"), true)

	got := buf.String()
	if want := "nlm: trying profiles in the order shown\n"; !bytes.HasSuffix([]byte(got), []byte(want)) {
		t.Errorf("printProfileTable(tryAll) = %q, want it to end with %q", got, want)
	}
	if bytes.Contains([]byte(got), []byte("*")) {
		t.Errorf("printProfileTable(tryAll) marked a row:\n%s", got)
	}
}

func TestSelectProfile(t *testing.T) {
	profiles := testProfiles()
	sortProfilesByLastUsed(profiles)

	if got := selectProfile(profiles, "Profile 3"); got == nil || got.Name != "Profile 3" {
		t.Errorf("selectProfile(%q) = %v, want the named profile", "Profile 3", got)
	}
	// An unknown name falls back to the most recently used profile, which is
	// the row the inventory numbers 1.
	if got := selectProfile(profiles, "Nonexistent"); got == nil || got.Name != "Default" {
		t.Errorf("selectProfile(%q) = %v, want Default", "Nonexistent", got)
	}
	if got := selectProfile(nil, "Default"); got != nil {
		t.Errorf("selectProfile(nil) = %v, want nil", got)
	}
}

// TestWantProfileTable covers the gate: the inventory appears only when the
// user asked for it, never on the default login path.
func TestWantProfileTable(t *testing.T) {
	tests := []struct {
		name  string
		opts  Options
		debug bool
		want  bool
	}{
		{name: "default login", want: false},
		{name: "list-profiles", opts: Options{ListProfiles: true}, want: true},
		{name: "try-all-profiles", opts: Options{TryAllProfiles: true}, want: true},
		{name: "check-notebooks", opts: Options{CheckNotebooks: true}, want: true},
		{name: "debug", debug: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.wantProfileTable(tt.debug); got != tt.want {
				t.Errorf("wantProfileTable(%v) = %v, want %v", tt.debug, got, tt.want)
			}
		})
	}
}

// TestDefaultOptionsHideProfileTable guards the default that regressed: the
// inventory used to be on unless a caller turned it off.
func TestDefaultOptionsHideProfileTable(t *testing.T) {
	if o := defaultBrowserAuthOptions(); o.wantProfileTable(false) {
		t.Error("default options print the profile inventory")
	}
}
