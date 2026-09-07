package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tmc/nlm/internal/auth"
	"github.com/tmc/nlm/internal/authuser"
	"github.com/tmc/nlm/nlmauth"
	"golang.org/x/term"
)

var extractNotebookLMPageState = nlmauth.ExtractNotebookLMPageState

// authOptions contains the CLI options for the auth command
type authOptions struct {
	TryAllProfiles  bool
	ListProfiles    bool // Print the browser-profile inventory before authenticating
	ProfileName     string
	TargetURL       string
	CheckNotebooks  bool
	Debug           bool
	Help            bool
	PrintEnv        bool // Print shell-safe export lines for the current session
	KeepOpenSeconds int
	RemoteCDPURL    string
	AuthUser        string // Google account index (0, 1, 2, ...) for multi-account profiles

	// Identity is the name the harvested session is stored as (--as). Empty
	// means the current identity, which is what a single-identity user has.
	Identity string
	Login    bool // explicit login subcommand

	// Subcommand is the identity-management verb, if any: list, use, remove.
	Subcommand string

	// IdentityOperand is the identity named by "use" and "remove".
	IdentityOperand string
}

// authNarration selects the status lines the auth path writes to stderr.
type authNarration int

const (
	// narrateAuth is explicit `nlm auth`: it says what it is doing, with
	// which profile, and where the credentials landed.
	narrateAuth authNarration = iota

	// narrateSilent is a refresh inside another command. The 401 was already
	// announced at the point it was detected, so nothing more is printed and
	// the command's own result stays alone on stdout.
	narrateSilent
)

// getAuthData obtains credentials from a browser profile. It is a variable so
// the script tests can replace the browser with a scripted outcome.
var getAuthData = func(a *auth.BrowserAuth, opts ...auth.Option) (*auth.AuthData, error) {
	return a.GetAuthData(opts...)
}

func runAuth(args []string, globals globalOptions, narration authNarration) (string, string, error) {
	command, ok := lookupCommand("auth")
	if !ok {
		return "", "", fmt.Errorf("auth command not found")
	}
	parsed, err := parseBoundCommand(command, "auth", args, globals)
	if err != nil {
		return "", "", err
	}
	decoded := decodeAuthArgs(parsed)
	decoded.Narration = narration
	return handleDecodedAuth(decoded)
}

