package env

import (
    "bufio"
    "os"
    "path/filepath"
    "strconv"
    "strings"
)

// LoadStoredEnv loads environment variables from .nlm/env file
func LoadStoredEnv() {
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

