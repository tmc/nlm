# Sync attempt receipts

`nlm source sync --json` emits progress events followed by one `action: "result"`
event after packing succeeds and a receipt can be created. Earlier discovery,
packing, or receipt-creation errors return nonzero without a result event.
Plain output keeps source IDs on stdout and reports the family result on stderr.

The result has these fields:

- `version`: receipt schema version, currently 1.
- `notebook_id`, `name`, `started`: the target family and attempt start time.
- `status`: `complete`, `incomplete`, or `planned` for a successful dry run.
- `readiness`: currently always `unchecked`.
- `cleanup_complete`: whether this run finished its upload and orphan-cleanup path.
- `max_bytes`, `auto_split`, `dry_run`: effective packing threshold and run modes.
- `parts`: intended top-level parts with `name`, exact payload `sha256`, and `bytes`.
- `operations`: observed progress events, including successful uploads/replacements,
  skips, splits, deletions, and failed upload attempts (`action: "error"`).
- `error`: the final failure, when present.
- `receipt`: the local attempt file; absent for dry runs.

A failed parent upload can appear in `operations` even when auto-splitting recovers
it and the run completes. Use the final status to decide whether the run finished.
An upload error includes the prepared payload's byte count. This is not an inferred
server size limit; code 9 does not supply one and remains ineligible for auto-split.

Each mutating attempt gets a separate file under
`~/.cache/nlm/sync-attempts/<family-hash>/attempt-*.json`. The hash namespaces the
notebook ID and family name. Before contacting the server, sync records the plan
with `status: "in-progress"`. It replaces that file after progress events and at
completion. Receipts contain names, IDs, hashes and errors, not source bodies.
They are not pruned automatically. Dry runs do not create receipt files.

An interrupted attempt may remain `in-progress`. Its last event can lag remote
operations: a crash can occur between a successful RPC and recording it. Treat
that state as requiring reconciliation, never as a completed family. Receipts
are local evidence of attempts, not remote manifests, locks, or proof that another
process has not changed the notebook.

An incomplete run returns `*nlmsync.IncompleteError`, which unwraps its cause.
Existing CLI authentication, precondition and transient exit classifications
remain intact. Consumers can distinguish incomplete syncs through the JSON result
rather than assigning a new meaning to the underlying RPC exit code.

Sync rejects duplicate family titles and sources already listed with status
`error` before modifying the family. The diagnostic includes the affected source
IDs. It does not choose a duplicate to delete or automatically repair errored
sources. A previously healthy source can still fail indexing after the initial
list or after a successful upload; `complete` means operations finished, not that
all sources are ready for queries. Consumers that require readiness must check
server status separately.

Replacement is still per part. A failed run can leave a mixture of old and new
parts, and an accepted replacement can later fail indexing. Receipts expose this
uncertainty; they do not provide family rollback or atomic query visibility.