func handleDecodedAuth(args authArgs) (string, string, error) {
	raw := args.Raw
	globals := args.Globals

	// Check if help flag is present directly
	for _, arg := range raw {
		if arg == "-h" || arg == "--help" || arg == "-help" || arg == "help" {
			printCommandHelpForPath("auth")
			return "", "", nil // Help was shown, exit gracefully
		}
	}

	// Handle --print-env before any browser or stdin paths so it is safe to run
	// from CI contexts (no TTY, no local profile).
	for _, arg := range raw {
		if arg == "--print-env" || arg == "-print-env" {
			return printAuthEnv(os.Stdout)
		}
	}

	// Identity management never opens a browser and never reads stdin, so it
	// runs before the paths that do.
	if args.FlagError == nil {
		switch args.Options.Subcommand {
		case "list":
			return "", "", listIdentities(os.Stdout)
		case "use":
			return "", "", useIdentityCommand(args.Options.IdentityOperand)
		case "remove":
			return "", "", removeIdentityCommand(args.Options.IdentityOperand)
		}
	}

	isTty := term.IsTerminal(int(os.Stdin.Fd()))

	if globals.debug {
		fmt.Fprintf(os.Stderr, "nlm: debug: stdin is a TTY: %v\n", isTty)
	}

	if args.FlagError != nil {
		return "", "", fmt.Errorf("error parsing auth flags: %w", args.FlagError)
	}
	forceBrowser := args.Options.Login

	// Only parse from stdin if it's not a TTY and we're not forcing browser auth
	if !isTty && !forceBrowser {
		// Check if there's input without blocking
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Parse HAR/curl from stdin
			input, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", "", fmt.Errorf("failed to read stdin: %w", err)
			}

			if len(input) > 0 {
				if globals.debug {
					fmt.Fprintf(os.Stderr, "nlm: debug: parsing auth info from stdin (%d bytes)\n", len(input))
				}
				return detectAuthInfo(string(input), args.Options.Identity)
			} else if globals.debug {
				fmt.Fprintf(os.Stderr, "nlm: debug: stdin is not a TTY but has no data; using browser auth\n")
			}
		} else if globals.debug {
			fmt.Fprintf(os.Stderr, "nlm: debug: stdin is a character device; using browser auth\n")
		}
	}

	isLoginCommand := args.Options.Login
	opts := &args.Options
	if opts.Help {
		printCommandHelpForPath("auth")
		return "", "", nil
	}

	// Say what is about to happen, before the browser window appears.
	if args.Narration == narrateAuth {
		switch {
		case opts.RemoteCDPURL != "":
			fmt.Fprintf(os.Stderr, "nlm: authenticating via remote CDP session at %s...\n", opts.RemoteCDPURL)
		case opts.TryAllProfiles:
			fmt.Fprintf(os.Stderr, "nlm: authenticating via browser (trying all profiles)...\n")
		default:
			fmt.Fprintf(os.Stderr, "nlm: authenticating via browser (%s)...\n", browserIdentity(opts.ProfileName, opts.AuthUser))
		}
	}

	// Use the debug flag from options if set, otherwise use the global debug flag
	useDebug := opts.Debug || globals.debug

	a := auth.New(useDebug)

	// Prepare options for auth call
	// Custom options
	authOpts := []auth.Option{auth.WithTargetURL(opts.TargetURL)}
	if opts.ProfileName == "nlm" && opts.RemoteCDPURL == "" && !opts.TryAllProfiles {
		identity := firstNonEmpty(opts.Identity, activeIdentity, nlmauth.DefaultIdentity)
		if err := nlmauth.ValidateIdentityName(identity); err != nil {
			return "", "", err
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", fmt.Errorf("locate browser profile: %w", err)
		}
		profileDir := filepath.Join(home, ".nlm", "browser", identity)
		authOpts = append(authOpts, func(o *auth.Options) { o.UserDataDir = profileDir })
	}
	if browserAuthAllowed() && opts.RemoteCDPURL == "" && !opts.TryAllProfiles {
		authOpts = append(authOpts, func(o *auth.Options) { o.InteractiveLogin = true })
	}

	if opts.ListProfiles {
		authOpts = append(authOpts, auth.WithListProfiles())
	}

	// Add more verbose output for login command
	if isLoginCommand && useDebug {
		fmt.Fprintf(os.Stderr, "nlm: debug: explicit login mode; using browser authentication\n")
	}

	if opts.TryAllProfiles {
		authOpts = append(authOpts, auth.WithTryAllProfiles())
	} else {
		authOpts = append(authOpts, auth.WithProfileName(opts.ProfileName))
	}

	if opts.CheckNotebooks {
		authOpts = append(authOpts, auth.WithCheckNotebooks())
	}

	if opts.KeepOpenSeconds > 0 {
		authOpts = append(authOpts, auth.WithKeepOpenSeconds(opts.KeepOpenSeconds))
	}

	if opts.RemoteCDPURL != "" {
		authOpts = append(authOpts, auth.WithRemoteCDPURL(opts.RemoteCDPURL))
	}

	if opts.AuthUser != "" {
		authOpts = append(authOpts, auth.WithAuthUser(opts.AuthUser))
	}

	// Get auth data (use GetAuthData to capture session ID and BL param)
	authData, err := getAuthData(a, authOpts...)
	if err != nil {
		return "", "", loginFailure(err, opts.ProfileName, opts.RemoteCDPURL, opts.TargetURL, useDebug)
	}

	authToken, cookies, err := persistAuthToDisk(authData.Cookies, authData.Token, opts.ProfileName, authData.SessionID, authData.BLParam, opts.AuthUser, opts.Identity)
	if err != nil {
		return "", "", err
	}
	if err := persistSignalerAuthorization(authData.SignalerAuth); err != nil {
		return "", "", err
	}
	if args.Narration == narrateAuth {
		reportCredentialsWritten(opts.Identity)
	}
	return authToken, cookies, nil
}

// browserIdentity names the browser profile a login will read, and the Google
// account within it when that is not the profile's default. Both halves matter
// to a multi-account user: the profile picks the cookie jar, the account index
// picks the identity inside it.
func browserIdentity(profileName, authUser string) string {
	if authUser := authuser.Normalize(authUser); authUser != "" {
		return fmt.Sprintf("profile %s, account %s", profileName, authUser)
	}
	return "profile " + profileName
}

