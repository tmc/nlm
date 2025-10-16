# Debug Output Documentation

This document describes all debug output locations in the nlm codebase and standardizes the debug output format.

## Environment Variables

- `NLM_DEBUG=true` - Enable debug output globally across all components
- `NLM_SKIP_SOURCES=true` - Skip source validation in chat operations
- `NLM_USE_ORIGINAL_PROFILE=true` - Use original browser profile directory (auth)

## Debug Output Format Standards

### Standard Format
```go
if debug {
    fmt.Fprintf(os.Stderr, "DEBUG: Component: description: %v\n", value)
}
```

### Error/Warning Format
```go
if debug {
    fmt.Fprintf(os.Stderr, "WARNING: Component: description: %v\n", err)
}
```

### Success/Info Format
```go
if debug {
    fmt.Fprintf(os.Stderr, "INFO: Component: description: %v\n", info)
}
```

## Debug Output Locations

### cmd/nlm/main.go

**Initialization and Setup:**
- Line 154: Profile selection status
- Line 174: Profile usage information
- Line 180: Protobuf parsing debug activation
- Line 463: CLI command information
- Line 525: Notebook selection details
- Line 534: Note creation confirmation
- Line 537: Note listing count
- Line 545: Audio overview status
- Line 564: Source addition details
- Line 579: Authentication status
- Line 591: Credential save status
- Line 1462: Chat session information

**Session Management:**
- Line 2084: Chat session auto-save failures
- Line 2097: Chat session exit save failures
- Line 2126: Token manager initialization
- Line 2133: Token manager debug configuration
- Line 2135: Auto-refresh startup status
- Line 2141: Session cleanup information

### cmd/nlm/auth.go

**Authentication Flow:**
- Line 132: Profile path information
- Line 141: Browser profile discovery
- Line 160: Authentication attempt details
- Line 164: Authentication success/failure
- Line 167: Alternative authentication paths
- Line 327: Debug flag detection
- Line 347: Profile analysis results
- Line 355: Authentication configuration

### internal/auth/auth.go

**Browser Automation:**
- Line 88: Profile preparation status
- Line 97: Browser launch configuration
- Line 105: Navigation events
- Line 119: Token extraction attempts
- Line 175: Profile validation results
- Line 183: Cookie extraction status
- Line 189: Authentication completion
- Line 608: Profile copy operations
- Line 657: Browser path resolution
- Line 680: Chrome profile detection
- Line 689: Brave profile detection
- Line 695: Chrome Canary profile detection
- Line 707: Profile sorting criteria
- Line 740: Profile validation results
- Line 748: Keep-open mode activation
- Line 823: Graceful shutdown initiation
- Line 838: Anti-detection script injection
- Line 847: Debugger connection status
- Line 873: Additional debug events

### internal/auth/safari_darwin.go

**Safari Integration:**
- Line 19: Safari cookie extraction (macOS)

### internal/auth/refresh.go

**Credential Refresh:**
- Line 96-100: Refresh request details
- Line 116-119: Refresh response status
- Line 128: Refresh success confirmation
- Line 227-229: Background refresh failures
- Line 379: Auto-refresh manager startup
- Line 395-400: Token expiry monitoring
- Line 425: Token validity status
- Line 431: Token refresh initiation
- Line 447: Debug mode activation
- Line 454-459: gsessionID extraction
- Line 468: Credential refresh success

### internal/api/client.go

**API Operations:**
- Line 108-110: Project source parsing
- Line 305-307: JSON file handling
- Line 424-428: YouTube source addition
- Line 444-446: Payload structure display
- Line 457-459: Raw API responses
- Line 686-688: Audio creation confirmation
- Line 903-905: Video creation initiation
- Line 936-962: Audio download attempts
- Line 994-996: Audio overview errors
- Line 1026-1028: Project metadata display
- Line 1061-1075: Video overview approaches
- Line 1364-1366: Video URL discovery
- Line 1509-1512: Video download authentication
- Line 1533-1538: Video download progress
- Line 1570-1572: Artifacts response parsing
- Line 1620-1622: Rename artifact responses
- Line 1783-1798: Chat source resolution
- Line 1833-1847: Streaming chat sources

### internal/api/test_helpers.go

**Test Utilities:**
- Line 77-78: Debug helper skip messages
- Line 116-117: Direct request skip messages

### internal/batchexecute/client.go

**HTTP/RPC Layer:**
- Comprehensive request/response logging
- Header and body inspection
- Error diagnostics
- Retry attempt tracking

### internal/beprotojson/beprotojson.go

**Protocol Buffer Parsing:**
- Line 193-198: Message type and array length
- Line 200-207: Field mapping information
- Line 210-234: Array position mapping
- Line 485-487: Nested message parsing
- Line 505-535: Field assignment details

## Component-Specific Debug Patterns

### Authentication (internal/auth)

**Purpose:** Track browser automation, profile selection, and token extraction

**Key Events:**
- Profile discovery and validation
- Browser launch and navigation
- Token extraction and verification
- Cookie handling
- Graceful shutdown

**Output Format:**
```
DEBUG: Auth: Finding Chrome profiles...
DEBUG: Auth: Found 3 valid profiles
DEBUG: Auth: Using profile: Profile 1
DEBUG: Auth: Token extracted: abc...xyz (truncated)
DEBUG: Auth: Authentication successful
```

### API Client (internal/api)

**Purpose:** Debug API interactions, request/response cycles, and data parsing

