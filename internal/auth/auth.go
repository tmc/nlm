package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type BrowserAuth struct {
	debug           bool
	tempDir         string
	chromeCmd       *exec.Cmd
	cancel          context.CancelFunc
	useExec         bool
	keepOpenSeconds int // Keep browser open for N seconds after auth
}

func New(debug bool) *BrowserAuth {
	return &BrowserAuth{
		debug:   debug,
		useExec: false,
	}
}

type Options struct {
	ProfileName       string
	TryAllProfiles    bool
	ScanBeforeAuth    bool
	TargetURL         string
	PreferredBrowsers []string
	CheckNotebooks    bool
	KeepOpenSeconds   int // Keep browser open for N seconds after auth
}

type Option func(*Options)

func WithProfileName(p string) Option { return func(o *Options) { o.ProfileName = p } }
func WithTryAllProfiles() Option      { return func(o *Options) { o.TryAllProfiles = true } }
func WithScanBeforeAuth() Option      { return func(o *Options) { o.ScanBeforeAuth = true } }
func WithTargetURL(url string) Option { return func(o *Options) { o.TargetURL = url } }
func WithPreferredBrowsers(browsers []string) Option {
	return func(o *Options) { o.PreferredBrowsers = browsers }
}
func WithCheckNotebooks() Option             { return func(o *Options) { o.CheckNotebooks = true } }
func WithKeepOpenSeconds(seconds int) Option { return func(o *Options) { o.KeepOpenSeconds = seconds } }

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
		if ba.debug {
			fmt.Printf("Trying profile: %s [%s]\n", profile.Name, profile.Browser)
		}

		// Clean up previous attempts
		ba.cleanup()

		// Check if we should use original profile directory
		useOriginal := os.Getenv("NLM_USE_ORIGINAL_PROFILE")
		if ba.debug {
			fmt.Printf("NLM_USE_ORIGINAL_PROFILE=%s\n", useOriginal)
		}

		var userDataDir string
		if useOriginal == "1" {
			// Use parent directory of the profile path for session continuity
			userDataDir = filepath.Dir(profile.Path)
			if ba.debug {
				fmt.Printf("Using original profile directory: %s\n", userDataDir)
			}
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
				if ba.debug {
					fmt.Printf("Error copying profile %s: %v\n", profile.Name, err)
				}
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
		ctx, cancel = chromedp.NewContext(allocCtx)
		defer cancel()

		// Use a longer timeout (45 seconds) to give more time for login processes
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()

		if ba.debug {
			ctx, _ = chromedp.NewContext(ctx, chromedp.WithLogf(func(format string, args ...interface{}) {
				fmt.Printf("ChromeDP: "+format+"\n", args...)
			}))
		}

		token, cookies, err = ba.extractAuthDataForURL(ctx, targetURL)
		if err == nil && token != "" {
			if ba.debug {
				fmt.Printf("Successfully authenticated with profile: %s [%s]\n", profile.Name, profile.Browser)
			}
			return token, cookies, nil
		}

		if ba.debug {
			fmt.Printf("Profile %s [%s] could not authenticate: %v\n", profile.Name, profile.Browser, err)
		}
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
func countNotebooks(token, cookies string) (int, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create a new request to the notebooks API
	req, err := http.NewRequest("GET", "https://notebooklm.google.com/gen_notebook/notebook", nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	// Add headers
	req.Header.Add("Cookie", cookies)
	req.Header.Add("x-goog-api-key", "AIzaSyDRYGVeXVJ5EQwWNjBORFQdrgzjbGsEYg0")
	req.Header.Add("x-goog-authuser", "0")
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
	o := &Options{
		ProfileName:       "Default",
		TryAllProfiles:    false,
		ScanBeforeAuth:    true, // Default to showing profile information
		TargetURL:         "https://notebooklm.google.com",
		PreferredBrowsers: []string{},
		CheckNotebooks:    false,
		KeepOpenSeconds:   0,
	}
	for _, opt := range opts {
		opt(o)
	}

	// Store keep-open setting in the struct
	ba.keepOpenSeconds = o.KeepOpenSeconds

	defer ba.cleanup()

	// Extract domain from target URL for cookie checks
	targetDomain := ""
	if o.TargetURL != "" {
		if u, err := url.Parse(o.TargetURL); err == nil {
			targetDomain = u.Hostname()
		}
	}

	// If scan is requested, show available profiles
	if o.ScanBeforeAuth {
		profiles, err := ba.scanProfilesForDomain(targetDomain)
		if err != nil {
			return "", "", fmt.Errorf("scan profiles: %w", err)
		}

		// If requested, check notebooks for each profile that has valid cookies
		if o.CheckNotebooks {
			fmt.Println("Checking notebook access for profiles...")

			// Create a pool of profiles to check
			var profilesToCheck []ProfileInfo
			for _, p := range profiles {
				if p.HasTargetCookies {
					profilesToCheck = append(profilesToCheck, p)
				}
			}

			// Check a maximum of 5 profiles to avoid taking too long
			maxToCheck := 5
			if len(profilesToCheck) > maxToCheck {
				profilesToCheck = profilesToCheck[:maxToCheck]
			}

			// Process each profile to check for notebook access
			updatedProfiles := make([]ProfileInfo, 0, len(profiles))
			for _, p := range profiles {
				// Only check profiles with target cookies that are in our check list
				shouldCheck := false
				for _, check := range profilesToCheck {
					if p.Path == check.Path {
						shouldCheck = true
						break
					}
				}

				if shouldCheck {
					fmt.Printf("  Checking notebooks for %s [%s]...", p.Name, p.Browser)

					// Set up a temporary Chrome instance to authenticate
					tempDir, err := os.MkdirTemp("", "nlm-notebook-check-*")
					if err != nil {
						fmt.Println(" Error: could not create temp dir")
						updatedProfiles = append(updatedProfiles, p)
						continue
					}

					// Create a temporary BrowserAuth
					tempAuth := &BrowserAuth{
						debug:   false,
						tempDir: tempDir,
					}
					defer os.RemoveAll(tempDir)

					// Copy profile data
					err = tempAuth.copyProfileDataFromPath(p.Path)
					if err != nil {
						fmt.Println(" Error: could not copy profile data")
						updatedProfiles = append(updatedProfiles, p)
						continue
					}

					// Try to authenticate
					authCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

					// Set up Chrome
					opts := []chromedp.ExecAllocatorOption{
						chromedp.NoFirstRun,
						chromedp.NoDefaultBrowserCheck,
						chromedp.DisableGPU,
						chromedp.Flag("disable-extensions", true),
						chromedp.Flag("headless", true),
						chromedp.UserDataDir(tempDir),
					}

					allocCtx, allocCancel := chromedp.NewExecAllocator(authCtx, opts...)
					defer allocCancel()

					ctx, ctxCancel := chromedp.NewContext(allocCtx)
					defer ctxCancel()

					// Try to authenticate
					token, cookies, err := tempAuth.extractAuthDataForURL(ctx, o.TargetURL)
					cancel()

					if err != nil || token == "" {
						fmt.Println(" Not authenticated")
						updatedProfiles = append(updatedProfiles, p)
						continue
					}

					// Store auth data
					profile := p
					profile.AuthToken = token
					profile.AuthCookies = cookies

					// Try to get notebooks
					notebookCount, err := countNotebooks(token, cookies)
					if err != nil {
						fmt.Println(" Error counting notebooks")
						updatedProfiles = append(updatedProfiles, profile)
						continue
					}

					profile.NotebookCount = notebookCount
					fmt.Printf(" Found %d notebooks\n", notebookCount)
					updatedProfiles = append(updatedProfiles, profile)
				} else {
					// Skip notebook check for this profile
					updatedProfiles = append(updatedProfiles, p)
				}
			}

			// Replace profiles with updated ones
			profiles = updatedProfiles
		}

		// Show profile information
		fmt.Println("Available browser profiles:")
		fmt.Println("===========================")
		for _, p := range profiles {
			cookieStatus := ""
			if targetDomain != "" {
				if p.HasTargetCookies {
					cookieStatus = fmt.Sprintf(" [✓ Has %s cookies]", targetDomain)
				} else {
					cookieStatus = fmt.Sprintf(" [✗ No %s cookies]", targetDomain)
				}
			}

			notebookStatus := ""
			if p.NotebookCount > 0 {
				notebookStatus = fmt.Sprintf(" [%d notebooks]", p.NotebookCount)
			}

			fmt.Printf("%d. %s [%s] - Last used: %s (%d files, %.1f MB)%s%s\n",
				1, p.Name, p.Browser,
				p.LastUsed.Format("2006-01-02 15:04:05"),
				len(p.Files),
				float64(p.Size)/(1024*1024),
				cookieStatus,
				notebookStatus)
		}
		fmt.Println("===========================")

		if o.TryAllProfiles {
			fmt.Println("Will try profiles in order shown above...")
		} else {
			fmt.Printf("Using profile: %s\n", o.ProfileName)
		}
		fmt.Println()
	}

	// If trying all profiles, try to find one that works
	if o.TryAllProfiles {
		return ba.tryMultipleProfiles(o.TargetURL)
	}

	// Find the actual profile to use (similar to multi-profile approach)
	profiles, err := ba.scanProfiles()
	if err != nil {
		return "", "", fmt.Errorf("scan profiles: %w", err)
	}

	// Find the profile that matches the requested name
	var selectedProfile *ProfileInfo
	for _, p := range profiles {
		if p.Name == o.ProfileName {
			selectedProfile = &p
			break
		}
	}

	// If no exact match, use the first profile (most recently used)
	if selectedProfile == nil && len(profiles) > 0 {
		selectedProfile = &profiles[0]
		if ba.debug {
			fmt.Printf("Profile '%s' not found, using most recently used profile: %s [%s]\n",
				o.ProfileName, selectedProfile.Name, selectedProfile.Browser)
		}
	}

	if selectedProfile == nil {
		return "", "", fmt.Errorf("no valid profiles found")
	}

	// Create a temporary directory and copy profile data to preserve encryption keys
	tempDir, err := os.MkdirTemp("", "nlm-chrome-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}
	ba.tempDir = tempDir

	// Copy the profile data
	if err := ba.copyProfileDataFromPath(selectedProfile.Path); err != nil {
		return "", "", fmt.Errorf("copy profile: %w", err)
	}

	var ctx context.Context
	var cancel context.CancelFunc

	// Use chromedp.ExecAllocator approach with minimal automation flags
	chromeOpts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.UserDataDir(ba.tempDir),
		chromedp.Flag("headless", !ba.debug),
		chromedp.Flag("window-size", "1280,800"),
		chromedp.Flag("new-window", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("remote-debugging-port", "0"), // Use random port

		// Use the appropriate browser executable for this profile type
		chromedp.ExecPath(getBrowserPathForProfile(selectedProfile.Browser)),
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), chromeOpts...)
	ba.cancel = allocCancel
	ctx, cancel = chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if ba.debug {
		ctx, _ = chromedp.NewContext(ctx, chromedp.WithLogf(func(format string, args ...interface{}) {
			fmt.Printf("ChromeDP: "+format+"\n", args...)
		}))
	}

	return ba.extractAuthData(ctx)
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
			if ba.debug {
				fmt.Printf("Using Chrome Canary profile: %s\n", sourceDir)
			}
		} else if profileName == "Default" {
			// If still not found and this is Default, try to find any recent profile
			// Try to find the most recently used profile
			profiles, _ := ba.scanProfiles()
			if len(profiles) > 0 {
				sourceDir = profiles[0].Path
				if ba.debug {
					fmt.Printf("Profile 'Default' not found, using most recently used profile: %s [%s]\n",
						profiles[0].Name, profiles[0].Browser)
				}
			} else if foundProfile := findMostRecentProfile(profilePath); foundProfile != "" {
				sourceDir = foundProfile
				if ba.debug {
					fmt.Printf("Profile 'Default' not found, using most recently used profile: %s\n", sourceDir)
				}
			}
		}
	}

	return ba.copyProfileDataFromPath(sourceDir)
}