// reportCredentialsWritten names the file the credentials landed in. Only
// explicit `nlm auth` prints it: during a silent refresh the path is noise
// between the user's command and its result.
func reportCredentialsWritten(identity string) {
	if identity != "" {
		fmt.Fprintf(os.Stderr, "nlm: credentials written to identity %s\n", identity)
		return
	}
	fmt.Fprintf(os.Stderr, "nlm: credentials written to %s\n", displayPath(storedEnvPath()))
}

// storedEnvPath returns the credential file path, or the bare file name if
// the home directory cannot be determined.
func storedEnvPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".nlm", "env")
	}
	return filepath.Join(homeDir, ".nlm", "env")
}

// displayPath shortens a path under the home directory to ~ form.
func displayPath(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, homeDir+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// printAuthEnv writes POSIX `export KEY=value` lines for the current session
// to w. Only NLM_AUTH_TOKEN and NLM_COOKIES are written; other values
// (profile name, session id, build label, signaler auth) are derived per
// environment and not portable across machines. Diagnostics go to stderr.
//
// The output is suitable for `eval "$(nlm auth --print-env)"` or redirection
// to a file that is later `source`d. It works under bash, zsh, and fish (via
// `eval (nlm auth --print-env)`).
func printAuthEnv(w io.Writer) (string, string, error) {
	authToken := firstNonEmpty(os.Getenv("NLM_AUTH_TOKEN"))
	cookies := firstNonEmpty(os.Getenv("NLM_COOKIES"))
	authUser := firstNonEmpty(os.Getenv("NLM_AUTHUSER"))
	if authToken == "" || cookies == "" {
		stored := readStoredEnv()
		if authToken == "" {
			authToken = stored["NLM_AUTH_TOKEN"]
		}
		if cookies == "" {
			cookies = stored["NLM_COOKIES"]
		}
		if authUser == "" {
			authUser = stored["NLM_AUTHUSER"]
		}
	}
	if authToken == "" || cookies == "" {
		return "", "", fmt.Errorf("no authenticated session found; run 'nlm auth' or export NLM_AUTH_TOKEN and NLM_COOKIES")
	}
	fmt.Fprintf(w, "export NLM_AUTH_TOKEN=%s\n", shellQuote(authToken))
	fmt.Fprintf(w, "export NLM_COOKIES=%s\n", shellQuote(cookies))
	if authUser != "" {
		fmt.Fprintf(w, "export NLM_AUTHUSER=%s\n", shellQuote(authUser))
	}
	return authToken, cookies, nil
}

// shellQuote returns s quoted so it survives a POSIX shell unchanged.
// It uses single-quote wrapping with embedded single quotes escaped as '\”.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b.WriteString(`'\''`)
			continue
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('\'')
	return b.String()
}

func detectAuthInfo(cmd, identity string) (string, string, error) {
	// Extract cookies
	cookieRe := regexp.MustCompile(`-H ['"]cookie: ([^'"]+)['"]`)
	cookieMatch := cookieRe.FindStringSubmatch(cmd)
	if len(cookieMatch) < 2 {
		return "", "", fmt.Errorf("no cookies found in input (looking for cookie header in curl format)")
	}
	cookies := cookieMatch[1]

	// Extract auth token
	atRe := regexp.MustCompile(`at=([^&\s]+)`)
	atMatch := atRe.FindStringSubmatch(cmd)
	if len(atMatch) < 2 {
		return "", "", fmt.Errorf("no auth token found")
	}
	authToken := atMatch[1]
	authToken, cookies, err := persistAuthToDisk(cookies, authToken, "", "", "", "", identity)
	if err != nil {
		return "", "", err
	}
	reportCredentialsWritten(identity)
	return authToken, cookies, nil
}

