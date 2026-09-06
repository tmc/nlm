package nlmauth

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNoCredentials reports that no usable NotebookLM session was found in the
// environment or the shared store. Recover by running `nlm auth`.
var ErrNoCredentials = errors.New("nlmauth: no NotebookLM session; run 'nlm auth' or set NLM_AUTH_TOKEN and NLM_COOKIES")

// Credentials are the NotebookLM web-session values needed to authenticate API
// requests.
type Credentials struct {
	AuthToken string // the "at" token
	Cookies   string // raw Cookie header value
	AuthUser  string // X-Goog-AuthUser index; "" selects the default account
}

// Session is the full persisted authentication state written to the shared
// store. It embeds [Credentials] and adds the browser-harvest provenance that
// credential refresh needs.
type Session struct {
	Credentials
	BrowserProfile string // Chrome/Brave profile the session was harvested from
	SessionID      string // FdrFJe page-bootstrap value
	BLParam        string // cfb2h build label
	SignalerAuth   string // Signaler authorization, when harvested via CDP
}

// storePath returns the path to the shared credential store, $HOME/.nlm/env.
func storePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("nlmauth: locate home directory: %w", err)
	}
	return filepath.Join(home, ".nlm", "env"), nil
}

// Load resolves credentials from the environment (NLM_AUTH_TOKEN, NLM_COOKIES,
// NLM_AUTHUSER), falling back to the shared store for any field left empty. It
// returns [ErrNoCredentials] when neither yields both an auth token and
// cookies.
func Load() (Credentials, error) {
	creds := Credentials{
		AuthToken: os.Getenv("NLM_AUTH_TOKEN"),
		Cookies:   os.Getenv("NLM_COOKIES"),
		AuthUser:  os.Getenv("NLM_AUTHUSER"),
	}
	if creds.AuthToken != "" && creds.Cookies != "" {
		return creds, nil
	}
	if creds.AuthToken == "" || creds.Cookies == "" {
		stored, err := LoadSession()
		if err != nil {
			return Credentials{}, err
		}
		if creds.AuthToken == "" {
			creds.AuthToken = stored.AuthToken
		}
		if creds.Cookies == "" {
			creds.Cookies = stored.Cookies
		}
		if creds.AuthUser == "" {
			creds.AuthUser = stored.AuthUser
		}
	}
	if creds.AuthToken == "" || creds.Cookies == "" {
		return Credentials{}, ErrNoCredentials
	}
	return creds, nil
}

// LoadSession returns the current identity's session from the configured store.
// A missing identity is not an error: it yields the zero Session.
func LoadSession() (Session, error) {
	store, err := OpenStore()
	if err != nil {
		return Session{}, err
	}
	// List adopts a legacy store before reading its current selection.
	if _, err := store.List(); err != nil {
		return Session{}, err
	}
	name, err := CurrentIdentity()
	if err != nil {
		return Session{}, err
	}
	session, err := store.Get(name)
	if errors.Is(err, ErrNoCredentials) {
		return Session{}, nil
	}
	return session, err
}

// sessionFromValues builds a Session from raw store keys. A key that is absent
// yields an empty field, which is how every reader already treats it.
func sessionFromValues(values map[string]string) Session {
	return Session{
		Credentials: Credentials{
			AuthToken: values["NLM_AUTH_TOKEN"],
			Cookies:   values["NLM_COOKIES"],
			AuthUser:  values["NLM_AUTHUSER"],
		},
		BrowserProfile: values["NLM_BROWSER_PROFILE"],
		SessionID:      values["NLM_SESSION_ID"],
		BLParam:        values["NLM_BL_PARAM"],
		SignalerAuth:   values["NLM_SIGNALER_AUTH"],
	}
}

// Save writes s as the current identity, creating $HOME/.nlm (0700) if needed.
//
// Two files are written: the identity itself, through the configured store,
// and $HOME/.nlm/env, which remains the compatibility mirror of the current
// identity for everything that reads the store path directly. Files are written with 0600 permissions.
func Save(s Session) error {
	name, err := CurrentIdentity()
	if err != nil {
		return err
	}
	return SaveIdentity(name, s)
}

// SaveIdentity writes s as the named identity. When name is the current
// identity, the compatibility mirror is refreshed too; when it is not, the
// mirror is left alone, so authenticating a second identity cannot disturb the
// session the rest of the system is using.
func SaveIdentity(name string, s Session) error {
	store, err := OpenStore()
	if err != nil {
		return err
	}
	if err := store.Set(name, s); err != nil {
		return err
	}
	current, err := CurrentIdentity()
	if err != nil {
		return err
	}
	if current != name {
		return nil
	}
	return saveMirror(s)
}

// saveMirror writes the current identity to $HOME/.nlm/env.
func saveMirror(s Session) error {
	path, err := storePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("nlmauth: create store directory: %w", err)
	}
	return writeSessionFile(path, s)
}

// writeSessionFile writes s to path in the KEY="value" store format.
func writeSessionFile(path string, s Session) error {
	content := fmt.Sprintf(
		"NLM_COOKIES=%q\nNLM_AUTH_TOKEN=%q\nNLM_BROWSER_PROFILE=%q\nNLM_SESSION_ID=%q\nNLM_BL_PARAM=%q\nNLM_SIGNALER_AUTH=%q\nNLM_AUTHUSER=%q\n",
		s.Cookies,
		s.AuthToken,
		s.BrowserProfile,
		s.SessionID,
		s.BLParam,
		s.SignalerAuth,
		s.AuthUser,
	)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("nlmauth: write store: %w", err)
	}
	return nil
}

// Refresh renews the stored session in place: it re-reads NotebookLM page state,
// persists the refreshed session identifiers, and refreshes credentials against
// the Signaler API. It returns [ErrNoCredentials] when no stored cookies exist.
func Refresh() error {
	s, err := LoadSession()
	if err != nil {
		return err
	}
	if s.Cookies == "" {
		return ErrNoCredentials
	}
	state, err := ExtractNotebookLMPageState(s.Cookies)
	if err != nil {
		return fmt.Errorf("nlmauth: refresh page state: %w", err)
	}
	s.SessionID = state.SessionID
	s.BLParam = state.BLParam
	if err := Save(s); err != nil {
		return fmt.Errorf("nlmauth: persist page state: %w", err)
	}
	client, err := NewRefreshClient(s.Cookies)
	if err != nil {
		return err
	}
	if err := client.RefreshCredentials(state.GSessionID); err != nil {
		return fmt.Errorf("nlmauth: refresh credentials: %w", err)
	}
	return nil
}

// readEnvFile parses the shared store into raw KEY=value pairs. A missing file
// yields an empty map and no error; values are unquoted when Go-quoted.
func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("nlmauth: read store: %w", err)
	}

	values := make(map[string]string)
	s := bufio.NewScanner(strings.NewReader(string(data)))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values[key] = value
	}
	return values, nil
}
