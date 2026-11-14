// claude-sessions saves open Claude Code sessions before a reboot and restores them after.
//
//	claude-sessions save                      snapshot every running interactive session
//	claude-sessions restore                   reopen each in a new tab/window: claude --resume <id>
//	claude-sessions restore --dry-run         print the commands, open nothing
//	claude-sessions restore --terminal NAME   force a launcher (see terminals.go)
//	claude-sessions list                      show running vs saved
//	claude-sessions version
//
// Source:   <claude config dir>/sessions/<pid>.json (Claude Code writes one per live session)
// Snapshot: <claude config dir>/saved-sessions.tsv  (pid, sessionId, cwd, name)
// The config dir is $CLAUDE_CONFIG_DIR if set, else ~/.claude.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// version is set at build time with -ldflags "-X main.version=..."
var version = "dev"

const usage = `claude-sessions: save open Claude Code sessions before a reboot, restore them after.

Usage:
  claude-sessions save                      snapshot every running interactive session
  claude-sessions restore                   reopen each saved session in a new tab/window
  claude-sessions restore --dry-run         print the commands, open nothing
  claude-sessions restore --terminal NAME   force a launcher: %s
  claude-sessions list                      show running vs saved sessions (default)
  claude-sessions version

Environment:
  CLAUDE_CONFIG_DIR      Claude Code config dir (default ~/.claude)
  CLAUDE_SESSIONS_FILE   snapshot file (default $CLAUDE_CONFIG_DIR/saved-sessions.tsv)
`

// Session is one Claude Code session, live or saved.
type Session struct {
	PID     string
	ID      string
	Cwd     string
	Name    string
	Status  string
	Updated int64
}

// validID guards everything we later splice into shell/AppleScript, since the snapshot file is user-editable.
var validID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// processName returns the image name of a live pid, or "" if it isn't running. Swappable in tests.
var processName = osProcessName

func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func saveFile() string {
	if f := os.Getenv("CLAUDE_SESSIONS_FILE"); f != "" {
		return f
	}
	return filepath.Join(claudeDir(), "saved-sessions.tsv")
}

func flatten(s string) string { return strings.Join(strings.Fields(s), " ") }

// liveSessions reads the per-process status files and keeps the ones whose process is still a running claude.
func liveSessions() []Session {
	files, _ := filepath.Glob(filepath.Join(claudeDir(), "sessions", "*.json"))
	sort.Strings(files)
	var out []Session
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var d struct {
			PID       int64  `json:"pid"`
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			Kind      string `json:"kind"`
			UpdatedAt int64  `json:"updatedAt"`
		}
		if json.Unmarshal(b, &d) != nil || d.SessionID == "" || !validID.MatchString(d.SessionID) {
			continue
		}
		if d.Kind != "" && d.Kind != "interactive" {
			continue
		}
		name := processName(int(d.PID))
		if name == "" || !strings.Contains(strings.ToLower(name), "claude") { // gone, or pid reused
			continue
		}
		cwd := d.Cwd
		if cwd == "" {
			cwd, _ = os.UserHomeDir()
		}
		out = append(out, Session{PID: strconv.FormatInt(d.PID, 10), ID: d.SessionID, Cwd: cwd,
			Name: flatten(d.Name), Status: d.Status, Updated: d.UpdatedAt})
	}
	return out
}

// uniqueSessions keeps one entry per session ID (the most recently active) when a session is open twice.
func uniqueSessions(rows []Session, w io.Writer) []Session {
	best := map[string]int{}
	for i, r := range rows {
		if j, ok := best[r.ID]; !ok || r.Updated > rows[j].Updated {
			best[r.ID] = i
		}
	}
	if dupes := len(rows) - len(best); dupes > 0 {
		fmt.Fprintf(w, "note: %d session(s) are open in more than one process; saving each once.\n", dupes)
	}
	var out []Session
	for i, r := range rows {
		if best[r.ID] == i {
			out = append(out, r)
		}
	}
	return out
}

func readSaved() []Session {
	b, err := os.ReadFile(saveFile())
	if err != nil {
		return nil
	}
	var out []Session
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		p := strings.Split(line, "\t")
		if len(p) < 3 || p[1] == "" {
			continue
		}
		s := Session{PID: p[0], ID: p[1], Cwd: p[2]}
		if len(p) > 3 {
			s.Name = p[3]
		}
		out = append(out, s)
	}
	return out
}