// persistAuthToDisk writes a harvested session. identity names the identity to
// write; empty means the current one, which is the single-identity case.
func persistAuthToDisk(cookies, authToken, profileName, sessionID, blParam, authUser, identity string) (string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("get home dir: %w", err)
	}

	// Defaults come from the identity being written, not from whichever
	// identity is current: a login for "work" must not inherit "personal"'s
	// profile, session id, or build label.
	if identity == "" {
		identity = activeIdentity
	}
	existing := readStoredEnv()
	value := os.Getenv
	if identity != "" {
		existing = storedIdentityValues(identity)
		// Environment values may have come from another selected identity.
		if identity != activeIdentity {
			value = func(string) string { return "" }
		}
	}
	if profileName == "" {
		profileName = firstNonEmpty(value("NLM_BROWSER_PROFILE"), existing["NLM_BROWSER_PROFILE"])
	}
	if sessionID == "" {
		sessionID = firstNonEmpty(value("NLM_SESSION_ID"), existing["NLM_SESSION_ID"])
	}
	if blParam == "" {
		blParam = firstNonEmpty(value("NLM_BL_PARAM"), existing["NLM_BL_PARAM"])
	}
	signalerAuth := firstNonEmpty(value("NLM_SIGNALER_AUTH"), existing["NLM_SIGNALER_AUTH"])
	authUser = authuser.Normalize(authUser)

	// Create .nlm directory if it doesn't exist
	nlmDir := filepath.Join(homeDir, ".nlm")
	if err := os.MkdirAll(nlmDir, 0700); err != nil {
		return "", "", fmt.Errorf("create .nlm directory: %w", err)
	}

	// Create or update env file
	envFile := filepath.Join(nlmDir, "env")
	values := map[string]string{
		"NLM_COOKIES":         cookies,
		"NLM_AUTH_TOKEN":      authToken,
		"NLM_BROWSER_PROFILE": profileName,
		"NLM_SESSION_ID":      sessionID,
		"NLM_BL_PARAM":        blParam,
		"NLM_SIGNALER_AUTH":   signalerAuth,
		"NLM_AUTHUSER":        authUser,
	}
	if identity != "" {
		// Writing a named identity touches only that identity. The mirror and
		// every other identity are left exactly as they were (R5).
		if err := nlmauth.SaveIdentity(identity, sessionFromStoredValues(values)); err != nil {
			return "", "", err
		}
		// An installation with no usable current identity has just acquired
		// one. Adopting it here is what makes the first `nlm auth login --as
		// work` on a fresh machine leave a working default behind, instead of
		// a stored identity nothing selects.
		if err := adoptIdentityIfUnset(identity); err != nil {
			return "", "", err
		}
	} else if err := writeStoredEnvFile(envFile, values); err != nil {
		return "", "", err
	}

	for key, value := range values {
		if err := os.Setenv(key, value); err != nil {
			return "", "", fmt.Errorf("set %s: %w", key, err)
		}
	}

	return authToken, cookies, nil
}

// storedIdentityValues returns a named identity's stored values in the same
// shape readStoredEnv uses, or nothing when the identity does not exist yet.
func storedIdentityValues(identity string) map[string]string {
	store, err := nlmauth.OpenStore()
	if err != nil {
		return nil
	}
	session, err := store.Get(identity)
	if err != nil {
		return nil
	}
	return map[string]string{
		"NLM_COOKIES":         session.Cookies,
		"NLM_AUTH_TOKEN":      session.AuthToken,
		"NLM_BROWSER_PROFILE": session.BrowserProfile,
		"NLM_SESSION_ID":      session.SessionID,
		"NLM_BL_PARAM":        session.BLParam,
		"NLM_SIGNALER_AUTH":   session.SignalerAuth,
		"NLM_AUTHUSER":        session.AuthUser,
	}
}

