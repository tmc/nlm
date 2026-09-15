# Label handling and source selectors

Status: reviewed draft, implementation-ready. Covers `cmd/nlm/source_match.go`
(selector resolution), its consuming commands, and `nlmsync` label
preservation.

Revisions 2 and 3 incorporate two read-only reviews by a peer agent; the
decisions they raised are resolved in place and marked **[R2]** / **[R3]** / **[R4]**.

## 1. Current behavior

`resolveSelectorIDs` takes six flags — `--source-ids`, `--source-match`,
`--source-exclude`, `--label-ids`, `--label-match`, `--label-exclude` — and
returns a source ID list. Today:

- Include filters **union**. `--source-match foo --label-match bar` selects
  foo ∪ bar, not foo ∩ bar.
- Label include filters silently drop unlabeled sources: the include set is
  built from `label.SourceIDs`, and a source with no label appears in no
  label.
- `--label-exclude` alone takes a different path (`hasOnlyExcludes`), seeding
  the include set with *every* source — so unlabeled sources are **kept** here
  and **dropped** in the case above.
- `--label-ids` selects by label ID; `--label-exclude` is name-regex only
  (`matchLabels(labels, nil, labelExcludeRE)`). There is no exclude-by-ID.
- There is no way to select "sources that have no label".
- `resolveSelectorIDs` returns `nil, nil` when no selector is set, and
  consumers treat an empty ID list as "all sources"
  (`notebooklm/client_video.go:createArtifactSourceIDs`,
  `runInteractiveChat`). An empty *selection* is therefore indistinguishable
  from *no selection*. **[R2]**

In `nlmsync`, labels are read per existing part, unioned across the whole
family, and that union is attached to every part. Labels are never detached.
Under `--dry-run` labels are not read at all.

## 2. Principles

1. **A dimension ORs; dimensions AND.** Within one dimension (sources,
   labels) the filters are alternatives; across dimensions they narrow.
2. **Include and exclude are symmetric.** Anything selectable is excludable,
   by the same addressing (ID and name regex).
3. **Unlabeled is a value, not an absence.** "Has no label" is expressible.
4. **An empty selection is never a full selection.** A selector that resolves
   to nothing is an error, never silently promoted to every source. **[R2]**
5. **Dry run reports every mutation it plans**, labels included, and issues
   zero remote mutations.
6. **Sync preserves the labels a part actually had**, not the union of its
   siblings' accidents.

## 3. Selector semantics

Resolution is three steps.

**Step 1 — source dimension.** If `--source-ids` or `--source-match` is set,
the source set is their union. Otherwise it is every source in the notebook.

**Step 2 — label dimension.** If `--label-ids`, `--label-match` or
`--label-none` is set, the label set is the union of their matches
(`--label-none` contributing every source carrying no label). Otherwise it is
every source in the notebook.

**Step 3 — intersect, then subtract.** The result is (step 1 ∩ step 2) minus
everything matched by `--source-exclude`, `--label-exclude` or
`--label-exclude-ids`. Exclusion always wins, including over an ID named
directly in `--source-ids`.

Result ordering is the source order of the notebook as returned by that run's
`GetProject`, de-duplicated, so two runs against the same snapshot produce the
same list. **[R3]** Ordering is not stable across independently reordered
server results, and the compatibility guarantee in section 4 covers set
*membership*, not order: today's resolver emits explicit IDs first and then
matches, and that incidental order is not preserved.

### 3.1 Selection state, not a bare list **[R2]**

`selection` lives in `cmd/nlm`, and the `notebooklm` library keeps its current
public contract: an omitted ID list still means "all sources", for the benefit
of direct library callers. **[R3]** The CLI adapters — not the library — are
responsible for validating `Explicit` and passing an explicit, non-empty list
down. `createArtifactSourceIDs` keeps its signature *and its behavior*: a `[]string`
carries no `Explicit` bit, so it cannot distinguish the two states and must
not try. **[R4]** The defensive check lives in the CLI adapters, where
`selection` still exists — an adapter never passes an explicit-empty list
down, and the library's documented empty-means-all contract
(`notebooklm/client_video.go:117`) is preserved untouched. Introducing a source-scope type into the library would
be a separate API slice with its own package, exported shape, compatibility
statement and caller inventory; it is deliberately not part of this work.

