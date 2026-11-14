# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-11-14

### Added
- `save`: snapshot every running interactive Claude Code session (ID, working directory, name) to a TSV file, keeping the previous one as `.bak`.
- `restore`: reopen each saved session with `claude --resume <id>` in its original directory, one tab or window each.
- `list`: show running and saved sessions, and which terminal restore would use.
- Terminal launchers for iTerm2, Terminal.app, tmux, WezTerm, kitty, Ghostty, Alacritty, GNOME Terminal, Konsole, Xfce Terminal, xterm, Windows Terminal and the plain Windows console, plus a `print` fallback.
- `--dry-run`, `--terminal NAME` and `--version` options.
- Skips sessions that are already running, have no transcript, or whose directory is gone. Saves a session once if it's open in two processes.
- Honours `CLAUDE_CONFIG_DIR` and `CLAUDE_SESSIONS_FILE`.

[Unreleased]: https://github.com/challapradyumna/claude-sessions/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/challapradyumna/claude-sessions/releases/tag/v0.1.0