// sessionFromStoredValues is the inverse: store keys to a session.
func sessionFromStoredValues(values map[string]string) nlmauth.Session {
	return nlmauth.Session{
		Credentials: nlmauth.Credentials{
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

func persistSignalerAuthorization(authz string) error {
	authz = strings.TrimSpace(authz)
	if authz == "" {
		return nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	envFile := filepath.Join(homeDir, ".nlm", "env")
	values := readStoredEnv()
	if values == nil {
		values = make(map[string]string)
	}
	values["NLM_COOKIES"] = firstNonEmpty(os.Getenv("NLM_COOKIES"), values["NLM_COOKIES"])
	values["NLM_AUTH_TOKEN"] = firstNonEmpty(os.Getenv("NLM_AUTH_TOKEN"), values["NLM_AUTH_TOKEN"])
	values["NLM_BROWSER_PROFILE"] = firstNonEmpty(os.Getenv("NLM_BROWSER_PROFILE"), values["NLM_BROWSER_PROFILE"])
	values["NLM_SESSION_ID"] = firstNonEmpty(os.Getenv("NLM_SESSION_ID"), values["NLM_SESSION_ID"])
	values["NLM_BL_PARAM"] = firstNonEmpty(os.Getenv("NLM_BL_PARAM"), values["NLM_BL_PARAM"])
	values["NLM_AUTHUSER"] = firstNonEmpty(os.Getenv("NLM_AUTHUSER"), values["NLM_AUTHUSER"])
	values["NLM_SIGNALER_AUTH"] = authz
	if err := writeStoredEnvFile(envFile, values); err != nil {
		return err
	}
	if err := os.Setenv("NLM_SIGNALER_AUTH", authz); err != nil {
		return fmt.Errorf("set NLM_SIGNALER_AUTH: %w", err)
	}
	return nil
}

func writeStoredEnvFile(path string, values map[string]string) error {
	if activeIdentity != "" {
		return nlmauth.SaveIdentity(activeIdentity, sessionFromStoredValues(values))
	}
	// The store location and file format are owned by nlmauth.Save; path is
	// retained for call-site compatibility and is always $HOME/.nlm/env.
	_ = path
	return nlmauth.Save(nlmauth.Session{
		Credentials: nlmauth.Credentials{
			AuthToken: values["NLM_AUTH_TOKEN"],
			Cookies:   values["NLM_COOKIES"],
			AuthUser:  values["NLM_AUTHUSER"],
		},
		BrowserProfile: values["NLM_BROWSER_PROFILE"],
		SessionID:      values["NLM_SESSION_ID"],
		BLParam:        values["NLM_BL_PARAM"],
		SignalerAuth:   values["NLM_SIGNALER_AUTH"],
	})
}

func loadStoredEnv() {
	for key, value := range readStoredEnv() {
		// Check if environment variable is explicitly set (including empty string)
		// This respects test environment isolation where env vars are cleared
		if _, isSet := os.LookupEnv(key); isSet {
			continue
		}
		os.Setenv(key, value)
	}
}

func readStoredEnv() map[string]string {
	if activeIdentity != "" {
		return storedIdentityValues(activeIdentity)
	}
	s, err := nlmauth.LoadSession()
	if err != nil || s == (nlmauth.Session{}) {
		return nil
	}
	return map[string]string{
		"NLM_COOKIES":         s.Cookies,
		"NLM_AUTH_TOKEN":      s.AuthToken,
		"NLM_BROWSER_PROFILE": s.BrowserProfile,
		"NLM_SESSION_ID":      s.SessionID,
		"NLM_BL_PARAM":        s.BLParam,
		"NLM_SIGNALER_AUTH":   s.SignalerAuth,
		"NLM_AUTHUSER":        s.AuthUser,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// refreshCredentials refreshes the authentication credentials using Google's signaler API
func refreshCredentials(debugFlag bool) error {
	// Check for -debug flag in os.Args
	debug := debugFlag
	for _, arg := range os.Args {
		if arg == "-debug" || arg == "--debug" {
			debug = true
			break
		}
	}

	// Load stored credentials
	loadStoredEnv()

	cookies := os.Getenv("NLM_COOKIES")
	if cookies == "" {
		return fmt.Errorf("no stored credentials found. Run 'nlm auth' first")
	}
	authToken := os.Getenv("NLM_AUTH_TOKEN")

	// Create refresh client
	refreshClient, err := nlmauth.NewRefreshClient(cookies)
	if err != nil {
		return fmt.Errorf("failed to create refresh client: %w", err)
	}

	if debug {
		refreshClient.SetDebug(true)
		fmt.Fprintf(os.Stderr, "nlm: refreshing credentials...\n")
	}

	state, err := extractNotebookLMPageState(cookies)
	if err != nil {
		return fmt.Errorf("refresh notebooklm page state: %w", err)
	}
	if _, _, err := persistAuthToDisk(cookies, authToken, "", state.SessionID, state.BLParam, authUser, activeIdentity); err != nil {
		return fmt.Errorf("persist notebooklm page state: %w", err)
	}
	gsessionID := state.GSessionID
	if debug {
		fmt.Fprintf(os.Stderr, "nlm: using gsessionid: %s\n", gsessionID)
	}

	// Perform refresh
	if err := refreshClient.RefreshCredentials(gsessionID); err != nil {
		return fmt.Errorf("failed to refresh credentials: %w", err)
	}

	fmt.Fprintf(os.Stderr, "nlm: credentials refreshed successfully\n")
	return nil
}

func refreshNotebookLMPageState(debugFlag bool) error {
	loadStoredEnv()

	cookies := os.Getenv("NLM_COOKIES")
	if cookies == "" {
		return fmt.Errorf("no stored credentials found. Run 'nlm auth' first")
	}

	authToken := os.Getenv("NLM_AUTH_TOKEN")
	state, err := extractNotebookLMPageState(cookies)
	if err != nil {
		return fmt.Errorf("extract notebooklm page state: %w", err)
	}
	if state.SessionID == "" || state.BLParam == "" {
		return fmt.Errorf("incomplete notebooklm page state")
	}
	if debugFlag {
		fmt.Fprintf(os.Stderr, "nlm: refreshed notebooklm page state\n")
		if state.SessionID != "" {
			fmt.Fprintf(os.Stderr, "nlm: session id: %s\n", state.SessionID)
		}
		if state.BLParam != "" {
			fmt.Fprintf(os.Stderr, "nlm: build label: %s\n", state.BLParam)
		}
	}
	if _, _, err := persistAuthToDisk(cookies, authToken, "", state.SessionID, state.BLParam, authUser, activeIdentity); err != nil {
		return fmt.Errorf("persist notebooklm page state: %w", err)
	}
	return nil
}

func refreshNotebookLMSignalerAuthorization(debugFlag bool) (string, error) {
	loadStoredEnv()
	authz := strings.TrimSpace(os.Getenv("NLM_SIGNALER_AUTH"))
	if authz == "" {
		return "", fmt.Errorf("stored signaler authorization not found; run 'nlm auth --cdp-url ws://localhost:9222'")
	}
	if debugFlag {
		fmt.Fprintf(os.Stderr, "nlm: reusing stored notebooklm signaler authorization (%d bytes)\n", len(authz))
	}
	return authz, nil
}

// autoRefreshEnabled reports whether authentication errors may trigger one
// browser-profile re-harvest and command retry.
func autoRefreshEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_AUTO_REFRESH")), "false")
}

// hasCachedBrowserProfile reports whether the stored credentials identify the
// browser profile from which they were harvested. Environment-only credentials
// intentionally do not qualify: there is no local profile to re-harvest.
func hasCachedBrowserProfile() bool {
	_, _, ok := cachedBrowserProfile()
	return ok
}

func cachedBrowserProfile() (profile, authUser string, ok bool) {
	stored := readStoredEnv()
	profile = strings.TrimSpace(stored["NLM_BROWSER_PROFILE"])
	if profile == "" {
		return "", "", false
	}
	account, set := os.LookupEnv("NLM_AUTHUSER")
	if !set {
		account = stored["NLM_AUTHUSER"]
	}
	authUser = authuser.Normalize(account)
	return profile, authUser, true
}

// reharvestCachedBrowserProfile obtains a fresh token and cookies from the
// exact browser profile recorded by the last successful login. The explicit
// login argument prevents runAuth from inspecting command stdin,
// which may belong to the command being retried.
func reharvestCachedBrowserProfile(debugFlag bool) (string, string, error) {
	profile, authUser, ok := cachedBrowserProfile()
	if !ok {
		return "", "", fmt.Errorf("cached browser profile not found")
	}
	return runAuth([]string{"login"}, globalOptions{
		chromeProfile: profile,
		authUser:      authUser,
		authUserSet:   true,
		debug:         debugFlag,
	}, narrateSilent)
}

// adoptIdentityIfUnset makes identity current when nothing usable is current
// yet. An existing selection is never overridden: `nlm auth login --as other`
// refreshes that identity's credentials, it does not switch to it.
func adoptIdentityIfUnset(identity string) error {
	store, err := nlmauth.OpenStore()
	if err != nil {
		return err
	}
	current, err := nlmauth.CurrentIdentity()
	if err == nil && current != "" && current != identity {
		if session, err := store.Get(current); err == nil && session.Cookies != "" {
			return nil
		}
	}
	if err := nlmauth.SetCurrentIdentity(identity); err != nil {
		return err
	}
	session, err := store.Get(identity)
	if err != nil {
		return err
	}
	return nlmauth.SaveIdentity(identity, session)
}
