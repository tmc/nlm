# nlm - NotebookLM CLI Tool 📚

[![Go Reference](https://pkg.go.dev/badge/github.com/tmc/nlm.svg)](https://pkg.go.dev/github.com/tmc/nlm)
[![Go Report Card](https://goreportcard.com/badge/github.com/tmc/nlm)](https://goreportcard.com/report/github.com/tmc/nlm)

A powerful command-line interface for Google's NotebookLM, allowing you to manage notebooks, sources, notes, audio/video overviews, and AI-powered content transformation from your terminal.

🔊 **Listen to an Audio Overview of this tool**: [NotebookLM Audio Demo](https://notebooklm.google.com/notebook/437c839c-5a24-455b-b8da-d35ba8931811/audio)

## ✨ Features

- **📖 Complete Notebook Management** - Create, list, delete, and analyze notebooks
- **📁 Source Operations** - Add URLs, files, text, and YouTube videos as sources
- **📝 Note Management** - Create, edit, and organize notes within notebooks
- **🎧 Audio/Video Overviews** - Generate and download AI-powered audio and video summaries
- **🛠️ Artifact Creation** - Create interactive reports, apps, and other content artifacts
- **🤖 AI Content Transformation** - Rephrase, summarize, expand, critique, and transform content
- **💬 Interactive Chat** - Have conversations with your notebook content
- **🔍 Advanced Generation** - Create guides, outlines, study materials, timelines, and mindmaps
- **🔐 Multi-Browser Authentication** - Seamless auth with Chrome, Brave, Edge, and more
- **⚡ Batch Operations** - Execute multiple commands efficiently
- **🔗 Smart Sharing** - Share notebooks publicly or privately with fine-grained control

## 🚀 Installation

### Quick Install (Recommended)

```bash
go install github.com/tmc/nlm/cmd/nlm@latest
```

### Build from Source

```bash
git clone https://github.com/tmc/nlm.git
cd nlm
go build -o nlm ./cmd/nlm
```

<details>
<summary>📦 Installing Go (if needed)</summary>

### Option 1: Using Package Managers

**macOS (using Homebrew):**
```bash
brew install go
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt update
sudo apt install golang
```

**Linux (Fedora):**
```bash
sudo dnf install golang
```

### Option 2: Direct Download

1. Visit the [Go Downloads page](https://go.dev/dl/)
2. Download the appropriate version for your OS
3. Follow the installation instructions

**macOS:**
- Download the .pkg file and run the installer

**Linux:**
```bash
# Example for Linux AMD64
wget https://go.dev/dl/go1.24.6.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.6.linux-amd64.tar.gz
```

### Post-Installation Setup

Add Go to your PATH:
```bash
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$(go env GOPATH)/bin
```

Verify installation:
```bash
go version
```
</details>

## 🔑 Authentication

### Quick Start

```bash
# First-time setup - opens browser for Google account authentication
nlm auth

# List your notebooks to verify authentication
nlm list
```

### Advanced Authentication

The nlm tool provides sophisticated browser-based authentication with automatic profile detection:

```bash
# Use specific browser profile
nlm auth --profile "Work Profile"

# Try all available browser profiles
nlm auth --all

# Check which profiles have NotebookLM access
nlm auth --notebooks

# Keep browser open for manual login (useful for troubleshooting)
nlm auth --keep-open 30

# Enable detailed authentication debugging
nlm auth --debug
```

### Supported Browsers

The tool automatically detects and works with:
- **Google Chrome** (default)
- **Google Chrome Canary**
- **Brave Browser**
- **Microsoft Edge**
- **Chromium**

### Browser Profile Management

The authentication system intelligently:
- Scans all browser profiles automatically
- Prioritizes profiles with existing NotebookLM cookies
- Uses most recently used profiles when possible
- Preserves complete browser session state for better compatibility


## 💻 Command Reference

### 📖 Notebook Operations

```bash
# List all notebooks
nlm list
nlm ls                           # Shorthand

# Create a new notebook
nlm create "AI Research Notes"

# Delete a notebook
nlm rm <notebook-id>

# Show notebook analytics
nlm analytics <notebook-id>

# List featured/recommended notebooks
nlm list-featured
```

### 📁 Source Management

```bash
# List sources in a notebook
nlm sources <notebook-id>

# Add sources from various inputs
nlm add <notebook-id> https://example.com/article
nlm add <notebook-id> research-paper.pdf
nlm add <notebook-id> data.json
nlm add <notebook-id> "Direct text content"
nlm add <notebook-id> https://youtube.com/watch?v=abc123

# Add from stdin with MIME type specification
cat document.xml | nlm add <notebook-id> - --mime="text/xml"
echo "Meeting notes" | nlm add <notebook-id> -

# Source operations
nlm rename-source <source-id> "New Title"
nlm refresh-source <source-id>
nlm check-source <source-id>
nlm rm-source <notebook-id> <source-id>

# Discover relevant sources
nlm discover-sources <notebook-id> "machine learning papers"
```

### 📝 Note Management

```bash
# List notes in a notebook
nlm notes <notebook-id>

# Create and manage notes
nlm new-note <notebook-id> "Meeting Summary"
nlm update-note <notebook-id> <note-id> "Updated content" "New Title"
nlm rm-note <note-id>
```

### 🎧 Audio & Video Overviews

```bash
# Audio overview operations
nlm audio-list <notebook-id>
nlm audio-create <notebook-id> "Speak in a professional tone, focus on key insights"
nlm audio-get <notebook-id>
nlm audio-download <notebook-id> overview.mp3
nlm audio-share <notebook-id>
nlm audio-rm <notebook-id>

# Video overview operations
nlm video-list <notebook-id>
nlm video-create <notebook-id> "Create an engaging visual summary"
nlm video-download <notebook-id> summary.mp4
```

### 🛠️ Artifact Creation

```bash
# Create interactive artifacts
nlm create-artifact <notebook-id> note     # Interactive note
nlm create-artifact <notebook-id> audio    # Audio overview
nlm create-artifact <notebook-id> report   # Structured report
nlm create-artifact <notebook-id> app      # Interactive app

# Manage artifacts
nlm artifacts <notebook-id>
nlm list-artifacts <notebook-id>           # Alias
nlm get-artifact <artifact-id>
nlm rename-artifact <artifact-id> "New Title"
nlm delete-artifact <artifact-id>
```

### 🤖 AI Content Transformation

Transform your notebook content with AI-powered operations:

```bash
# Content transformation (specify source IDs to focus on specific sources)
nlm rephrase <notebook-id> <source-id1> <source-id2>
nlm expand <notebook-id> <source-id1>
nlm summarize <notebook-id> <source-id1> <source-id2>
nlm critique <notebook-id> <source-id1>
nlm brainstorm <notebook-id> <source-id1> <source-id2>

# Analysis and verification
nlm verify <notebook-id> <source-id1>
nlm explain <notebook-id> <source-id1>

# Study materials generation
nlm study-guide <notebook-id> <source-id1> <source-id2>
nlm faq <notebook-id> <source-id1>
nlm briefing-doc <notebook-id> <source-id1> <source-id2>

# Visual organization
nlm mindmap <notebook-id> <source-id1> <source-id2>
nlm timeline <notebook-id> <source-id1>
nlm outline <notebook-id> <source-id1> <source-id2>
nlm toc <notebook-id> <source-id1>
```

### 🔍 AI Generation & Chat

```bash
# Content generation
nlm generate-guide <notebook-id>
nlm generate-outline <notebook-id>
nlm generate-section <notebook-id>
nlm generate-chat <notebook-id> "Explain the main themes"
nlm generate-magic <notebook-id> <source-id1> <source-id2>

# Interactive chat
nlm chat <notebook-id>              # Start interactive session
nlm chat-list                       # List saved chats
```

### 🔗 Sharing & Collaboration

```bash
# Share notebooks
nlm share <notebook-id>             # Public sharing
nlm share-private <notebook-id>     # Private sharing
nlm share-details <share-id>        # Get sharing details
```

### 🔧 Utility Commands

```bash
# Authentication management
nlm auth [profile]                  # Setup/refresh authentication
nlm refresh                         # Refresh credentials

# System operations
nlm feedback "Great tool!"          # Submit feedback
nlm hb                              # Send heartbeat
```

## 📋 Usage Examples

### Complete Workflow Example

```bash
# 1. Create a new research notebook
notebook_id=$(nlm create "AI Ethics Research" | grep -o 'notebook [^ ]*' | cut -d' ' -f2)

# 2. Add diverse sources
nlm add $notebook_id https://arxiv.org/abs/2103.00020
nlm add $notebook_id "AI ethics guidelines.pdf"
nlm add $notebook_id https://youtube.com/watch?v=ethics-talk
nlm add $notebook_id "Personal notes: Key ethical concerns in AI deployment"

# 3. Generate study materials
nlm study-guide $notebook_id
nlm generate-outline $notebook_id
nlm faq $notebook_id

# 4. Create audio overview
nlm audio-create $notebook_id "Professional tone, focus on practical implications"

# 5. Transform content for different audiences
nlm rephrase $notebook_id        # Academic to accessible language
nlm briefing-doc $notebook_id    # Executive summary
nlm mindmap $notebook_id         # Visual organization

# 6. Share your research
nlm share-private $notebook_id
```

### Batch Content Processing

```bash
# Process multiple sources for comprehensive analysis
sources=$(nlm sources $notebook_id | grep -o 'Source [^ ]*' | cut -d' ' -f2)

# Generate multiple content types
for source in $sources; do
    nlm summarize $notebook_id $source
    nlm explain $notebook_id $source
done

# Create comprehensive study package
nlm study-guide $notebook_id
nlm timeline $notebook_id
nlm generate-guide $notebook_id
```

## 🔧 Advanced Configuration

### Environment Variables

```bash
# Authentication (automatically managed by nlm auth)
export NLM_AUTH_TOKEN="your-token"
export NLM_COOKIES="your-cookies"
export NLM_BROWSER_PROFILE="Work Profile"

```

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
nlm --debug list
nlm --debug auth --profile "Work Profile"
```

### Testing

```bash
# Run unit tests
go test ./...

# Run integration tests (requires credentials)
go test -tags=integration ./...

# Run CLI tests
go test ./cmd/nlm
```

## 🛠️ Development

### Project Structure

```
├── cmd/nlm/              # CLI application
├── internal/
│   ├── api/              # NotebookLM API client
│   ├── auth/             # Browser-based authentication
│   ├── batchexecute/     # Google BatchExecute protocol
│   └── httprr/           # HTTP record/replay for testing
├── gen/                  # Generated protobuf code
└── proto/                # Protocol buffer definitions
```

### Building

```bash
# Build for current platform
go build ./cmd/nlm

# Build for multiple platforms
GOOS=linux go build ./cmd/nlm
GOOS=windows go build ./cmd/nlm
GOOS=darwin go build ./cmd/nlm
```

## 🔒 Security & Privacy

- **Local Authentication**: Tokens stored securely in `~/.nlm/env`
- **Browser Integration**: Uses existing browser sessions, no password storage
- **Profile Isolation**: Copies essential browser data only, preserves privacy
- **No Data Collection**: Tool operates entirely locally with direct Google API calls

## 🐛 Troubleshooting

### Common Issues

**Authentication fails:**
```bash
# Try with debug output
nlm auth --debug

# Try different browser profile
nlm auth --all --notebooks

# Use manual authentication
nlm auth --keep-open 60
```

**Network errors:**
```bash
# Check connection
nlm hb

# Refresh credentials
nlm refresh
```


### Getting Help

1. Check the debug output: `nlm --debug <command>`
2. Verify authentication: `nlm auth --debug`
3. Test connectivity: `nlm hb`
4. Submit feedback: `nlm feedback "Describe your issue"`

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Submit a pull request

### Development Guidelines

- Follow standard Go conventions (`gofmt`)
- Add comprehensive tests for new features
- Update documentation for user-facing changes
- Use conventional commit messages

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

## 🙏 Acknowledgments

- Built on Google's NotebookLM platform
- Powered by Google's Batchexecute API protocol
- Inspired by the need for efficient notebook management workflows