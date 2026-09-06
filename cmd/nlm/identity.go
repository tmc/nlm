package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/tmc/nlm/nlmauth"
)

// identitySource records how the identity in use was chosen.
// An explicit selection must fail if that identity is unavailable.
type identitySource int

const (
	identityUnset       identitySource = iota // credentials came from the environment
	identityFromFlag                          // --identity
	identityFromEnv                           // NLM_IDENTITY
	identityFromCurrent                       // ~/.nlm/current
)

func (s identitySource) explicit() bool {
	return s == identityFromFlag || s == identityFromEnv
}

// The identity in use, resolved once per invocation in prepareRuntime.
var (
	activeIdentity       string
	activeIdentitySource identitySource
	identityResolveErr   error
)

// storeDir returns the .nlm directory, which holds the identities, the current
// selection and the compatibility mirror.
func storeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nlm"
	}
	return filepath.Join(home, ".nlm")
}

// environmentCredentials reports whether the process environment carried a
// complete session when nlm started. That is the CI path, and it bypasses the
// identity machinery entirely: no store is opened and no identity is resolved.
//
// It is answered once, because publishing an identity fills the same variables
// in: asking again later would report every run as environment-driven.
func environmentCredentials() bool {
	envCredentialsOnce.Do(func() {
		envCredentials = os.Getenv("NLM_AUTH_TOKEN") != "" && os.Getenv("NLM_COOKIES") != ""
	})
	return envCredentials
}

var (
	envCredentialsOnce sync.Once
	envCredentials     bool
)

// resolveIdentityName applies the selection chain that does not depend on the
// command's arguments: --identity, then NLM_IDENTITY, then the current
// identity.
func resolveIdentityName(globals globalOptions) (string, identitySource, error) {
	if name := strings.TrimSpace(globals.identity); globals.identitySet || name != "" {
		if err := nlmauth.ValidateIdentityName(name); err != nil {
			return "", identityUnset, err
		}
		source := identityFromFlag
		if !globals.identitySet {
			source = identityFromEnv
		}
		return name, source, nil
	}
	name, err := nlmauth.CurrentIdentity()
	if err != nil {
		return "", identityUnset, err
	}
	return name, identityFromCurrent, nil
}

// publishIdentityEnv resolves the identity and publishes its session into the
// environment, where the rest of the CLI already reads credentials from.
//
// Values already set in the environment are left alone, so an explicit
// NLM_COOKIES or NLM_BROWSER_PROFILE still wins, and empty identity fields stay empty, so another identity cannot supply them.
func publishIdentityEnv(globals globalOptions) {
	if environmentCredentials() {
		return
	}
	name, source, err := resolveIdentityName(globals)
	if err != nil {
		identityResolveErr = err
		return
	}
	if name == "" {
		return
	}
	store, err := nlmauth.OpenStore()
	if err != nil {
		identityResolveErr = err
		return
	}
	session, err := store.Get(name)
	if err != nil {
		if !errors.Is(err, nlmauth.ErrNoCredentials) {
			identityResolveErr = err
			return
		}
		// A named identity that does not exist is an error: falling back to
		// another account is the quiet wrong-identity bug this design exists
		// to prevent. An absent current identity is not an error, because a
		// fresh installation has none.
		if source.explicit() {
			identityResolveErr = unknownIdentityError(store, name)
		}
		return
	}
	applyIdentitySession(name, source, session)
}

// applyIdentitySession publishes a session as the identity in use.
func applyIdentitySession(name string, source identitySource, session nlmauth.Session) {
	for key, value := range map[string]string{
		"NLM_AUTH_TOKEN":      session.AuthToken,
		"NLM_COOKIES":         session.Cookies,
		"NLM_AUTHUSER":        session.AuthUser,
		"NLM_BROWSER_PROFILE": session.BrowserProfile,
		"NLM_SESSION_ID":      session.SessionID,
		"NLM_BL_PARAM":        session.BLParam,
		"NLM_SIGNALER_AUTH":   session.SignalerAuth,
	} {
		if _, isSet := os.LookupEnv(key); isSet {
			continue
		}
		os.Setenv(key, value)
	}
	activeIdentity = name
	activeIdentitySource = source
}

