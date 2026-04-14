---
title: Authentication
---
# Authentication

nlm authenticates with Google NotebookLM using session cookies extracted from your browser. This is an unofficial API — there are no OAuth scopes or API keys.

## Browser-based auth (recommended)

The `auth` command launches a headless browser, opens NotebookLM, and extracts session cookies:

```bash
nlm auth
```

This opens your default Chrome/Brave profile. To use a specific profile:

```bash
nlm auth --profile "Work"
```

To try all discovered profiles:

```bash
nlm auth --all
```

Credentials are saved to `~/.config/nlm/.env` and loaded automatically on subsequent runs.

### Supported browsers

- Google Chrome
- Brave Browser
- Chrome Canary

The auth flow uses Chrome DevTools Protocol (CDP) to automate cookie extraction. You must be signed in to your Google account in the selected profile.

### CDP URL

If you have a browser already running with remote debugging enabled, you can connect directly:

```bash
nlm auth --cdp-url ws://localhost:9222
```

## Manual auth

You can also provide credentials directly via flags or environment variables.

### Environment variables

```bash
export NLM_AUTH_TOKEN="your-SAPISID-token"
export NLM_COOKIES="SID=...; HSID=...; SSID=...; APISID=...; SAPISID=..."
```

### Flags

```bash
nlm --auth "your-token" --cookies "SID=...; HSID=..." list
```

## Credential refresh

Session cookies expire. nlm includes automatic background refresh:

```bash
# Manual refresh
nlm refresh

# Auto-refresh runs in the background during long sessions (chat, etc.)
```

## Troubleshooting

**"Authentication failed"** — Make sure you're signed into NotebookLM in the browser profile you're using. Try `nlm auth --debug` for detailed output.

**Cookies expire quickly** — Run `nlm refresh` or re-run `nlm auth`. The auto-refresh manager handles this during interactive sessions.

**Wrong Google account** — Use `--profile` to select the browser profile associated with the correct account.

**Multiple Google accounts in one profile** — Use `--authuser 1` (or `NLM_AUTHUSER=1`) to authenticate with a non-default account.

**Windows with Google 2FA** — The automated browser may trigger "unsafe browser" warnings. As a workaround, use `--cdp-url` with a manually-launched Chrome instance that has remote debugging enabled:

```bash
# Launch Chrome with debugging
chrome.exe --remote-debugging-port=9222

# In another terminal, log into NotebookLM manually, then:
nlm auth --cdp-url ws://localhost:9222
```
