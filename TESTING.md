# Testing Guide for nlm

This document outlines the testing patterns and practices established in the nlm project. It serves as a guide for contributors to understand how to write effective tests and follow the established Test-Driven Development (TDD) workflow.

## Overview

The nlm project uses a multi-layered testing approach that combines:
- **Scripttest** for CLI validation and user interface testing
- **Unit tests** for isolated component testing
- **Integration tests** for end-to-end functionality testing

## Testing Architecture

### 1. Scripttest Tests (`cmd/nlm/testdata/`)

Scripttest tests are the primary tool for testing CLI behavior and user-facing functionality. They run the compiled binary directly and verify command-line argument parsing, help text, and error handling.

**Location**: `/Users/tmc/go/src/github.com/tmc/nlm/cmd/nlm/testdata/`

**What scripttest tests excel at**:
- Command-line argument validation
- Help text verification
- Error message consistency
- Exit code validation
- Authentication requirement checks
- Flag parsing verification

**Test file naming convention**:
- `*_commands.txt` - Command-specific functionality
- `validation.txt` - Input validation tests
- `basic.txt` - Core functionality tests
- `*_bug.txt` - Regression tests for specific bugs

### 2. Unit Tests (`*_test.go`)

Unit tests focus on testing individual functions and components in isolation.

**Examples**:
- `/Users/tmc/go/src/github.com/tmc/nlm/cmd/nlm/main_test.go` - CLI framework testing
- `/Users/tmc/go/src/github.com/tmc/nlm/cmd/nlm/integration_test.go` - Command routing logic
- `/Users/tmc/go/src/github.com/tmc/nlm/internal/*/***_test.go` - Internal package testing

### 3. Integration Tests

Integration tests verify complete workflows with mocked or real external dependencies.

**Example**: `/Users/tmc/go/src/github.com/tmc/nlm/internal/batchexecute/integration_test.go`

## TDD Workflow: The Sources Command Pattern

The sources command fix demonstrates our established TDD workflow:

### Step 1: Create Failing Tests

Start by writing tests that capture the expected behavior:

```txt
# Test sources command requires notebook ID argument
! exec ./nlm_test sources
stderr 'usage: nlm sources <notebook-id>'

# Test sources command with too many arguments
! exec ./nlm_test sources notebook-id extra-arg
stderr 'usage: nlm sources <notebook-id>'
```

**Files created**:
- `cmd/nlm/testdata/sources_comprehensive.txt`
- `cmd/nlm/testdata/sources_display_bug.txt`

### Step 2: Add Debug Logging

When investigating issues, add temporary debug logging to understand the flow:

```go
// Example debug logging pattern
if debug {
    fmt.Printf("Debug: sources fetched, count=%d\n", len(sources))
    for i, source := range sources {
        fmt.Printf("Debug: source[%d] = %+v\n", i, source)
    }
}
```

### Step 3: Implement Targeted Fixes

Make minimal changes to fix the failing tests:

```go
// Fix argument validation
case "sources":
    if len(flag.Args()) != 2 {
        fmt.Fprintf(os.Stderr, "usage: nlm sources <notebook-id>\n")
        os.Exit(1)
    }
```

### Step 4: Verify Fixes Work

Run the tests to ensure they pass:

```bash
go test ./cmd/nlm -run TestCLICommands
```

### Step 5: Clean Up Debug Logging

Remove temporary debug statements once the fix is confirmed working.

## Testing Limitations and Guidelines

### What Scripttest Can Test

✅ **Excellent for**:
- CLI argument validation
- Help text and usage messages
- Error message consistency
- Exit codes
- Flag parsing
- Command routing logic
- Authentication checks (without network calls)

✅ **Example test patterns**:
```txt
# Argument validation
! exec ./nlm_test sources
stderr 'usage: nlm sources <notebook-id>'

# Authentication checks
! exec ./nlm_test sources notebook123
stderr 'Authentication required'

# Flag parsing
exec ./nlm_test -debug help
stdout 'Usage: nlm <command>'
```

### What Requires Integration Tests

❌ **Scripttest limitations**:
- API response parsing
- Network communication
- Complex data transformations
- HTTP error handling
- Real authentication flows

✅ **Use integration tests for**:
- API response parsing logic
- Error code handling from remote services
- Data format validation
- Network retry mechanisms
- End-to-end workflows with external dependencies

### When to Use Each Approach

| Test Type | Use When | Example |
|-----------|----------|---------|
| **Scripttest** | Testing CLI behavior, argument validation | `sources` command argument checking |
| **Unit Tests** | Testing isolated functions, data structures | Error code parsing, data transformation |
| **Integration Tests** | Testing complete workflows, API interactions | Full notebook creation flow |

## Test Environment Setup

### Scripttest Environment

The scripttest environment is carefully controlled:

```go
// Minimal environment setup
env := []string{
    "PATH=" + os.Getenv("PATH"),
    "HOME=" + tmpHome,
    "TERM=" + os.Getenv("TERM"),
}
```

