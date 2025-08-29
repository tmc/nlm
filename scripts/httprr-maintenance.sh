#!/bin/bash

# httprr-maintenance.sh - Maintenance script for httprr recordings
# This script provides compression, cleanup, and management of httprr recordings

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TESTDATA_DIRS=("internal/api/testdata" "internal/cmd/beproto/testdata" "cmd/nlm/testdata")
DEFAULT_MAX_AGE_DAYS=30
DEFAULT_COMPRESSION_LEVEL=6
LOG_FILE="${PROJECT_ROOT}/logs/httprr-maintenance.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging function
log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Create logs directory if it doesn't exist
    mkdir -p "$(dirname "$LOG_FILE")"
    
    # Log to file
    echo "[$timestamp] [$level] $message" >> "$LOG_FILE"
    
    # Log to stdout with colors
    case "$level" in
        "INFO")  echo -e "${GREEN}[INFO]${NC} $message" ;;
        "WARN")  echo -e "${YELLOW}[WARN]${NC} $message" ;;
        "ERROR") echo -e "${RED}[ERROR]${NC} $message" ;;
        "DEBUG") echo -e "${BLUE}[DEBUG]${NC} $message" ;;
        *)       echo "[$level] $message" ;;
    esac
}

# Function to show usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS] COMMAND

httprr maintenance script for managing HTTP recordings

COMMANDS:
    compress    Compress uncompressed .httprr files with gzip
    cleanup     Remove old recordings based on age
    stats       Show statistics about recordings
    verify      Verify integrity of compressed recordings
    all         Run compress, cleanup, and verify in sequence

OPTIONS:
    -h, --help                 Show this help message
    -v, --verbose              Enable verbose output
    -d, --max-age-days DAYS    Maximum age for recordings in days (default: $DEFAULT_MAX_AGE_DAYS)
    -c, --compression-level N  Gzip compression level 1-9 (default: $DEFAULT_COMPRESSION_LEVEL)
    -n, --dry-run              Show what would be done without making changes
    --force                    Force operations without confirmation

EXAMPLES:
    $0 compress                          # Compress all uncompressed recordings
    $0 cleanup -d 7                      # Remove recordings older than 7 days
    $0 stats                             # Show recording statistics
    $0 all --dry-run                     # Show what would be done
    $0 verify                            # Verify compressed recordings

EOF
}

# Function to find all httprr files
find_httprr_files() {
    local compressed="$1"  # true for .gz files, false for uncompressed
    local pattern="*.httprr"
    
    if [[ "$compressed" == "true" ]]; then
        pattern="*.httprr.gz"
    fi
    
    for testdata_dir in "${TESTDATA_DIRS[@]}"; do
        local full_path="$PROJECT_ROOT/$testdata_dir"
        if [[ -d "$full_path" ]]; then
            find "$full_path" -name "$pattern" -type f 2>/dev/null || true
        fi
    done
}