func hasTranscript(id string) bool {
	m, _ := filepath.Glob(filepath.Join(claudeDir(), "projects", "*", id+".jsonl"))
	return len(m) > 0
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func cmdSave(w io.Writer) int {
	rows := uniqueSessions(liveSessions(), w)
	if len(rows) == 0 {
		fmt.Fprintln(w, "No running Claude sessions found in "+filepath.Join(claudeDir(), "sessions"))
		return 1
	}
	path := saveFile()
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".bak", old, 0o600)
	}
	var sb strings.Builder
	for _, r := range rows {
		sb.WriteString(strings.Join([]string{r.PID, r.ID, r.Cwd, r.Name}, "\t") + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintln(w, "error:", err)
		return 1
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		fmt.Fprintln(w, "error:", err)
		return 1
	}
	fmt.Fprintf(w, "Saved %d session(s) to %s:\n", len(rows), path)
	for _, r := range rows {
		fmt.Fprintf(w, "  %-36s  %-5s  %s  (%s)\n", r.ID, r.Status, r.Cwd, r.Name)
		if r.Status == "busy" {
			fmt.Fprintln(w, "      ^ still busy: let it finish, or it resumes mid-turn")
		}
	}
	return 0
}

func cmdList(w io.Writer) int {
	fmt.Fprintln(w, "== Running now ==")
	for _, r := range liveSessions() {
		fmt.Fprintf(w, "  %-7s %-36s %-5s %s  (%s)\n", r.PID, r.ID, r.Status, r.Cwd, r.Name)
	}
	fmt.Fprintf(w, "== Saved (%s) ==\n", saveFile())
	saved := readSaved()
	for _, r := range saved {
		fmt.Fprintf(w, "  %-36s %s  (%s)\n", r.ID, r.Cwd, r.Name)
	}
	if len(saved) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	fmt.Fprintln(w, "Restore would use terminal: "+detectTerminal())
	return 0
}

func cmdRestore(args []string, w io.Writer) int {
	dry, term := false, ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run", "-n":
			dry = true
		case "--terminal", "-t":
			if i+1 < len(args) {
				term = args[i+1]
				i++
			}
		default:
			fmt.Fprintf(w, "unknown option %q\n", args[i])
			return 2
		}
	}
	if term == "" {
		term = detectTerminal()
	}
	t, ok := terminals[term]
	if !ok {
		fmt.Fprintf(w, "Unknown terminal %q. Choose from: %s\n", term, strings.Join(terminalNames(), ", "))
		return 2
	}
	saved := readSaved()
	if len(saved) == 0 {
		fmt.Fprintf(w, "Nothing saved at %s. Run 'claude-sessions save' first.\n", saveFile())
		return 1
	}
	running := map[string]bool{}
	for _, r := range liveSessions() {
		running[r.ID] = true
	}
	var jobs []Session
	for _, r := range saved {
		switch {
		case !validID.MatchString(r.ID):
			fmt.Fprintf(w, "skip (invalid id):      %q\n", r.ID)
		case running[r.ID]:
			fmt.Fprintf(w, "skip (already running): %s %s\n", r.ID, r.Name)
		case !hasTranscript(r.ID):
			fmt.Fprintf(w, "skip (no transcript):   %s %s\n", r.ID, r.Name)
		case !isDir(r.Cwd):
			fmt.Fprintf(w, "skip (folder missing):  %s %s\n", r.ID, r.Cwd)
		default:
			jobs = append(jobs, r)
		}
	}
	if len(jobs) == 0 {
		fmt.Fprintln(w, "Nothing to restore.")
		return 0
	}
	if dry || t.run == nil {
		if t.run == nil {
			fmt.Fprintln(w, "No supported terminal detected; run each of these in its own tab:")
		}
		for _, j := range jobs {
			fmt.Fprintln(w, shellLine(j))
		}
		return 0
	}
	if t.needs != "" && !onPath(t.needs) {
		fmt.Fprintf(w, "%q is not on PATH. Use --terminal to pick another, or --dry-run to print the commands.\n", t.needs)
		return 1
	}
	if err := t.run(jobs, w); err != nil {
		fmt.Fprintf(w, "restore via %s failed: %v\n", term, err)
		return 1
	}
	fmt.Fprintf(w, "Restored %d session(s) via %s.\n", len(jobs), term)
	return 0
}

func run(args []string, w io.Writer) int {
	cmd := "list"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "save":
		return cmdSave(w)
	case "restore":
		return cmdRestore(args[1:], w)
	case "list":
		return cmdList(w)
	case "version", "--version", "-V":
		fmt.Fprintln(w, "claude-sessions "+version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprintf(w, usage, strings.Join(terminalNames(), ", "))
		return 0
	}
	fmt.Fprintf(w, usage, strings.Join(terminalNames(), ", "))
	return 2
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
