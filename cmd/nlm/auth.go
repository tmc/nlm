package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tmc/nlm/internal/auth"
	"golang.org/x/term"
)

// maskProfileName masks sensitive profile names in debug output
func maskProfileName(profile string) string {
	if profile == "" {
		return ""
	}
	if len(profile) > 8 {
		return profile[:4] + "****" + profile[len(profile)-4:]
	} else if len(profile) > 2 {
		return profile[:2] + "****"
	}
	return "****"
}

// AuthOptions contains the CLI options for the auth command
type AuthOptions struct {
	TryAllProfiles  bool
	ProfileName     string
	TargetURL       string
	CheckNotebooks  bool
	Debug           bool
	Help            bool
	KeepOpenSeconds int
}

func parseAuthFlags(args []string) (*AuthOptions, []string, error) {
	// Create a new FlagSet
	authFlags := flag.NewFlagSet("auth", flag.ContinueOnError)

	// Define auth-specific flags
	opts := &AuthOptions{
		ProfileName: chromeProfile,
		TargetURL:   "https://notebooklm.google.com",
	}

	authFlags.BoolVar(&opts.TryAllProfiles, "all", false, "Try all available browser profiles")
	authFlags.BoolVar(&opts.TryAllProfiles, "a", false, "Try all available browser profiles (shorthand)")
	authFlags.StringVar(&opts.ProfileName, "profile", opts.ProfileName, "Specific Chrome profile to use")
	authFlags.StringVar(&opts.ProfileName, "p", opts.ProfileName, "Specific Chrome profile to use (shorthand)")
	authFlags.StringVar(&opts.TargetURL, "url", opts.TargetURL, "Target URL to authenticate against")
	authFlags.StringVar(&opts.TargetURL, "u", opts.TargetURL, "Target URL to authenticate against (shorthand)")
	authFlags.BoolVar(&opts.CheckNotebooks, "notebooks", false, "Check notebook count for profiles")
	authFlags.BoolVar(&opts.CheckNotebooks, "n", false, "Check notebook count for profiles (shorthand)")
	authFlags.BoolVar(&opts.Debug, "debug", debug, "Enable debug output")
	authFlags.BoolVar(&opts.Debug, "d", debug, "Enable debug output (shorthand)")
	authFlags.BoolVar(&opts.Help, "help", false, "Show help for auth command")
	authFlags.BoolVar(&opts.Help, "h", false, "Show help for auth command (shorthand)")
	authFlags.IntVar(&opts.KeepOpenSeconds, "keep-open", 0, "Keep browser open for N seconds after successful auth")
	authFlags.IntVar(&opts.KeepOpenSeconds, "k", 0, "Keep browser open for N seconds after successful auth (shorthand)")

	// Set custom usage
	authFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: nlm auth [login] [options] [profile-name]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  login            Explicitly use browser authentication (recommended)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		authFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample: nlm auth login -all -notebooks\n")
		fmt.Fprintf(os.Stderr, "Example: nlm auth login -profile Work\n")
		fmt.Fprintf(os.Stderr, "Example: nlm auth login -keep-open 10\n")
		fmt.Fprintf(os.Stderr, "Example: nlm auth -all\n")
	}

	// Filter out the 'login' argument if present
	filteredArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "login" {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	// Parse the flags
	err := authFlags.Parse(filteredArgs)
	if err != nil {
		return nil, nil, err
	}

	// If help is requested, show usage and return nil
	if opts.Help {
		authFlags.Usage()
		return nil, nil, fmt.Errorf("help shown")
	}

	// Remaining arguments after flag parsing
	remainingArgs := authFlags.Args()

	// If there's an argument and no specific profile is set via flag, treat the first arg as profile name
	if !opts.TryAllProfiles && opts.ProfileName == "" && len(remainingArgs) > 0 {
		opts.ProfileName = remainingArgs[0]
		remainingArgs = remainingArgs[1:]
	}

	// Set default profile name if needed
	if !opts.TryAllProfiles && opts.ProfileName == "" {
		opts.ProfileName = "Default"
		if v := os.Getenv("NLM_BROWSER_PROFILE"); v != "" {
			opts.ProfileName = v
		}
	}

	return opts, remainingArgs, nil
}

