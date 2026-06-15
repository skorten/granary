# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Granary is a macOS-only Go CLI that exports Granola meeting **transcripts** to
markdown files. Granola encrypted its local cache and moved meeting documents
into an encrypted SQLite DB that can't be read offline, so Granary fetches
documents and transcripts from Granola's **private, undocumented HTTP API**
(`https://api.granola.ai`) using the access token stored locally by the Granola
app. AI-generated notes are intentionally **not** exported. Module:
`github.com/skorten/granary`. Independent fork (no upstream remote); background:
GitHub issue #13. In maintenance mode — v0.4.0 is intended as the final release
unless a bug is reported.

## Commands

```bash
go build ./...                       # build all packages (CI gate)
go run .                             # run an export against the live Granola API (bare command == `run`)
go test ./...                        # run all tests
go test ./exporter -run TestName     # run a single test
go test ./exporter -run TestAPIClientFetchState -v
GOOS=linux GOARCH=amd64 go build ./...   # CI builds on Linux; keep it compiling
```

There is no separate lint step; CI runs `go build -v ./...` then the test suite
(`.github/workflows/ci.yml`) **on Linux**. The macOS-specific code (Keychain via
`security`, `defaults`, `launchctl`) compiles on Linux and only fails at runtime,
so keep everything cross-compiling. Releases are free, **unsigned** GitHub
Releases cut by GoReleaser (`.goreleaser.yaml`) on a `v*` tag via
`.github/workflows/release.yml` (runs on `ubuntu-latest`, uses only the
auto-provided `GITHUB_TOKEN`): darwin amd64/arm64 archives, no Apple signing or
notarization, no Homebrew tap. `version` is injected via `-ldflags -X main.version=`.
Unsigned binaries trigger macOS Gatekeeper; the README documents the one-time
`xattr -d com.apple.quarantine` workaround.

## Architecture

Layers wired together in `main.go` (cobra commands: `run`, `install`,
`uninstall`, `status`, `version`). `runExport` orchestrates:
`GranolaSupportDir` → `AccessToken` → `APIClient.FetchState` → `Exporter.Export`.

- **`exporter/`** — token recovery, the API client, and the markdown export logic.
- **`service/`** — macOS LaunchAgent management via `launchctl bootstrap`/`bootout`.
  Generates a plist (`com.skorten.granary`) with `StartInterval 7200` (2 hours).
- **`main.go`** — CLI wiring and `runExport` console output.

### Token recovery (`api.go` + `safestorage.go`)

`AccessToken(supportDir)` reads Granola's API token. It prefers the encrypted
`supabase.json.enc`, decrypting it via `decryptCache` (`safestorage.go`), and
falls back to plaintext `supabase.json`. The token is the `access_token` inside
the **JSON-encoded string** field `workos_tokens`.

`safestorage.go` implements the Electron safeStorage / Chromium OSCrypt scheme,
used only to decrypt the token file: the macOS Keychain entry
`Granola Safe Storage` / `Granola Key` → PBKDF2-SHA1 (salt `saltysalt`, 1003
iters, 16-byte key) → AES-128-CBC unwrap of `storage.dek` (strip `v10` prefix,
all-spaces IV, base64-decode) → 32-byte DEK → AES-256-GCM (`nonce||ct||tag`)
decrypt of the `.enc` payload. `keychainSecret` is a package var so tests inject
a known secret (`setKeychainSecret` lives in the test file).

### API client (`api.go`)

`APIClient.FetchState` returns a `*CacheState` (the same struct the exporter
consumes), so all the formatting/filename/preservation logic is reused unchanged:

- Paginates `POST /v2/get-documents` (offset-based) collecting `id`/`title`/
  `created_at` only — notes fields are ignored.
- Then `POST /v1/get-document-transcript` per document. Calls carry
  `Authorization: Bearer`, `User-Agent: Granola/<ver>`, `X-Client-Version: <ver>`
  where `<ver>` comes from `GranolaClientVersion()` (installed app version, with a
  fallback const).
- **Resilience**: a failed transcript fetch is logged and skipped so one error
  doesn't discard the run; `ErrUnauthorized` (401/403) is fatal and aborts, since
  a token problem affects every request.

### Markdown export (`exporter.go`, `formatter.go`, `filename.go`)

`CacheState` holds `Documents`/`SharedDocuments`/`Transcripts` maps keyed by doc
ID (`document.go`); `AllDocuments()` merges with owned taking precedence.
`Exporter.Export` writes one `YYYY-MM-DD_Title.md` per document.

- **Idempotent writes**: a file is only rewritten when content differs
  (`Skipped` otherwise); docs with no notes and no transcript are `Empty`.
- **Transcript preservation**: in `exportDocument`, if a file already exists and
  the new state has no transcript for that doc, it re-extracts the transcript from
  the existing markdown (`ExtractTranscriptFromMarkdown` in `extractor.go`) so a
  transient empty API response never deletes data. This depends on
  `SourceToSpeaker` (`formatter.go`) and `SpeakerToSource` (`extractor.go`) being
  inverses (`microphone`↔`Me`, `system`↔`Them`), and on the `**Speaker:** text`
  format matching `transcriptEntryRegex`.
- **Filename collisions**: `buildFilenameMap` appends an 8-char ID suffix when
  title+date collide. Default output dir: `~/Documents/Granola Transcripts/`
  (Finder-visible). Files are written `0600` in a `0700` directory.

## Constraints

- macOS-only at runtime: Keychain via `security`, app version via `defaults`,
  Granola file locations, `service/` shells out to `launchctl`; GoReleaser builds
  darwin only. All of it still cross-compiles for the Linux CI build.
- Standard-library-only; the sole direct dependency is `spf13/cobra`.
- Depends on a private, undocumented Granola API — see `README.md` "Caveats".