// copyProfileDataFromPath copies profile data from a specific path
func (ba *BrowserAuth) copyProfileDataFromPath(sourceDir string) error {
	if ba.debug {
		fmt.Printf("Copying profile data from: %s\n", sourceDir)
	}

	// Create Default profile directory
	defaultDir := filepath.Join(ba.tempDir, "Default")
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}

	// Copy only essential files for authentication (not entire profile)
	essentialFiles := []string{
		"Cookies",           // Authentication cookies
		"Cookies-journal",   // Cookie database journal
		"Login Data",        // Saved login information
		"Login Data-journal", // Login database journal
		"Web Data",          // Form data and autofill
		"Web Data-journal",  // Web data journal
		"Preferences",       // Browser preferences
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
			if ba.debug {
				fmt.Printf("Warning: Failed to copy %s: %v\n", file, err)
			}
			continue
		}
		copiedCount++
	}

	if ba.debug {
		fmt.Printf("Copied %d essential files for authentication\n", copiedCount)
	}

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

	if ba.debug {
		fmt.Printf("Starting Chrome from: %s\n", chromePath)
		fmt.Printf("Using profile: %s\n", ba.tempDir)
	}

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
	fmt.Println("Waiting for Chrome debugger...")

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
				fmt.Println("Chrome debugger ready")
				return nil
			}
			if ba.debug {
				fmt.Printf(".")
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
					fmt.Printf("Failed to create directory %s: %v\n", dstPath, err)
				}
				continue
			}

			if dirCount != nil {
				*dirCount++
			}

			// Recursively copy subdirectory
			if err := copyDirectoryRecursiveWithCount(srcPath, dstPath, debug, fileCount, dirCount); err != nil {
				if debug {
					fmt.Printf("Failed to copy subdirectory %s: %v\n", srcPath, err)
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
	// First try to close all tabs gracefully using JavaScript
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`
			// Try to close the window gracefully
			if (window.close) {
				window.close();
			}
			// Set a flag that we're closing normally
			window.localStorage.setItem('normal_shutdown', 'true');
		`, nil),
	)

	// Give the browser a moment to process the close
	time.Sleep(100 * time.Millisecond)

	// Now cancel the context which will close the browser
	if ba.cancel != nil {
		ba.cancel()
	}

	return err
}

