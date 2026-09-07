package auth

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/tmc/nlm/internal/authuser"
)

const appOrigin = "https://notebook.google.com"

type BrowserAuth struct {
	debug            bool
	tempDir          string
	chromeCmd        *exec.Cmd
	cancel           context.CancelFunc
	useExec          bool
	isRemote         bool   // Connected to remote CDP session (skip shutdown)
	interactiveLogin bool   // Allow the user to complete sign-in in a visible browser
	keepOpenSeconds  int    // Keep browser open for N seconds after auth
	sessionID        string // Captured session ID for Jules API
	blParam          string // Build label parameter for Jules API
	signalerAuth     string // Captured signaler Authorization header for punctual APIs
	sourcePath       string // Source path parameter for Jules API
	rtParam          string // RT parameter for Jules API
}

// AuthData holds authentication information extracted from the browser
type AuthData struct {
	Token        string
	Cookies      string
	SessionID    string
	BLParam      string // Build label parameter for Jules API
	SignalerAuth string // Authorization header for NotebookLM signaler APIs
	SourcePath   string // Source path parameter for Jules API
	RTParam      string // RT parameter for Jules API
}

func New(debug bool) *BrowserAuth {
	return &BrowserAuth{
		debug:   debug,
		useExec: false,
	}
}

// GetAuthData returns all authentication data including session ID and Jules parameters
func (ba *BrowserAuth) GetAuthData(opts ...Option) (*AuthData, error) {
	token, cookies, err := ba.GetAuth(opts...)
	if err != nil {
		return nil, err
	}
	return &AuthData{
		Token:        token,
		Cookies:      cookies,
		SessionID:    ba.sessionID,
		BLParam:      ba.blParam,
		SignalerAuth: ba.signalerAuth,
		SourcePath:   ba.sourcePath,
		RTParam:      ba.rtParam,
	}, nil
}

// KeepAlive prevents the browser from being cleaned up automatically.
// Call this before returning from auth if you want to keep the browser session alive.
// You must call Cleanup() manually when done.
func (ba *BrowserAuth) KeepAlive() {
	// Clear the cancel function so cleanup() doesn't close the browser
	ba.cancel = nil
}

// Cleanup manually cleans up browser resources
func (ba *BrowserAuth) Cleanup() {
	ba.cleanup()
}

type Options struct {
	UserDataDir       string // Use this owned directory without scanning or copying browser profiles
	InteractiveLogin  bool   // Show the browser and wait up to five minutes for sign-in
	ProfileName       string
	TryAllProfiles    bool
	ListProfiles      bool
	TargetURL         string
	PreferredBrowsers []string
	CheckNotebooks    bool
	KeepOpenSeconds   int    // Keep browser open for N seconds after auth
	RemoteCDPURL      string // Remote CDP WebSocket URL (e.g. "ws://localhost:9222")
	AuthUser          string // Google account index for multi-account profiles (e.g. "1")
}

type Option func(*Options)

func WithProfileName(p string) Option { return func(o *Options) { o.ProfileName = p } }
func WithTryAllProfiles() Option      { return func(o *Options) { o.TryAllProfiles = true } }
func WithTargetURL(url string) Option { return func(o *Options) { o.TargetURL = url } }

// WithListProfiles prints the browser-profile inventory before authenticating.
// Without it the inventory stays out of the way of the command's own output.
func WithListProfiles() Option { return func(o *Options) { o.ListProfiles = true } }
func WithPreferredBrowsers(browsers []string) Option {
	return func(o *Options) { o.PreferredBrowsers = browsers }
}
func WithCheckNotebooks() Option             { return func(o *Options) { o.CheckNotebooks = true } }
func WithKeepOpenSeconds(seconds int) Option { return func(o *Options) { o.KeepOpenSeconds = seconds } }
func WithRemoteCDPURL(url string) Option     { return func(o *Options) { o.RemoteCDPURL = url } }
func WithAuthUser(authUser string) Option    { return func(o *Options) { o.AuthUser = authUser } }

func defaultBrowserAuthOptions() *Options {
	return &Options{
		ProfileName:       "Default",
		TryAllProfiles:    false,
		ListProfiles:      false,
		TargetURL:         appOrigin,
		PreferredBrowsers: []string{},
		CheckNotebooks:    false,
		KeepOpenSeconds:   0,
	}
}

// authViaRemoteCDP connects to an existing CDP session and extracts auth data.
func (ba *BrowserAuth) authViaRemoteCDP(remoteCDPURL, targetURL string) (token, cookies string, err error) {
	ba.isRemote = true

	ba.debugf("connecting to remote CDP at %s", remoteCDPURL)

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), remoteCDPURL)
	defer allocCancel()

	ctx, ctxCancel := newChromeContext(allocCtx, ba.debug)
	defer ctxCancel()

	ba.debugf("connected; navigating to %s", targetURL)

	return ba.extractAuthDataForURL(ctx, targetURL)
}

