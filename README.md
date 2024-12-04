# nlm - NotebookLM CLI Tool 📚

`nlm` is a command-line interface for Google's NotebookLM, allowing you to manage notebooks, sources, and audio overviews from your terminal.

🔊 Listen to an Audio Overview of this tool here: [https://notebooklm.google.com/notebook/437c839c-5a24-455b-b8da-d35ba8931811/audio](https://notebooklm.google.com/notebook/437c839c-5a24-455b-b8da-d35ba8931811/audio).

## Installation 🚀

```bash
go install github.com/tmc/nlm/cmd/nlm@latest
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
3. Follow the installation instructions:

**macOS:**
- Download the .pkg file
- Double-click to install
- Follow the installer prompts

**Linux:**
```bash
# Example for Linux AMD64 (adjust version as needed)
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
```

### Post-Installation Setup

Add Go to your PATH by adding these lines to your `~/.bashrc`, `~/.zshrc`, or equivalent:
```bash
export PATH=$PATH:/usr/local/go/bin
export PATH=$PATH:$(go env GOPATH)/bin
```

Verify installation:
```bash
go version
```
</details>

## Authentication 🔑

First, authenticate with your Google account:

```bash
nlm auth
```

This will launch Chrome to authenticate with your Google account. The authentication tokens will be saved in `.env` file.

## Usage 💻

### Notebook Operations

```bash
# List all notebooks
nlm list

# Create a new notebook
nlm create "My Research Notes"

# Delete a notebook
nlm rm <notebook-id>

# Get notebook analytics
nlm analytics <notebook-id>
```

### Source Management

```bash
# List sources in a notebook
nlm sources <notebook-id>

# Add a source from URL
nlm add <notebook-id> https://example.com/article

# Add a source from file
nlm add <notebook-id> document.pdf

# Add source from stdin
echo "Some text" | nlm add <notebook-id> -

# Rename a source
nlm rename-source <source-id> "New Title"

# Remove a source
nlm rm-source <notebook-id> <source-id>
```

### Note Operations

```bash
# List notes in a notebook
nlm notes <notebook-id>

# Create a new note
nlm new-note <notebook-id> "Note Title"

# Edit a note
nlm edit-note <notebook-id> <note-id> "New content"

# Remove a note
nlm rm-note <note-id>
```

### Audio Overview

```bash
# Create an audio overview
nlm audio-create <notebook-id> "speak in a professional tone"

# Get audio overview status/content
nlm audio-get <notebook-id>

# Share audio overview (private)
nlm audio-share <notebook-id>

# Share audio overview (public)
nlm audio-share <notebook-id> --public
```

## Examples 📋

Create a notebook and add some content:
```bash
# Create a new notebook
notebook_id=$(nlm create "Research Notes" | grep -o 'notebook [^ ]*' | cut -d' ' -f2)

# Add some sources
nlm add $notebook_id https://example.com/research-paper
nlm add $notebook_id research-data.pdf

# Create an audio overview
nlm audio-create $notebook_id "summarize in a professional tone"

# Check the audio overview
nlm audio-get $notebook_id
```

## Advanced Usage 🔧

### Debug Mode

Add `-debug` flag to see detailed API interactions:

```bash
nlm -debug list
```

### Environment Variables

- `NLM_AUTH_TOKEN`: Authentication token
- `NLM_COOKIES`: Authentication cookies

These are typically set by the `auth` command, but can be manually configured if needed.

## Contributing 🤝

Contributions are welcome! Please feel free to submit a Pull Request.

## License 📄

MIT License - see [LICENSE](LICENSE) for details.