**Key features**:
- Isolated temporary home directory
- Minimal environment variables
- Compiled test binary (`nlm_test`)
- Clean state for each test

### Test Binary

Tests use a dedicated binary built in `TestMain`:

```go
func TestMain(m *testing.M) {
    // Build the nlm binary for testing
    cmd := exec.Command("go", "build", "-o", "nlm_test", ".")
    if err := cmd.Run(); err != nil {
        panic("failed to build nlm for testing: " + err.Error())
    }
    defer os.Remove("nlm_test")

    code := m.Run()
    os.Exit(code)
}
```

## Best Practices

### 1. Test-First Development

- Write failing tests before implementing features
- Start with scripttest for CLI behavior
- Add unit tests for complex logic
- Use integration tests for end-to-end verification

### 2. Comprehensive Coverage

**For each new command, ensure**:
- Argument validation tests
- Help text verification
- Error message consistency
- Authentication requirement checks
- Edge case handling

### 3. Test Organization

**Scripttest files**:
- Group related functionality in single files
- Use descriptive file names
- Include comments explaining test purpose
- Test both positive and negative cases

**Go test files**:
- One test file per source file when possible
- Use table-driven tests for multiple scenarios
- Include edge cases and error conditions

### 4. Error Testing

**Always test**:
- Missing required arguments
- Too many arguments
- Invalid argument formats
- Authentication failures
- Network failures (in integration tests)

### 5. Debugging Practices

**During development**:
- Add temporary debug logging to understand flow
- Use descriptive variable names in tests
- Include helpful error messages in test failures
- Clean up debug code once tests pass

## Examples from the Codebase

### Scripttest Example: Argument Validation

```txt
# File: cmd/nlm/testdata/source_commands.txt

# Test sources without arguments (should fail with usage)
! exec ./nlm_test sources
stderr 'usage: nlm sources <notebook-id>'
! stderr 'panic'

# Test sources with too many arguments
! exec ./nlm_test sources notebook123 extra
stderr 'usage: nlm sources <notebook-id>'
! stderr 'panic'
```

### Unit Test Example: Command Routing

```go
// File: cmd/nlm/integration_test.go

func TestAuthCommand(t *testing.T) {
    tests := []struct {
        cmd      string
        expected bool
    }{
        {"help", false},
        {"auth", false},
        {"sources", true},
        {"list", true},
    }

    for _, tt := range tests {
        t.Run(tt.cmd, func(t *testing.T) {
            result := isAuthCommand(tt.cmd)
            if result != tt.expected {
                t.Errorf("isAuthCommand(%q) = %v, want %v", tt.cmd, result, tt.expected)
            }
        })
    }
}
```

### Integration Test Example: Error Handling

```go
// File: internal/batchexecute/integration_test.go

func TestErrorHandlingIntegration(t *testing.T) {
    tests := []struct {
        name           string
        responseBody   string
        expectError    bool
        expectedErrMsg string
        isRetryable    bool
    }{
        {
            name:           "Authentication error response",
            responseBody:   ")]}'\n277566",
            expectError:    true,
            expectedErrMsg: "Authentication required",
            isRetryable:    false,
        },
        // ... more test cases
    }
    // ... test implementation
}
```

## Running Tests

### All Tests
```bash
go test ./...
```

### Scripttest Only
```bash
go test ./cmd/nlm -run TestCLICommands
```

### Specific Test File
```bash
go test ./cmd/nlm -run TestCLICommands/sources_comprehensive.txt
```

### With Verbose Output
```bash
go test -v ./cmd/nlm
```

### Integration Tests
```bash
go test ./internal/batchexecute -run Integration
```

## Contributing New Tests

### For New Commands

1. **Start with scripttest** (`cmd/nlm/testdata/your_command.txt`):
   - Test argument validation
   - Test help text
   - Test authentication requirements
   - Test error cases

2. **Add unit tests** if complex logic is involved:
   - Test parsing functions
   - Test data transformation
   - Test error conditions

3. **Add integration tests** for end-to-end workflows:
   - Test with mocked HTTP responses
   - Test complete user workflows
   - Test error recovery

### For Bug Fixes

1. **Reproduce with failing test**:
   - Create scripttest that demonstrates the bug
   - Name file descriptively (e.g., `bug_sources_display.txt`)

2. **Add debug logging** if needed:
   - Understand the code flow
   - Identify the root cause

3. **Implement minimal fix**:
   - Make targeted changes
   - Verify tests pass

4. **Clean up**:
   - Remove debug logging
   - Ensure no regressions

## Test Maintenance

### Regular Tasks

- **Update test environments** when dependencies change
- **Add regression tests** for reported bugs
- **Review test coverage** for new features
- **Clean up obsolete tests** when features are removed

### Code Review Guidelines

- **Verify test coverage** for new features
- **Check test organization** and naming
- **Ensure proper error testing**
- **Validate test isolation** and cleanup

This testing guide ensures consistent, thorough testing practices across the nlm project and provides clear patterns for future development.