// tryMultipleProfiles attempts to authenticate using each profile until one succeeds
func (ba *BrowserAuth) tryMultipleProfiles(targetURL string) (token, cookies string, err error) {
	// Scan all profiles from all browsers
	profiles, err := ba.scanProfiles()
	if err != nil {
		return "", "", fmt.Errorf("scan profiles: %w", err)
	}

	if len(profiles) == 0 {
		return "", "", fmt.Errorf("no valid browser profiles found")
	}

	// Convert to profile names by browser
	type BrowserProfile struct {
		Browser string
		Name    string
		Path    string
	}

	var browserProfiles []BrowserProfile
	for _, p := range profiles {
		browserProfiles = append(browserProfiles, BrowserProfile{
			Browser: p.Browser,
			Name:    p.Name,
			Path:    p.Path,
		})
	}

	// Try each profile
	for _, profile := range profiles {
		ba.debugf("trying profile %s [%s]", profile.Name, profile.Browser)

		// Clean up previous attempts
		ba.cleanup()

		// Check if we should use original profile directory
		useOriginal := os.Getenv("NLM_USE_ORIGINAL_PROFILE")
		ba.debugf("NLM_USE_ORIGINAL_PROFILE=%s", useOriginal)

		var userDataDir string
		if useOriginal == "1" {
			// Use parent directory of the profile path for session continuity
			userDataDir = filepath.Dir(profile.Path)
			ba.debugf("using original profile directory %s", userDataDir)
		} else {
			// Create a temporary directory and copy the profile data
			tempDir, err := os.MkdirTemp("", "nlm-chrome-*")
			if err != nil {
				continue
			}
			ba.tempDir = tempDir
			userDataDir = tempDir

			// Copy the entire profile directory to temp location
			if err := ba.copyProfileDataFromPath(profile.Path); err != nil {
				ba.debugf("copy profile %s: %v", profile.Name, err)
				os.RemoveAll(tempDir)
				continue
			}
		}

		// Set up Chrome and try to authenticate
		var ctx context.Context
		var cancel context.CancelFunc

		// Use chromedp.ExecAllocator approach with stealth flags to avoid detection
		opts := []chromedp.ExecAllocatorOption{
			chromedp.NoFirstRun,
			chromedp.NoDefaultBrowserCheck,
			chromedp.UserDataDir(userDataDir),
			chromedp.Flag("headless", !ba.debug),
			chromedp.Flag("window-size", "1280,800"),
			chromedp.Flag("new-window", true),
			chromedp.Flag("no-first-run", true),
			chromedp.Flag("disable-default-apps", true),
			chromedp.Flag("remote-debugging-port", "0"), // Use random port

			// Anti-detection flags
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.Flag("exclude-switches", "enable-automation"),
			chromedp.Flag("disable-extensions-except", ""),
			chromedp.Flag("disable-plugins-discovery", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Flag("no-sandbox", false), // Keep sandbox enabled for security

			// Make it look more like a regular browser
			chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),

			// Use the appropriate browser executable for this profile type
			chromedp.ExecPath(getBrowserPathForProfile(profile.Browser)),
		}

		// If using original profile, add the specific profile directory flag
		if useOriginal == "1" {
			profileName := filepath.Base(profile.Path)
			if profileName != "Default" {
				opts = append(opts, chromedp.Flag("profile-directory", profileName))
			}
		}

		allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
		ba.cancel = allocCancel
		ctx, cancel = newChromeContext(allocCtx, ba.debug)
		defer cancel()

		// Use a longer timeout (45 seconds) to give more time for login processes
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()

		token, cookies, err = ba.extractAuthDataForURL(ctx, targetURL)
		if err == nil && token != "" {
			ba.debugf("authenticated with profile %s [%s]", profile.Name, profile.Browser)
			return token, cookies, nil
		}

		ba.debugf("profile %s [%s] could not authenticate: %v", profile.Name, profile.Browser, err)
	}

	return "", "", fmt.Errorf("no profiles could authenticate")
}

type ProfileInfo struct {
	Name             string
	Path             string
	LastUsed         time.Time
	Files            []string
	Size             int64
	Browser          string
	HasTargetCookies bool
	TargetDomain     string
	NotebookCount    int
	AuthToken        string
	AuthCookies      string
}

// scanProfiles finds all available Chrome profiles across different browsers
func (ba *BrowserAuth) scanProfiles() ([]ProfileInfo, error) {
	return ba.scanProfilesForDomain("")
}

