# claude-sessions

[![CI](https://github.com/challapradyumna/claude-sessions/actions/workflows/ci.yml/badge.svg)](https://github.com/challapradyumna/claude-sessions/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/challapradyumna/claude-sessions)](https://github.com/challapradyumna/claude-sessions/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Save your open [Claude Code](https://claude.com/claude-code) sessions before a reboot and bring them all back afterwards, each in its own terminal tab.

```console
$ claude-sessions save
Saved 7 session(s) to ~/.claude/saved-sessions.tsv
# ...reboot...
$ claude-sessions restore
Restored 7 session(s) via iterm.
```

It's a single standalone binary for macOS, Linux and Windows on Intel and ARM. You don't need Python, Node or anything else installed.

## Install

**macOS / Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/challapradyumna/claude-sessions/main/install.sh | sh
```

This installs to `~/.local/bin` and checks the download against the release's SHA-256 checksums. Set `CLAUDE_SESSIONS_INSTALL_DIR` to install somewhere else, or `CLAUDE_SESSIONS_VERSION=v0.1.0` to install a specific release.

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/challapradyumna/claude-sessions/main/install.ps1 | iex
```

This installs to `%LOCALAPPDATA%\Programs\claude-sessions` and adds that folder to your user `PATH`.

**Manual download**

Download the binary for your platform from the [latest release](https://github.com/challapradyumna/claude-sessions/releases/latest), rename it to `claude-sessions` (`claude-sessions.exe` on Windows), make it executable and put it on your `PATH`.

**With Go**

```sh
go install github.com/challapradyumna/claude-sessions@latest
```

## Usage

| Command | What it does |
|---|---|
| `claude-sessions save` | Writes every running interactive session to the snapshot file |
| `claude-sessions restore` | Reopens each saved session in a new tab or window with `claude --resume <id>` |
| `claude-sessions restore --dry-run` | Prints the commands and opens nothing |
| `claude-sessions restore --terminal NAME` | Uses a specific terminal launcher instead of the detected one |
| `claude-sessions list` | Shows running and saved sessions, and the terminal that restore would use |
| `claude-sessions version` | Prints the version |

Run `save` right before you restart. Let busy sessions finish their response first, or they'll resume mid-turn.

## Supported terminals

`restore` uses the terminal you run it from. If it can't tell which one that is, it picks one that's installed. If there's none, it prints the commands for you to paste.

| Platform | Terminals | `--terminal` value |
|---|---|---|
| macOS | iTerm2, Terminal.app | `iterm`, `terminal` |
| macOS / Linux | Ghostty, WezTerm, kitty, Alacritty | `ghostty`, `wezterm`, `kitty`, `alacritty` |
| Linux | GNOME Terminal, Konsole, Xfce Terminal, xterm | `gnome-terminal`, `konsole`, `xfce4-terminal`, `xterm` |
| Windows | Windows Terminal, plain PowerShell console | `wt`, `windows-console` |
| Anywhere | tmux | `tmux` |
| Anywhere | none: print the commands | `print` |

Inside tmux, `restore` adds one window per session. Outside tmux, it creates a detached `claude-restore` session that you attach to with `tmux attach -t claude-restore`.

iTerm2 is the launcher that has been used on a real machine. The others are built from each terminal's documented command-line options. If one doesn't work for you, please [open an issue](https://github.com/challapradyumna/claude-sessions/issues/new/choose).

## How it works

Claude Code writes a status file for each live session to `~/.claude/sessions/<pid>.json`, which records the session ID and working directory. `save` reads these files and skips any whose process has exited or whose PID now belongs to another program. It writes one line per session to `~/.claude/saved-sessions.tsv`:

```
pid    sessionId                              cwd                 name
29735  94ba182f-ef99-4ad8-bb50-cab57b041d57   /home/me/project    my-session
```

Conversations aren't copied. They already live on disk at `~/.claude/projects/<dir>/<sessionId>.jsonl` and survive a reboot. The snapshot only records which sessions were open and where each one ran. `restore` changes to that directory before resuming, because Claude Code looks up a session under the directory it was started from.

`restore` skips sessions that are already running, have no transcript, or whose directory no longer exists. Running it twice won't open duplicates. Each `save` keeps the previous snapshot as `saved-sessions.tsv.bak`.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `CLAUDE_CONFIG_DIR` | `~/.claude` | Claude Code's config directory, the same variable Claude Code uses |
| `CLAUDE_SESSIONS_FILE` | `$CLAUDE_CONFIG_DIR/saved-sessions.tsv` | Where the snapshot is written |

## Limitations

- Background processes a session started (dev servers, watchers) aren't restored. Only the conversation comes back.
- `~/.claude/sessions/*.json` is an internal Claude Code file, not a documented API. If its format changes, `save` may need an update.
- If the same session is open in two processes, it's saved once. Close one of them, since both write to the same transcript.
- The binaries aren't code-signed. The install scripts don't trigger macOS Gatekeeper, but a binary downloaded in a browser may. Run `xattr -d com.apple.quarantine claude-sessions` to clear it.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). New terminal launchers are especially welcome.

## License

[MIT](LICENSE)

This is an independent project. It is not affiliated with or endorsed by Anthropic.
