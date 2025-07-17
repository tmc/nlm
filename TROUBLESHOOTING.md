# nlm Troubleshooting Guide

This guide covers common issues and solutions for the nlm (NotebookLM CLI) tool.

## Authentication Issues

### 1. Authentication Failure

**Symptoms:**
- `nlm auth` fails with "browser auth failed"
- Error: "no profiles could authenticate"
- Error: "redirected to authentication page - not logged in"

**Solutions:**

1. **Try all available profiles:**
   ```bash
   nlm auth --all
   ```

2. **Check available profiles:**
   ```bash
   nlm auth --all --notebooks
   ```

3. **Use debug mode to see details:**
   ```bash
   nlm auth --debug
   ```

4. **Manually sign in to NotebookLM:**
   - Open your browser
   - Go to https://notebooklm.google.com
   - Sign in with your Google account
   - Try authentication again

### 2. Browser Not Found

**Symptoms:**
- Error: "chrome not found"
- Error: "no supported browsers found"

**Solutions:**

1. **Install a supported browser:**
   - Google Chrome: https://www.google.com/chrome/
   - Brave Browser: https://brave.com/
   - Chrome Canary: https://www.google.com/chrome/canary/

2. **Check browser installation:**
   ```bash
   # For Chrome
   ls "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
   
   # For Brave
   ls "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
   ```

3. **Use mdfind to locate browsers:**
   ```bash
   mdfind "kMDItemCFBundleIdentifier == 'com.google.Chrome'"
   mdfind "kMDItemCFBundleIdentifier == 'com.brave.Browser'"
   ```

### 3. Profile Issues

**Symptoms:**
- Error: "Profile 'Default' not found"
- Error: "no valid profiles found"

**Solutions:**

1. **List available profiles:**
   ```bash
   # Check Chrome profiles
   ls ~/Library/Application\ Support/Google/Chrome/
   
   # Check Brave profiles
   ls ~/Library/Application\ Support/BraveSoftware/Brave-Browser/
   ```

2. **Try with a specific profile:**
   ```bash
   nlm auth --profile "Profile 1"
   ```

3. **Use profile scanning:**
   ```bash
   nlm auth --all --debug
   ```

### 4. Browser Session Conflicts

**Symptoms:**
- Authentication works but tokens are invalid
- Error: "unauthorized" after successful auth
- Commands fail with 401 errors

**Solutions:**

1. **Clear browser cache and cookies:**
   - Open your browser
   - Go to Settings > Privacy > Clear browsing data
   - Select "Cookies and site data" and "Cached images and files"
   - Clear data for the last hour

2. **Use incognito/private browsing:**
   - Sign in to NotebookLM in an incognito window
   - Keep the window open while running auth

3. **Try different browser profile:**
   ```bash
   nlm auth --profile "Work"
   ```

4. **Force re-authentication:**
   ```bash
   rm ~/.nlm/env
   nlm auth
   ```

## API and Network Issues

### 1. Unauthorized Errors

**Symptoms:**
- Error: "unauthorized" when running commands
- 401 HTTP status codes
- Commands worked before but now fail

**Solutions:**

1. **Re-authenticate:**
   ```bash
   nlm auth
   ```

2. **Check stored credentials:**
   ```bash
   cat ~/.nlm/env
   ```

3. **Use debug mode:**
   ```bash
   nlm -debug list
   ```

4. **Clear and re-authenticate:**
   ```bash
   rm ~/.nlm/env
   nlm auth --all
   ```

### 2. Network Timeout Issues

**Symptoms:**
- Commands hang or timeout
- Error: "context deadline exceeded"
- Slow responses

**Solutions:**

1. **Check internet connection:**
   ```bash
   ping google.com
   ```

2. **Try with debug mode:**
   ```bash
   nlm -debug list
   ```

3. **Check firewall/proxy settings:**
   - Ensure https://notebooklm.google.com is accessible
   - Check corporate firewall rules

### 3. API Parsing Errors

**Symptoms:**
- Error: "failed to parse response"
- Error: "invalid character" in JSON parsing
- Unexpected response format

**Solutions:**

1. **Enable debug output:**
   ```bash
   nlm -debug <command>
   ```

2. **Check API response:**
   - Look for HTML responses instead of JSON
   - Check for Google error pages
   - Verify you're not hitting rate limits

3. **Re-authenticate with fresh tokens:**
   ```bash
   rm ~/.nlm/env
   nlm auth
   ```

## File and Source Issues

### 1. File Upload Problems

**Symptoms:**
- Error: "failed to upload file"
- Error: "unsupported file type"
- Large files fail to upload

**Solutions:**

1. **Check file size:**
   ```bash
   ls -lh yourfile.pdf
   ```