// scanProfilesForDomain finds all available Chrome profiles and checks for cookies matching the domain
func (ba *BrowserAuth) scanProfilesForDomain(targetDomain string) ([]ProfileInfo, error) {
	var allProfiles []ProfileInfo

	// Check Chrome profiles
	chromePath := getProfilePath()
	chromeProfiles, err := scanBrowserProfiles(chromePath, "Chrome", targetDomain)
	if err == nil {
		allProfiles = append(allProfiles, chromeProfiles...)
	}

	// Check Chrome Canary profiles
	canaryPath := getCanaryProfilePath()
	canaryProfiles, err := scanBrowserProfiles(canaryPath, "Chrome Canary", targetDomain)
	if err == nil {
		allProfiles = append(allProfiles, canaryProfiles...)
	}

	// Check Brave profiles
	bravePath := getBraveProfilePath()
	braveProfiles, err := scanBrowserProfiles(bravePath, "Brave", targetDomain)
	if err == nil {
		allProfiles = append(allProfiles, braveProfiles...)
	}

	// First sort by whether they have target cookies (if a target domain was specified)
	if targetDomain != "" {
		sort.Slice(allProfiles, func(i, j int) bool {
			if allProfiles[i].HasTargetCookies && !allProfiles[j].HasTargetCookies {
				return true
			}
			if !allProfiles[i].HasTargetCookies && allProfiles[j].HasTargetCookies {
				return false
			}
			// Both have or don't have target cookies, so sort by last used
			return allProfiles[i].LastUsed.After(allProfiles[j].LastUsed)
		})
	} else {
		// Sort just by last used (most recent first)
		sort.Slice(allProfiles, func(i, j int) bool {
			return allProfiles[i].LastUsed.After(allProfiles[j].LastUsed)
		})
	}

	return allProfiles, nil
}

