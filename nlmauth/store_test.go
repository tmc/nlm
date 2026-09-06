package nlmauth

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestValidateIdentityName(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"work", true},
		{"personal", true},
		{"acct-2", true},
		{"a.b_c", true},
		{"", false},
		{"Work", false}, // uppercase would collide on case-insensitive filesystems
		{"../../etc/passwd", false},
		{"..", false},
		{".hidden", false},
		{"work/other", false},
		{"work space", false},
	}
	for _, test := range tests {
		err := ValidateIdentityName(test.name)
		if (err == nil) != test.ok {
			t.Errorf("ValidateIdentityName(%q) = %v, want ok=%v", test.name, err, test.ok)
		}
	}
}

// A name that escaped the identities directory would let a login overwrite an
// arbitrary file, so the store must reject it before touching the filesystem.
func TestFileStoreRejectsEscapingNames(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	if err := store.Set("../escaped", Session{Credentials: Credentials{Cookies: "c"}}); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("Set(../escaped) = %v, want ErrInvalidIdentity", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escaped.env")); err == nil {
		t.Fatal("Set wrote outside the store directory")
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	store := NewFileStore(t.TempDir())

	if _, err := store.Get("work"); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Get(missing) = %v, want ErrNoCredentials", err)
	}

	want := Session{
		Credentials:    Credentials{AuthToken: "tok", Cookies: "ck", AuthUser: "3"},
		BrowserProfile: "Profile 3",
		SessionID:      "sid",
		BLParam:        "bl",
		SignalerAuth:   "sig",
	}
	if err := store.Set("work", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("work")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("Get = %+v, want %+v", got, want)
	}

	names, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !slices.Equal(names, []string{"work"}) {
		t.Fatalf("List = %v, want [work]", names)
	}

	if err := store.Delete("work"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete("work"); err != nil {
		t.Fatalf("Delete(absent) = %v, want nil", err)
	}
	if _, err := store.Get("work"); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Get after Delete = %v, want ErrNoCredentials", err)
	}
}

// R5: authenticating a second identity must leave the first readable and
// unchanged. This is the regression the whole design exists to prevent.
func TestFileStoreSecondIdentityDoesNotDisturbFirst(t *testing.T) {
	store := NewFileStore(t.TempDir())
	first := Session{Credentials: Credentials{AuthToken: "tok-1", Cookies: "ck-1"}, BrowserProfile: "Default"}
	second := Session{Credentials: Credentials{AuthToken: "tok-2", Cookies: "ck-2", AuthUser: "3"}, BrowserProfile: "Profile 3"}

	if err := store.Set("personal", first); err != nil {
		t.Fatalf("Set(personal): %v", err)
	}
	if err := store.Set("work", second); err != nil {
		t.Fatalf("Set(work): %v", err)
	}

	got, err := store.Get("personal")
	if err != nil {
		t.Fatalf("Get(personal): %v", err)
	}
	if got != first {
		t.Fatalf("Get(personal) = %+v, want %+v", got, first)
	}
	names, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !slices.Equal(names, []string{"personal", "work"}) {
		t.Fatalf("List = %v, want [personal work]", names)
	}
}

// A single-identity installation is adopted without a prompt or a flag, and
// $HOME/.nlm/env survives as the compatibility mirror (R7).
func TestFileStoreMigratesSingleIdentity(t *testing.T) {
	dir := t.TempDir()
	legacy := Session{
		Credentials:    Credentials{AuthToken: "tok", Cookies: "ck", AuthUser: "2"},
		BrowserProfile: "Profile 3",
	}
	if err := writeSessionFile(filepath.Join(dir, "env"), legacy); err != nil {
		t.Fatalf("seed env: %v", err)
	}

	store := NewFileStore(dir)
	got, err := store.Get(DefaultIdentity)
	if err != nil {
		t.Fatalf("Get(default): %v", err)
	}
	if got != legacy {
		t.Fatalf("Get(default) = %+v, want %+v", got, legacy)
	}
	if name, err := readCurrentIdentity(dir); err != nil || name != DefaultIdentity {
		t.Fatalf("current = %q, %v; want %q", name, err, DefaultIdentity)
	}
	if _, err := os.Stat(filepath.Join(dir, "env")); err != nil {
		t.Fatalf("env mirror removed by migration: %v", err)
	}
}

// A fresh installation has nothing to adopt and must not invent an identity.
func TestFileStoreFreshInstallHasNoIdentities(t *testing.T) {
	store := NewFileStore(t.TempDir())
	names, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("List = %v, want none", names)
	}
}

// SaveIdentity refreshes $HOME/.nlm/env only for the current identity: writing
// another identity must not repoint the session the rest of the system uses.
func TestSaveIdentityMirrorsOnlyCurrent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nlm")

	current := Session{Credentials: Credentials{AuthToken: "tok-current", Cookies: "ck-current"}}
	if err := SetCurrentIdentity("personal"); err != nil {
		t.Fatalf("SetCurrentIdentity: %v", err)
	}
	if err := SaveIdentity("personal", current); err != nil {
		t.Fatalf("SaveIdentity(personal): %v", err)
	}
	other := Session{Credentials: Credentials{AuthToken: "tok-other", Cookies: "ck-other"}}
	if err := SaveIdentity("work", other); err != nil {
		t.Fatalf("SaveIdentity(work): %v", err)
	}

	values, err := readEnvFile(filepath.Join(dir, "env"))
	if err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	if values["NLM_AUTH_TOKEN"] != "tok-current" {
		t.Fatalf("mirror token = %q, want tok-current", values["NLM_AUTH_TOKEN"])
	}
}

func TestOpenStoreRejectsUnknownKind(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NLM_CREDENTIAL_STORE", "vault")
	if _, err := OpenStore(); err == nil {
		t.Fatal("OpenStore() = nil error, want unknown-store error")
	}
}

func TestCurrentIdentityRejectsCorruptSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nlm")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "current"), []byte("../../other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CurrentIdentity(); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("CurrentIdentity = %v", err)
	}
}

func TestLoadSessionFollowsSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NLM_CREDENTIAL_STORE", "file")
	want := Session{Credentials: Credentials{Cookies: "work", AuthToken: "token"}}
	if err := Save(Session{Credentials: Credentials{Cookies: "default"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveIdentity("work", want); err != nil {
		t.Fatal(err)
	}
	if err := SetCurrentIdentity("work"); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadSession(); err != nil || got != want {
		t.Fatalf("LoadSession = %v, %v", got, err)
	}
}

func TestLoadEnvironmentDoesNotInheritStoredAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NLM_CREDENTIAL_STORE", "file")
	if err := Save(Session{Credentials: Credentials{Cookies: "stored", AuthToken: "stored", AuthUser: "3"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NLM_AUTH_TOKEN", "environment")
	t.Setenv("NLM_COOKIES", "environment")
	t.Setenv("NLM_AUTHUSER", "")
	got, err := Load()
	if err != nil || got.AuthUser != "" {
		t.Fatalf("Load = %v, %v", got, err)
	}
}
