// chagne this to use x/term and write the auth file to the users's home dir in a cache file.
package main

import (
	"bufio"
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

func handleAuth(args []string, debug bool) (string, string, error) {
	isTty := term.IsTerminal(int(os.Stdin.Fd()))

	if !isTty {
		// Parse HAR/curl from stdin
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", "", fmt.Errorf("failed to read stdin: %w", err)
		}
		return detectAuthInfo(string(input))
	}

	profileName := "Default"
	if v := os.Getenv("NLM_BROWSER_PROFILE"); v != "" {
		profileName = v
	}
	if len(args) > 0 {
		profileName = args[0]
	}

	a := auth.New(debug)
	fmt.Fprintf(os.Stderr, "nlm: launching browser to login... (profile:%v)  (set with NLM_BROWSER_PROFILE)\n", profileName)
	token, cookies, err := a.GetAuth(auth.WithProfileName(profileName))
	if err != nil {
		return "", "", fmt.Errorf("browser auth failed: %w", err)
	}
	return persistAuthToDisk(cookies, token, profileName)
}

func readFromStdin() (string, error) {
	var input strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		input.Write(buf[:n])
	}
	return input.String(), nil
}

func detectAuthInfo(cmd string) (string, string, error) {
	// Extract cookies
	cookieRe := regexp.MustCompile(`-H ['"]cookie: ([^'"]+)['"]`)
	cookieMatch := cookieRe.FindStringSubmatch(cmd)
	if len(cookieMatch) < 2 {
		return "", "", fmt.Errorf("no cookies found")
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
		if os.Getenv(key) != "" {
			continue
		}

		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		os.Setenv(key, value)
	}
}
