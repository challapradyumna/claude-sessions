package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setup points CLAUDE_CONFIG_DIR at a temp dir and replaces process lookup with the returned map.
func setup(t *testing.T) (dir string, live map[int]string) {
	t.Helper()
	dir = t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("CLAUDE_SESSIONS_FILE", "")
	live = map[int]string{}
	old := processName
	processName = func(pid int) string { return live[pid] }
	t.Cleanup(func() { processName = old })
	return dir, live
}

func writeSession(t *testing.T, dir string, pid int, fields map[string]any) {
	t.Helper()
	d := map[string]any{"pid": pid, "kind": "interactive", "status": "idle", "updatedAt": 1, "cwd": dir}
	for k, v := range fields {
		d[k] = v
	}
	b, _ := json.Marshal(d)
	must(t, os.MkdirAll(filepath.Join(dir, "sessions"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "sessions", fmt.Sprintf("%d.json", pid)), b, 0o600))
}

func writeTranscript(t *testing.T, dir, id string) {
	t.Helper()
	p := filepath.Join(dir, "projects", "-some-project")
	must(t, os.MkdirAll(p, 0o700))
	must(t, os.WriteFile(filepath.Join(p, id+".jsonl"), nil, 0o600))
}

func writeSaved(t *testing.T, lines ...string) {
	t.Helper()
	must(t, os.WriteFile(saveFile(), []byte(strings.Join(lines, "\n")+"\n"), 0o600))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func runCLI(args ...string) (int, string) {
	var b bytes.Buffer
	code := run(args, &b)
	return code, b.String()
}

func ids(s []Session) []string {
	var out []string
	for _, r := range s {
		out = append(out, r.ID)
	}
	return out
}

func TestLiveSessionsFiltersDeadReusedAndNonInteractive(t *testing.T) {
	dir, live := setup(t)
	writeSession(t, dir, 1, map[string]any{"sessionId": "alive"})
	writeSession(t, dir, 2, map[string]any{"sessionId": "dead"})
	writeSession(t, dir, 3, map[string]any{"sessionId": "reused-pid"})
	writeSession(t, dir, 4, map[string]any{"sessionId": "headless", "kind": "sdk"})
	writeSession(t, dir, 5, map[string]any{"sessionId": "bad id; rm -rf"})
	live[1], live[3], live[4], live[5] = "claude", "nginx", "claude", "claude"
	if got := ids(liveSessions()); strings.Join(got, ",") != "alive" {
		t.Fatalf("got %v", got)
	}
}

func TestCorruptJSONIgnored(t *testing.T) {
	dir, _ := setup(t)
	must(t, os.MkdirAll(filepath.Join(dir, "sessions"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "sessions", "9.json"), []byte("{nope"), 0o600))
	if got := liveSessions(); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestNamesFlattenedForTSV(t *testing.T) {
	dir, live := setup(t)
	writeSession(t, dir, 1, map[string]any{"sessionId": "a", "name": "line one\n\tline two"})
	live[1] = "claude"
	if got := liveSessions()[0].Name; got != "line one line two" {
		t.Fatalf("got %q", got)
	}
}

func TestDuplicatesKeepMostRecent(t *testing.T) {
	dir, live := setup(t)
	writeSession(t, dir, 1, map[string]any{"sessionId": "same", "updatedAt": 10, "name": "old"})
	writeSession(t, dir, 2, map[string]any{"sessionId": "same", "updatedAt": 20, "name": "new"})
	live[1], live[2] = "claude", "claude"
	got := uniqueSessions(liveSessions(), io.Discard)
	if len(got) != 1 || got[0].Name != "new" {
		t.Fatalf("got %+v", got)
	}
}

func TestSaveThenRestoreDryRun(t *testing.T) {
	dir, live := setup(t)
	writeSession(t, dir, 1, map[string]any{"sessionId": "sid-1", "name": "one"})
	writeTranscript(t, dir, "sid-1")
	live[1] = "claude"
	if code, out := runCLI("save"); code != 0 {
		t.Fatalf("save: %d %s", code, out)
	}
	if n := len(readSaved()); n != 1 {
		t.Fatalf("saved %d", n)
	}
	delete(live, 1) // "after reboot"
	code, out := runCLI("restore", "--dry-run")
	if code != 0 || !strings.Contains(out, "claude --resume sid-1") {
		t.Fatalf("restore: %d %s", code, out)
	}
}

func TestSaveKeepsBackup(t *testing.T) {
	dir, live := setup(t)
	writeSession(t, dir, 1, map[string]any{"sessionId": "sid-1"})
	live[1] = "claude"
	runCLI("save")
	runCLI("save")
	if _, err := os.Stat(saveFile() + ".bak"); err != nil {
		t.Fatal(err)
	}
}

func TestSaveWithNothingRunning(t *testing.T) {
	setup(t)
	if code, out := runCLI("save"); code != 1 || !strings.Contains(out, "No running") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestRestoreSkips(t *testing.T) {
	dir, live := setup(t)
	writeSaved(t,
		"1\trunning\t"+dir+"\ta",
		"2\tno-transcript\t"+dir+"\tb",
		"3\tgone-dir\t"+filepath.Join(dir, "nope")+"\tc",
		"4\t$(evil)\t"+dir+"\td",
		"5\tok\t"+dir+"\te")
	for _, id := range []string{"running", "gone-dir", "ok"} {
		writeTranscript(t, dir, id)
	}
	writeSession(t, dir, 1, map[string]any{"sessionId": "running"})
	live[1] = "claude"
	_, out := runCLI("restore", "--dry-run")
	for _, want := range []string{"skip (already running)", "skip (no transcript)", "skip (folder missing)",
		"skip (invalid id)", "claude --resume ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "claude --resume"); n != 1 {
		t.Errorf("%d resume lines:\n%s", n, out)
	}
}

func TestRestoreDispatchesToLauncher(t *testing.T) {
	dir, _ := setup(t)
	writeSaved(t, "1\tsid\t"+dir+"\tname")
	writeTranscript(t, dir, "sid")
	var got []Session
	old := terminals["tmux"]
	terminals["tmux"] = launcher{func(j []Session, _ io.Writer) error { got = j; return nil }, "tmux"}
	oldLook := lookPath
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	t.Cleanup(func() { terminals["tmux"] = old; lookPath = oldLook })

	if code, out := runCLI("restore", "--terminal", "tmux"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if strings.Join(ids(got), ",") != "sid" {
		t.Fatalf("got %v", got)
	}
}

func TestRestoreMissingBinary(t *testing.T) {
	dir, _ := setup(t)
	writeSaved(t, "1\tsid\t"+dir+"\tname")
	writeTranscript(t, dir, "sid")
	oldLook := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("nope") }
	t.Cleanup(func() { lookPath = oldLook })
	if code, out := runCLI("restore", "--terminal", "kitty"); code != 1 || !strings.Contains(out, "not on PATH") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestBadArgs(t *testing.T) {
	setup(t)
	if code, _ := runCLI("restore", "--terminal", "teletype"); code != 2 {
		t.Error("unknown terminal should exit 2")
	}
	if code, _ := runCLI("restore", "--bogus"); code != 2 {
		t.Error("unknown flag should exit 2")
	}
	if code, _ := runCLI("restore"); code != 1 {
		t.Error("restore without snapshot should exit 1")
	}
	if code, _ := runCLI("frobnicate"); code != 2 {
		t.Error("unknown command should exit 2")
	}
}

func TestQuoting(t *testing.T) {
	if got := osa(`a "b" \c`); got != `"a \"b\" \\c"` {
		t.Errorf("osa: %s", got)
	}
	if got := posixQuote("it's"); got != `'it'\''s'` {
		t.Errorf("posixQuote: %s", got)
	}
	line := shellLine(Session{Cwd: "/tmp/my dir", ID: "x"})
	if !strings.Contains(line, "my dir") || !strings.HasSuffix(line, "claude --resume x") {
		t.Errorf("shellLine: %s", line)
	}
}

func TestDetectTerminalPrefersTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	if got := detectTerminal(); got != "tmux" {
		t.Errorf("got %s", got)
	}
}

func TestVersion(t *testing.T) {
	if code, out := runCLI("version"); code != 0 || !strings.Contains(out, version) {
		t.Errorf("%d %s", code, out)
	}
}

func TestOSProcessNameSelf(t *testing.T) {
	if osProcessName(os.Getpid()) == "" {
		t.Error("current process should be found")
	}
	if osProcessName(-1) != "" {
		t.Error("invalid pid should not be found")
	}
}
