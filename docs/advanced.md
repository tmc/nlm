# Advanced Usage

Advanced features, automation, and API details for power users.

## Table of Contents
- [Configuration Management](#configuration-management)
- [Scripting & Automation](#scripting--automation)
- [API Integration](#api-integration)
- [Batch Operations](#batch-operations)
- [Custom Workflows](#custom-workflows)
- [Performance Optimization](#performance-optimization)
- [Security Best Practices](#security-best-practices)
- [Development & Contributing](#development--contributing)

## Configuration Management

### Environment Variables

Complete list of environment variables:

```bash
# Authentication
export NLM_AUTH_TOKEN="your-token"           # Auth token from browser
export NLM_COOKIES="cookie-string"           # Session cookies
export NLM_SAPISID="sapisid-value"          # SAPISID for refresh
export NLM_BROWSER_PROFILE="Profile Name"    # Default browser profile

# Behavior
export NLM_AUTO_REFRESH="true"              # Auto-refresh tokens (default: true)
export NLM_DEBUG="true"                     # Enable debug output
export NLM_TIMEOUT="30"                     # Request timeout in seconds
export NLM_RETRY_COUNT="3"                  # Number of retries for failed requests
export NLM_RETRY_DELAY="1"                  # Initial retry delay in seconds

# Paths
export NLM_CONFIG_DIR="$HOME/.nlm"          # Config directory
export NLM_CACHE_DIR="$HOME/.nlm/cache"     # Cache directory
export NLM_BROWSER_PATH="/path/to/browser"  # Custom browser path
export NLM_PROFILE_PATH="/path/to/profile"  # Custom profile path
```

### Configuration Files

#### ~/.nlm/env
Primary configuration file:
```bash
NLM_AUTH_TOKEN="token"
NLM_COOKIES="cookies"
NLM_BROWSER_PROFILE="Default"
NLM_AUTO_REFRESH="true"
```

#### ~/.nlm/config.yaml (future)
Advanced configuration:
```yaml
auth:
  auto_refresh: true
  refresh_advance: 300  # seconds before expiry
  
network:
  timeout: 30
  retry_count: 3
  retry_delay: 1
  max_retry_delay: 10
  
browser:
  default: chrome
  profiles:
    - name: Work
      path: ~/profiles/work
    - name: Personal
      path: ~/profiles/personal
```

### Multiple Configurations

Manage multiple accounts/configurations:

```bash
#!/bin/bash
# switch-config.sh - Switch between configurations

CONFIG_NAME="$1"
CONFIG_BASE="$HOME/.nlm-configs"

if [ -z "$CONFIG_NAME" ]; then
  echo "Usage: $0 <config-name>"
  echo "Available configs:"
  ls -1 "$CONFIG_BASE"
  exit 1
fi

# Backup current config
if [ -d "$HOME/.nlm" ]; then
  CURRENT=$(readlink "$HOME/.nlm" | xargs basename)
  echo "Current config: $CURRENT"
fi

# Switch to new config
rm -f "$HOME/.nlm"
ln -s "$CONFIG_BASE/$CONFIG_NAME" "$HOME/.nlm"
echo "Switched to: $CONFIG_NAME"

# Verify
nlm auth --check
```

## Scripting & Automation

### Shell Functions

Add to your `.bashrc` or `.zshrc`:

```bash
# Quick notebook creation with ID capture
nlm-create() {
  local title="$1"
  local id=$(nlm create "$title" | grep -o 'notebook/[^"]*' | cut -d'/' -f2)
  echo "$id"
  export LAST_NOTEBOOK="$id"
}

# Add multiple files to last notebook
nlm-add-all() {
  local pattern="${1:-*}"
  for file in $pattern; do
    echo "Adding: $file"
    nlm add "$LAST_NOTEBOOK" "$file"
  done
}

# Generate all content types
nlm-generate-all() {
  local id="${1:-$LAST_NOTEBOOK}"
  local dir="${2:-.}"
  
  nlm generate-guide "$id" > "$dir/guide.md"
  nlm generate-outline "$id" > "$dir/outline.md"
  nlm faq "$id" > "$dir/faq.md"
  nlm glossary "$id" > "$dir/glossary.md"
  nlm timeline "$id" > "$dir/timeline.md"
  nlm briefing-doc "$id" > "$dir/briefing.md"
}

# Search notebooks by title
nlm-search() {
  local query="$1"
  nlm list | grep -i "$query"
}

# Quick share
nlm-quick-share() {
  local id="${1:-$LAST_NOTEBOOK}"
  nlm share-public "$id" | grep -o 'https://[^"]*'
}
```

### Python Integration

```python
#!/usr/bin/env python3
"""nlm_wrapper.py - Python wrapper for nlm CLI"""

import subprocess
import json
import os
from typing import List, Dict, Optional

class NLMClient:
    def __init__(self, auth_token: Optional[str] = None):
        self.auth_token = auth_token or os.environ.get('NLM_AUTH_TOKEN')
        
    def _run_command(self, args: List[str]) -> str:
        """Execute nlm command and return output."""
        env = os.environ.copy()
        if self.auth_token:
            env['NLM_AUTH_TOKEN'] = self.auth_token
            
        result = subprocess.run(
            ['nlm'] + args,
            capture_output=True,
            text=True,
            env=env
        )
        
        if result.returncode != 0:
            raise Exception(f"Command failed: {result.stderr}")
            
        return result.stdout
    
    def list_notebooks(self) -> List[Dict]:
        """List all notebooks."""
        output = self._run_command(['list', '--json'])
        return json.loads(output)['notebooks']
    
    def create_notebook(self, title: str) -> str:
        """Create a new notebook and return its ID."""
        output = self._run_command(['create', title])
        # Parse ID from output
        for line in output.split('\n'):
            if 'notebook/' in line:
                return line.split('notebook/')[1].split('"')[0]
        raise Exception("Failed to parse notebook ID")
    
    def add_source(self, notebook_id: str, source: str) -> None:
        """Add a source to a notebook."""
        self._run_command(['add', notebook_id, source])
    
    def generate_guide(self, notebook_id: str) -> str:
        """Generate a study guide."""
        return self._run_command(['generate-guide', notebook_id])
    
    def create_audio(self, notebook_id: str, instructions: str) -> str:
        """Create an audio overview."""
        return self._run_command(['audio-create', notebook_id, instructions])

# Example usage
if __name__ == "__main__":
    client = NLMClient()
    
    # Create notebook
    notebook_id = client.create_notebook("Python Research")
    print(f"Created notebook: {notebook_id}")
    
    # Add sources
    client.add_source(notebook_id, "python_tutorial.pdf")
    client.add_source(notebook_id, "https://python.org")
    
    # Generate content
    guide = client.generate_guide(notebook_id)
    with open("python_guide.md", "w") as f:
        f.write(guide)
```

### Node.js Integration

```javascript
#!/usr/bin/env node
// nlm-client.js - Node.js wrapper for nlm

const { exec } = require('child_process');
const util = require('util');
const execAsync = util.promisify(exec);

class NLMClient {
  constructor(authToken = process.env.NLM_AUTH_TOKEN) {
    this.authToken = authToken;
  }

  async runCommand(args) {
    const env = { ...process.env };
    if (this.authToken) {
      env.NLM_AUTH_TOKEN = this.authToken;
    }

    try {
      const { stdout, stderr } = await execAsync(`nlm ${args.join(' ')}`, { env });
      return stdout;
    } catch (error) {
      throw new Error(`Command failed: ${error.stderr || error.message}`);
    }
  }

  async listNotebooks() {
    const output = await this.runCommand(['list', '--json']);
    return JSON.parse(output).notebooks;
  }

  async createNotebook(title) {
    const output = await this.runCommand(['create', title]);
    const match = output.match(/notebook\/([^"]+)/);
    if (match) return match[1];
    throw new Error('Failed to parse notebook ID');
  }

  async addSource(notebookId, source) {
    await this.runCommand(['add', notebookId, source]);
  }

  async generateGuide(notebookId) {
    return await this.runCommand(['generate-guide', notebookId]);
  }
}

// Example usage
async function main() {
  const client = new NLMClient();
  
  // Create notebook
  const notebookId = await client.createNotebook('JavaScript Research');
  console.log(`Created notebook: ${notebookId}`);
  
  // Add sources
  await client.addSource(notebookId, 'javascript_guide.pdf');
  
  // Generate guide
  const guide = await client.generateGuide(notebookId);
  require('fs').writeFileSync('js_guide.md', guide);
}

if (require.main === module) {
  main().catch(console.error);
}

module.exports = NLMClient;
```

## API Integration

### Direct API Calls

Understanding the underlying API:

```bash
# BatchExecute endpoint
curl -X POST https://notebooklm.google.com/_/BardChatUi/data/batchexecute \
  -H "Authorization: SAPISIDHASH $HASH" \
  -H "Cookie: $COOKIES" \
  -d "f.req=[[[\"$RPC_ID\",$ARGS]]]"
```

### RPC IDs and Arguments

Common RPC IDs used by nlm:

```go
// From proto definitions
const (
  ListNotebooks    = "8hyCT"
  CreateNotebook   = "D0Ozxc"
  DeleteNotebook   = "h0aFre"
  ListSources      = "tEvFJ"
  CreateSource     = "GmI61b"
  DeleteSource     = "gCHBG"
  GenerateGuide    = "z4tR2d"
  GenerateOutline  = "SfmZu"
  CreateAudio      = "N17Jwe"
)
```

### Custom RPC Calls

```bash
#!/bin/bash
# custom-rpc.sh - Make custom RPC calls

make_rpc_call() {
  local rpc_id="$1"
  local args="$2"
  local token="$NLM_AUTH_TOKEN"
  local cookies="$NLM_COOKIES"
  
  # Generate SAPISIDHASH
  local timestamp=$(date +%s)
  local sapisid=$(echo "$cookies" | grep -o 'SAPISID=[^;]*' | cut -d= -f2)
  local hash=$(echo -n "$timestamp $sapisid https://notebooklm.google.com" | sha1sum | cut -d' ' -f1)
  
  # Make request
  curl -s -X POST \
    "https://notebooklm.google.com/_/BardChatUi/data/batchexecute" \
    -H "Authorization: SAPISIDHASH ${timestamp}_${hash}" \
    -H "Cookie: $cookies" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "f.req=[[['$rpc_id',$args]]]&at=$token"
}

# Example: Custom notebook query
make_rpc_call "8hyCT" '["",null,null,null,null,true]'
```

## Batch Operations

### Parallel Processing

Process multiple notebooks concurrently:

```bash
#!/bin/bash
# parallel-process.sh - Process notebooks in parallel

process_notebook() {
  local id="$1"
  local output_dir="output/$id"
  
  mkdir -p "$output_dir"
  
  # Generate all content types
  nlm generate-guide "$id" > "$output_dir/guide.md" &
  nlm generate-outline "$id" > "$output_dir/outline.md" &
  nlm faq "$id" > "$output_dir/faq.md" &
  nlm glossary "$id" > "$output_dir/glossary.md" &
  
  wait  # Wait for all background jobs
  echo "Completed: $id"
}

export -f process_notebook

# Process all notebooks in parallel (max 4 at a time)
nlm list --json | jq -r '.notebooks[].id' | xargs -P 4 -I {} bash -c 'process_notebook {}'
```

### Bulk Import

Import multiple sources efficiently:

```python
#!/usr/bin/env python3
"""bulk_import.py - Import sources from CSV"""

import csv
import subprocess
import time
from pathlib import Path

def bulk_import(csv_file, notebook_id):
    """Import sources from CSV file.
    
    CSV format:
    type,source,title
    url,https://example.com,Example Site
    file,document.pdf,Important Document
    text,"Direct text content",Note 1
    """
    
    with open(csv_file, 'r') as f:
        reader = csv.DictReader(f)
        
        for row in reader:
            source_type = row['type']
            source = row['source']
            title = row.get('title', '')
            
            print(f"Adding: {title or source}")
            
            if source_type == 'file':
                # Check file exists
                if not Path(source).exists():
                    print(f"  Skipping: File not found - {source}")
                    continue
                    
                cmd = ['nlm', 'add', notebook_id, source]
                
            elif source_type == 'url':
                cmd = ['nlm', 'add', notebook_id, source]
                
            elif source_type == 'text':
                # Use stdin for text
                cmd = ['nlm', 'add', notebook_id, '-']
                
            else:
                print(f"  Skipping: Unknown type - {source_type}")
                continue
            
            # Add title if provided
            if title and source_type != 'text':
                cmd.extend(['--title', title])
            
            # Execute command
            try:
                if source_type == 'text':
                    subprocess.run(cmd, input=source, text=True, check=True)
                else:
                    subprocess.run(cmd, check=True)
                print(f"  Success")
            except subprocess.CalledProcessError as e:
                print(f"  Failed: {e}")
            
            # Rate limiting
            time.sleep(1)

if __name__ == "__main__":
    import sys
    
    if len(sys.argv) != 3:
        print("Usage: bulk_import.py <csv_file> <notebook_id>")
        sys.exit(1)
    
    bulk_import(sys.argv[1], sys.argv[2])
```

### Export All Notebooks

Complete backup solution:

```bash
#!/bin/bash
# export-all.sh - Export all notebooks to markdown

EXPORT_DIR="nlm-export-$(date +%Y%m%d)"
mkdir -p "$EXPORT_DIR"

# Create index file
cat > "$EXPORT_DIR/index.md" << EOF
# NotebookLM Export
Date: $(date)
Total Notebooks: $(nlm list --json | jq '.notebooks | length')

## Notebooks
EOF

# Export each notebook
nlm list --json | jq -r '.notebooks[]' | while read -r notebook; do
  id=$(echo "$notebook" | jq -r '.id')
  title=$(echo "$notebook" | jq -r '.title')
  created=$(echo "$notebook" | jq -r '.created // "unknown"')
  
  echo "Exporting: $title ($id)"
  
  # Create notebook directory
  safe_title=$(echo "$title" | tr ' /' '__' | tr -cd '[:alnum:]_-')
  notebook_dir="$EXPORT_DIR/$safe_title"
  mkdir -p "$notebook_dir"
  
  # Export metadata
  cat > "$notebook_dir/metadata.json" << JSON
{
  "id": "$id",
  "title": "$title",
  "created": "$created",
  "exported": "$(date -Iseconds)"
}
JSON
  
  # Export sources list
  nlm sources "$id" > "$notebook_dir/sources.txt" 2>/dev/null || echo "No sources" > "$notebook_dir/sources.txt"
  
  # Export notes
  nlm notes "$id" > "$notebook_dir/notes.md" 2>/dev/null || echo "No notes" > "$notebook_dir/notes.md"
  
  # Export generated content
  nlm generate-guide "$id" > "$notebook_dir/guide.md" 2>/dev/null
  nlm generate-outline "$id" > "$notebook_dir/outline.md" 2>/dev/null
  nlm faq "$id" > "$notebook_dir/faq.md" 2>/dev/null
  nlm glossary "$id" > "$notebook_dir/glossary.md" 2>/dev/null
  
  # Add to index
  echo "- [$title]($safe_title/guide.md) - $created" >> "$EXPORT_DIR/index.md"
done

# Create archive
tar -czf "$EXPORT_DIR.tar.gz" "$EXPORT_DIR"
echo "Export complete: $EXPORT_DIR.tar.gz"
```

## Custom Workflows

### Research Pipeline

Complete research automation:

```python
#!/usr/bin/env python3
"""research_pipeline.py - Automated research workflow"""

import subprocess
import json
import time
from datetime import datetime
from pathlib import Path
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class ResearchPipeline:
    def __init__(self, topic):
        self.topic = topic
        self.notebook_id = None
        self.output_dir = Path(f"research_{topic.replace(' ', '_')}_{datetime.now():%Y%m%d}")
        self.output_dir.mkdir(exist_ok=True)
        
    def create_notebook(self):
        """Create research notebook."""
        logger.info(f"Creating notebook for: {self.topic}")
        output = subprocess.check_output(['nlm', 'create', f"Research: {self.topic}"], text=True)
        
        # Parse notebook ID
        for line in output.split('\n'):
            if 'notebook/' in line:
                self.notebook_id = line.split('notebook/')[1].split('"')[0]
                logger.info(f"Created notebook: {self.notebook_id}")
                return
                
        raise Exception("Failed to create notebook")
    
    def add_sources(self, sources):
        """Add multiple sources."""
        for source in sources:
            logger.info(f"Adding source: {source}")
            try:
                subprocess.check_call(['nlm', 'add', self.notebook_id, source])
                time.sleep(1)  # Rate limiting
            except subprocess.CalledProcessError as e:
                logger.error(f"Failed to add source: {e}")
    
    def generate_content(self):
        """Generate all content types."""
        content_types = [
            ('guide', 'generate-guide'),
            ('outline', 'generate-outline'),
            ('faq', 'faq'),
            ('glossary', 'glossary'),
            ('timeline', 'timeline'),
            ('briefing', 'briefing-doc')
        ]
        
        for name, command in content_types:
            logger.info(f"Generating: {name}")
            output_file = self.output_dir / f"{name}.md"
            
            try:
                output = subprocess.check_output(['nlm', command, self.notebook_id], text=True)
                output_file.write_text(output)
            except subprocess.CalledProcessError as e:
                logger.error(f"Failed to generate {name}: {e}")
    
    def create_audio(self, instructions=None):
        """Create audio overview."""
        logger.info("Creating audio overview")
        
        if instructions is None:
            instructions = f"Provide a comprehensive overview of {self.topic}"
        
        try:
            subprocess.check_call(['nlm', 'audio-create', self.notebook_id, instructions])
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to create audio: {e}")
    
    def share_notebook(self):
        """Create public share link."""
        logger.info("Creating share link")
        
        try:
            output = subprocess.check_output(['nlm', 'share-public', self.notebook_id], text=True)
            
            # Parse share URL
            for line in output.split('\n'):
                if 'https://' in line:
                    share_url = line.strip()
                    logger.info(f"Share URL: {share_url}")
                    
                    # Save to file
                    (self.output_dir / "share_link.txt").write_text(share_url)
                    return share_url
                    
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to share: {e}")
            
        return None
    
    def create_report(self):
        """Create final report."""
        logger.info("Creating final report")
        
        report = f"""# Research Report: {self.topic}

Generated: {datetime.now():%Y-%m-%d %H:%M:%S}
Notebook ID: {self.notebook_id}

## Summary

This research was automatically generated using NotebookLM.

## Contents

- [Study Guide](guide.md)
- [Outline](outline.md)
- [FAQ](faq.md)
- [Glossary](glossary.md)
- [Timeline](timeline.md)
- [Executive Briefing](briefing.md)

## Sources

See sources.txt for complete list.

## Access

Share link available in share_link.txt
"""
        
        (self.output_dir / "README.md").write_text(report)
        
        # List sources
        sources = subprocess.check_output(['nlm', 'sources', self.notebook_id], text=True)
        (self.output_dir / "sources.txt").write_text(sources)
    
    def run(self, sources):
        """Run complete pipeline."""
        logger.info(f"Starting research pipeline for: {self.topic}")
        
        self.create_notebook()
        self.add_sources(sources)
        self.generate_content()
        self.create_audio()
        share_url = self.share_notebook()
        self.create_report()
        
        logger.info(f"Pipeline complete! Output in: {self.output_dir}")
        return self.output_dir

# Example usage
if __name__ == "__main__":
    pipeline = ResearchPipeline("Artificial Intelligence Ethics")
    
    sources = [
        "https://en.wikipedia.org/wiki/Ethics_of_artificial_intelligence",
        "https://www.nature.com/articles/s42256-019-0088-2",
        "ai_ethics_paper.pdf"
    ]
    
    pipeline.run(sources)
```

### Git Integration

Track research in git:

```bash
#!/bin/bash
# git-research.sh - Version control for research

REPO_DIR="research-repo"
NOTEBOOK_ID="$1"

if [ -z "$NOTEBOOK_ID" ]; then
  echo "Usage: $0 <notebook-id>"
  exit 1
fi

# Initialize repo if needed
if [ ! -d "$REPO_DIR/.git" ]; then
  mkdir -p "$REPO_DIR"
  cd "$REPO_DIR"
  git init
  echo "# Research Repository" > README.md
  git add README.md
  git commit -m "Initial commit"
else
  cd "$REPO_DIR"
fi

# Create branch for this notebook
BRANCH="notebook-$NOTEBOOK_ID-$(date +%Y%m%d)"
git checkout -b "$BRANCH"

# Export notebook content
nlm generate-guide "$NOTEBOOK_ID" > guide.md
nlm generate-outline "$NOTEBOOK_ID" > outline.md
nlm faq "$NOTEBOOK_ID" > faq.md
nlm sources "$NOTEBOOK_ID" > sources.txt

# Commit changes
git add -A
git commit -m "Update research from notebook $NOTEBOOK_ID

$(nlm list --json | jq -r ".notebooks[] | select(.id==\"$NOTEBOOK_ID\") | .title")"

# Push if remote exists
if git remote | grep -q origin; then
  git push -u origin "$BRANCH"
fi

echo "Research committed to branch: $BRANCH"
```

## Performance Optimization

### Caching Strategies

Implement local caching:

```python
#!/usr/bin/env python3
"""cache_manager.py - Local caching for nlm operations"""

import json
import hashlib
import time
from pathlib import Path
import subprocess

class CacheManager:
    def __init__(self, cache_dir="~/.nlm/cache"):
        self.cache_dir = Path(cache_dir).expanduser()
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        
    def _get_cache_key(self, command, args):
        """Generate cache key from command and args."""
        key_string = f"{command}:{':'.join(args)}"
        return hashlib.md5(key_string.encode()).hexdigest()
    
    def _get_cache_file(self, key):
        """Get cache file path."""
        return self.cache_dir / f"{key}.json"
    
    def get(self, command, args, max_age=3600):
        """Get cached result if available and fresh."""
        key = self._get_cache_key(command, args)
        cache_file = self._get_cache_file(key)
        
        if cache_file.exists():
            data = json.loads(cache_file.read_text())
            age = time.time() - data['timestamp']
            
            if age < max_age:
                return data['result']
                
        return None
    
    def set(self, command, args, result):
        """Cache a result."""
        key = self._get_cache_key(command, args)
        cache_file = self._get_cache_file(key)
        
        data = {
            'command': command,
            'args': args,
            'result': result,
            'timestamp': time.time()
        }
        
        cache_file.write_text(json.dumps(data, indent=2))
    
    def clear(self, older_than=None):
        """Clear cache, optionally only items older than specified seconds."""
        now = time.time()
        
        for cache_file in self.cache_dir.glob("*.json"):
            if older_than:
                data = json.loads(cache_file.read_text())
                age = now - data['timestamp']
                
                if age > older_than:
                    cache_file.unlink()
            else:
                cache_file.unlink()

# Wrapper function for cached nlm calls
def cached_nlm(command, args, cache_manager=None, max_age=3600):
    """Execute nlm command with caching."""
    if cache_manager is None:
        cache_manager = CacheManager()
    
    # Check cache
    result = cache_manager.get(command, args, max_age)
    if result:
        return result
    
    # Execute command
    full_command = ['nlm', command] + args
    result = subprocess.check_output(full_command, text=True)
    
    # Cache result
    cache_manager.set(command, args, result)
    
    return result

# Example usage
if __name__ == "__main__":
    cache = CacheManager()
    
    # Cached list operation
    notebooks = cached_nlm('list', ['--json'], cache, max_age=300)
    print(f"Found {len(json.loads(notebooks)['notebooks'])} notebooks")
    
    # Clear old cache entries
    cache.clear(older_than=86400)  # Clear items older than 1 day
```

### Connection Pooling

Optimize network connections:

```go
// connection_pool.go - HTTP client with connection pooling

package main

import (
    "net/http"
    "time"
)

func NewOptimizedClient() *http.Client {
    transport := &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
        DisableKeepAlives:   false,
        DisableCompression:  false,
    }
    
    return &http.Client{
        Transport: transport,
        Timeout:   30 * time.Second,
    }
}
```

## Security Best Practices

### Credential Management

Secure credential storage:

```bash
#!/bin/bash
# secure-creds.sh - Encrypted credential storage

# Encrypt credentials
encrypt_creds() {
  local passphrase="$1"
  
  # Encrypt env file
  openssl enc -aes-256-cbc -salt -in ~/.nlm/env -out ~/.nlm/env.enc -pass pass:"$passphrase"
  
  # Remove plain text
  shred -u ~/.nlm/env
  
  echo "Credentials encrypted"
}

# Decrypt credentials
decrypt_creds() {
  local passphrase="$1"
  
  # Decrypt to memory
  export NLM_AUTH_TOKEN=$(openssl enc -d -aes-256-cbc -in ~/.nlm/env.enc -pass pass:"$passphrase" | grep NLM_AUTH_TOKEN | cut -d= -f2)
  export NLM_COOKIES=$(openssl enc -d -aes-256-cbc -in ~/.nlm/env.enc -pass pass:"$passphrase" | grep NLM_COOKIES | cut -d= -f2)
  
  echo "Credentials loaded into environment"
}

# Use with nlm
secure_nlm() {
  read -s -p "Passphrase: " passphrase
  echo
  
  decrypt_creds "$passphrase"
  nlm "$@"
  
  # Clear from environment
  unset NLM_AUTH_TOKEN
  unset NLM_COOKIES
}
```

### Audit Logging

Track all nlm operations:

```bash
#!/bin/bash
# audit-nlm.sh - Wrapper with audit logging

AUDIT_LOG="$HOME/.nlm/audit.log"

# Ensure log directory exists
mkdir -p "$(dirname "$AUDIT_LOG")"

# Log command
echo "[$(date -Iseconds)] USER=$USER CMD: nlm $*" >> "$AUDIT_LOG"

# Execute command
nlm "$@"
EXIT_CODE=$?

# Log result
echo "[$(date -Iseconds)] USER=$USER RESULT: $EXIT_CODE" >> "$AUDIT_LOG"

exit $EXIT_CODE
```

## Development & Contributing

### Building from Source

```bash
# Clone repository
git clone https://github.com/tmc/nlm.git
cd nlm

# Install dependencies
go mod download

# Build
go build -o nlm ./cmd/nlm

# Run tests
go test ./...

# Install locally
go install ./cmd/nlm
```

### Adding New Commands

Example of adding a new command:

```go
// cmd/nlm/custom.go

package main

import (
    "fmt"
    "os"
)

func cmdCustom(args []string) error {
    if len(args) < 1 {
        fmt.Fprintf(os.Stderr, "usage: nlm custom <notebook-id>\n")
        return fmt.Errorf("invalid arguments")
    }
    
    notebookID := args[0]
    
    // Implement custom logic
    client := getClient()
    result, err := client.CustomOperation(notebookID)
    if err != nil {
        return fmt.Errorf("custom operation failed: %w", err)
    }
    
    fmt.Println(result)
    return nil
}

// Add to main.go command switch:
// case "custom":
//     return cmdCustom(args[1:])
```

### Testing

Write tests for new functionality:

```go
// custom_test.go

package main

import (
    "testing"
)

func TestCustomCommand(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        wantErr bool
    }{
        {
            name:    "valid notebook ID",
            args:    []string{"abc123"},
            wantErr: false,
        },
        {
            name:    "missing notebook ID",
            args:    []string{},
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := cmdCustom(tt.args)
            if (err != nil) != tt.wantErr {
                t.Errorf("cmdCustom() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Contributing Guidelines

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b feature/amazing-feature`
3. **Make changes and test**: `go test ./...`
4. **Commit with descriptive message**: `git commit -m "cmd: add custom command for X"`
5. **Push to your fork**: `git push origin feature/amazing-feature`
6. **Open a Pull Request**

### Debugging

Enable verbose debugging:

```bash
# Maximum debug output
export NLM_DEBUG=true
export NLM_VERBOSE=true
export NLM_LOG_LEVEL=trace

# Run with strace (Linux)
strace -e trace=network nlm list

# Run with dtruss (macOS)
sudo dtruss -f nlm list

# Profile performance
go build -o nlm.prof -cpuprofile=cpu.prof ./cmd/nlm
./nlm.prof list
go tool pprof cpu.prof
```

## Next Steps

- [Review command reference](commands.md)
- [See practical examples](examples.md)
- [Troubleshoot issues](troubleshooting.md)
- [Contribute on GitHub](https://github.com/tmc/nlm)