`resolveSelectorIDs` returns a `selection` value, not `[]string`:

```go
type selection struct {
    Explicit bool     // a selector was given
    IDs      []string // the resolved sources; meaningful only when Explicit
}
```

- No selectors → `Explicit: false`. Consumers keep their current "all
  sources" behavior.
- Selectors given, non-empty result → `Explicit: true`, `IDs` populated.
- Selectors given, empty result → **error at the resolver**, with a message
  naming which filters emptied it. No command ever receives an explicit-empty
  selection.

No CLI call site may branch on `len(IDs) == 0` to mean "all". Every one that
does today — `createArtifactSourceIDs`'s callers, `runInteractiveChat`,
`createReport`, `generateReport` — resolves a `selection` and treats
`Explicit` as the switch. This is the safety-critical part of the change:
`--label-none` in a fully labeled notebook must not generate from every
source. The consumer list is an inventory to be verified against the tree
during slice 1, not a promise: every caller of
`resolveSourceSelectorsWithOptions` gets a test. **[R3]**

### 3.2 New flags

- `--label-none` — sources carrying no label. A member of the label
  dimension, so it unions with `--label-ids`/`--label-match`. It matching
  nothing is not itself an error; only a final empty selection is.
- `--label-exclude-ids <a,b,c>` — exclude by label ID, mirroring
  `--label-ids`. Accepts `-` for stdin.

`--label-exclude` stays name-regex only; the ID form is the new flag, so
neither flag has two addressing modes.

There is deliberately **no `--label-exclude-none`**. Excluding the unlabeled
is `--label-ids`/`--label-match` with any label, or the label dimension
already excludes them by construction. This asymmetry is documented in the
help text rather than filled with a flag nothing would use. **[R2]**

I considered `--include-unlabeled` as a modifier on the label dimension.
Rejected: a modifier cannot express "only the unlabeled ones", and its meaning
would depend on whether another flag is set.

### 3.3 Flag parsing rules **[R2]**

- **At most one `-` stdin consumer per invocation**, across `--source-ids`,
  `--label-ids`, `--label-exclude-ids` and any command operand that reads
  stdin. A second `-` is an error raised *before* any read, since
  `resolveIDList` reads `os.Stdin` independently and a later reader would
  silently see EOF.
- All regexes compile **before** any network read, so an invalid regex costs
  no RPC. An invalid regex is never retried as a literal ID.
- Each regex flag is a scalar; a repeated occurrence is **last-wins**, matching
  the rest of the CLI. "Filters within a dimension OR" therefore means across
  *distinct* flags, not across repeats of one flag. Stated in help text.
- ID lists de-duplicate. An ID matching no source or label is an error naming
  the unknown IDs — partly-unknown is an error too, so a typo cannot silently
  shrink a destructive selection.

## 4. Migration **[R2]**

The union → intersect change can silently shrink a selection, and these flags
drive destructive and generative commands. A status line is output, not
consent. Therefore:

- An invocation setting **both** a source include filter and a label include
  filter is **rejected** unless `--selector-mode=intersect|union` is given.
  The error names both interpretations and the flag.
- `--selector-mode` is **permanent**, and an explicitly named mode always
  means what it says. **[R3]** Only the meaning of an *omitted* mode changes
  over time: today omitted-and-mixed is an error; one release later omitted
  means `intersect`. A script that opts into `union` keeps getting union
  forever. The flag never becomes a no-op — silently redefining the mode a
  caller explicitly asked for is the exact failure this migration exists to
  prevent.
- **Union is defined over the *active* include dimensions only.** **[R3]**
  It is the union of the dimensions the caller actually specified; an
  unspecified dimension contributes nothing, and never its implicit "all".
  (Unioning a specified dimension with an unspecified dimension's "all" would
  select every source, which is what today's `includeAll` handling already
  avoids and what a naive reading of "union" would break.)
