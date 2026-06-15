# Granary — Incremental Fetch + Daily Schedule

**Date:** 2026-06-15
**Status:** Approved design, pending implementation plan

## Problem

`APIClient.FetchState` (`exporter/api.go`) unconditionally fetches a transcript
for **every** document on **every** run (`api.go:172-187`). With ~313 documents
and the LaunchAgent firing every 2 hours (`StartInterval 7200`), that is ~313
transcript calls every 2 hours — roughly 3,700/day against Granola's private,
undocumented API, growing without bound as the meeting corpus grows.

Two problems result:
1. **Slowness** — every run re-downloads the entire corpus.
2. **Etiquette** — hammering an undocumented private API at this volume is the
   kind of pattern that gets a token or client signature flagged.

The existing idempotent-write logic (`exporter.go:157-162`) only saves disk
churn; the network cost is paid in full every run regardless.

## Goals

- Nightly run fetches only **new** meetings and **still-partial** transcripts.
- Cost is bounded by *new activity*, not total corpus size.
- User retains full local ownership of transcripts; no AI cost to read them.
- Be a polite client of Granola's private API.

## Non-goals

- No change to what is exported (transcripts only; AI notes still excluded).
- No new persistent state/index file. The files on disk remain the source of
  truth.
- No configurability beyond a fixed-time override and a force-all flag (YAGNI).

## Design

Two independent, complementary levers.

### Lever 1 — Daily schedule (`service/`)

- Replace `StartInterval 7200` with a once-daily `StartCalendarInterval`.
- The run time is a **per-user random time in `[00:00, 03:00)`** (random
  `Hour ∈ {0,1,2}`, `Minute ∈ 0–59`), chosen at `install` and baked into the
  plist. Randomizing per user de-synchronizes granary installs so they do not
  spike Granola's API at the same instant. Running in the early morning also
  means meetings are over, so transcripts are complete (see Lever 2 — this is
  what makes `is_final` a reliable "done" signal).
- `granary install --at HH:MM` overrides with a fixed time. Validate
  `HH ∈ 0–23`, `MM ∈ 0–59`; reject malformed input with an actionable error.
- launchd behavior: if the Mac is asleep at the scheduled minute, launchd runs
  the missed calendar job on next wake, so laptops closed overnight still get a
  daily run. Document this.
- Update `printStatus` (`main.go:162`) and the `install` command help text,
  which currently say "every 2 hours".

### Lever 2 — Incremental fetch (`exporter/`)

**Completeness marker (`formatter.go`):**
- Transcript entries already carry `IsFinal` (`document.go:20`); the formatter
  currently ignores it.
- For each non-final entry (`IsFinal == false`), emit the marker
  `<!--granary:partial-->` immediately after the speaker prefix, e.g.:
  `**Me:** <!--granary:partial--> and then we can`
- Complete transcripts carry **no** markers. Convention: *absence of any marker
  means complete.* This auto-migrates the existing ~313 files (no markers →
  treated as complete → skipped on the first nightly run; no full re-pull, no
  hand-editing of real data).

**Skip decision (`api.go`, `FetchState`):**
- `APIClient` gains an `OutputDir string` field. Empty ⇒ skip logic disabled,
  fetch everything (preserves current behavior and existing tests).
- A `ForceAll bool` field (wired to `run --all`) ⇒ fetch everything, ignore
  skip rules.
- Filenames are computed over the **full document set** (`AllDocuments()`) in
  **both** `FetchState` and `Export`, via the same `buildFilenameMap` input, so
  the file a skip-check looks for is exactly the file the writer would produce.
  This prevents a new same-title/same-day meeting from being falsely skipped.
  (Note: this changes `Export` to map over all docs instead of the exportable
  subset; in rare title+date collision cases an existing clean-named file may be
  rewritten once under an ID-suffixed name. Benign, deterministic, documented.)

Per document, decide whether to fetch the transcript:

| State on disk | Meeting age | Decision |
|---|---|---|
| Complete file (exists, no marker) | any | **skip** |
| Partial file (exists, has marker) | any | **fetch** |
| No file | `< recencyWindow` | **fetch** (new meeting) |
| No file | `≥ recencyWindow` | **skip** (assumed transcript-less) |

`ForceAll` or a first/backfill run overrides the table and fetches all.

### The recency window (the "first look" guard)

The marker answers *"is this transcript complete?"* It cannot answer *"should I
fetch a doc I have no file for?"* — a genuinely transcript-less meeting
(notes-only, calendar hold) is indistinguishable from a brand-new meeting; both
are "no file." Without a guard, every transcript-less doc would be re-fetched
every night forever — the original problem, nightly.

Therefore, for the **missing-file case only**, fetch when the meeting's
`created_at` is within a recency window (`recencyWindow = 7 days`, a tunable
package const), else leave it alone. This is orthogonal to the completeness
decision: it is "is this worth a first look," not "is this done." `created_at`
comes from `get-documents`; parse it as RFC3339 (fall back to fetch if it cannot
be parsed, to avoid silently dropping a doc).

### Backfill and the escape hatch

- **First run / empty output dir:** `main.go` already detects a missing output
  dir (`main.go:109`). When the output dir is missing or contains no `.md`
  files, ignore the window and backfill everything, so day one pulls full
  history.
- **`granary run --all` (and bare `granary --all`):** force a full re-fetch,
  ignoring all skip rules. This is the documented way to re-pull old transcripts
  (deleting a single >7-day-old file alone will not trigger a re-fetch, because
  it then looks transcript-less). Deleting the entire output dir also triggers a
  backfill.

### Output / reporting

- `FetchState` progress counter counts only documents actually being fetched.
- After the fetch loop, report how many were skipped because already complete,
  e.g. `N transcript(s) already saved (skipped download)`.

### Documentation

- **README:** daily-at-random behavior; `granary install --at HH:MM`; "run
  `granary` by hand to pull now"; `granary run --all` for a full re-pull; note
  that the nightly run completes any partial transcripts.
- **CLAUDE.md:** update the `StartInterval 7200` / every-2-hours references and
  the architecture notes to reflect the daily schedule and incremental fetch.

## Testing

- Skip logic: unit tests against temp-dir fixtures (TDD) covering each row of
  the decision table, plus `ForceAll`, backfill (empty dir), the full-set
  filename consistency, and unparseable `created_at`.
- Marker round-trip: formatter emits `<!--granary:partial-->` only for non-final
  entries; complete transcripts contain no marker; the skip check is a
  `strings.Contains` scan for the marker.
- Schedule: `service/` plist generation produces a valid `StartCalendarInterval`
  for both random and `--at` paths; `--at` validation rejects malformed input.
- Real end-to-end check (no data mutation): one manual `granary` run after
  deploy should skip the existing corpus and pull only what is new. The real
  ~313 transcripts are **not** edited for testing.

## Risks / accepted limitations

- A transcript that first appears more than `recencyWindow` (7 days) after the
  meeting's `created_at` would be missed by the nightly run. Rare; covered by a
  manual run or `run --all`.
- Full-set filename mapping may rewrite a colliding clean-named file once on
  upgrade. Benign and deterministic.
- This breaks the "v0.4.0 is final" maintenance posture; the over-fetch is
  treated as the reported bug that justifies a new release. Version bump handled
  at release time (out of scope for this spec).