func (ba *BrowserAuth) extractAuthData(ctx context.Context) (token, cookies string, err error) {
	targetURL := "https://notebooklm.google.com"
	return ba.extractAuthDataForURL(ctx, targetURL)
}

func (ba *BrowserAuth) extractAuthDataForURL(ctx context.Context, targetURL string) (token, cookies string, err error) {
	// Navigate and wait for initial page load
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	); err != nil {
		return "", "", fmt.Errorf("failed to load page: %w", err)
	}

	// Execute anti-detection JavaScript to hide automation traces
	if err := chromedp.Run(ctx, chromedp.Evaluate(`
		// Hide webdriver property
		delete window.navigator.webdriver;

		// Override the plugins property to look normal
		Object.defineProperty(navigator, 'plugins', {
			get: () => Array.from({length: Math.floor(Math.random() * 5) + 1}, () => ({}))
		});

		// Override permissions property
		const originalQuery = window.navigator.permissions.query;
		window.navigator.permissions.query = (parameters) => (
			parameters.name === 'notifications' ?
				Promise.resolve({ state: Notification.permission }) :
				originalQuery(parameters)
		);

		// Override chrome runtime if it exists
		if (window.chrome && window.chrome.runtime) {
			delete window.chrome.runtime.onConnect;
			delete window.chrome.runtime.onMessage;
		}
	`, nil)); err != nil {
		// Don't fail if anti-detection script fails, just log it
		if ba.debug {
			fmt.Printf("Anti-detection script failed: %v\n", err)
		}
	}

	// If keep-open is set, give user time to manually authenticate BEFORE checking
	if ba.keepOpenSeconds > 0 {
		fmt.Printf("\n⏳ Browser opened. You have %d seconds to manually log in if needed...\n", ba.keepOpenSeconds)
		fmt.Printf("  If already logged in, just wait for automatic authentication.\n\n")
		time.Sleep(time.Duration(ba.keepOpenSeconds) * time.Second)
	}

	// First check if we're already on a login page, which would indicate authentication failure
	var currentURL string
	if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err == nil {
		// Log the initial URL we landed on
		if ba.debug {
			fmt.Printf("Initial navigation landed on: %s\n", currentURL)
		}

		// If we immediately landed on an auth page, this profile is likely not authenticated
		if strings.Contains(currentURL, "accounts.google.com") ||
			strings.Contains(currentURL, "signin") ||
			strings.Contains(currentURL, "login") {
			if ba.debug {
				fmt.Printf("Redirected to auth page: %s\n", currentURL)
			}

			return "", "", fmt.Errorf("redirected to authentication page - not logged in")
		}
	}

	// Create timeout context for polling - increased timeout for better success with Brave
	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
					if authFailCount >= maxAuthFailures {
						return "", "", fmt.Errorf("definitive authentication failure: %w", err)
					}
				}

				if ba.debug {
					// Show seconds remaining from ctx at end of this:
					deadline, _ := ctx.Deadline()
					remaining := time.Until(deadline).Seconds()
					fmt.Printf("   Auth check failed: %v (%.1f seconds remaining)\n", err, remaining)
				}
				continue
			}

			// Only accept the token and cookies if we get a proper non-empty response
			// and tryExtractAuth has already done its validation
			if token != "" && cookies != "" {
				// Get the final URL to confirm we're on the right page
				var successURL string
				if err := chromedp.Run(ctx, chromedp.Location(&successURL)); err == nil {
					if ba.debug {
						fmt.Printf("Successful authentication URL: %s\n", successURL)
					}

					// Double-check we're not on a login page (shouldn't happen with our improved checks)
					if strings.Contains(successURL, "accounts.google.com") ||
						strings.Contains(successURL, "signin") {
						return "", "", fmt.Errorf("authentication appeared to succeed but we're on login page: %s", successURL)
					}
				}

				// Authentication successful - perform graceful shutdown
				if ba.debug {
					fmt.Printf("✓ Authentication successful!\n")
				}

				// Gracefully close the browser to avoid crash detection
				if err := ba.gracefulShutdown(ctx); err != nil {
					if ba.debug {
						fmt.Printf("Warning: graceful shutdown failed: %v\n", err)
					}
				}

				return token, cookies, nil
			}

			if ba.debug {
				fmt.Println("Waiting for auth data...")
			}
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
		if ba.debug {
			fmt.Printf("Error checking if on signin page: %v\n", err)
		}
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

	err = chromedp.Run(ctx,
		chromedp.Evaluate(`WIZ_global_data.SNlM0e`, &token),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cks, err := network.GetCookies().WithURLs([]string{"https://notebooklm.google.com"}).Do(ctx)
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
