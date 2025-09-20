# nlm Command Examples 📚

This document provides comprehensive examples of using the nlm CLI tool for various workflows and use cases.

## 🚀 Quick Start Examples

### Basic Workflow

```bash
# Authenticate with Google account
nlm auth

# List existing notebooks
nlm list

# Create a new notebook
nlm create "Meeting Notes - Q1 Planning"

# Add some sources (replace NOTEBOOK_ID with actual ID from create command)
nlm add NOTEBOOK_ID "https://docs.google.com/document/d/agenda-link"
nlm add NOTEBOOK_ID "project-requirements.pdf"
nlm add NOTEBOOK_ID "Meeting recordings and key decisions from Q1 planning session"

# Generate an audio overview
nlm audio-create NOTEBOOK_ID "Professional tone, focus on action items and decisions"
```

## 📖 Notebook Management

### Creating and Managing Notebooks

```bash
# Create notebooks with descriptive titles
nlm create "Research: AI Ethics in Healthcare"
nlm create "Project Alpha - Technical Specifications"
nlm create "Conference Notes: DevCon 2024"

# List all notebooks with formatting
nlm list
nlm ls  # Shorthand version

# Get detailed analytics for a notebook
nlm analytics NOTEBOOK_ID

# List featured/recommended notebooks
nlm list-featured

# Delete a notebook when done
nlm rm NOTEBOOK_ID
```

## 📁 Source Management

### Adding Different Types of Sources

```bash
# Web articles and documentation
nlm add NOTEBOOK_ID "https://arxiv.org/abs/2103.00020"
nlm add NOTEBOOK_ID "https://blog.example.com/ai-trends-2024"
nlm add NOTEBOOK_ID "https://github.com/example/repo/blob/main/README.md"

# YouTube videos and educational content
nlm add NOTEBOOK_ID "https://youtube.com/watch?v=dQw4w9WgXcQ"
nlm add NOTEBOOK_ID "https://youtu.be/shortlink"

# Local files (various formats)
nlm add NOTEBOOK_ID "./research-paper.pdf"
nlm add NOTEBOOK_ID "./data-analysis.xlsx"
nlm add NOTEBOOK_ID "./meeting-transcript.docx"
nlm add NOTEBOOK_ID "./presentation.pptx"

# Text content directly
nlm add NOTEBOOK_ID "Key insights from team meeting: Focus on user experience improvements"

# Content from stdin with explicit MIME types
cat important-data.xml | nlm add NOTEBOOK_ID - --mime="text/xml"
echo "Quick note about API changes" | nlm add NOTEBOOK_ID -
curl -s "https://api.example.com/data.json" | nlm add NOTEBOOK_ID - --mime="application/json"
```

### Managing Existing Sources

```bash
# List all sources in a notebook
nlm sources NOTEBOOK_ID

# Rename a source for better organization
nlm rename-source SOURCE_ID "Updated: Q3 Financial Report"

# Refresh content from a URL source
nlm refresh-source SOURCE_ID

# Check if source content is up to date
nlm check-source SOURCE_ID

# Remove a source that's no longer relevant
nlm rm-source NOTEBOOK_ID SOURCE_ID

# Discover related sources based on content
nlm discover-sources NOTEBOOK_ID "machine learning research papers"
nlm discover-sources NOTEBOOK_ID "project management best practices"
```

## 📝 Note Management

### Working with Notes

```bash
# List all notes in a notebook
nlm notes NOTEBOOK_ID

# Create a new note with a title
nlm new-note NOTEBOOK_ID "Executive Summary"
nlm new-note NOTEBOOK_ID "Action Items from Meeting"

# Update a note with new content and title
nlm update-note NOTEBOOK_ID NOTE_ID "Updated meeting notes with final decisions" "Final Meeting Summary"

# Remove a note that's no longer needed
nlm rm-note NOTE_ID
```

## 🎧 Audio and Video Overviews

### Audio Overview Workflows

```bash
# List existing audio overviews
nlm audio-list NOTEBOOK_ID

# Create audio overviews with different styles
nlm audio-create NOTEBOOK_ID "Professional tone, focus on key findings"
nlm audio-create NOTEBOOK_ID "Conversational style, explain complex concepts simply"
nlm audio-create NOTEBOOK_ID "Academic presentation style, include methodology details"

# Get status and details of audio overview
nlm audio-get NOTEBOOK_ID

# Download the audio file
nlm audio-download NOTEBOOK_ID meeting-summary.mp3
nlm audio-download NOTEBOOK_ID  # Uses default filename

# Share audio overview
nlm audio-share NOTEBOOK_ID

# Clean up when done
nlm audio-rm NOTEBOOK_ID
```

### Video Overview Workflows