// scanBrowserProfiles scans a browser's profile directory for valid profiles
func scanBrowserProfiles(profilePath, browserName string, targetDomain string) ([]ProfileInfo, error) {
	var profiles []ProfileInfo
	entries, err := os.ReadDir(profilePath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip special directories
		if entry.Name() == "System Profile" || entry.Name() == "Guest Profile" {
			continue
		}

		fullPath := filepath.Join(profilePath, entry.Name())

		// Check for key files that indicate it's a valid profile
		validFiles := []string{"Cookies", "Login Data", "History"}
		var foundFiles []string
		var isValid bool
		var totalSize int64

		for _, file := range validFiles {
			filePath := filepath.Join(fullPath, file)
			fileInfo, err := os.Stat(filePath)
			if err == nil {
				foundFiles = append(foundFiles, file)
				totalSize += fileInfo.Size()
				isValid = true
			}
		}

		if !isValid {
			continue
		}

		// Get last modified time as a proxy for "last used"
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		profile := ProfileInfo{
			Name:     entry.Name(),
			Path:     fullPath,
			LastUsed: info.ModTime(),
			Files:    foundFiles,
			Size:     totalSize,
			Browser:  browserName,
		}

		// Check if this profile has cookies for the target domain
		if targetDomain != "" {
			cookiesPath := filepath.Join(fullPath, "Cookies")
			hasCookies := checkProfileForDomainCookies(cookiesPath, targetDomain)
			profile.HasTargetCookies = hasCookies
			profile.TargetDomain = targetDomain
		}

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// checkProfileForDomainCookies checks if a profile's Cookies database contains entries for the target domain
// This function uses the file modification time as a proxy since we can't directly read the SQLite database
// (which would require including SQLite libraries and making database queries)
func checkProfileForDomainCookies(cookiesPath, targetDomain string) bool {
	// Check if the Cookies file exists and is accessible
	cookiesInfo, err := os.Stat(cookiesPath)
	if err != nil {
		return false
	}

	// Check if the file has a reasonable size (not empty)
	if cookiesInfo.Size() < 1000 { // SQLite databases with cookies are typically larger than 1KB
		return false
	}

	// Check if the file was modified recently (within the last 30 days)
	// This is a reasonable proxy for "has active cookies for this domain"
	if time.Since(cookiesInfo.ModTime()) > 30*24*time.Hour {
		return false
	}

	// Since we can't actually check the database content without SQLite,
	// we're making an educated guess based on file size and modification time
	// A more accurate implementation would use SQLite to query the database
	return true
}

// countNotebooks makes a request to list the user's notebooks and counts them
func countNotebooks(token, cookies, authUser string) (int, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create a new request to the notebooks API
	req, err := http.NewRequest("GET", appOrigin+"/gen_notebook/notebook", nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	// Add headers
	req.Header.Add("Cookie", cookies)
	// Public API key from the NotebookLM web client; identifies the app, not the user.
	req.Header.Add("x-goog-api-key", "AIzaSyDRYGVeXVJ5EQwWNjBORFQdrgzjbGsEYg0")
	if authUser = authuser.Normalize(authUser); authUser != "" {
		req.Header.Add("x-goog-authuser", authUser)
	}
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request notebooks: %w", err)
	}
	defer resp.Body.Close()

	// Check response code
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API error: status code %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read response body: %w", err)
	}

	// Simple check for notebook entries
	// This is a simplified approach - a full implementation would parse the JSON properly
	notebooks := strings.Count(string(body), `"notebookId"`)

	return notebooks, nil
}

func (ba *BrowserAuth) GetAuth(opts ...Option) (token, cookies string, err error) {
	o := defaultBrowserAuthOptions()
	for _, opt := range opts {
		opt(o)
	}

	// Append authuser parameter to target URL for multi-account profiles
	if o.AuthUser = authuser.Normalize(o.AuthUser); o.AuthUser != "" {
		if strings.Contains(o.TargetURL, "?") {
			o.TargetURL += "&authuser=" + o.AuthUser
		} else {
			o.TargetURL += "?authuser=" + o.AuthUser
		}
	}

	// Store keep-open setting in the struct
	ba.keepOpenSeconds = o.KeepOpenSeconds
	ba.interactiveLogin = o.InteractiveLogin

	// If a remote CDP URL is provided, connect to it directly
	if o.RemoteCDPURL != "" {
		return ba.authViaRemoteCDP(o.RemoteCDPURL, o.TargetURL)
	}

	defer ba.cleanup()

	// Extract domain from target URL for cookie checks
	targetDomain := ""
	if o.TargetURL != "" {
		if u, err := url.Parse(o.TargetURL); err == nil {
			targetDomain = u.Hostname()
		}
	}

	selectedProfile := &ProfileInfo{Name: o.ProfileName, Browser: "Brave"}
	if o.UserDataDir == "" || o.ListProfiles || o.CheckNotebooks || o.TryAllProfiles {
		profiles, err := ba.scanProfilesForDomain(targetDomain)
		if err != nil {
			return "", "", fmt.Errorf("scan profiles: %w", err)
		}
		sortProfilesByLastUsed(profiles)
		if o.CheckNotebooks {
			profiles = ba.checkNotebookAccess(profiles, o.TargetURL, o.AuthUser)
		}
		if o.UserDataDir == "" {
			selectedProfile = selectProfile(profiles, o.ProfileName)
		}
		if o.wantProfileTable(ba.debug) {
			printProfileTable(os.Stderr, profiles, targetDomain, selectedProfile, o.TryAllProfiles)
		}
		if o.TryAllProfiles {
			return ba.tryMultipleProfiles(o.TargetURL)
		}
	}
	if selectedProfile == nil {
		return "", "", fmt.Errorf("no valid browser profiles found")
	}
	browserPath := getBrowserPathForProfile(selectedProfile.Browser)
	if browserPath == "" {
		return "", "", fmt.Errorf("could not find browser")
	}

	userDataDir := o.UserDataDir
	if userDataDir == "" {
		tempDir, err := os.MkdirTemp("", "nlm-chrome-*")
		if err != nil {
			return "", "", fmt.Errorf("create temp dir: %w", err)
		}
		ba.tempDir = tempDir
		userDataDir = tempDir
		if err := ba.copyProfileDataFromPath(selectedProfile.Path); err != nil {
			return "", "", fmt.Errorf("copy profile: %w", err)
		}
	} else if err := os.MkdirAll(userDataDir, 0700); err != nil {
		return "", "", fmt.Errorf("create browser profile: %w", err)
	}

	var ctx context.Context
	var cancel context.CancelFunc

	// Port 0 enables Chromium's automation mode, which can reject manual
	// Google sign-in. Use a specific loopback port for an interactive browser.
	debugPort := "0"
	if o.InteractiveLogin {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", "", fmt.Errorf("choose browser debugging port: %w", err)
		}
		debugPort = fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
		if err := listener.Close(); err != nil {
			return "", "", fmt.Errorf("release browser debugging port: %w", err)
		}
	}

	// Use chromedp.ExecAllocator approach with minimal automation flags
	chromeOpts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.UserDataDir(userDataDir),
		chromedp.Flag("headless", !ba.debug && !o.InteractiveLogin),
		chromedp.Flag("window-size", "1280,800"),
		chromedp.Flag("new-window", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("remote-debugging-port", debugPort),

		// Use the appropriate browser executable for this profile type
		chromedp.ExecPath(browserPath),
	}

	if o.UserDataDir != "" {
		// Chromium's default crash directory ignores --user-data-dir.
		// Keep crash reporting out of the user's personal browser profile too.
		chromeOpts = append(chromeOpts, chromedp.Flag("breakpad-dump-location", filepath.Join(userDataDir, "Crashpad")))
		// The owned profile does not use another browser's encryption key.
		// Avoid a system keychain prompt before the browser can start CDP.
		chromeOpts = append(chromeOpts, chromedp.Flag("use-mock-keychain", true))
	}

	// The login process must not try to update the installed browser under
	// nlm's TCC identity. This flag affects only this Brave invocation.
	chromeOpts = append(chromeOpts, chromedp.Flag("disable-brave-update", true))
	if runtime.GOOS == "darwin" {
		// Chromium clones its application bundle to survive an in-place update.
		// A short-lived login browser does not need that update helper.
		chromeOpts = append(chromeOpts, chromedp.Flag("disable-features", "MacAppCodeSignClone"))
	}

	ba.debugf("launching %s", browserPath)
	if ba.debug {
		chromeOpts = append(chromeOpts, chromedp.CombinedOutput(browserDebugWriter{}))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), chromeOpts...)
	ba.cancel = allocCancel
	ctx, cancel = newChromeContext(allocCtx, ba.debug)
	defer cancel()

	timeout := 60 * time.Second
	if o.InteractiveLogin {
		timeout = 5 * time.Minute
	}
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	return ba.extractAuthDataForURL(ctx, o.TargetURL)
}