**Key Events:**
- Project/notebook operations
- Source management
- Audio/video overview operations
- Artifact management
- Chat/generation requests

**Output Format:**
```
DEBUG: API: Creating notebook: My Notebook
DEBUG: API: Added source: example.pdf
DEBUG: API: Audio creation initiated with ID: abc123
DEBUG: API: Using 5 sources for chat
```

### BatchExecute (internal/batchexecute)

**Purpose:** Low-level RPC debugging for NotebookLM API protocol

**Key Events:**
- Request construction
- Header configuration
- Response parsing
- Error diagnostics
- Retry logic

**Output Format:**
```
=== BatchExecute Request ===
URL: https://notebooklm.google.com/...
Auth Token: ab******yz
Request Body: {...}
Response Status: 200 OK
Response Body: {...}
```

### Protocol Buffer Parsing (internal/beprotojson)

**Purpose:** Debug Google's custom JSON-to-Protobuf format conversion

**Key Events:**
- Message type detection
- Field mapping
- Array position correlation
- Nested structure parsing
- Unknown field handling

**Output Format:**
```
=== BEPROTOJSON PARSING ===
Message Type: notebooklm.v1alpha1.Project
Array Length: 6
Available Fields: 5

=== FIELD MAPPING ===
Field #1: title (string)
Field #2: sources (repeated message)
...

=== ARRAY MAPPING ===
Position 0: maps to field #1 title (string) -> value: "My Project"
Position 1: maps to field #2 sources (repeated message) -> value: [...]
```

## Debug Output Best Practices

### 1. Always Use Stderr for Debug Output
```go
// ✅ GOOD
fmt.Fprintf(os.Stderr, "DEBUG: %s\n", message)

// ❌ BAD
fmt.Printf("DEBUG: %s\n", message)
```

### 2. Include Component Context
```go
// ✅ GOOD
fmt.Fprintf(os.Stderr, "DEBUG: Auth: Token extraction failed: %v\n", err)

// ❌ BAD
fmt.Fprintf(os.Stderr, "DEBUG: Failed: %v\n", err)
```

### 3. Mask Sensitive Data
```go
// ✅ GOOD
fmt.Fprintf(os.Stderr, "DEBUG: Auth: Token: %s\n", maskToken(token))

// ❌ BAD
fmt.Fprintf(os.Stderr, "DEBUG: Auth: Token: %s\n", token)
```

### 4. Use Structured Sections for Complex Output
```go
// ✅ GOOD
if debug {
    fmt.Fprintf(os.Stderr, "\n=== Component Operation ===\n")
    fmt.Fprintf(os.Stderr, "Parameter 1: %v\n", param1)
    fmt.Fprintf(os.Stderr, "Parameter 2: %v\n", param2)
    fmt.Fprintf(os.Stderr, "Result: %v\n", result)
}
```

### 5. Provide Actionable Information
```go
// ✅ GOOD
fmt.Fprintf(os.Stderr, "DEBUG: Auth: No valid profiles found. Run 'nlm auth' to configure.\n")

// ❌ BAD
fmt.Fprintf(os.Stderr, "DEBUG: Auth: Error\n")
```

## Testing Debug Output

### Enable Debug Mode in Tests
```go
func TestWithDebug(t *testing.T) {
    // Save original value
    origDebug := os.Getenv("NLM_DEBUG")
    defer os.Setenv("NLM_DEBUG", origDebug)

    // Enable debug
    os.Setenv("NLM_DEBUG", "true")

    // Test code that produces debug output
    client := api.New(token, cookies)
    // client.config.Debug should be true
}
```

### Capture Debug Output in Tests
```go
func TestDebugOutput(t *testing.T) {
    // Capture stderr
    oldStderr := os.Stderr
    r, w, _ := os.Pipe()
    os.Stderr = w

    // Code that produces debug output
    runCodeWithDebug()

    // Restore and read
    w.Close()
    os.Stderr = oldStderr
    var buf bytes.Buffer
    buf.ReadFrom(r)
    output := buf.String()

    // Verify output
    if !strings.Contains(output, "DEBUG:") {
        t.Error("Expected debug output")
    }
}
```

## Future Improvements

### 1. Structured Logging
Consider migrating to structured logging library (e.g., `log/slog`):
```go
slog.Debug("operation completed",
    "component", "auth",
    "profile", profileName,
    "duration", elapsed)
```

### 2. Debug Levels
Implement different debug verbosity levels:
- `NLM_DEBUG=1` - Basic operations
- `NLM_DEBUG=2` - Detailed operations
- `NLM_DEBUG=3` - Full request/response dumps

### 3. Debug Output Filtering
Add ability to filter debug output by component:
```bash
NLM_DEBUG=auth,api nlm list
```

### 4. Debug Performance Metrics
Include timing information in debug output:
```go
if debug {
    defer func(start time.Time) {
        fmt.Fprintf(os.Stderr, "DEBUG: Operation completed in %v\n", time.Since(start))
    }(time.Now())
}
```

### 5. Debug Output Formatting
Add color coding for different debug levels:
- 🟢 INFO (green)
- 🟡 WARNING (yellow)
- 🔴 ERROR (red)
- 🔵 DEBUG (blue)

## Related Documentation

- [Contributing Guide](../CONTRIBUTING.md) - Development guidelines
- [Examples](EXAMPLES.md) - Usage examples including debug mode
- [README](../README.md) - Basic usage and features
