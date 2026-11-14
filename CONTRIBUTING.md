# Contributing

Thanks for helping. Bug reports, fixes and new terminal launchers are all welcome.

## Ground rules

- **Stay one file, standard library only.** You install the tool by downloading a single script, so please don't add third-party dependencies.
- **Python 3.7+.** Don't use syntax newer than that, such as `match` or `X | Y` type unions.
- **Never call `os.kill(pid, 0)` on Windows.** On Windows it terminates the process. Use `process_name()` instead.

## Development

```sh
git clone https://github.com/challapradyumna/claude-sessions
cd claude-sessions
python3 -m unittest discover -s tests -v
./claude-sessions list
./claude-sessions restore --dry-run
```

The tests point `CLAUDE_CONFIG_DIR` at a temporary directory and mock the process lookup, so they never touch your real sessions.

## Adding a terminal

1. Write a `run_<name>(jobs)` function. Each job is a dict with `cwd`, `sid` and `name`, and the function should open one tab or window per job running `claude_argv(job["sid"])` in `job["cwd"]`. Pass the directory with the terminal's own flag (for example `--working-directory`) rather than building a shell string where you can, because it avoids quoting bugs.
2. Register it in `TERMINALS`.
3. Teach `detect_terminal()` how to recognise it, usually from an environment variable the terminal sets.
4. Add it to the table in `README.md` and to `CHANGELOG.md` under *Unreleased*.
5. In the pull request, say which OS and terminal version you tested on.

## Releasing (maintainers)

1. Bump `__version__` in `claude-sessions`, and move the *Unreleased* entries in `CHANGELOG.md` under the new version.
2. Commit, then `git tag vX.Y.Z && git push --tags`.
3. The release workflow runs the tests, checks that the tag matches `__version__`, and publishes a GitHub release with the script attached.