// copyProfileData first resolves the profile name to a path and then calls copyProfileDataFromPath
func (ba *BrowserAuth) copyProfileData(profileName string) error {
	// If profileName is "Default" and it doesn't exist, find the most recently used profile
	profilePath := getProfilePath()
	sourceDir := filepath.Join(profilePath, profileName)

	// Check if the requested profile exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		// First try the same profile name in Chrome Canary
		canaryPath := getCanaryProfilePath()
		canarySourceDir := filepath.Join(canaryPath, profileName)

		if _, err := os.Stat(canarySourceDir); err == nil {
			sourceDir = canarySourceDir
			ba.debugf("using Chrome Canary profile %s", sourceDir)
		} else if profileName == "Default" {
			// If still not found and this is Default, try to find any recent profile
			// Try to find the most recently used profile
			profiles, _ := ba.scanProfiles()
			if len(profiles) > 0 {
				sourceDir = profiles[0].Path
				ba.debugf("profile \"Default\" not found; using most recently used profile %s [%s]",
					profiles[0].Name, profiles[0].Browser)
			} else if foundProfile := findMostRecentProfile(profilePath); foundProfile != "" {
				sourceDir = foundProfile
				ba.debugf("profile \"Default\" not found; using most recently used profile %s", sourceDir)
			}
		}
	}

	return ba.copyProfileDataFromPath(sourceDir)
}

// copyProfileDataFromPath copies profile data from a specific path
func (ba *BrowserAuth) copyProfileDataFromPath(sourceDir string) error {
	ba.debugf("copying profile data from %s", sourceDir)

	// Create Default profile directory
	defaultDir := filepath.Join(ba.tempDir, "Default")
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}

	// Copy only essential files for authentication (not entire profile)
	essentialFiles := []string{
		"Cookies",            // Authentication cookies
		"Cookies-journal",    // Cookie database journal
		"Login Data",         // Saved login information
		"Login Data-journal", // Login database journal
		"Web Data",           // Form data and autofill
		"Web Data-journal",   // Web data journal
		"Preferences",        // Browser preferences
		"Secure Preferences", // Secure browser settings
	}

	copiedCount := 0
	for _, file := range essentialFiles {
		srcPath := filepath.Join(sourceDir, file)
		dstPath := filepath.Join(defaultDir, file)

		// Check if source file exists
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue // Skip if file doesn't exist
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			ba.debugf("copy %s: %v", file, err)
			continue
		}
		copiedCount++
	}

	ba.debugf("copied %d essential files for authentication", copiedCount)

	// Create minimal Local State file
	localState := `{"os_crypt":{"encrypted_key":""}}`
	if err := os.WriteFile(filepath.Join(ba.tempDir, "Local State"), []byte(localState), 0644); err != nil {
		return fmt.Errorf("write local state: %w", err)
	}

	return nil
}

// findMostRecentProfile finds the most recently used profile in the Chrome profile directory
func findMostRecentProfile(profilePath string) string {
	entries, err := os.ReadDir(profilePath)
	if err != nil {
		return ""
	}

	var mostRecent string
	var mostRecentTime time.Time

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip special directories
		if entry.Name() == "System Profile" || entry.Name() == "Guest Profile" {
			continue
		}

		// Check for existence of key files that indicate it's a valid profile
		validFiles := []string{"Cookies", "Login Data", "History"}
		hasValidFiles := false

		for _, file := range validFiles {
			filePath := filepath.Join(profilePath, entry.Name(), file)
			if _, err := os.Stat(filePath); err == nil {
				hasValidFiles = true
				break
			}
		}

		if !hasValidFiles {
			continue
		}

		// Check profile directory's modification time
		fullPath := filepath.Join(profilePath, entry.Name())
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		modTime := info.ModTime()
		if mostRecent == "" || modTime.After(mostRecentTime) {
			mostRecent = fullPath
			mostRecentTime = modTime
		}
	}

	return mostRecent
}

func (ba *BrowserAuth) startChromeExec() (string, error) {
	debugPort := "9222"
	debugURL := fmt.Sprintf("http://localhost:%s", debugPort)

	chromePath := getChromePath()
	if chromePath == "" {
		return "", fmt.Errorf("chrome not found")
	}

	ba.debugf("starting Chrome from %s", chromePath)
	ba.debugf("using profile directory %s", ba.tempDir)

	ba.chromeCmd = exec.Command(chromePath,
		fmt.Sprintf("--remote-debugging-port=%s", debugPort),
		fmt.Sprintf("--user-data-dir=%s", ba.tempDir),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-sync",
		"--window-size=1280,800",
	)

	if ba.debug {
		ba.chromeCmd.Stdout = os.Stdout
		ba.chromeCmd.Stderr = os.Stderr
	}

	if err := ba.chromeCmd.Start(); err != nil {
		return "", fmt.Errorf("start chrome: %w", err)
	}

	if err := ba.waitForDebugger(debugURL); err != nil {
		ba.cleanup()
		return "", err
	}

	return debugURL, nil
}