2. **Specify MIME type explicitly:**
   ```bash
   nlm add <notebook-id> yourfile.pdf -mime="application/pdf"
   ```

3. **Try different file formats:**
   - PDF, TXT, DOCX are well supported
   - Large files (>10MB) may have issues

### 2. URL Source Issues

**Symptoms:**
- Error: "failed to fetch URL"
- YouTube URLs not working
- Website access denied

**Solutions:**

1. **Check URL accessibility:**
   ```bash
   curl -I "https://example.com/page"
   ```

2. **For YouTube URLs, ensure proper format:**
   ```bash
   # These formats work:
   nlm add <notebook-id> https://www.youtube.com/watch?v=VIDEO_ID
   nlm add <notebook-id> https://youtu.be/VIDEO_ID
   ```

3. **Try with debug mode:**
   ```bash
   nlm -debug add <notebook-id> "https://example.com"
   ```

## Environment and Setup Issues

### 1. Go Installation Problems

**Symptoms:**
- Command: `go: command not found`
- Error: "go version not supported"

**Solutions:**

1. **Install Go:**
   ```bash
   # macOS with Homebrew
   brew install go
   
   # Or download from https://golang.org/dl/
   ```

2. **Check Go version:**
   ```bash
   go version
   ```

3. **Set up Go path:**
   ```bash
   export PATH=$PATH:/usr/local/go/bin
   export PATH=$PATH:$(go env GOPATH)/bin
   ```

### 2. Permission Issues

**Symptoms:**
- Error: "permission denied"
- Cannot write to ~/.nlm/env
- Cannot create temporary files

**Solutions:**

1. **Check home directory permissions:**
   ```bash
   ls -la ~/.nlm/
   ```

2. **Create .nlm directory:**
   ```bash
   mkdir -p ~/.nlm
   chmod 700 ~/.nlm
   ```

3. **Fix file permissions:**
   ```bash
   chmod 600 ~/.nlm/env
   ```

## Debug Mode and Logging

### Enable Debug Output

Always use debug mode when troubleshooting:

```bash
# For authentication
nlm auth --debug

# For commands
nlm -debug list
nlm -debug add <notebook-id> file.pdf

# For maximum verbosity
nlm -debug auth --all --notebooks
```

### Common Debug Information

When debug mode is enabled, you'll see:
- Browser profile scanning results
- Authentication token extraction
- HTTP requests and responses
- Cookie validation
- Error stack traces

### Log Files

The tool doesn't create log files by default, but you can capture output:

```bash
# Capture debug output
nlm -debug auth 2>&1 | tee nlm-debug.log

# Capture both stdout and stderr
nlm -debug list > nlm-output.log 2>&1
```

## Getting Help

### 1. Command Help

```bash
# General help
nlm --help

# Authentication help
nlm auth --help

# Command-specific help
nlm <command> --help
```

### 2. Version Information

```bash
# Check installed version
nlm version

# Check Go version
go version
```

### 3. Report Issues

When reporting issues, include:
- Operating system and version
- Browser type and version
- nlm version
- Full error message
- Debug output (if applicable)
- Steps to reproduce

### 4. Community Support

- GitHub Issues: https://github.com/tmc/nlm/issues
- Include debug output and error messages
- Provide minimal reproduction steps

## Common Error Messages

### Authentication Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `browser auth failed` | Browser not found or authentication failed | Try `nlm auth --all --debug` |
| `no profiles could authenticate` | No valid browser profiles | Sign in to NotebookLM in browser first |
| `redirected to authentication page` | Not logged in to Google | Sign in to Google in browser |
| `missing essential authentication cookies` | Invalid session | Clear browser cookies and re-authenticate |

### API Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `unauthorized` | Invalid or expired tokens | Run `nlm auth` to re-authenticate |
| `failed to parse response` | Unexpected API response | Check debug output, may need to re-auth |
| `context deadline exceeded` | Network timeout | Check internet connection |

### File Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `failed to upload file` | File too large or unsupported | Check file size and format |
| `unsupported file type` | MIME type not recognized | Use `-mime` flag to specify type |
| `file not found` | Invalid file path | Check file exists and path is correct |

## Prevention Tips

1. **Regular re-authentication:** Re-run `nlm auth` periodically
2. **Keep browser sessions active:** Don't sign out of Google in your browser
3. **Use specific profiles:** Specify profile names for consistent behavior
4. **Check file sizes:** Keep files under 10MB for reliable uploads
5. **Use debug mode:** Always use `-debug` when troubleshooting
6. **Keep tools updated:** Regularly update nlm to get latest fixes

This troubleshooting guide should help resolve most common issues. If problems persist, please report them with debug output included.