// unknownIdentityError names the identities that do exist. A bare "not found"
// leaves the user guessing at a name they chose themselves.
func unknownIdentityError(store nlmauth.Store, name string) error {
	names, err := store.List()
	if err != nil || len(names) == 0 {
		return badArgsf("unknown identity %q; none are stored (run: nlm auth login --as %s)", name, name)
	}
	return badArgsf("unknown identity %q; stored identities: %s", name, strings.Join(names, ", "))
}

// listIdentities writes the identity table: who am I, and what else is stored.
func listIdentities(out io.Writer) error {
	store, err := nlmauth.OpenStore()
	if err != nil {
		return err
	}
	names, err := store.List()
	if err != nil {
		return err
	}
	current, err := nlmauth.CurrentIdentity()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintln(out, "no identities stored; run 'nlm auth login' to create one")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROFILE\tACCOUNT\tSTATUS")
	for _, name := range names {
		session, err := store.Get(name)
		if err != nil && !errors.Is(err, nlmauth.ErrNoCredentials) {
			return err
		}
		account := session.AuthUser
		if account == "" {
			account = "-"
		}
		profile := session.BrowserProfile
		if profile == "" {
			profile = "-"
		}
		status := ""
		switch {
		case session.Cookies == "":
			status = "no session (run: nlm auth login --as " + name + ")"
		case name == current:
			status = "current"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, profile, account, status)
	}
	return w.Flush()
}

// useIdentityCommand makes name the current identity.
func useIdentityCommand(name string) error {
	store, err := nlmauth.OpenStore()
	if err != nil {
		return err
	}
	if err := nlmauth.ValidateIdentityName(name); err != nil {
		return err
	}
	if _, err := store.Get(name); err != nil {
		if errors.Is(err, nlmauth.ErrNoCredentials) {
			return unknownIdentityError(store, name)
		}
		return err
	}
	if err := nlmauth.SetCurrentIdentity(name); err != nil {
		return err
	}
	// The mirror follows the current identity, so anything reading
	// $HOME/.nlm/env directly sees the switch too.
	session, err := store.Get(name)
	if err != nil {
		return err
	}
	if err := nlmauth.SaveIdentity(name, session); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "nlm: current identity is now %s\n", name)
	return nil
}

// removeIdentityCommand forgets an identity.
func removeIdentityCommand(name string) error {
	store, err := nlmauth.OpenStore()
	if err != nil {
		return err
	}
	if err := nlmauth.ValidateIdentityName(name); err != nil {
		return err
	}
	if err := store.Delete(name); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "nlm: removed identity %s\n", name)

	current, err := nlmauth.CurrentIdentity()
	if err != nil || current != name {
		return nil
	}
	// The removed identity was the current one, so its credentials are still
	// in the mirror. Leaving them there would keep every later command running
	// as an identity the user just deleted. Move to another stored identity if
	// there is one, and otherwise blank the mirror so the next command asks
	// for a login instead of silently reusing a removed session.
	remaining, err := store.List()
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		next := remaining[0]
		if err := nlmauth.SetCurrentIdentity(next); err != nil {
			return err
		}
		session, err := store.Get(next)
		if err != nil {
			return err
		}
		if err := nlmauth.SaveIdentity(next, session); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "nlm: current identity is now %s\n", next)
		return nil
	}
	for _, file := range []string{"env", "current"} {
		if err := os.Remove(filepath.Join(storeDir(), file)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", file, err)
		}
	}
	fmt.Fprintln(os.Stderr, "nlm: no identities remain; run 'nlm auth login' to create one")
	return nil
}
