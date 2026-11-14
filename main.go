// claude-sessions saves named sets of open Claude Code sessions and restores them later,
// each in its own terminal tab.
//
//	claude-sessions save [NAME] [--dir PATH]                     save the running sessions as a set
//	claude-sessions restore [NAME] [--dry-run] [--terminal T]    reopen every session in a set
//	claude-sessions sets | show NAME | delete NAME | list | version
//
// Source: <claude config dir>/sessions/<pid>.json (Claude Code writes one per live session)
// Sets:   <claude config dir>/session-sets/<NAME>.tsv  (pid, sessionId, cwd, name)
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

const defaultSet = "default"

const usage = `claude-sessions: save a set of open Claude Code sessions and restore it later.

Usage:
  claude-sessions save [NAME] [--dir PATH]   save the running sessions as set NAME (default "default");
                                             --dir keeps only sessions working under PATH
  claude-sessions restore [NAME]             reopen every session in the set, one tab each
        --dry-run                            print the commands, open nothing
        --terminal T                         force a launcher: %s
  claude-sessions sets                       list saved sets
  claude-sessions show NAME                  list the sessions in a set
  claude-sessions delete NAME                delete a set
  claude-sessions list                       show running sessions and saved sets (default)
  claude-sessions version

Environment:
  CLAUDE_CONFIG_DIR      Claude Code config dir (default ~/.claude)
  CLAUDE_SESSIONS_DIR    where sets are stored (default $CLAUDE_CONFIG_DIR/session-sets)
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

// validID guards everything we later splice into shell/AppleScript, since set files are user-editable.
var validID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// validSet keeps set names usable as file names on every OS.
var validSet = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// processName returns the image name of a live pid, or "" if it isn't running. Swappable in tests.
var processName = osProcessName

func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func setsDir() string {
	if d := os.Getenv("CLAUDE_SESSIONS_DIR"); d != "" {
		return d
	}
	return filepath.Join(claudeDir(), "session-sets")
}

func setPath(name string) string { return filepath.Join(setsDir(), name+".tsv") }

// legacyFile is where v0.1.0 kept its single snapshot; it is read as the "default" set.
func legacyFile() string { return filepath.Join(claudeDir(), "saved-sessions.tsv") }

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

// under reports whether path is dir or inside it.
func under(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func parseSet(b []byte) []Session {
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

// readSet returns the sessions in a set and whether the set exists.
func readSet(name string) ([]Session, bool) {
	b, err := os.ReadFile(setPath(name))
	if err != nil && name == defaultSet {
		b, err = os.ReadFile(legacyFile())
	}
	if err != nil {
		return nil, false
	}
	return parseSet(b), true
}

type setInfo struct {
	Name    string
	Count   int
	ModTime string
}

func listSets() []setInfo {
	files, _ := filepath.Glob(filepath.Join(setsDir(), "*.tsv"))
	seen := map[string]bool{}
	var out []setInfo
	add := func(name, path string) {
		st, err := os.Stat(path)
		if err != nil || seen[name] {
			return
		}
		b, _ := os.ReadFile(path)
		seen[name] = true
		out = append(out, setInfo{name, len(parseSet(b)), st.ModTime().Format("2006-01-02 15:04")})
	}
	for _, f := range files {
		add(strings.TrimSuffix(filepath.Base(f), ".tsv"), f)
	}
	add(defaultSet, legacyFile())
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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

// splitArgs separates positional arguments from --flag [value] options.
func splitArgs(args []string, valued map[string]bool) (pos []string, opts map[string]string, err error) {
	opts = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		if k, v, ok := strings.Cut(a, "="); ok && valued[k] {
			opts[k] = v
			continue
		}
		switch {
		case valued[a]:
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("%s needs a value", a)
			}
			opts[a] = args[i+1]
			i++
		case a == "--dry-run" || a == "-n":
			opts["--dry-run"] = "1"
		default:
			return nil, nil, fmt.Errorf("unknown option %q", a)
		}
	}
	return pos, opts, nil
}

func setName(pos []string, w io.Writer) (string, bool) {
	if len(pos) > 1 {
		fmt.Fprintln(w, "expected at most one set name, got:", strings.Join(pos, " "))
		return "", false
	}
	name := defaultSet
	if len(pos) == 1 {
		name = pos[0]
	}
	if !validSet.MatchString(name) {
		fmt.Fprintf(w, "invalid set name %q: use letters, digits, '.', '_' or '-'\n", name)
		return "", false
	}
	return name, true
}

func cmdSave(args []string, w io.Writer) int {
	pos, opts, err := splitArgs(args, map[string]bool{"--dir": true, "-d": true})
	if err != nil {
		fmt.Fprintln(w, err)
		return 2
	}
	name, ok := setName(pos, w)
	if !ok {
		return 2
	}
	rows := uniqueSessions(liveSessions(), w)
	if dir := opts["--dir"] + opts["-d"]; dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintln(w, "error:", err)
			return 2
		}
		var kept []Session
		for _, r := range rows {
			if under(r.Cwd, abs) {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "No matching running Claude sessions; nothing saved.")
		return 1
	}
	path := setPath(name)
	if err := os.MkdirAll(setsDir(), 0o700); err != nil {
		fmt.Fprintln(w, "error:", err)
		return 1
	}
	if old, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".bak", old, 0o600)
	}
	var sb strings.Builder
	for _, r := range rows {
		sb.WriteString(strings.Join([]string{r.PID, r.ID, r.Cwd, r.Name}, "\t") + "\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		fmt.Fprintln(w, "error:", err)
		return 1
	}
	fmt.Fprintf(w, "Saved %d session(s) as set %q (%s):\n", len(rows), name, path)
	for _, r := range rows {
		fmt.Fprintf(w, "  %-36s  %-5s  %s  (%s)\n", r.ID, r.Status, r.Cwd, r.Name)
		if r.Status == "busy" {
			fmt.Fprintln(w, "      ^ still busy: if you close it now, it resumes mid-turn")
		}
	}
	return 0
}

func cmdSets(w io.Writer) int {
	sets := listSets()
	if len(sets) == 0 {
		fmt.Fprintln(w, "No saved sets. Create one with: claude-sessions save NAME")
		return 0
	}
	for _, s := range sets {
		fmt.Fprintf(w, "  %-24s %3d session(s)   saved %s\n", s.Name, s.Count, s.ModTime)
	}
	return 0
}

func cmdShow(args []string, w io.Writer) int {
	name, ok := setName(args, w)
	if !ok {
		return 2
	}
	rows, exists := readSet(name)
	if !exists {
		fmt.Fprintf(w, "No set named %q. See: claude-sessions sets\n", name)
		return 1
	}
	running := map[string]bool{}
	for _, r := range liveSessions() {
		running[r.ID] = true
	}
	for _, r := range rows {
		state := ""
		if running[r.ID] {
			state = "running"
		}
		fmt.Fprintf(w, "  %-36s %-7s %s  (%s)\n", r.ID, state, r.Cwd, r.Name)
	}
	return 0
}

func cmdDelete(args []string, w io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(w, "usage: claude-sessions delete NAME")
		return 2
	}
	name, ok := setName(args, w)
	if !ok {
		return 2
	}
	removed := false
	for _, p := range []string{setPath(name), setPath(name) + ".bak"} {
		if os.Remove(p) == nil && !strings.HasSuffix(p, ".bak") {
			removed = true
		}
	}
	if name == defaultSet && os.Remove(legacyFile()) == nil {
		removed = true
	}
	if !removed {
		fmt.Fprintf(w, "No set named %q.\n", name)
		return 1
	}
	fmt.Fprintf(w, "Deleted set %q.\n", name)
	return 0
}

func cmdList(w io.Writer) int {
	fmt.Fprintln(w, "== Running now ==")
	live := liveSessions()
	for _, r := range live {
		fmt.Fprintf(w, "  %-7s %-36s %-5s %s  (%s)\n", r.PID, r.ID, r.Status, r.Cwd, r.Name)
	}
	if len(live) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	fmt.Fprintf(w, "== Saved sets (%s) ==\n", setsDir())
	cmdSets(w)
	fmt.Fprintln(w, "Restore would use terminal: "+detectTerminal())
	return 0
}

func cmdRestore(args []string, w io.Writer) int {
	pos, opts, err := splitArgs(args, map[string]bool{"--terminal": true, "-t": true})
	if err != nil {
		fmt.Fprintln(w, err)
		return 2
	}
	name, ok := setName(pos, w)
	if !ok {
		return 2
	}
	term := opts["--terminal"] + opts["-t"]
	if term == "" {
		term = detectTerminal()
	}
	t, ok := terminals[term]
	if !ok {
		fmt.Fprintf(w, "Unknown terminal %q. Choose from: %s\n", term, strings.Join(terminalNames(), ", "))
		return 2
	}
	saved, exists := readSet(name)
	if !exists {
		fmt.Fprintf(w, "No set named %q. Save one with 'claude-sessions save %s', or see 'claude-sessions sets'.\n",
			name, name)
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
	if opts["--dry-run"] != "" || t.run == nil {
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
	fmt.Fprintf(w, "Restored %d session(s) from set %q via %s.\n", len(jobs), name, term)
	return 0
}

func run(args []string, w io.Writer) int {
	cmd := "list"
	if len(args) > 0 {
		cmd = args[0]
	}
	rest := []string{}
	if len(args) > 1 {
		rest = args[1:]
	}
	switch cmd {
	case "save":
		return cmdSave(rest, w)
	case "restore":
		return cmdRestore(rest, w)
	case "sets", "ls":
		return cmdSets(w)
	case "show":
		return cmdShow(rest, w)
	case "delete", "rm":
		return cmdDelete(rest, w)
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
