package auth

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/chromedp/chromedp"
)

// wantProfileTable reports whether the profile inventory was asked for. It is
// browser trivia the user did not request on the default path.
func (o *Options) wantProfileTable(debug bool) bool {
	return o.ListProfiles || o.TryAllProfiles || o.CheckNotebooks || debug
}

// sortProfilesByLastUsed orders profiles most recently used first. The
// inventory is numbered in this order and the unnamed profile is taken from
// the front, so the two always agree.
func sortProfilesByLastUsed(profiles []ProfileInfo) {
	sort.SliceStable(profiles, func(i, j int) bool {
		return profiles[i].LastUsed.After(profiles[j].LastUsed)
	})
}

// selectProfile returns the profile to authenticate with: the one named, or
// else the most recently used. It returns nil if there are no profiles.
func selectProfile(profiles []ProfileInfo, name string) *ProfileInfo {
	for i := range profiles {
		if profiles[i].Name == name {
			return &profiles[i]
		}
	}
	if len(profiles) == 0 {
		return nil
	}
	return &profiles[0]
}

// printProfileTable writes the profile inventory. Rows are numbered by their
// position in profiles, which sortProfilesByLastUsed has ordered most recent
// first, and the row that will be used is marked.
func printProfileTable(w io.Writer, profiles []ProfileInfo, targetDomain string, selected *ProfileInfo, tryAll bool) {
	fmt.Fprintln(w, "nlm: browser profiles:")
	for i, p := range profiles {
		mark := " "
		if !tryAll && selected != nil && p.Path == selected.Path {
			mark = "*"
		}
		cookies := ""
		if targetDomain != "" {
			cookies = fmt.Sprintf(" [no %s cookies]", targetDomain)
			if p.HasTargetCookies {
				cookies = fmt.Sprintf(" [%s cookies]", targetDomain)
			}
		}
		notebooks := ""
		if p.NotebookCount > 0 {
			notebooks = fmt.Sprintf(" [%d notebooks]", p.NotebookCount)
		}
		fmt.Fprintf(w, "nlm: %s %d. %s [%s] - last used %s (%d files, %.1f MB)%s%s\n",
			mark, i+1, p.Name, p.Browser,
			p.LastUsed.Format("2006-01-02 15:04:05"),
			len(p.Files),
			float64(p.Size)/(1024*1024),
			cookies,
			notebooks)
	}
	switch {
	case tryAll:
		fmt.Fprintln(w, "nlm: trying profiles in the order shown")
	case selected != nil:
		fmt.Fprintf(w, "nlm: using profile %s [%s]\n", selected.Name, selected.Browser)
	}
}

// checkNotebookAccess authenticates with each profile that has cookies for
// the target domain and records how many notebooks it can see. It is slow —
// one headless browser per profile — so it runs only when asked for, and only
// for the first few candidates.
func (ba *BrowserAuth) checkNotebookAccess(profiles []ProfileInfo, targetURL, authUser string) []ProfileInfo {
	const maxToCheck = 5

	statusf("checking notebook access for profiles...")

	checked := 0
	updated := make([]ProfileInfo, 0, len(profiles))
	for _, p := range profiles {
		if !p.HasTargetCookies || checked >= maxToCheck {
			updated = append(updated, p)
			continue
		}
		checked++

		profile, err := ba.checkProfileNotebooks(p, targetURL, authUser)
		if err != nil {
			statusf("%s [%s]: %v", p.Name, p.Browser, err)
			updated = append(updated, p)
			continue
		}
		statusf("%s [%s]: %d notebooks", p.Name, p.Browser, profile.NotebookCount)
		updated = append(updated, profile)
	}
	return updated
}

// checkProfileNotebooks authenticates with one profile and counts the
// notebooks it can reach.
func (ba *BrowserAuth) checkProfileNotebooks(p ProfileInfo, targetURL, authUser string) (ProfileInfo, error) {
	tempDir, err := os.MkdirTemp("", "nlm-notebook-check-*")
	if err != nil {
		return p, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempAuth := &BrowserAuth{debug: ba.debug, tempDir: tempDir}
	if err := tempAuth.copyProfileDataFromPath(p.Path); err != nil {
		return p, fmt.Errorf("copy profile data: %w", err)
	}

	authCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewExecAllocator(authCtx,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("headless", true),
		chromedp.UserDataDir(tempDir),
	)
	defer allocCancel()

	ctx, ctxCancel := newChromeContext(allocCtx, ba.debug)
	defer ctxCancel()

	token, cookies, err := tempAuth.extractAuthDataForURL(ctx, targetURL)
	if err != nil {
		return p, fmt.Errorf("not authenticated")
	}
	if token == "" {
		return p, fmt.Errorf("not authenticated")
	}

	count, err := countNotebooks(token, cookies, authUser)
	if err != nil {
		return p, fmt.Errorf("count notebooks: %w", err)
	}

	p.AuthToken = token
	p.AuthCookies = cookies
	p.NotebookCount = count
	return p, nil
}

// ProfilesWithCookies returns the names of the browser profiles that hold
// cookies for targetURL's domain, most recently used first. It is used to
// suggest an alternative after a login attempt fails, so it reports nothing
// rather than an error when the profiles cannot be scanned.
func ProfilesWithCookies(targetURL string) []string {
	u, err := url.Parse(targetURL)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	profiles, err := New(false).scanProfilesForDomain(u.Hostname())
	if err != nil {
		return nil
	}
	sortProfilesByLastUsed(profiles)
	var names []string
	for _, p := range profiles {
		if p.HasTargetCookies {
			names = append(names, p.Name)
		}
	}
	return names
}
