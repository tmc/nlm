---
title: Authentication
---
# Authentication

nlm authenticates with Google NotebookLM using session cookies extracted from your browser. This is an unofficial API — there are no OAuth scopes or API keys.

## Browser-based auth (recommended)

On a fresh machine, run `nlm ls` in a terminal. nlm opens a visible browser
and waits up to five minutes for you to sign in to Google and open NotebookLM.
It saves your session and continues with the notebook list automatically.
A supported browser must be installed. nlm uses its own persistent browser
profile in `~/.nlm/browser/default`; it does not scan or copy personal browser
profiles during default sign-in. Named nlm identities use separate directories.

To sign in explicitly:

```bash
nlm auth
```

This opens nlm's browser profile. To import an existing personal browser profile
instead (which may require macOS permission to read that browser's data):

```bash
nlm auth --profile "Work"
```

To try all discovered profiles:

```bash
nlm auth --all
```

Credentials are saved to `~/.nlm/env` and loaded automatically on subsequent runs.

### Supported browsers

- Google Chrome
- Brave Browser
- Chrome Canary

The auth flow uses Chrome DevTools Protocol (CDP) to capture the completed
session. Sign in to your Google account when the browser opens. Later commands
reuse the saved credentials; refreshes can reuse the browser session.

### CDP URL

If you have a browser already running with remote debugging enabled, you can connect directly:

```bash
nlm auth --cdp-url ws://localhost:9222
```

Commands run without a terminal or display, or with `NLM_NONINTERACTIVE=1`,
print an authentication hint and exit with code 3 when credentials are missing.
They do not open a browser. Explicit `nlm auth` can still connect via CDP.

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

**Multiple Google accounts in one profile** — Use `nlm auth --authuser 1` to
authenticate with a non-default account. For other commands, pass
`--authuser 1` or export `NLM_AUTHUSER=1`.

**Windows with Google 2FA** — The automated browser may trigger "unsafe browser" warnings. As a workaround, use `--cdp-url` with a manually-launched Chrome instance that has remote debugging enabled:

```bash
# Launch Chrome with debugging
chrome.exe --remote-debugging-port=9222

# In another terminal, log into NotebookLM manually, then:
nlm auth --cdp-url ws://localhost:9222
```
