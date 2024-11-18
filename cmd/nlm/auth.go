package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tmc/nlm/internal/auth"
)

func handleAuth(args []string, debug bool) error {
	if len(args) > 0 {
		// Handle existing curl/HAR parsing
		return detectAuthInfo(strings.Join(args, " "))
	}

	auth := auth.New(debug)

	fmt.Println("Launching Chrome to extract authentication...")
	token, cookies, err := auth.GetAuth()
	if err != nil {
		return fmt.Errorf("browser auth failed: %w", err)
	}

	if debug {
		fmt.Printf("Token: %s\n", token)
		fmt.Printf("Cookies: %s\n", cookies)
	}

	return writeAuthToEnv(cookies, token)
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

func detectAuthInfo(cmd string) error {
	// Extract cookies
	cookieRe := regexp.MustCompile(`-H ['"]cookie: ([^'"]+)['"]`)
	cookieMatch := cookieRe.FindStringSubmatch(cmd)
	if len(cookieMatch) < 2 {
		return fmt.Errorf("no cookies found")
	}
	cookies := cookieMatch[1]

	// Extract auth token
	atRe := regexp.MustCompile(`at=([^&\s]+)`)
	atMatch := atRe.FindStringSubmatch(cmd)
	if len(atMatch) < 2 {
		return fmt.Errorf("no auth token found")
	}
	authToken := atMatch[1]
	return writeAuthToEnv(cookies, authToken)
}

func writeAuthToEnv(cookies, authToken string) error {
	// Create or update .env file
	envFile := ".env"
	content := fmt.Sprintf("export NLM_COOKIES=%q\nexport NLM_AUTH_TOKEN=%q\n", cookies, authToken)

	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}

	fmt.Printf("Auth info written to %s\n", envFile)
	fmt.Println("Run 'source .env' to load the variables")
	return nil
}
