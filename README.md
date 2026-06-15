# Granary

Exports full meeting transcripts from [Granola](https://www.granola.so) to markdown files.

Granary fetches your meetings and their transcripts from Granola's API and writes one markdown file per meeting. It reads your existing Granola login from the app's local config (no separate sign-in), only writes changed files, and preserves previously exported transcripts. A built-in macOS LaunchAgent can run exports automatically every 2 hours.

> **Heads up:** Granary now talks to Granola's private, undocumented API and uses your locally stored Granola access token. This is unsupported and can break at any time. Read [How it works](#-how-it-works) and [Caveats](#-caveats) before using it.

## 🛠️ Installation

### Homebrew

```bash
brew install wassimk/tap/granary
```

### From source

```bash
go install github.com/wassimk/granary@latest
```

## 💻 Usage

### Export transcripts

```bash
granary run
```

By default, Granary reads your Granola access token from `~/Library/Application Support/Granola/` (see [How it works](#-how-it-works)), fetches your meetings and transcripts from Granola's API, and exports markdown files to `~/.local/share/granola-transcripts/`. Each file is named `YYYY-MM-DD_Meeting_Title.md`.

#### Options

```
-o, --output-dir   Custom output directory (default: ~/.local/share/granola-transcripts)
```

### Background service (LaunchAgent)

Install a macOS LaunchAgent that automatically exports every 2 hours:

```bash
granary install
```

Check the service status:

```bash
granary status
```

Remove the background service:

```bash
granary uninstall
```

### Other commands

```bash
granary version    # Show version
granary help       # Show help
```

## ⚙️ How it works

Granola encrypts its local cache and no longer stores meeting documents in a file Granary can read. To export your transcripts, Granary uses Granola's **private, undocumented API**:

1. It reads your Granola access token from `~/Library/Application Support/Granola/supabase.json.enc`, decrypting it with the key from your macOS Keychain entry `Granola Safe Storage` (the same mechanism the Granola app uses). It falls back to the plaintext `supabase.json` if present.
2. It calls `https://api.granola.ai` to list your meetings (`/v2/get-documents`) and fetch each meeting's transcript (`/v1/get-document-transcript`), sending your token as a bearer credential and a client-version header derived from your installed Granola app.
3. It writes one markdown file per meeting to the output directory.

Granary only requests transcript data; it does not export Granola's AI-generated notes.

## ⚠️ Caveats

Please understand these before relying on Granary:

- **Unofficial API.** `api.granola.ai` is private and undocumented. Granola can change, restrict, or remove it at any time with no warning, which will break Granary. This project is not affiliated with Granola (see [Disclaimer](#-disclaimer)).
- **Uses your credentials.** Granary reads your Granola access token from disk and sends it to Granola's servers. It only ever accesses your own account and your own data, and the token is never written to logs or output, but you are using your login outside the official app.
- **Network on every run.** Each `granary run` makes one API call to list meetings (paginated) plus one call per meeting to fetch its transcript. With many meetings that is many requests per run — including each automatic LaunchAgent run every 2 hours.
- **Token expiry.** If your token has expired, Granary fails with a clear message. There is no refresh endpoint — open the Granola app to sign in again, then re-run.
- **Client-version header.** Granola may reject requests that don't carry a recognized client version. Granary sends your installed app's version (or a built-in fallback); if Granola tightens this check, requests may start failing.
- **macOS only.** Granary depends on the macOS Keychain and Granola's macOS file locations.
- **Terms of service.** Using an undocumented API may conflict with Granola's terms. Use Granary responsibly, only with your own account, and at your own risk.

## 📄 Output format

```markdown
# Meeting Title
Date: 2025-01-24 14:30
Meeting ID: abc-123

---

## Transcript

**Me:** [Your words]

**Them:** [Other participant's words]
```

Once Granary exports a transcript, it preserves it: on future runs it keeps any previously exported transcript if the API returns nothing for that meeting, so you never lose data.

## 📝 Disclaimer

This project is not affiliated with, endorsed by, or connected to [Granola](https://www.granola.so) in any way. I love Granola and use it every day. This is just a personal utility to export my meeting data.