# Function to compress httprr files
compress_recordings() {
    local compression_level="$1"
    local dry_run="$2"
    
    log "INFO" "Starting compression with level $compression_level"
    
    local files_to_compress=()
    readarray -t files_to_compress < <(find_httprr_files false)
    
    if [[ ${#files_to_compress[@]} -eq 0 ]]; then
        log "INFO" "No uncompressed .httprr files found"
        return 0
    fi
    
    log "INFO" "Found ${#files_to_compress[@]} uncompressed recordings"
    
    local compressed_count=0
    local total_saved=0
    
    for file in "${files_to_compress[@]}"; do
        if [[ ! -f "$file" ]]; then
            continue
        fi
        
        local original_size=$(stat -c%s "$file" 2>/dev/null || stat -f%z "$file" 2>/dev/null)
        local compressed_file="${file}.gz"
        
        if [[ "$dry_run" == "true" ]]; then
            log "INFO" "Would compress: $file"
            continue
        fi
        
        # Check if compressed version already exists
        if [[ -f "$compressed_file" ]]; then
            log "WARN" "Compressed version already exists: $compressed_file"
            continue
        fi
        
        # Compress the file
        if gzip -c -"$compression_level" "$file" > "$compressed_file"; then
            local compressed_size=$(stat -c%s "$compressed_file" 2>/dev/null || stat -f%z "$compressed_file" 2>/dev/null)
            local saved=$((original_size - compressed_size))
            local percent_saved=$((saved * 100 / original_size))
            
            log "INFO" "Compressed: $file ($original_size → $compressed_size bytes, ${percent_saved}% saved)"
            
            # Remove original file after successful compression
            rm "$file"
            
            compressed_count=$((compressed_count + 1))
            total_saved=$((total_saved + saved))
        else
            log "ERROR" "Failed to compress: $file"
        fi
    done
    
    if [[ "$dry_run" != "true" ]]; then
        log "INFO" "Compression complete: $compressed_count files compressed, $total_saved bytes saved"
    fi
}

# Function to cleanup old recordings
cleanup_old_recordings() {
    local max_age_days="$1"
    local dry_run="$2"
    
    log "INFO" "Starting cleanup of recordings older than $max_age_days days"
    
    local all_files=()
    readarray -t uncompressed_files < <(find_httprr_files false)
    readarray -t compressed_files < <(find_httprr_files true)
    
    all_files=("${uncompressed_files[@]}" "${compressed_files[@]}")
    
    if [[ ${#all_files[@]} -eq 0 ]]; then
        log "INFO" "No httprr files found"
        return 0
    fi
    
    local removed_count=0
    local total_size_removed=0
    
    for file in "${all_files[@]}"; do
        if [[ ! -f "$file" ]]; then
            continue
        fi
        
        # Get file modification time
        local file_age_days
        if command -v stat >/dev/null 2>&1; then
            # Linux/GNU stat
            file_age_days=$(( ($(date +%s) - $(stat -c%Y "$file" 2>/dev/null || stat -f%m "$file" 2>/dev/null)) / 86400 ))
        else
            # Fallback for systems without stat
            file_age_days=$(( ($(date +%s) - $(date -r "$file" +%s 2>/dev/null || echo "0")) / 86400 ))
        fi
        
        if [[ $file_age_days -gt $max_age_days ]]; then
            local file_size=$(stat -c%s "$file" 2>/dev/null || stat -f%z "$file" 2>/dev/null || echo "0")
            
            if [[ "$dry_run" == "true" ]]; then
                log "INFO" "Would remove: $file (age: ${file_age_days} days)"
            else
                if rm "$file"; then
                    log "INFO" "Removed: $file (age: ${file_age_days} days, size: ${file_size} bytes)"
                    removed_count=$((removed_count + 1))
                    total_size_removed=$((total_size_removed + file_size))
                else
                    log "ERROR" "Failed to remove: $file"
                fi
            fi
        fi
    done
    
    if [[ "$dry_run" != "true" ]]; then
        log "INFO" "Cleanup complete: $removed_count files removed, $total_size_removed bytes freed"
    fi
}

# Function to show statistics
show_stats() {
    log "INFO" "Gathering httprr recording statistics"
    
    local uncompressed_files=()
    local compressed_files=()
    readarray -t uncompressed_files < <(find_httprr_files false)
    readarray -t compressed_files < <(find_httprr_files true)
    
    local uncompressed_count=${#uncompressed_files[@]}
    local compressed_count=${#compressed_files[@]}
    local total_count=$((uncompressed_count + compressed_count))
    
    local uncompressed_size=0
    local compressed_size=0
    
    # Calculate sizes
    for file in "${uncompressed_files[@]}"; do
        if [[ -f "$file" ]]; then
            local size=$(stat -c%s "$file" 2>/dev/null || stat -f%z "$file" 2>/dev/null || echo "0")
            uncompressed_size=$((uncompressed_size + size))
        fi
    done
    
    for file in "${compressed_files[@]}"; do
        if [[ -f "$file" ]]; then
            local size=$(stat -c%s "$file" 2>/dev/null || stat -f%z "$file" 2>/dev/null || echo "0")
            compressed_size=$((compressed_size + size))
        fi
    done
    
    local total_size=$((uncompressed_size + compressed_size))
    
    # Format sizes
    local uncompressed_size_human=$(numfmt --to=iec-i --suffix=B "$uncompressed_size" 2>/dev/null || echo "${uncompressed_size}B")
    local compressed_size_human=$(numfmt --to=iec-i --suffix=B "$compressed_size" 2>/dev/null || echo "${compressed_size}B")
    local total_size_human=$(numfmt --to=iec-i --suffix=B "$total_size" 2>/dev/null || echo "${total_size}B")
    
    echo
    echo "==================== HTTPRR STATISTICS ===================="
    echo "Total recordings:     $total_count"
    echo "Uncompressed:         $uncompressed_count ($uncompressed_size_human)"
    echo "Compressed:           $compressed_count ($compressed_size_human)"
    echo "Total size:           $total_size_human"
    echo
    
    if [[ $compressed_count -gt 0 && $total_count -gt 0 ]]; then
        local compression_ratio=$((compressed_count * 100 / total_count))
        echo "Compression ratio:    ${compression_ratio}%"
    fi
    
    echo
    echo "Recordings by directory:"
    for testdata_dir in "${TESTDATA_DIRS[@]}"; do
        local full_path="$PROJECT_ROOT/$testdata_dir"
        if [[ -d "$full_path" ]]; then
            local dir_count=$(find "$full_path" -name "*.httprr*" -type f 2>/dev/null | wc -l)
            if [[ $dir_count -gt 0 ]]; then
                echo "  $testdata_dir: $dir_count files"
            fi
        fi
    done
    echo "======================================================="
}

# Function to verify compressed recordings
verify_recordings() {
    local dry_run="$1"
    
    log "INFO" "Verifying compressed recordings"
    
    local compressed_files=()
    readarray -t compressed_files < <(find_httprr_files true)
    
    if [[ ${#compressed_files[@]} -eq 0 ]]; then
        log "INFO" "No compressed recordings found"
        return 0
    fi
    
    local verified_count=0
    local failed_count=0
    
    for file in "${compressed_files[@]}"; do
        if [[ ! -f "$file" ]]; then
            continue
        fi
        
        if [[ "$dry_run" == "true" ]]; then
            log "INFO" "Would verify: $file"
            continue
        fi
        
        # Test gzip integrity
        if gzip -t "$file" 2>/dev/null; then
            # Test that decompressed content has valid httprr header
            if zcat "$file" | head -n1 | grep -q "httprr trace v1"; then
                log "INFO" "Verified: $file"
                verified_count=$((verified_count + 1))
            else
                log "ERROR" "Invalid httprr format: $file"
                failed_count=$((failed_count + 1))
            fi
        else
            log "ERROR" "Gzip corruption detected: $file"
            failed_count=$((failed_count + 1))
        fi
    done
    
    if [[ "$dry_run" != "true" ]]; then
        log "INFO" "Verification complete: $verified_count verified, $failed_count failed"
        if [[ $failed_count -gt 0 ]]; then
            return 1
        fi
    fi
}

# Main function
main() {
    local command=""
    local max_age_days="$DEFAULT_MAX_AGE_DAYS"
    local compression_level="$DEFAULT_COMPRESSION_LEVEL"
    local dry_run="false"
    local verbose="false"
    local force="false"
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            -v|--verbose)
                verbose="true"
                shift
                ;;
            -d|--max-age-days)
                max_age_days="$2"
                shift 2
                ;;
            -c|--compression-level)
                compression_level="$2"
                shift 2
                ;;
            -n|--dry-run)
                dry_run="true"
                shift
                ;;
            --force)
                force="true"
                shift
                ;;
            compress|cleanup|stats|verify|all)
                command="$1"
                shift
                ;;
            *)
                log "ERROR" "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
    
    # Check if command is provided
    if [[ -z "$command" ]]; then
        log "ERROR" "No command specified"
        usage
        exit 1
    fi
    
    # Validate arguments
    if [[ ! "$max_age_days" =~ ^[0-9]+$ ]] || [[ "$max_age_days" -lt 1 ]]; then
        log "ERROR" "Invalid max-age-days: $max_age_days (must be positive integer)"
        exit 1
    fi
    
    if [[ ! "$compression_level" =~ ^[1-9]$ ]]; then
        log "ERROR" "Invalid compression-level: $compression_level (must be 1-9)"
        exit 1
    fi
    
    # Change to project root
    cd "$PROJECT_ROOT"
    
    log "INFO" "Starting httprr maintenance"
    log "INFO" "Project root: $PROJECT_ROOT"
    log "INFO" "Command: $command"
    log "INFO" "Max age days: $max_age_days"
    log "INFO" "Compression level: $compression_level"
    log "INFO" "Dry run: $dry_run"
    
    # Execute command
    case "$command" in
        compress)
            compress_recordings "$compression_level" "$dry_run"
            ;;
        cleanup)
            cleanup_old_recordings "$max_age_days" "$dry_run"
            ;;
        stats)
            show_stats
            ;;
        verify)
            verify_recordings "$dry_run"
            ;;
        all)
            compress_recordings "$compression_level" "$dry_run"
            cleanup_old_recordings "$max_age_days" "$dry_run"
            verify_recordings "$dry_run"
            ;;
        *)
            log "ERROR" "Unknown command: $command"
            exit 1
            ;;
    esac
    
    log "INFO" "httprr maintenance completed"
}

# Run main function
main "$@"