- Compatibility under `union` preserves **set membership, not ordering** (see
  section 3). **[R3]**
- Every other combination (one dimension only, excludes only) is unaffected
  and needs no mode.
- The rejection is uniform across interactive, `-y`, generative and
  destructive consumers, and happens before any read or mutation. **Zero RPCs
  of any kind** for a mixed invocation with no mode, an invalid mode value, or
  `--selector-mode` with no selectors: these are input-shape errors, decidable
  without the network. **[R3]**

Modes are tested as five cases: explicit `union`, explicit `intersect`,
omitted-and-mixed, an invalid mode value, and mode with no selectors.

### 4.1 Consumers that add their own IDs **[R2]**

"Dimensions AND" describes the resolver, not automatically each command. Three
call sites compose differently and are settled here:

- `source-guide` ignores selectors when positional source IDs are present
  (`command_specs_selector.go`). This becomes an **error**: positional IDs and
  selector flags are incompatible inputs, not an override.
- **Report creation**: `createReport` currently computes
  `unionIDs(flagIDs, suggestionIDs)`, which can re-add a source the caller
  explicitly excluded. **[R3]** Order of operations is fixed as: resolve the
  selector's include set, add the suggestion IDs, then apply **all exclusions
  last**. Final exclusions win — an excluded source never returns via a
  suggestion. A suggestion cannot rescue an empty selector either: the
  explicit-empty error of section 3.1 fires at resolution, before suggestions
  are considered. Both cases get tests.
- **`generateReport` mutates before it validates**: `cmd/nlm/main.go` calls
  `SetInstructions` *before* `resolveSourceSelectorsWithOptions`. **[R3]**
  Selector validation moves ahead of it, so a mixed-mode rejection or an
  explicit-empty selection leaves the notebook's instructions untouched. The
  negative control is: non-empty `--instructions` plus invalid or mixed
  selectors ⇒ no `SetInstructions` RPC and no generation RPC.

Every caller of `resolveSourceSelectorsWithOptions` is enumerated in
`docs/commands.md` with which shape it uses, and that enumeration is derived
from the tree during slice 1 rather than trusted from this document.

## 5. nlmsync label handling

### 5.1 Lineage contract **[R2]**

- **A part's own labels are authoritative, including the empty set.** An
  existing part that carries no label stays unlabeled. "No parent" and
  "parent known to have no labels" are distinct states, and neither falls
  back to the family union — that fallback is exactly the sibling bleed this
  change removes.
- **In-process lineage is passed, not parsed.** When a split creates halves
  during a run, the parent's label set is passed down through the recursion.
  Titles are never consulted for a split the process just performed.
- **Names are parsed relative to the caller's literal `--name`, and only the
  suffix is validated.** **[R3]** `--name 'notes (draft)'` is valid today, so
  `notes (draft)` is the owned family root and `notes (draft) (pt2)` is one of
  its parts. Ownership is never inferred by globally parsing a title for
  something that looks like a base: sync knows the base it was given, strips
  it, and validates what remains. Anything whose title does not begin with the
  literal base is an unrelated source.

  The owned suffix grammar, applied to the remainder after the base:

  | Remainder | Meaning |
  | --- | --- |
  | *(empty)* | chunk 1 |
  | ` (ptN)`, `N` matching `[1-9][0-9]*` | chunk N |
  | a chunk suffix followed by ` (X)`, `X` a non-empty string over `{a,b}` | that split path |
  | ` (split1)` / ` (split2)` chains, optionally after a chunk suffix — e.g. `base (pt2) (split1) (split2)` | legacy split path **[R4]** |

  `pt0`, leading zeros, letters outside `{a,b}` (`isLetters` currently accepts
  all of a–z), and `splitN` for any other `N` are **not** owned: they are
  unrelated sources, never renamed and never deleted.