func (ba *BrowserAuth) waitForDebugger(debugURL string) error {
	statusf("waiting for Chrome debugger...")

	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for chrome debugger")
		case <-ticker.C:
			resp, err := http.Get(debugURL + "/json/version")
			if err == nil {
				resp.Body.Close()
				statusf("Chrome debugger ready")
				return nil
			}
		}
	}
}

func (ba *BrowserAuth) cleanup() {
	if ba.cancel != nil {
		ba.cancel()
	}
	if ba.chromeCmd != nil && ba.chromeCmd.Process != nil {
		ba.chromeCmd.Process.Kill()
	}
	if ba.tempDir != "" {
		os.RemoveAll(ba.tempDir)
	}
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

// copyDirectoryRecursive recursively copies all files and subdirectories from src to dst
func copyDirectoryRecursive(src, dst string, debug bool) error {
	return copyDirectoryRecursiveWithCount(src, dst, debug, nil, nil)
}

// copyDirectoryRecursiveWithCount recursively copies with file counting
func copyDirectoryRecursiveWithCount(src, dst string, debug bool, fileCount, dirCount *int) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read directory %s: %w", src, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Create destination directory
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				if debug {
					debugf("create directory %s: %v", dstPath, err)
				}
				continue
			}

			if dirCount != nil {
				*dirCount++
			}

			// Recursively copy subdirectory
			if err := copyDirectoryRecursiveWithCount(srcPath, dstPath, debug, fileCount, dirCount); err != nil {
				if debug {
					debugf("copy subdirectory %s: %v", srcPath, err)
				}
				continue
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				// Silently skip files that can't be copied
				continue
			}

			if fileCount != nil {
				*fileCount++
			}
		}
	}

	return nil
}

// gracefulShutdown performs a graceful browser shutdown to avoid crash detection
func (ba *BrowserAuth) gracefulShutdown(ctx context.Context) error {
	// Browser.close flushes the owned profile's cookies before the process exits.
	closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return chromedp.Cancel(closeCtx)
}

func (ba *BrowserAuth) extractAuthData(ctx context.Context) (token, cookies string, err error) {
	return ba.extractAuthDataForURL(ctx, appOrigin)
}

func (ba *BrowserAuth) extractAuthDataForURL(ctx context.Context, targetURL string) (token, cookies string, err error) {
	// Navigate and wait for initial page load
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	); err != nil {
		return "", "", fmt.Errorf("failed to load page: %w", err)
	}

	// If keep-open is set, give user time to manually authenticate BEFORE checking
	if ba.keepOpenSeconds > 0 {
		statusf("browser open; you have %d seconds to log in manually if needed", ba.keepOpenSeconds)
		time.Sleep(time.Duration(ba.keepOpenSeconds) * time.Second)
	}

	// First check if we're already on a login page, which would indicate authentication failure
	var currentURL string
	if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err == nil && !ba.interactiveLogin {
		// Log the initial URL we landed on
		ba.debugf("initial navigation landed on %s", currentURL)

		// If we immediately landed on an auth page, this profile is likely not authenticated
		if strings.Contains(currentURL, "accounts.google.com") ||
			strings.Contains(currentURL, "signin") ||
			strings.Contains(currentURL, "login") {
			ba.debugf("redirected to auth page %s", currentURL)

			return "", "", fmt.Errorf("redirected to authentication page - not logged in")
		}
	}

	// Create timeout context for polling - increased timeout for better success with Brave
	pollTimeout := 30 * time.Second
	if ba.interactiveLogin {
		pollTimeout = 5 * time.Minute
	}
	pollCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	authFailCount := 0   // Count consecutive auth failures
	maxAuthFailures := 3 // Max consecutive failures before giving up

	for {
		select {
		case <-pollCtx.Done():
			var finalURL string
			_ = chromedp.Run(ctx, chromedp.Location(&finalURL))

			return "", "", fmt.Errorf("auth data not found after timeout (URL: %s)", finalURL)

		case <-ticker.C:
			token, cookies, err = ba.tryExtractAuth(ctx)
			if err != nil {
				// Count specific failures that indicate we're definitely not authenticated
				if strings.Contains(err.Error(), "sign-in") ||
					strings.Contains(err.Error(), "login") ||
					strings.Contains(err.Error(), "missing essential") {
					authFailCount++

					// If we've had too many clear auth failures, give up earlier
					if authFailCount >= maxAuthFailures && !ba.interactiveLogin {
						return "", "", fmt.Errorf("definitive authentication failure: %w", err)
					}
				}

				if ba.debug {
					deadline, _ := ctx.Deadline()
					debugf("auth check failed: %v (%.1f seconds remaining)", err, time.Until(deadline).Seconds())
				}
				continue
			}

			// Only accept the token and cookies if we get a proper non-empty response
			// and tryExtractAuth has already done its validation
			if token != "" && cookies != "" {
				// Get the final URL to confirm we're on the right page
				var successURL string
				if err := chromedp.Run(ctx, chromedp.Location(&successURL)); err == nil {
					ba.debugf("authenticated at %s", successURL)

					// Double-check we're not on a login page (shouldn't happen with our improved checks)
					if strings.Contains(successURL, "accounts.google.com") ||
						strings.Contains(successURL, "signin") {
						return "", "", fmt.Errorf("authentication appeared to succeed but we're on login page: %s", successURL)
					}
				}

				// Authentication successful - perform graceful shutdown
				ba.debugf("authentication successful")

				// Gracefully close the browser to avoid crash detection (skip for remote CDP sessions)
				if !ba.isRemote {
					if err := ba.gracefulShutdown(ctx); err != nil {
						ba.debugf("graceful shutdown failed: %v", err)
					}
				}

				return token, cookies, nil
			}

			ba.debugf("waiting for auth data...")
		}
	}
}

