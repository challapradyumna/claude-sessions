"""Tests for claude-sessions. Run with:  python -m unittest discover -s tests"""
import importlib.machinery
import importlib.util
import io
import json
import os
import tempfile
import unittest
from contextlib import redirect_stdout
from unittest import mock

SCRIPT = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "claude-sessions")


def load_module(config_dir):
    """Import the extensionless script with CLAUDE_CONFIG_DIR pointed at a temp dir."""
    with mock.patch.dict(os.environ, {"CLAUDE_CONFIG_DIR": config_dir}, clear=False):
        os.environ.pop("CLAUDE_SESSIONS_FILE", None)
        loader = importlib.machinery.SourceFileLoader("claude_sessions", SCRIPT)
        spec = importlib.util.spec_from_loader("claude_sessions", loader)
        mod = importlib.util.module_from_spec(spec)
        loader.exec_module(mod)
    return mod


def make_session(config_dir, pid, sid, cwd, name="s", status="idle", updated=1, kind="interactive"):
    os.makedirs(os.path.join(config_dir, "sessions"), exist_ok=True)
    with open(os.path.join(config_dir, "sessions", "%d.json" % pid), "w") as fh:
        json.dump({"pid": pid, "sessionId": sid, "cwd": cwd, "name": name, "status": status,
                   "updatedAt": updated, "kind": kind}, fh)


def make_transcript(config_dir, sid):
    d = os.path.join(config_dir, "projects", "-some-project")
    os.makedirs(d, exist_ok=True)
    open(os.path.join(d, sid + ".jsonl"), "w").close()


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = self.tmp.name
        self.cs = load_module(self.dir)
        self.live = {}   # pid -> process name; anything absent is "not running"
        p = mock.patch.object(self.cs, "process_name", side_effect=lambda pid: self.live.get(pid))
        p.start()
        self.addCleanup(p.stop)
        self.addCleanup(self.tmp.cleanup)

    def run_cmd(self, *argv):
        buf = io.StringIO()
        with redirect_stdout(buf):
            code = self.cs.main(list(argv))
        return code, buf.getvalue()


class DiscoveryTests(Base):
    def test_only_live_claude_interactive_sessions(self):
        make_session(self.dir, 1, "alive", self.dir)
        make_session(self.dir, 2, "dead", self.dir)
        make_session(self.dir, 3, "reused-pid", self.dir)
        make_session(self.dir, 4, "headless", self.dir, kind="sdk")
        self.live = {1: "claude", 3: "nginx", 4: "claude"}
        self.assertEqual([r["sid"] for r in self.cs.live_sessions()], ["alive"])

    def test_corrupt_json_is_ignored(self):
        os.makedirs(os.path.join(self.dir, "sessions"))
        with open(os.path.join(self.dir, "sessions", "9.json"), "w") as fh:
            fh.write("{not json")
        self.assertEqual(self.cs.live_sessions(), [])

    def test_names_are_flattened_for_tsv(self):
        make_session(self.dir, 1, "a", self.dir, name="line one\n\tline two")
        self.live = {1: "claude"}
        self.assertEqual(self.cs.live_sessions()[0]["name"], "line one line two")

    def test_duplicate_sessions_keep_most_recent(self):
        make_session(self.dir, 1, "same", self.dir, updated=10, name="old")
        make_session(self.dir, 2, "same", self.dir, updated=20, name="new")
        self.live = {1: "claude", 2: "claude"}
        with redirect_stdout(io.StringIO()):
            rows = self.cs.unique_sessions(self.cs.live_sessions())
        self.assertEqual([r["name"] for r in rows], ["new"])


class SaveRestoreTests(Base):
    def test_save_then_restore_dry_run(self):
        make_session(self.dir, 1, "sid-1", self.dir, name="one")
        make_transcript(self.dir, "sid-1")
        self.live = {1: "claude"}
        code, _ = self.run_cmd("save")
        self.assertEqual(code, 0)
        self.assertEqual(len(self.cs.read_saved()), 1)

        self.live = {}   # "after reboot"
        code, out = self.run_cmd("restore", "--dry-run")
        self.assertEqual(code, 0)
        self.assertIn("claude --resume sid-1", out)

    def test_save_keeps_backup(self):
        make_session(self.dir, 1, "sid-1", self.dir)
        self.live = {1: "claude"}
        self.run_cmd("save")
        self.run_cmd("save")
        self.assertTrue(os.path.exists(self.cs.SAVE_FILE + ".bak"))

    def test_save_with_nothing_running(self):
        code, out = self.run_cmd("save")
        self.assertEqual(code, 1)
        self.assertIn("No running", out)

    def test_restore_skips_running_missing_transcript_and_missing_dir(self):
        with open(self.cs.SAVE_FILE, "w") as fh:
            fh.write("1\trunning\t%s\ta\n" % self.dir)
            fh.write("2\tno-transcript\t%s\tb\n" % self.dir)
            fh.write("3\tgone-dir\t%s\tc\n" % os.path.join(self.dir, "nope"))
            fh.write("4\tok\t%s\td\n" % self.dir)
        for sid in ("running", "gone-dir", "ok"):
            make_transcript(self.dir, sid)
        make_session(self.dir, 1, "running", self.dir)
        self.live = {1: "claude"}
        code, out = self.run_cmd("restore", "--dry-run")
        self.assertIn("skip (already running)", out)
        self.assertIn("skip (no transcript)", out)
        self.assertIn("skip (folder missing)", out)
        self.assertEqual(out.count("claude --resume"), 1)
        self.assertIn("claude --resume ok", out)

    def test_restore_dispatches_to_launcher(self):
        with open(self.cs.SAVE_FILE, "w") as fh:
            fh.write("1\tsid\t%s\tname\n" % self.dir)
        make_transcript(self.dir, "sid")
        launcher = mock.Mock()
        with mock.patch.dict(self.cs.TERMINALS, {"tmux": launcher}), \
                mock.patch.object(self.cs.shutil, "which", return_value="/usr/bin/tmux"):
            code, out = self.run_cmd("restore", "--terminal", "tmux")
        self.assertEqual(code, 0)
        jobs = launcher.call_args[0][0]
        self.assertEqual([j["sid"] for j in jobs], ["sid"])

    def test_unknown_terminal(self):
        code, out = self.run_cmd("restore", "--terminal", "teletype")
        self.assertEqual(code, 2)

    def test_restore_without_snapshot(self):
        code, out = self.run_cmd("restore")
        self.assertEqual(code, 1)


class HelperTests(Base):
    def test_applescript_quoting(self):
        self.assertEqual(self.cs.osa('a "b" \\c'), '"a \\"b\\" \\\\c"')

    def test_shell_line_quotes_cwd(self):
        line = self.cs.shell_line({"cwd": "/tmp/my dir", "sid": "x"})
        self.assertIn("claude --resume x", line)
        self.assertIn("my dir", line)

    def test_detect_terminal_prefers_tmux(self):
        with mock.patch.dict(os.environ, {"TMUX": "/tmp/tmux-1/default,1,0"}):
            self.assertEqual(self.cs.detect_terminal(), "tmux")

    def test_version(self):
        code, out = self.run_cmd("--version")
        self.assertEqual(code, 0)
        self.assertIn(self.cs.__version__, out)


if __name__ == "__main__":
    unittest.main()