func handleAuth(args []string, debug bool) (string, string, error) {
	// Check if help flag is present directly
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" || arg == "help" {
			// Parse auth-specific flags which will display help
			parseAuthFlags([]string{"--help"})
			return "", "", nil // Help was shown, exit gracefully
		}
	}

	isTty := term.IsTerminal(int(os.Stdin.Fd()))

	if debug {
		fmt.Fprintf(os.Stderr, "Input is from a TTY: %v\n", isTty)
	}

	// Look for 'login' command which forces browser auth
	forceBrowser := false
	for _, arg := range args {
		if arg == "login" {
			forceBrowser = true
			if debug {
				fmt.Fprintf(os.Stderr, "Found 'login' command, forcing browser authentication\n")
			}
			break
		}
	}

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
				if debug {
					fmt.Fprintf(os.Stderr, "Parsing auth info from stdin input (%d bytes)\n", len(input))
				}
				return detectAuthInfo(string(input))
			} else if debug {
				fmt.Fprintf(os.Stderr, "Stdin is not a TTY but has no data, proceeding to browser auth\n")
			}
		} else if debug {
			fmt.Fprintf(os.Stderr, "Stdin is not a TTY but is a character device, proceeding to browser auth\n")
		}
	}

	// Check for login subcommand which explicitly indicates browser auth
	isLoginCommand := false
	for _, arg := range args {
		if arg == "login" {
			isLoginCommand = true
			break
		}
	}

	// Parse auth-specific flags
	opts, _, err := parseAuthFlags(args)
	if err != nil {
		if err.Error() == "help shown" {
			return "", "", nil // Help was shown, exit gracefully
		}
		return "", "", fmt.Errorf("error parsing auth flags: %w", err)
	}

	// Show what we're going to do based on options
	if opts.TryAllProfiles {
		fmt.Fprintf(os.Stderr, "nlm: trying all browser profiles to find one with valid authentication...\n")
	} else {
		// Mask potentially sensitive profile name
		maskedProfile := maskProfileName(opts.ProfileName)
		fmt.Fprintf(os.Stderr, "nlm: launching browser to login... (profile:%v)\n", maskedProfile)
	}

	// Use the debug flag from options if set, otherwise use the global debug flag
	useDebug := opts.Debug || debug

	a := auth.New(useDebug)

	// Prepare options for auth call
	// Custom options
	authOpts := []auth.Option{auth.WithScanBeforeAuth(), auth.WithTargetURL(opts.TargetURL)}

	// Add more verbose output for login command
	if isLoginCommand && useDebug {
		fmt.Fprintf(os.Stderr, "Using explicit login mode with browser authentication\n")
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

	// Get auth data
	token, cookies, err := a.GetAuth(authOpts...)
	if err != nil {
		return "", "", fmt.Errorf("browser auth failed: %w", err)
	}

	return persistAuthToDisk(cookies, token, opts.ProfileName)
}

func detectAuthInfo(cmd string) (string, string, error) {
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
	persistAuthToDisk(cookies, authToken, "")
	return authToken, cookies, nil
}

func persistAuthToDisk(cookies, authToken, profileName string) (string, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("get home dir: %w", err)
	}

	// Create .nlm directory if it doesn't exist
	nlmDir := filepath.Join(homeDir, ".nlm")
	if err := os.MkdirAll(nlmDir, 0700); err != nil {
		return "", "", fmt.Errorf("create .nlm directory: %w", err)
	}

	// Create or update env file
	envFile := filepath.Join(nlmDir, "env")
	content := fmt.Sprintf("NLM_COOKIES=%q\nNLM_AUTH_TOKEN=%q\nNLM_BROWSER_PROFILE=%q\n",
		cookies,
		authToken,
		profileName,
	)

	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		return "", "", fmt.Errorf("write env file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "nlm: auth info written to %s\n", envFile)
	return authToken, cookies, nil
}

func loadStoredEnv() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	data, err := os.ReadFile(filepath.Join(home, ".nlm", "env"))
	if err != nil {
		return
	}

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
		// Check if environment variable is explicitly set (including empty string)
		// This respects test environment isolation where env vars are cleared
		if _, isSet := os.LookupEnv(key); isSet {
			continue
		}

		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		os.Setenv(key, value)
	}
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

	// Create refresh client
	refreshClient, err := auth.NewRefreshClient(cookies)
	if err != nil {
		return fmt.Errorf("failed to create refresh client: %w", err)
	}

	if debug {
		refreshClient.SetDebug(true)
		fmt.Fprintf(os.Stderr, "nlm: refreshing credentials...\n")
	}

	// For now, use a hardcoded gsessionid from the user's example
	// TODO: Extract this dynamically from the NotebookLM page
	gsessionID := "LsWt3iCG3ezhLlQau_BO2Gu853yG1uLi0RnZlSwqVfg"
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