func (ba *BrowserAuth) tryExtractAuth(ctx context.Context) (token, cookies string, err error) {
	var hasAuth bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`!!window.WIZ_global_data`, &hasAuth),
	)
	if err != nil {
		return "", "", fmt.Errorf("check auth presence: %w", err)
	}

	if !hasAuth {
		return "", "", nil
	}

	// Check if we're on a signin page - this means we're not actually authenticated
	var isSigninPage bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector("form[action^='/signin']") !== null || 
                           document.querySelector("form[action^='/ServiceLogin']") !== null || 
                           document.querySelector("input[type='email']") !== null ||
                           window.location.href.includes("accounts.google.com")`, &isSigninPage),
	)
	if err != nil {
		// If there's an error evaluating, just continue
		ba.debugf("check for signin page: %v", err)
	}

	if isSigninPage {
		// We're on a login page, not actually authenticated
		return "", "", fmt.Errorf("detected sign-in page - not authenticated")
	}

	// Additional check - get current URL to verify we're on the expected domain
	var currentURL string
	err = chromedp.Run(ctx, chromedp.Location(&currentURL))
	if err == nil {
		if strings.Contains(currentURL, "accounts.google.com") ||
			strings.Contains(currentURL, "signin") ||
			strings.Contains(currentURL, "login") {
			return "", "", fmt.Errorf("detected sign-in URL: %s", currentURL)
		}
	}

	// Check for token presence and validity
	var tokenExists bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`typeof WIZ_global_data.SNlM0e === 'string' && 
                          WIZ_global_data.SNlM0e.length > 10`, &tokenExists),
	)
	if err != nil {
		return "", "", fmt.Errorf("check token presence: %w", err)
	}

	if !tokenExists {
		return "", "", fmt.Errorf("token not found or invalid")
	}

	// Extract session ID and Jules-specific parameters (for Jules API)
	var sessionID, blParam, cookieURL string

	// Get current URL to determine which domain's cookies to fetch
	if err := chromedp.Run(ctx, chromedp.Location(&cookieURL)); err == nil && cookieURL != "" {
		// Use current URL for cookies
	} else {
		// Fallback to NotebookLM
		cookieURL = appOrigin
	}

	err = chromedp.Run(ctx,
		chromedp.Evaluate(`WIZ_global_data.SNlM0e`, &token),
		chromedp.Evaluate(`WIZ_global_data.FdrFJe || ""`, &sessionID),
		chromedp.Evaluate(`WIZ_global_data.cfb2h || ""`, &blParam),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cks, err := network.GetCookies().WithUrls([]string{cookieURL}).Do(ctx)
			if err != nil {
				return fmt.Errorf("get cookies: %w", err)
			}

			var cookieStrs []string
			for _, ck := range cks {
				cookieStrs = append(cookieStrs, fmt.Sprintf("%s=%s", ck.Name, ck.Value))
			}
			cookies = strings.Join(cookieStrs, "; ")
			return nil
		}),
	)

	// Store extracted values in struct for later retrieval
	ba.sessionID = sessionID
	ba.blParam = blParam
	ba.sourcePath = "/session" // Default source path for Jules
	ba.rtParam = "c"           // Default rt parameter

	var signalerAuth string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		try {
			if (typeof gapi !== "undefined" && gapi.auth && typeof gapi.auth.getToken === "function") {
				const token = gapi.auth.getToken();
				if (token && token.access_token) {
					return (token.token_type || "Bearer") + " " + token.access_token;
				}
			}
		} catch (_) {}
		try {
			if (typeof gapi !== "undefined" && gapi.auth2 && typeof gapi.auth2.getAuthInstance === "function") {
				const instance = gapi.auth2.getAuthInstance();
				if (instance && instance.isSignedIn && instance.isSignedIn.get()) {
					const response = instance.currentUser.get().getAuthResponse(true);
					if (response && response.access_token) {
						return (response.token_type || "Bearer") + " " + response.access_token;
					}
				}
			}
		} catch (_) {}
		try {
			if (typeof _ !== "undefined" && _ && typeof _.Fu === "function") {
				return _.Fu([]) || "";
			}
		} catch (_) {}
		return "";
	})()`, &signalerAuth)); err != nil {
		ba.debugf("extract signaler authorization: %v", err)
	}
	ba.signalerAuth = strings.TrimSpace(signalerAuth)

	if sessionID != "" {
		ba.debugf("extracted session id %s", sessionID)
	}
	if blParam != "" {
		ba.debugf("extracted build label %s", blParam)
	}
	if ba.signalerAuth != "" {
		ba.debugf("extracted signaler authorization (%d bytes)", len(ba.signalerAuth))
	}
	if err != nil {
		return "", "", fmt.Errorf("extract auth data: %w", err)
	}

	// Validate token format - should be a non-trivial string
	if token == "" || len(token) < 20 {
		return "", "", fmt.Errorf("invalid token format (too short): %s", token)
	}

	// Validate cookies - we should have some essential cookies
	if cookies == "" || len(cookies) < 50 {
		return "", "", fmt.Errorf("insufficient cookies data")
	}

	// Check for specific cookies that should be present when authenticated
	requiredCookies := []string{"SID", "HSID", "SSID", "APISID"}
	var foundRequired bool
	for _, required := range requiredCookies {
		if strings.Contains(cookies, required+"=") {
			foundRequired = true
			break
		}
	}

	if !foundRequired {
		return "", "", fmt.Errorf("missing essential authentication cookies")
	}

	return token, cookies, nil
}