```bash
# List video overviews
nlm video-list NOTEBOOK_ID

# Create video overview with specific instructions
nlm video-create NOTEBOOK_ID "Create engaging visual summary with key charts and graphs"
nlm video-create NOTEBOOK_ID "Educational style video, suitable for team training"

# Download video file
nlm video-download NOTEBOOK_ID project-overview.mp4
```

## 🛠️ Artifact Creation

### Creating Interactive Content

```bash
# Create different types of artifacts
nlm create-artifact NOTEBOOK_ID note     # Interactive note
nlm create-artifact NOTEBOOK_ID audio    # Audio overview artifact
nlm create-artifact NOTEBOOK_ID report   # Structured report
nlm create-artifact NOTEBOOK_ID app      # Interactive application

# Manage artifacts
nlm artifacts NOTEBOOK_ID                # List all artifacts
nlm get-artifact ARTIFACT_ID            # Get artifact details
nlm rename-artifact ARTIFACT_ID "Q3 Analysis Dashboard"
nlm delete-artifact ARTIFACT_ID         # Remove when done
```

## 🤖 AI Content Transformation

### Content Analysis and Transformation

```bash
# Transform content for different audiences
nlm rephrase NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2      # Make more accessible
nlm expand NOTEBOOK_ID SOURCE_ID1                   # Add more detail
nlm summarize NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2     # Create concise summary
nlm critique NOTEBOOK_ID SOURCE_ID1                 # Provide critical analysis

# Generate new content types
nlm brainstorm NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2    # Generate ideas
nlm verify NOTEBOOK_ID SOURCE_ID1                   # Fact-check content
nlm explain NOTEBOOK_ID SOURCE_ID1                  # Clarify concepts

# Create study materials
nlm study-guide NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2   # Comprehensive study guide
nlm faq NOTEBOOK_ID SOURCE_ID1                      # Frequently asked questions
nlm briefing-doc NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2  # Executive briefing

# Visual organization
nlm mindmap NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2       # Mind map visualization
nlm timeline NOTEBOOK_ID SOURCE_ID1                 # Chronological timeline
nlm outline NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2       # Structured outline
nlm toc NOTEBOOK_ID SOURCE_ID1                      # Table of contents
```

### Advanced Generation

```bash
# Generate comprehensive content
nlm generate-guide NOTEBOOK_ID          # Complete notebook guide
nlm generate-outline NOTEBOOK_ID        # Content outline
nlm generate-section NOTEBOOK_ID        # New section
nlm generate-chat NOTEBOOK_ID "What are the main themes in this research?"
nlm generate-magic NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2  # Magic view

# Interactive chat sessions
nlm chat NOTEBOOK_ID                     # Start interactive chat
nlm chat-list                           # List saved conversations
```

## 🔗 Sharing and Collaboration

### Sharing Workflows

```bash
# Share notebooks publicly
nlm share NOTEBOOK_ID

# Share privately with specific people
nlm share-private NOTEBOOK_ID

# Get sharing details and manage access
nlm share-details SHARE_ID
```

## 🔧 Advanced Workflows

### Research Paper Analysis

```bash
# Create a research notebook
notebook_id=$(nlm create "AI Research Analysis" | grep -o 'notebook [^ ]*' | cut -d' ' -f2)

# Add diverse academic sources
nlm add $notebook_id "https://arxiv.org/abs/2103.00020"
nlm add $notebook_id "https://arxiv.org/abs/2005.14165"
nlm add $notebook_id "research-notes.pdf"
nlm add $notebook_id "Personal observations on current AI trends and implications"

# Generate comprehensive analysis
nlm study-guide $notebook_id
nlm generate-outline $notebook_id
nlm summarize $notebook_id
nlm critique $notebook_id

# Create presentation materials
nlm briefing-doc $notebook_id
nlm mindmap $notebook_id
nlm timeline $notebook_id

# Generate audio for review
nlm audio-create $notebook_id "Academic tone, suitable for research presentation"
```

### Project Documentation

```bash
# Create project documentation notebook
project_notebook=$(nlm create "Project Alpha Documentation" | grep -o 'notebook [^ ]*' | cut -d' ' -f2)

# Add project files and requirements
nlm add $project_notebook "project-requirements.pdf"
nlm add $project_notebook "https://github.com/company/project-alpha"
nlm add $project_notebook "team-meeting-notes.docx"
nlm add $project_notebook "Initial project scope and technical requirements"

# Generate different views for stakeholders
nlm briefing-doc $project_notebook     # For executives
nlm study-guide $project_notebook      # For team members
nlm faq $project_notebook              # For support team
nlm timeline $project_notebook         # For project managers

# Create interactive artifacts
nlm create-artifact $project_notebook report
nlm create-artifact $project_notebook app
```

