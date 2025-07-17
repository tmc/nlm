#!/bin/bash

# Script to record httprr tests for all nlm commands
set -e

echo "Setting up environment for recording tests..."

# Export authentication from stored file
if [ -f ~/.nlm/env ]; then
    echo "Loading authentication from ~/.nlm/env"
    source ~/.nlm/env
else
    echo "Error: ~/.nlm/env not found. Please run 'nlm auth login' first."
    exit 1
fi

# Check that we have the required environment variables
if [ -z "$NLM_AUTH_TOKEN" ] || [ -z "$NLM_COOKIES" ]; then
    echo "Error: Missing required authentication environment variables"
    echo "NLM_AUTH_TOKEN: ${NLM_AUTH_TOKEN:0:20}..."
    echo "NLM_COOKIES: ${NLM_COOKIES:0:50}..."
    exit 1
fi

echo "Authentication loaded successfully"
echo "Auth token: ${NLM_AUTH_TOKEN:0:30}..."
echo "Cookies length: ${#NLM_COOKIES} characters"

# Recording flags
RECORD_FLAGS="-httprecord=. -httprecord-debug -v"

echo ""
echo "=== Recording Notebook Commands ==="

echo "Recording: List Projects"
go test ./internal/api $RECORD_FLAGS -run TestNotebookCommands_ListProjects || echo "Test failed, continuing..."

echo "Recording: Create Project"
go test ./internal/api $RECORD_FLAGS -run TestNotebookCommands_CreateProject || echo "Test failed, continuing..."

echo "Recording: Delete Project"
go test ./internal/api $RECORD_FLAGS -run TestNotebookCommands_DeleteProject || echo "Test failed, continuing..."

echo ""
echo "=== Recording Source Commands ==="

echo "Recording: List Sources"
go test ./internal/api $RECORD_FLAGS -run TestSourceCommands_ListSources || echo "Test failed, continuing..."

echo "Recording: Add Text Source"
go test ./internal/api $RECORD_FLAGS -run TestSourceCommands_AddTextSource || echo "Test failed, continuing..."

echo "Recording: Add URL Source"
go test ./internal/api $RECORD_FLAGS -run TestSourceCommands_AddURLSource || echo "Test failed, continuing..."

echo "Recording: Delete Source"
go test ./internal/api $RECORD_FLAGS -run TestSourceCommands_DeleteSource || echo "Test failed, continuing..."

echo "Recording: Rename Source"
go test ./internal/api $RECORD_FLAGS -run TestSourceCommands_RenameSource || echo "Test failed, continuing..."

echo ""
echo "=== Recording Audio Commands ==="

echo "Recording: Create Audio Overview"
go test ./internal/api $RECORD_FLAGS -run TestAudioCommands_CreateAudioOverview || echo "Test failed, continuing..."

echo "Recording: Get Audio Overview"
go test ./internal/api $RECORD_FLAGS -run TestAudioCommands_GetAudioOverview || echo "Test failed, continuing..."

echo ""
echo "=== Recording Generation Commands ==="

echo "Recording: Generate Notebook Guide"
go test ./internal/api $RECORD_FLAGS -run TestGenerationCommands_GenerateNotebookGuide || echo "Test failed, continuing..."

echo "Recording: Generate Outline"
go test ./internal/api $RECORD_FLAGS -run TestGenerationCommands_GenerateOutline || echo "Test failed, continuing..."

echo ""
echo "=== Recording Misc Commands ==="

echo "Recording: Heartbeat"
go test ./internal/api $RECORD_FLAGS -run TestMiscCommands_Heartbeat || echo "Test failed, continuing..."

echo ""
echo "=== Recording Complete ==="
echo "Generated httprr files in internal/api/testdata/"
ls -la internal/api/testdata/*.httprr* 2>/dev/null || echo "No .httprr files found"

echo ""
echo "To use these recordings in tests, run:"
echo "go test ./internal/api -v"