- **Chunk 1 is written bare** (`chunkNames`), so ` (pt1)` with no split
  letters is an **alias**, not a second part. **[R3]** Sync reads it as chunk
  1 and never writes it; a family containing both the bare title and the
  ` (pt1)` alias is a duplicate identity and aborts before mutation. (` (pt1)`
  *with* letters — `base (pt1) (a)` — is a normal split path, since
  `splitname.go` needs an anchor for the letters.)
- **`… [old]` recovery titles are recovery artifacts, not parts** — the
  suffix form `chunkName + " [old]"` that `uploadChunk` writes (`sync.go:378`),
  not a prefix. **[R4]** They are excluded from duplicate detection so a
  family left mid-replace can still be repaired. Canonical presence alone does
  **not** authorize sweeping one: a crash can leave the canonical replacement
  holding only some of its labels while the `[old]` source still holds the
  complete set. So a recovery source is a **label donor**, and this is an
  explicit exception to "the exact leaf's empty set is authoritative":
  1. read the `[old]` source's labels;
  2. attach any the canonical leaf is missing, and verify;
  3. only then delete the `[old]` source — per the ordering in section 5.3, an
     attach failure or cancellation leaves it in place.
  If **only** the `[old]` source exists, it is the part: it is renamed back to
  the canonical title with its labels intact rather than re-uploaded.
- **Duplicate or conflicting canonical identities abort before any
  mutation**, as the receipt check does today.
- **Precedence for a part's labels**: (1) a **consolidation**, if this part
  replaces more than one existing part — the union of its donors, per 5.2;
  (2) the exact existing leaf with that title, for a one-to-one replacement;
  (3) for a newly created descendant, its live parent's labels; (4) otherwise
  no labels. **[R3]** Consolidation outranks the exact leaf: `base` labeled A
  and `base (pt2)` labeled B collapsing into `base` must yield A ∪ B, not A
  with B's source deleted.
- Case (4) — no labels — applies only to a **new** part with no other
  provenance. An **existing** labeled leaf whose ancestor is gone keeps its
  own labels via case (2); a missing ancestor never strips an existing part.
  **[R3]**

### 5.2 Consolidation and rechunking **[R2]**

When `--max-bytes` grows, or auto-split is turned off, several parts collapse
into one. "No detach" does not protect labels here, because the donors are
deleted. Rule: the surviving part receives the **union of the labels of the
actual donors it replaces**, computed before the donors are deleted, and the
donors are deleted only after the survivor carries them.

**Identifying donors requires evidence, not arithmetic.** **[R3]** `hashCache`
stores one hash per title and `sourceCache` stores source metadata; neither
records which members or byte ranges went into a part, so "old pt1 and pt2
became new pt1, pt2 and pt3" is not derivable from titles or part numbers.
Identical part numbers across runs are **not** evidence of content ancestry.
Therefore:

**This slice ships the conservative half only.** **[R4]** A partition manifest
is real work with its own validity contract, and a half-specified one would
authorize *deletions* on stale evidence. So:

- **Identifiable now:** a **complete subtree collapsing into its ancestor**.
  The names encode the tree, so the donors are known without stored state.
  The survivor takes their label union, and the donors are deleted only after
  it carries them. A member crossing a boundary between two differently
  labeled donors in this case yields both labels.
- **Identifiable now:** a family that only **gains** chunks. Every existing
  chunk keeps its slot, so no part is deleted and no assignment has to be
  attributed to a donor: existing parts keep their own labels and each new
  part takes the labels **every** existing part carries, never one only some
  siblings hold.
- **Everything else — a rechunk that drops a labeled chunk:** a
  **pre-mutation error** naming the parts whose labels have no donor. There is no
  self-healing "re-run once" instruction — an identical re-run reproduces the
  same error — and **no `--force` escape**: `Options.Force` today means only
  "ignore unchanged hashes" and supplies no donor evidence. The caller's
  remedies are to detach the labels, or to sync into a fresh family name.
- An **unlabeled** family rechunks freely; there is nothing to lose.