// DownloadWithBrowser downloads a file using the browser with profile authentication
// This is useful for downloading files from Google CDN that require session cookies
func (ba *BrowserAuth) DownloadWithBrowser(urlToDownload string, profileName string) ([]byte, error) {
	// Find the profile to use
	profiles, err := ba.scanProfiles()
	if err != nil {
		return nil, fmt.Errorf("scan profiles: %w", err)
	}

	var selectedProfile *ProfileInfo
	for _, p := range profiles {
		if p.Name == profileName || profileName == "" {
			selectedProfile = &p
			break
		}
	}

	if selectedProfile == nil && len(profiles) > 0 {
		selectedProfile = &profiles[0] // Use most recently used
	}

	if selectedProfile == nil {
		return nil, fmt.Errorf("no valid profiles found")
	}

	// Create temp dir and copy profile
	tempDir, err := os.MkdirTemp("", "nlm-download-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	ba.tempDir = tempDir
	defer ba.cleanup()

	if err := ba.copyProfileDataFromPath(selectedProfile.Path); err != nil {
		return nil, fmt.Errorf("copy profile: %w", err)
	}

	// Set up chromedp
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.UserDataDir(ba.tempDir),
		chromedp.Flag("headless", true),
		chromedp.ExecPath(getBrowserPathForProfile(selectedProfile.Browser)),
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := newChromeContext(allocCtx, ba.debug)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Use CDP Network domain to capture the response
	var responseBody []byte
	var responseReceived bool

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventResponseReceived:
			if ev.Response.URL == urlToDownload {
				responseReceived = true
			}
		case *network.EventLoadingFinished:
			if responseReceived {
				// Get the response body
				go func() {
					body, err := network.GetResponseBody(ev.RequestID).Do(ctx)
					if err == nil {
						responseBody = body
					}
				}()
			}
		}
	})

	// Enable network and navigate to URL
	if err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(urlToDownload),
	); err != nil {
		return nil, fmt.Errorf("navigate to URL: %w", err)
	}

	// Wait for response or timeout
	time.Sleep(2 * time.Second)

	if len(responseBody) == 0 {
		return nil, fmt.Errorf("failed to download: no response body captured")
	}

	return responseBody, nil
}

// ReadTextWithRemoteBrowser downloads a text URL through an existing browser
// debugging session. It is useful when a download host requires the browser's
// live session cookies rather than the copied cookie header used by the CLI.
func (ba *BrowserAuth) ReadTextWithRemoteBrowser(urlToRead, remoteCDPURL string) ([]byte, error) {
	if remoteCDPURL == "" {
		return nil, fmt.Errorf("missing remote CDP URL")
	}
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), remoteCDPURL)
	defer allocCancel()

	ctx, cancel := newChromeContext(allocCtx, ba.debug)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	downloadDir, err := os.MkdirTemp("", "nlm-remote-download-*")
	if err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}
	defer os.RemoveAll(downloadDir)

	if err := chromedp.Run(ctx, browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorAllowAndName).
		WithDownloadPath(downloadDir).
		WithEventsEnabled(true)); err != nil {
		return nil, fmt.Errorf("configure browser download: %w", err)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(urlToRead)); err != nil && !strings.Contains(err.Error(), "net::ERR_ABORTED") {
		return nil, fmt.Errorf("navigate to download: %w", err)
	}
	wake := time.NewTicker(100 * time.Millisecond)
	defer wake.Stop()
	for {
		entries, err := os.ReadDir(downloadDir)
		if err != nil {
			return nil, fmt.Errorf("read download directory: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasSuffix(entry.Name(), ".crdownload") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(downloadDir, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("read downloaded file: %w", err)
			}
			return data, nil
		}
		select {
		case <-wake.C:
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for download: %w", ctx.Err())
		}
	}
}