### Content Curation and Learning

```bash
# Create learning notebook
learning_notebook=$(nlm create "Machine Learning Fundamentals" | grep -o 'notebook [^ ]*' | cut -d' ' -f2)

# Add educational content
nlm add $learning_notebook "https://youtube.com/watch?v=ml-course"
nlm add $learning_notebook "https://course.example.com/ml-basics"
nlm add $learning_notebook "textbook-chapter3.pdf"
nlm add $learning_notebook "Practice problems and solutions from course"

# Generate study materials
nlm study-guide $learning_notebook
nlm faq $learning_notebook
nlm outline $learning_notebook
nlm explain $learning_notebook

# Create audio for learning
nlm audio-create $learning_notebook "Educational tone, explain concepts clearly for beginners"
```

### Meeting Documentation

```bash
# Create meeting notebook
meeting_notebook=$(nlm create "Weekly Team Meeting - $(date +%Y-%m-%d)" | grep -o 'notebook [^ ]*' | cut -d' ' -f2)

# Add meeting materials
nlm add $meeting_notebook "meeting-agenda.pdf"
nlm add $meeting_notebook "https://docs.google.com/presentation/slides"
echo "Action items: 1) Complete feature X by Friday 2) Review design docs 3) Schedule client demo" | nlm add $meeting_notebook -

# Generate follow-up materials
nlm summarize $meeting_notebook
nlm outline $meeting_notebook           # Action items outline
nlm briefing-doc $meeting_notebook      # For absent team members

# Create audio summary for easy consumption
nlm audio-create $meeting_notebook "Professional but conversational, focus on decisions and action items"
```

## 🔍 Debugging and Troubleshooting

### Debug Commands

```bash
# Enable debug mode for any command
nlm --debug list
nlm --debug auth
nlm --debug create "Test Notebook"

# Authentication debugging
nlm auth --debug
nlm auth --all --debug                  # Try all profiles with debug
nlm auth --profile "Work Profile" --debug

# Network connectivity testing
nlm hb                                  # Heartbeat check
nlm refresh                            # Refresh credentials
```

### Environment Variable Examples

```bash
# Set up environment for automation
export NLM_AUTH_TOKEN="your-token-here"
export NLM_COOKIES="your-cookies-here"
export NLM_BROWSER_PROFILE="Work Profile"

# Use in scripts
./nlm list
./nlm create "Automated Notebook $(date)"
```

## 📊 Batch Processing Examples

### Processing Multiple Sources

```bash
# Get list of source IDs for processing
sources=$(nlm sources NOTEBOOK_ID | grep -o 'Source [^ ]*' | cut -d' ' -f2)

# Process each source individually
for source in $sources; do
    echo "Processing source: $source"
    nlm summarize NOTEBOOK_ID $source
    nlm explain NOTEBOOK_ID $source
    sleep 1  # Rate limiting
done
```

### Automated Content Generation

```bash
#!/bin/bash
# Script to generate comprehensive content package

NOTEBOOK_ID="your-notebook-id"

echo "Generating comprehensive content package..."

# Generate different content types
nlm generate-guide $NOTEBOOK_ID > notebook-guide.txt
nlm study-guide $NOTEBOOK_ID > study-guide.txt
nlm faq $NOTEBOOK_ID > faq.txt
nlm timeline $NOTEBOOK_ID > timeline.txt

# Create audio overview
nlm audio-create $NOTEBOOK_ID "Professional summary of all content"
nlm audio-download $NOTEBOOK_ID complete-overview.mp3

echo "Content package generated successfully!"
```

## 🎯 Use Case Examples

### Academic Research

```bash
# Literature review workflow
nlm create "Literature Review: Neural Networks"
nlm add NOTEBOOK_ID "paper1.pdf" "paper2.pdf" "paper3.pdf"
nlm summarize NOTEBOOK_ID
nlm critique NOTEBOOK_ID
nlm generate-outline NOTEBOOK_ID
```

### Business Analysis

```bash
# Market research compilation
nlm create "Q3 Market Analysis"
nlm add NOTEBOOK_ID "competitor-report.pdf" "market-data.xlsx"
nlm briefing-doc NOTEBOOK_ID
nlm mindmap NOTEBOOK_ID
nlm audio-create NOTEBOOK_ID "Executive summary style"
```

### Educational Content

```bash
# Course material organization
nlm create "Programming Course Materials"
nlm add NOTEBOOK_ID "lecture-slides.pdf" "coding-examples.zip"
nlm study-guide NOTEBOOK_ID
nlm faq NOTEBOOK_ID
nlm explain NOTEBOOK_ID
```

This comprehensive examples document should help users understand the full capabilities of nlm and how to use it effectively for various workflows!