A general manifest is a **later slice**, not this one. When it is written it
must, at minimum: bind each record to the notebook, the family base, and the
old parts' **source IDs and content hashes**; be rejected as stale whenever a
remote identity or hash no longer matches, since a manifest saved only after a
fully successful run is stale after a partial replacement; define byte-range
overlap for cut members; and specify a per-part update protocol so a partial
run leaves recoverable rather than misleading provenance. Its tests: stale
manifest after partial success, changed member content, overlapping byte
ranges, and repeated invocation after a missing-manifest failure.

### 5.3 Attach ordering and failure

Unchanged in shape from today and now stated: a replacement's labels are
attached to the **new** source before the old source is deleted, so a failed
or cancelled attach leaves the old assignment intact. A retry attaches only
what is missing, so it never duplicates an assignment.

### 5.4 Dry run **[R2]**

- The label snapshot is read under `--dry-run` (a read-only RPC), so a dry run
  can report label work.
- A label event carries `label_id`, and either `source_id` for an existing
  target or the planned part name for a source that does not exist yet, plus
  `dry_run: true`. `action`/`name`/`reason` alone cannot tell the reader which
  assignment would change.
- A dry run reports the **known initial plan** plus the inheritance rule for
  contingent splits. It cannot predict which uploads the server will reject
  and therefore cannot enumerate every eventual split; the output says so.
### 5.4.1 Missing capability is *unknown*, not *empty* **[R4]**

A client that is not a `LabelPreserver` cannot be asked whether a family has
labels: `nlmsync.Source` exposes only ID, Title and Status, and `Client` has no
other label read. "The family carries no labels" is therefore not a state sync
can observe — it is unknown. One uniform rule, identical in both modes:

- **Absent capability fails, in dry run and in real run alike**, before any
  mutation, unless the caller sets `Options.NoLabels` — an explicit
  declaration that labels are outside the requested sync scope. It bypasses
  label planning and preservation entirely — in both modes, and whether or not
  the client implements `LabelPreserver`, which is how it is tested. **[R4]**
  Both modes honor that declaration identically, so a real run can never succeed on a
  plan a dry run refused, and neither can silently drop labels it could not
  see.
- **A label *read failure* fails before mutation in both modes**, since the
  family may carry labels.
- **Known-empty** (capability present, read succeeded, no labels) proceeds
  normally and is a distinct test case from unknown-capability.
- The claim is **zero remote mutations**, not "mutates nothing": a dry run
  still writes local cache and receipt state.

### 5.5 Skip path

`runAutoSplit`'s unchanged branch attaches every family label on every run.
It attaches only what the part is missing, as the non-autosplit path already
does via `labelsBySource`. An unchanged, correctly-labeled part issues zero
attach calls.

## 6. Tests

Selector resolution (`resolveSelectorIDs`, table-driven):

- Each dimension alone; cross-dimension intersection with a disjoint result;
  a union-control fixture proving AND actually narrows.
- No selectors vs. explicit empty selection vs. empty stdin — and, on a real
  generative consumer, that no empty-to-all promotion occurs.
- `--label-none` alone; with `--label-match`; with zero hits alongside a
  matching `--label-match`; an unlabeled part beside a labeled sibling.
- Exclusion wins over a directly named `--source-ids` entry; include-by-name
  with exclude-by-ID on the same label; a source carrying both an included
  and an excluded label.
- Stable ordering and de-duplication; stale `label.SourceIDs` naming a source
  absent from the project; unknown and partly-unknown IDs; empty notebook.
- Duplicate label names with distinct IDs; regex metacharacters; a second `-`
  rejected before any read; invalid regex rejected before any RPC.
- Mixed include without `--selector-mode`, an invalid mode value, and a mode
  with no selectors each perform **zero RPCs of any kind** — these are decided
  before any read. An explicit-empty selection *after* resolution may have
  issued read RPCs, but issues zero writes and zero generations. **[R3]**
- Explicit `union` and explicit `intersect` each produce their named
  semantics, with union defined over active dimensions only. **[R3]**
- Stale `label.SourceIDs` naming a source absent from the current project is
  **discarded** against the current source universe, silently; an unknown ID
  the *caller* named is an error. These are different cases. **[R3]**
