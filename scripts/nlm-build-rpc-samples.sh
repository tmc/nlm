#!/usr/bin/env bash
set -euo pipefail

capture_root="${NLM_CAPTURE_ROOT:-docs/captures}"
output_dir="${capture_root}/sources/notebooklm.google.com/_rpc-samples"
samples_per_rpc=2
max_text_chars=12000

usage() {
	cat <<'EOF'
Usage: nlm-build-rpc-samples.sh [options]

Build compact sample files for NotebookLM batchexecute RPCs found in HAR JSONL
captures. For each rpcids entry, the script keeps the smallest representative
captures and writes one JSON file per RPC ID plus an index.

Options:
  --capture-root DIR     Capture root to scan
  --output-dir DIR       Output directory for sample files
  --samples-per-rpc N    Number of samples to keep per RPC ID (default: 2)
  --max-text-chars N     Max chars kept for request/response text previews
  --help                 Show this help
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--capture-root)
		capture_root="$2"
		shift 2
		;;
	--output-dir)
		output_dir="$2"
		shift 2
		;;
	--samples-per-rpc)
		samples_per_rpc="$2"
		shift 2
		;;
	--max-text-chars)
		max_text_chars="$2"
		shift 2
		;;
	--help|-h)
		usage
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if ! command -v jq >/dev/null 2>&1; then
	echo "jq is required" >&2
	exit 1
fi

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/nlm-rpc-samples.XXXXXX")"
trap 'rm -rf "$tmpdir"' EXIT

candidates="${tmpdir}/candidates.ndjson"
rpc_ids="${tmpdir}/rpc_ids.txt"

mkdir -p "$output_dir"
find "$output_dir" -type f \( -name 'rpc-*.json' -o -name 'index.json' \) -delete

find "$capture_root" -type f -name 'notebooklm.google.com.jsonl' -print0 |
while IFS= read -r -d '' file; do
	jq -c \
		--arg capture_file "$file" \
		--argjson max_chars "$max_text_chars" '
		def preview($n):
			if . == null then null
			elif (type != "string") then tostring
			elif (length > $n) then .[:$n] + "\n...[truncated]"
			else .
			end;
		def headers($names):
			[
				(. // [])[]
				| select(type == "object")
				| select(.name? != null)
				| . as $header
				| select($names | index($header.name | ascii_downcase))
				| {name: $header.name, value: $header.value}
			];
		select(.request.url? and (.request.url | contains("rpcids=")))
		| . as $entry
		| ($entry.request.url | capture("rpcids=(?<ids>[^&]+)").ids | split(","))[]
		| {
			rpc_id: .,
			sort_size: (
				($entry.request.bodySize // 0)
				+ ($entry.response.bodySize // 0)
				+ ($entry.response.content.size // 0)
			),
			sample: {
				capture_file: $capture_file,
				started_date_time: $entry.startedDateTime,
				request: {
					method: $entry.request.method,
					url: $entry.request.url,
					body_size: $entry.request.bodySize,
					post_data_mime_type: ($entry.request.postData.mimeType? // null),
					post_data_text: (($entry.request.postData.text? // null) | preview($max_chars)),
					headers: (($entry.request.headers? // []) | headers(["content-type", "x-same-domain", "x-goog-ext-353267353-jspb", "referer"]))
				},
				response: {
					status: $entry.response.status,
					body_size: ($entry.response.bodySize // null),
					content_size: ($entry.response.content.size? // null),
					mime_type: ($entry.response.content.mimeType? // null),
					text: (($entry.response.content.text? // null) | preview($max_chars)),
					headers: (($entry.response.headers? // []) | headers(["content-type", "content-encoding", "x-content-type-options", "cache-control"]))
				}
			}
		}
	' "$file" >>"$candidates"
done

if [[ ! -s "$candidates" ]]; then
	echo "no rpc samples found under $capture_root" >&2
	exit 1
fi

jq -r '.rpc_id' "$candidates" | sort -u >"$rpc_ids"

while IFS= read -r rpc_id; do
	[[ -n "$rpc_id" ]] || continue
	outfile="${output_dir}/rpc-${rpc_id}.json"
	jq -s \
		--arg rpc_id "$rpc_id" \
		--argjson samples_per_rpc "$samples_per_rpc" '
		def key:
			.sample.capture_file + "\u0000"
			+ (.sample.request.url // "") + "\u0000"
			+ (.sample.started_date_time // "");
		[
			.[]
			| select(.rpc_id == $rpc_id)
		]
		| sort_by(.sort_size, .sample.capture_file, .sample.started_date_time)
		| unique_by(key)
		| {
			rpc_id: $rpc_id,
			samples_found: length,
			selected_samples: (if length < $samples_per_rpc then length else $samples_per_rpc end),
			samples: (.[0:$samples_per_rpc] | map(.sample))
		}
	' "$candidates" >"$outfile"
done <"$rpc_ids"

jq -s '
	sort_by(.rpc_id)
	| {
		generated_at: now | todate,
		rpc_count: length,
		items: map({
			rpc_id,
			samples_found,
			selected_samples
		})
	}
' "$output_dir"/rpc-*.json >"$output_dir/index.json"

echo "wrote $(wc -l <"$rpc_ids" | tr -d ' ') rpc sample files to $output_dir"
