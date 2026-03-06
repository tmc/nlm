# WSL Setup Guide for nlm

This guide explains how to set up `nlm` (NotebookLM CLI) on Windows Subsystem for Linux (WSL).

## The Problem

Chrome/Chromium on Linux encrypts cookies using the system keyring. When `nlm` copies the browser profile to a temporary directory, the encryption keys are lost and authentication fails with "redirected to authentication page - not logged in".

## Solution

Use the `NLM_USE_ORIGINAL_PROFILE=1` environment variable to make `nlm` use the original profile directory instead of copying it.

## Prerequisites

- WSL2 installed on Windows
- Go 1.21+ installed in WSL

## Installation Steps

### 1. Install Chromium in WSL

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install chromium-browser

# Or via snap (if apt version is not available)
sudo snap install chromium
```

### 2. Create Profile Symlink

`nlm` looks for Chrome profiles in `~/.config/google-chrome`. Since we're using Chromium, create a symlink:

```bash
# For apt-installed Chromium
ln -sf ~/.config/chromium ~/.config/google-chrome

# For snap-installed Chromium
ln -sf ~/snap/chromium/common/chromium ~/.config/google-chrome
```

### 3. Initial Browser Setup

Launch Chromium with basic password storage (to avoid keyring prompts):

```bash
chromium --password-store=basic
```

Then:
1. Navigate to https://notebooklm.google.com
2. Sign in with your Google account
3. Close the browser

### 4. Install nlm

```bash
go install github.com/tmc/nlm/cmd/nlm@latest
```

### 5. Authenticate nlm

Run the authentication with the original profile flag:

```bash
NLM_USE_ORIGINAL_PROFILE=1 ~/go/bin/nlm auth -debug
```

You should see:
```
Using original profile directory: /home/username/.config/google-chrome
...
Authentication successful!
```

### 6. Verify Installation

```bash
# List your notebooks
~/go/bin/nlm list

# List sources in a notebook
~/go/bin/nlm sources <notebook-id>
```

## Usage

Always use `NLM_USE_ORIGINAL_PROFILE=1` when running `nlm` commands in WSL:

```bash
export NLM_USE_ORIGINAL_PROFILE=1
nlm list
nlm generate-chat <notebook-id> "Your question here"
```

Or add to your `.bashrc`:

```bash
echo 'export NLM_USE_ORIGINAL_PROFILE=1' >> ~/.bashrc
source ~/.bashrc
```

## Troubleshooting

### "no valid profiles found"

Make sure the symlink exists:
```bash
ls -la ~/.config/google-chrome
```

Should point to your Chromium profile directory.

### "redirected to authentication page"

1. Make sure you're using `NLM_USE_ORIGINAL_PROFILE=1`
2. Re-login to NotebookLM in Chromium manually
3. Run `nlm auth -debug` again

### Chrome process conflicts

If authentication fails, close any running Chromium processes:
```bash
pkill -f chromium
```

Then try again.

## How It Works

The `NLM_USE_ORIGINAL_PROFILE` environment variable tells `nlm` to:

1. **Without flag (default)**: Copy browser profile to a temp directory, losing encryption keys
2. **With flag=1**: Use the original profile directory directly, preserving cookie encryption

This is implemented in `internal/auth/auth.go` in both `tryMultipleProfiles()` and `GetAuth()` functions.

## Security Note

When using `NLM_USE_ORIGINAL_PROFILE=1`, `nlm` has access to your actual browser profile. This is necessary for authentication but means:

- Close other Chromium windows before running `nlm` to avoid profile lock conflicts
- The browser automation has access to your real cookies and session data

This is the same level of access as your regular browser session.