- `createReport`: a source both excluded and present in the suggestion list is
  **absent** from the final set; an empty selector errors before suggestions
  are consulted. **[R3]**
- `generateReport`: non-empty `--instructions` with invalid or mixed selectors
  issues no `SetInstructions` RPC. **[R3]**

`nlmsync`:

- A part with a label is replaced and keeps exactly its own label, not its
  sibling's; an unlabeled part stays unlabeled.
- A split half inherits its live parent's labels through in-process lineage.
- Consolidation of two differently labeled leaves yields their union, and the
  donors are deleted only after the survivor carries it.
- Legacy ` (split1)`/` (split2)` and current `(ptN) (ab)` names resolve to the
  same canonical identity; a non-canonical lookalike (`pt0`, `(z)`, `split3`)
  is left untouched.
- A family whose literal base ends in parentheses (`--name 'notes (draft)'`)
  is owned, and so is `notes (draft) (pt2)`; a base that literally ends in
  `(pt2)` is likewise owned as written. Suffixes are parsed only after
  stripping the caller's exact base. **[R3]**
- The bare first chunk and a ` (pt1)` alias existing together abort before
  mutation; a `… [old]` recovery title does not count as a duplicate and is
  deleted only after its labels have been transferred to the canonical leaf
  and verified — not merely because the leaf exists. **[R4]**
- Consolidation outranks exact-leaf **in the supported complete-subtree
  collapse**: `base`=A and `base (pt2)`=B collapsing to `base` yields A ∪ B,
  and a member crossing a boundary between two differently labeled donors of
  that collapse yields both labels. General rechunk provenance is deferred
  slice 6; an ambiguous labeled rechunk errors before mutation instead.
  **[R4]**
- "Missing ancestor ⇒ no labels" is asserted for a **new** part with no
  provenance only; an **existing** labeled leaf whose ancestor is gone keeps
  its own labels. **[R4]**
- A missing ancestor and a nested partial-retry tree resolve to the no-labels
  case without falling back to a family union.
- Attach failure or cancellation before old-source deletion preserves the old
  assignment; a retry issues no duplicate attach.
- A label read failure causes zero remote mutations, in dry run and real run
  alike; a client without `LabelPreserver` fails both modes identically unless
  `Options.NoLabels` is set; known-empty labels are a separate passing case.
  **[R4]**
- `base (pt2) (split1) (split2)` resolves as a legacy split path under chunk 2.
  **[R4]**
- Recovery: `… [old]` labeled A beside a canonical leaf with none ⇒ the leaf
  gains A and only then is `[old]` deleted; a partial attach followed by a
  retry converges without duplicate attachments; an attach failure never
  deletes `[old]`; an `[old]` source alone is renamed back rather than
  re-uploaded. **[R4]**
- An ambiguous labeled rechunk errors before mutation, and the error repeats
  identically on re-run (no false self-healing instruction). **[R4]**
- A labeled family growing from one part to several syncs without error, and
  every part — old and new — ends up carrying the family's shared labels.
  **[R4]**
- Dry run emits exact planned label IDs and targets, and issues zero
  Add/Rename/Delete/Attach calls; an unchanged execution issues zero attach
  calls.

## 7. Implementation slices

1. **Selection safety** — `selection` type, explicit-empty error, convert
   every consumer off `len(IDs) == 0`.
2. **Mixed-include migration** — `--selector-mode`, rejection, help/doc text.
3. **New flags** — `--label-none`, `--label-exclude-ids`, parsing rules.
4. **Intersect semantics** — steps 1–3, delete `hasOnlyExcludes`.
5. **nlmsync label planning** — lineage contract, capability rule, recovery
   donors, subtree-collapse consolidation, dry-run events, skip-path repair.
6. **Partition manifest** — deferred; general rechunk provenance, with the
   validity contract in section 5.2 as its entry criteria. **[R4]**

Slices 1 and 2 land before 4: the safety net must exist before the semantics
that can shrink a selection. **No intermediate commit may accept
`--selector-mode=intersect` while still applying union** — either a mode is
accepted together with its implemented semantics, or it is not accepted yet.
**[R3]**
