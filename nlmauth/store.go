package nlmauth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultIdentity is the identity a store migrated from a single-identity
// installation is named, and the one used when none is selected.
const DefaultIdentity = "default"

// ErrInvalidIdentity reports a name that cannot be used as an identity.
var ErrInvalidIdentity = errors.New("nlmauth: invalid identity name")

// A Store holds named NotebookLM sessions. Implementations are not safe for
// concurrent use across processes: the last writer wins, as with the
// single-identity store it replaces.
type Store interface {
	// Get returns the named session, or ErrNoCredentials if it is absent.
	Get(name string) (Session, error)

	// Set writes the named session, replacing any previous one.
	Set(name string, s Session) error

	// Delete forgets the named session. Deleting an absent identity is not an
	// error.
	Delete(name string) error

	// List returns the stored identity names in sorted order.
	List() ([]string, error)
}

// ValidateIdentityName reports whether name may be used as an identity.
//
// The name is a path element in the file store, so this is a safety check, not
// a style one: without it, "../../x" would write outside the store.
func ValidateIdentityName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty", ErrInvalidIdentity)
	}
	if len(name) > 64 {
		return fmt.Errorf("%w: %q is longer than 64 characters", ErrInvalidIdentity, name)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.':
			if i == 0 {
				return fmt.Errorf("%w: %q must start with a letter or digit", ErrInvalidIdentity, name)
			}
		default:
			return fmt.Errorf("%w: %q may use only lowercase letters, digits, '-', '_' and '.'", ErrInvalidIdentity, name)
		}
	}
	return nil
}

// FileStore keeps one session per identity as $HOME/.nlm/identities/<name>.env,
// in the same KEY="value" format as the single-identity store.
type FileStore struct {
	dir string // the .nlm directory
}

// NewFileStore returns a store rooted at dir, which is the .nlm directory
// itself, not the identities subdirectory.
func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

func (s *FileStore) identitiesDir() string { return filepath.Join(s.dir, "identities") }

func (s *FileStore) path(name string) (string, error) {
	if err := ValidateIdentityName(name); err != nil {
		return "", err
	}
	return filepath.Join(s.identitiesDir(), name+".env"), nil
}

// Get implements [Store].
func (s *FileStore) Get(name string) (Session, error) {
	if err := ValidateIdentityName(name); err != nil {
		return Session{}, err
	}
	if err := s.migrate(); err != nil {
		return Session{}, err
	}
	path, err := s.path(name)
	if err != nil {
		return Session{}, err
	}
	values, err := readEnvFile(path)
	if err != nil {
		return Session{}, fmt.Errorf("nlmauth: read identity %s: %w", name, err)
	}
	// readEnvFile reports a missing file as an empty map, so absence and
	// emptiness are the same case here: there is no session to return.
	if sessionFromValues(values) == (Session{}) {
		return Session{}, ErrNoCredentials
	}
	return sessionFromValues(values), nil
}

// Set implements [Store].
func (s *FileStore) Set(name string, session Session) error {
	if err := ValidateIdentityName(name); err != nil {
		return err
	}
	if err := s.migrate(); err != nil {
		return err
	}
	path, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("nlmauth: create identities directory: %w", err)
	}
	return writeSessionFile(path, session)
}

// Delete implements [Store].
func (s *FileStore) Delete(name string) error {
	if err := ValidateIdentityName(name); err != nil {
		return err
	}
	if err := s.migrate(); err != nil {
		return err
	}
	path, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("nlmauth: remove identity %s: %w", name, err)
	}
	return nil
}

// List implements [Store].
func (s *FileStore) List() ([]string, error) {
	if err := s.migrate(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.identitiesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("nlmauth: list identities: %w", err)
	}
	var names []string
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".env")
		if !ok || entry.IsDir() {
			continue
		}
		if ValidateIdentityName(name) != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// migrate adopts a single-identity installation. The first time a store is
// used on a machine that has only $HOME/.nlm/env, that file becomes the
// "default" identity and the current selection. The env file is copied, not
// moved: it stays as the compatibility mirror everything else still reads.
func (s *FileStore) migrate() error {
	if _, err := os.Stat(s.identitiesDir()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("nlmauth: inspect identities directory: %w", err)
	}
	values, err := readEnvFile(filepath.Join(s.dir, "env"))
	if err != nil {
		return err
	}
	if sessionFromValues(values) == (Session{}) {
		// Nothing to adopt: a fresh installation, not a broken one.
		return nil
	}
	path := filepath.Join(s.identitiesDir(), DefaultIdentity+".env")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("nlmauth: create identities directory: %w", err)
	}
	if err := writeSessionFile(path, sessionFromValues(values)); err != nil {
		return err
	}
	return writeCurrentIdentity(s.dir, DefaultIdentity)
}

// currentPath is the file naming the current identity.
func currentPath(dir string) string { return filepath.Join(dir, "current") }

func writeCurrentIdentity(dir, name string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("nlmauth: create store directory: %w", err)
	}
	if err := os.WriteFile(currentPath(dir), []byte(name+"\n"), 0600); err != nil {
		return fmt.Errorf("nlmauth: write current identity: %w", err)
	}
	return nil
}

func readCurrentIdentity(dir string) (string, error) {
	data, err := os.ReadFile(currentPath(dir))
	if os.IsNotExist(err) {
		return DefaultIdentity, nil
	}
	if err != nil {
		return "", fmt.Errorf("nlmauth: read current identity: %w", err)
	}
	name := strings.TrimSpace(string(data))
	if err := ValidateIdentityName(name); err != nil {
		return "", err
	}
	return name, nil
}

// CurrentIdentity returns the identity used when none is selected explicitly.
// It is [DefaultIdentity] until something sets it.
func CurrentIdentity() (string, error) {
	dir, err := storeDir()
	if err != nil {
		return "", err
	}
	return readCurrentIdentity(dir)
}

// SetCurrentIdentity makes name the identity used when none is selected.
func SetCurrentIdentity(name string) error {
	if err := ValidateIdentityName(name); err != nil {
		return err
	}
	dir, err := storeDir()
	if err != nil {
		return err
	}
	return writeCurrentIdentity(dir, name)
}

// OpenStore returns the credential store named by NLM_CREDENTIAL_STORE:
// "file" (the default).
func OpenStore() (Store, error) {
	dir, err := storeDir()
	if err != nil {
		return nil, err
	}
	file := NewFileStore(dir)
	switch kind := strings.ToLower(strings.TrimSpace(os.Getenv("NLM_CREDENTIAL_STORE"))); kind {
	case "", "file":
		return file, nil
	default:
		return nil, fmt.Errorf("nlmauth: unknown NLM_CREDENTIAL_STORE %q (want \"file\")", kind)
	}
}

// storeDir returns $HOME/.nlm, the directory holding every stored identity.
func storeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("nlmauth: locate home directory: %w", err)
	}
	return filepath.Join(home, ".nlm"), nil
}
