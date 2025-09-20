# Contributing to nlm

Thank you for your interest in contributing to nlm! This document provides guidelines and information for contributors.

## 🚀 Getting Started

### Prerequisites

- **Go 1.24+** - The project uses Go 1.24 features
- **Git** - For version control
- **A Google account** - For testing authentication

### Setting Up Development Environment

1. **Fork and clone the repository:**
   ```bash
   git clone https://github.com/your-username/nlm.git
   cd nlm
   ```

2. **Install dependencies:**
   ```bash
   go mod download
   ```

3. **Build the project:**
   ```bash
   go build -o nlm ./cmd/nlm
   ```

4. **Run tests:**
   ```bash
   # Unit tests
   go test ./...

   # Integration tests (requires authentication)
   go test -tags=integration ./...
   ```

## 🧭 Project Structure

```
├── cmd/nlm/              # CLI application entry point
├── internal/
│   ├── api/              # NotebookLM API client
│   ├── auth/             # Browser-based authentication
│   ├── batchexecute/     # Google BatchExecute protocol
│   ├── beprotojson/      # Protocol buffer JSON handling
│   ├── httprr/           # HTTP record/replay for testing
│   └── rpc/              # RPC utilities
├── gen/                  # Generated protobuf code
├── proto/                # Protocol buffer definitions
└── testdata/             # Test data files
```

## 📝 Development Guidelines

### Code Style

- **Follow Go conventions**: Use `gofmt`, `go vet`, and `golint`
- **Variable naming**: Use descriptive names (e.g., `notebookID`, not `id`)
- **Error handling**: Always handle errors, use `fmt.Errorf("context: %w", err)` for wrapping
- **Exported functions**: Use PascalCase, unexported use camelCase
- **Comments**: Add godoc comments for exported functions and types

### Writing Code

1. **Error Handling Pattern:**
   ```go
   if err != nil {
       return fmt.Errorf("operation failed: %w", err)
   }
   ```

2. **Testing Pattern:**
   ```go
   func TestFeatureName(t *testing.T) {
       tests := []struct {
           name     string
           input    string
           expected string
           wantErr  bool
       }{
           // test cases
       }

       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               // test implementation
           })
       }
   }
   ```

3. **CLI Command Pattern:**
   ```go
   case "command-name":
       if len(args) != expectedCount {
           fmt.Fprintf(os.Stderr, "usage: nlm command-name <arg1> <arg2>\n")
           return fmt.Errorf("invalid arguments")
       }
       return handleCommand(args[0], args[1])
   ```

### Commit Messages

Use conventional commit format:

```
<type>(<scope>): <description>

<body>

<footer>
```

**Types:**
- `feat`: New features
- `fix`: Bug fixes
- `docs`: Documentation changes
- `style`: Code style changes
- `refactor`: Code refactoring
- `test`: Test-related changes
- `chore`: Maintenance tasks

**Examples:**
```
feat(auth): add support for Chrome Canary profiles

Add automatic detection and authentication support for Chrome Canary
browser profiles on macOS. Includes profile validation and fallback
mechanisms for better compatibility.

Closes #123
```

## 🧪 Testing

### Test Categories

1. **Unit Tests** - Fast, isolated tests for individual functions
2. **Integration Tests** - Tests requiring real API calls (use `-tags=integration`)
3. **CLI Tests** - End-to-end command-line interface tests

### Running Tests

```bash
# Run all unit tests
go test ./...

# Run with verbose output
go test -v ./...

# Run integration tests (requires auth setup)
go test -tags=integration ./...

# Run specific test
go test -run TestSpecificFunction ./internal/api

# Run CLI script tests
go test ./cmd/nlm
```

### Writing Tests

1. **Unit Tests**: Test individual functions with mocked dependencies
2. **Integration Tests**: Use build tag `//go:build integration`
3. **CLI Tests**: Use scripttest framework for command-line testing

Example unit test:
```go
func TestParseNotebookID(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "valid notebook ID",
            input:    "notebook-123",
            expected: "notebook-123",
            wantErr:  false,
        },
        // more test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := ParseNotebookID(tt.input)
            if tt.wantErr && err == nil {
                t.Error("expected error but got none")
            }
            if !tt.wantErr && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
            if result != tt.expected {
                t.Errorf("expected %q, got %q", tt.expected, result)
            }
        })
    }
}
```

## 🐛 Debugging

### Debug Mode

Enable debug output for troubleshooting:

```bash
# Debug CLI commands
./nlm --debug list

# Debug authentication
./nlm auth --debug

# Debug API calls
NLM_DEBUG=1 ./nlm list
```

### Common Debug Scenarios

1. **Authentication Issues**: Use `nlm auth --debug` to see browser automation
2. **API Errors**: Add `--debug` flag to see request/response details
3. **Test Failures**: Use `go test -v` for verbose output

## 📚 Adding New Features

### Adding a New Command

1. **Add command to help text** in `cmd/nlm/main.go`
2. **Add case to command switch** in `main()` function
3. **Add argument validation** in `validateArgs()` function
4. **Implement command function** following existing patterns
5. **Add tests** in `cmd/nlm/testdata/` directory
6. **Update documentation** in README.md

Example:
```go
// In validateArgs()
case "new-command":
    if len(args) != 2 {
        fmt.Fprintf(os.Stderr, "usage: nlm new-command <arg1> <arg2>\n")
        return fmt.Errorf("invalid arguments")
    }

// In main switch
case "new-command":
    err = handleNewCommand(client, args[0], args[1])
```

### Adding New API Endpoints

1. **Define RPC method** in appropriate proto file
2. **Generate code**: `go generate ./...`
3. **Implement client method** in `internal/api/`
4. **Add integration tests** with `//go:build integration`
5. **Update CLI command** to use new method

## 📖 Documentation

### Updating Documentation

- **README.md**: User-facing documentation and examples
- **Godoc comments**: For exported functions and types
- **CLI help text**: Keep in sync with actual commands
- **Test files**: Document complex test scenarios

### Documentation Standards

- Use clear, concise language
- Provide working examples
- Include error scenarios
- Keep examples up-to-date with current API

## 🔍 Code Review

### Before Submitting a PR

1. **Run tests**: `go test ./...`
2. **Check formatting**: `gofmt -w .`
3. **Run linters**: `go vet ./...`
4. **Update documentation** if needed
5. **Add/update tests** for new functionality

### PR Guidelines

- **Clear title** describing the change
- **Detailed description** explaining what and why
- **Link issues** if applicable
- **Include tests** for new features
- **Update docs** if user-facing changes

## 🆘 Getting Help

- **Issues**: Check existing issues or create a new one
- **Discussions**: Use GitHub Discussions for questions
- **Code review**: Submit PRs for feedback
- **Testing**: Use `nlm feedback "your message"` for tool feedback

## 🎯 Contribution Ideas

### Good First Issues

- **Documentation improvements**
- **Test coverage increases**
- **Bug fixes with clear reproduction steps**
- **CLI usability improvements**

### Advanced Contributions

- **New content transformation commands**
- **Performance optimizations**
- **Enhanced error handling**
- **Cross-platform compatibility improvements**

Thank you for contributing to nlm! 🚀