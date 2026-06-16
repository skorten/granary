# Granary

Saves your [Granola](https://www.granola.so) meeting transcripts as plain text (markdown) files on your Mac.

Each meeting becomes one file named like `2026-05-14_Team Standup.md`, saved in a folder you can open in Finder. Granary uses your existing Granola login — there's no separate sign-up or password.

> Granary is a small command-line tool. You'll need to open the **Terminal** app and type a couple of commands. If you've never used Terminal, that's okay — the steps below tell you exactly what to type.

## Why granary instead of the Granola MCP?

Granola offers an MCP server that lets an AI assistant query your meetings and
transcripts on demand. That is great for asking questions in the moment, but it
is a different tool for a different job:

| | Granola MCP | granary |
|---|---|---|
| Where your transcripts live | In Granola's cloud; fetched per request | Plain `.md` files on your own disk |
| Reading them | Through an AI assistant, online | Any tool — grep, an editor, your own scripts, any LLM |
| Cost to read | Spends AI tokens/round-trips each time | Free; they're just local files |
| Works offline | No | Yes, once exported |
| If you leave Granola | Access goes away | You keep the archive |

Use the MCP when you want to *ask Granola questions live*. Use granary when you
want to *own a durable, local, plain-text copy* of your transcripts that any
tool can read without ongoing AI cost. The two are complementary — granary is
your backup and your data, on your terms.

## ✅ Before you start

- A Mac.
- The [Granola app](https://www.granola.so) installed and **signed in**, with your meetings synced at least once. Granary reads your login from the Granola app, so Granola must be set up first.

## ⬇️ Install (download — easiest)

1. Go to the [**Releases** page](https://github.com/skorten/granary/releases).
2. Under the latest release, open **Assets** and download the file for your Mac:
   - Apple Silicon (M1/M2/M3/M4): `granary-Darwin-arm64.tar.gz`
   - Older Intel Macs: `granary-Darwin-amd64.tar.gz`
   - Not sure? Click the Apple menu  → **About This Mac**. "Apple M…" means Apple Silicon.
3. Double-click the downloaded `.tar.gz` in Finder to unzip it. You'll get a file named `granary`.

**macOS will warn that the app is from an unidentified developer.** That's expected — Granary isn't signed with a paid Apple certificate. To allow it, open **Terminal** (press `Command`+`Space`, type `Terminal`, press Return) and run these lines one at a time:

```bash
mkdir -p ~/bin
mv ~/Downloads/granary ~/bin/granary
chmod +x ~/bin/granary
xattr -d com.apple.quarantine ~/bin/granary
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

(That moves the program into a `bin` folder in your home folder, marks it as safe to run, and tells your Mac where to find it. You only do this once.)

Check it worked:

```bash
granary version
```

If you see a version number, you're ready.

## 🧑‍💻 Install (build from source — for developers)

Requires [Go](https://go.dev/dl/).

```bash
git clone https://github.com/skorten/granary.git
cd granary
go install .
```

This installs `granary` into `$(go env GOPATH)/bin` (usually `~/go/bin`). If `granary` isn't found afterward, add that folder to your `PATH`:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

## 💻 Use it

Export your transcripts:

```bash
granary
```

(You can also write `granary run` — they do the same thing.)

Granary connects to Granola, downloads your transcripts, and saves them. When it finishes, it prints the folder where your files are.

### Where your files go

By default, transcripts are saved to **`~/Documents/Granola Transcripts`** — open Finder, click **Documents**, then **Granola Transcripts**.

To open that folder automatically when the export finishes, add `--open`:

```bash
granary --open
```

To use a different folder:

```bash
granary -o ~/Desktop/My\ Transcripts
```

### Run it automatically every day

Turn on automatic background exports:

```bash
granary install
```

By default, granary picks a random time between midnight and 3 AM and runs once a day at that time. If your Mac is asleep when the scheduled time arrives, it runs automatically the next time your Mac wakes up.

To set a specific daily time instead (24-hour format):

```bash
granary install --at 02:30
```

You don't have to wait for the scheduled run — just run `granary` by hand any time to pull your latest transcripts immediately.

To force a full re-download of every transcript (for example, if you want to refresh older meetings):

```bash
granary run --all
```

Granary only downloads new transcripts and any still in progress, so scheduled runs are fast and light on Granola's servers.

```bash
granary status      # check whether automatic exports are on
granary uninstall   # turn them off
```

## ⚠️ Good to know

Granary works today, but a few honest caveats:

- **It uses an unofficial Granola feature.** Granary talks to Granola's private, undocumented service (`api.granola.ai`) — the same one the Granola app uses. Granola could change or remove it at any time, which would stop Granary from working. This project is not affiliated with Granola.
- **It uses your existing Granola login.** Granary reads your login from the Granola app on your Mac and uses it to download *your* transcripts from *your* account. Your login is never saved by Granary and is only ever sent to Granola's own servers. Nothing is shared with anyone else.
- **Keep your Granola login fresh.** If your login has expired, Granary will say so — just open the Granola app, let it load, and run Granary again.
- **It contacts Granola every time it runs.** Including each automatic background run.
- **macOS only.**

## 🛟 Troubleshooting

**"granary: command not found"**
Your Mac doesn't know where the program is. Make sure you ran the `export PATH=...` line during install, then either open a new Terminal window or run `source ~/.zshrc`. If you use the older `bash` shell instead of `zsh`, use `~/.bash_profile` in place of `~/.zshrc`. As a quick test you can always run it by full path: `~/bin/granary`.

**"granary" cannot be opened because it is from an unidentified developer** (or "Apple could not verify it is free of malware")
macOS quarantined the download. Clear it in Terminal:

```bash
xattr -d com.apple.quarantine ~/bin/granary
```

If you downloaded it somewhere else, point that command at wherever `granary` actually is. Alternatively, in Finder: right-click the file → **Open** → **Open**, or go to **System Settings → Privacy & Security** and click **Open Anyway**.

**"zsh: permission denied: granary"**
The file isn't marked runnable. Fix it with:

```bash
chmod +x ~/bin/granary
```

**"couldn't find your Granola login on this Mac"**
The Granola app isn't set up yet. Install [Granola](https://www.granola.so), sign in, let it sync at least one meeting, then run Granary again.

**"your access token is missing or expired" / "open the Granola app to refresh it"**
Your Granola login has expired. Open the Granola app, wait for it to finish loading, then run Granary again.

**"couldn't reach Granola's servers"**
Check your internet connection and try again. If you're on a VPN or restricted network, that may be blocking the connection.

**"No meetings were found"**
Granary connected fine but Granola has no meetings to export yet. Record or open a meeting in Granola, let it sync, then try again.

**I can't find my exported files**
They're in `~/Documents/Granola Transcripts` by default. In Finder, click **Documents** in the sidebar, then open **Granola Transcripts**. Or run `granary --open` to open the folder automatically when the export finishes.

**The download is a `.tar.gz` and macOS won't let me open the unzipped file**
Some browsers quarantine the archive too. After unzipping and moving `granary` into place, the `xattr -d com.apple.quarantine` step above clears it.

## ⚙️ How it works (for the curious)

Granola encrypted its local files, so Granary can no longer read meetings from a file on disk. Instead it:

1. Reads your Granola access token from `~/Library/Application Support/Granola/supabase.json.enc`, decrypting it with your macOS Keychain (the same mechanism the Granola app uses).
2. Calls `https://api.granola.ai` to list your meetings (`/v2/get-documents`) and download each transcript (`/v1/get-document-transcript`), sending your token and a client-version header from your installed Granola app.
3. Writes one markdown file per meeting.

Granary downloads transcripts only — it does not export Granola's AI-generated notes. Once a transcript is saved, Granary preserves it on future runs even if Granola later returns nothing for that meeting, so you don't lose data.

### Output format

```markdown
# Meeting Title
Date: 2026-05-14 14:30
Meeting ID: abc-123

---

## Transcript

**Me:** [Your words]

**Them:** [Other participant's words]
```

## 📝 Disclaimer

This project is not affiliated with, endorsed by, or connected to [Granola](https://www.granola.so) in any way. I love Granola and use it every day. This is just a personal utility to export my meeting data.

## 🍴 Credits

Originally forked from [wassimk/granary](https://github.com/wassimk/granary), then rewritten to fetch transcripts from Granola's API after Granola encrypted its local cache.
