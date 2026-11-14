# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-11-14

The tool now manages any number of named session sets. Restarts are one use case, alongside switching contexts and keeping standing setups.

### Added
- Named sets: `save NAME` and `restore NAME`. The name is optional and defaults to `default`.
- `save NAME --dir PATH` saves only the sessions working in `PATH` or below it.
- `sets` lists saved sets, `show NAME` lists a set's sessions and marks the ones running, and `delete NAME` removes a set.
- `CLAUDE_SESSIONS_DIR` sets where sets are stored. The default is `~/.claude/session-sets`.

### Changed
- Sets are stored as `~/.claude/session-sets/<NAME>.tsv`. A v0.1.0 snapshot at `~/.claude/saved-sessions.tsv` is still read as the `default` set.

### Removed
- `CLAUDE_SESSIONS_FILE`, replaced by `CLAUDE_SESSIONS_DIR`.

## [0.1.0] - 2026-11-14

### Added
- Standalone binaries for macOS, Linux and Windows (amd64 and arm64), with nothing else to install.
- `install.sh` (macOS/Linux) and `install.ps1` (Windows) one-line installers that verify SHA-256 checksums.
- `save`: snapshot every running interactive Claude Code session (ID, working directory, name) to a TSV file, keeping the previous one as `.bak`.
- `restore`: reopen each saved session with `claude --resume <id>` in its original directory, one tab or window each.
- `list`: show running and saved sessions, and which terminal restore would use.
- Terminal launchers for iTerm2, Terminal.app, tmux, WezTerm, kitty, Ghostty, Alacritty, GNOME Terminal, Konsole, Xfce Terminal, xterm, Windows Terminal and the plain Windows console, plus a `print` fallback.
- `--dry-run` and `--terminal NAME` options, and a `version` command.
- Skips sessions that are already running, have no transcript, or whose directory is gone. Saves a session once if it's open in two processes, and rejects malformed session IDs in an edited snapshot.
- Honours `CLAUDE_CONFIG_DIR` and `CLAUDE_SESSIONS_FILE`.

[Unreleased]: https://github.com/challapradyumna/claude-sessions/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/challapradyumna/claude-sessions/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/challapradyumna/claude-sessions/releases/tag/v0.1.0
