# Contributing

Thanks for helping. Bug reports, fixes and new terminal launchers are all welcome.

## Ground rules

- **Standard library only.** The tool ships as one dependency-free binary, so please don't add modules to `go.mod`.
- **Never probe whether a Windows process is alive by signalling it.** On Windows, signalling a process terminates it. Use `processName()` instead.
- **Validate before you splice.** Session IDs are checked against `validID` before they go into shell or AppleScript strings, because set files are user-editable. Set names are checked against `validSet`, which also stops path traversal. Keep it that way.

## Development

Requires Go 1.22 or later.

```sh
git clone https://github.com/challapradyumna/claude-sessions
cd claude-sessions
go test ./...
go run . list
go run . restore --dry-run
```

The tests point `CLAUDE_CONFIG_DIR` at a temporary directory and mock the process lookup, so they never touch your real sessions. `scripts/build.sh 0.0.0-dev` cross-compiles every release binary into `dist/`.

## Layout

| File | Contents |
|---|---|
| `main.go` | CLI, session discovery, sets (save / restore / sets / show / delete) |
| `terminals.go` | One launcher per terminal, plus `detectTerminal()` |
| `proc_unix.go`, `proc_windows.go` | Platform-specific process lookup and spawning |
| `install.sh`, `install.ps1` | Installers that download a release binary and verify its checksum |

## Adding a terminal

1. In `terminals.go`, write a `runX(jobs []Session, w io.Writer) error` function. It should open one tab or window per job, running `claudeArgv(j.ID)` in `j.Cwd`. Pass the directory with the terminal's own flag (for example `--working-directory`) rather than building a shell string where you can, because it avoids quoting bugs.
2. Register it in `terminals`, along with the executable that must be on `PATH`.
3. Teach `detectTerminal()` how to recognise it, usually from an environment variable the terminal sets.
4. Add it to the table in `README.md` and to `CHANGELOG.md` under *Unreleased*.
5. In the pull request, say which OS and terminal version you tested on.

## Releasing (maintainers)

1. Move the *Unreleased* entries in `CHANGELOG.md` under a new `## [X.Y.Z] - YYYY-MM-DD` heading.
2. Commit, then `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. The release workflow runs the tests, builds all six binaries with the version stamped in, and publishes a GitHub release with `checksums.txt` and the notes from that changelog section.
