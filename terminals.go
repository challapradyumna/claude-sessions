package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// A launcher opens one tab or window per job, each running `claude --resume <id>` in the job's directory.
// needs is the executable that must be on PATH ("" = don't check). run == nil means "print the commands".
type launcher struct {
	run   func(jobs []Session, w io.Writer) error
	needs string
}

var terminals = map[string]launcher{
	"iterm":           {runITerm, "osascript"},
	"terminal":        {runAppleTerminal, "osascript"},
	"tmux":            {runTmux, "tmux"},
	"wezterm":         {runWezterm, "wezterm"},
	"kitty":           {runKitty, "kitty"},
	"ghostty":         {runGhostty, ""},
	"alacritty":       {runAlacritty, "alacritty"},
	"gnome-terminal":  {runGnome, "gnome-terminal"},
	"konsole":         {runKonsole, "konsole"},
	"xfce4-terminal":  {runXfce, "xfce4-terminal"},
	"xterm":           {runXterm, "xterm"},
	"wt":              {runWindowsTerminal, "wt"},
	"windows-console": {runWindowsConsole, "powershell"},
	"print":           {nil, ""},
}

func terminalNames() []string {
	var n []string
	for k := range terminals {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

var lookPath = exec.LookPath // swappable in tests

func onPath(bin string) bool { _, err := lookPath(bin); return err == nil }

func claudeArgv(id string) []string { return []string{"claude", "--resume", id} }

func title(j Session) string {
	if j.Name != "" {
		return j.Name
	}
	return j.ID
}

// posixQuote single-quotes s for sh/zsh/bash.
func posixQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// shellLine is the copy-pasteable command for one job, used by --dry-run, print and the macOS launchers.
func shellLine(j Session) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`cd /d "%s" && claude --resume %s`, j.Cwd, j.ID)
	}
	return fmt.Sprintf("cd %s && claude --resume %s", posixQuote(j.Cwd), j.ID)
}

// osa quotes s as an AppleScript string literal.
func osa(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil && len(out) > 0 {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return err
}

// spawn starts a GUI terminal without waiting for it, detached from this process.
func spawn(dir, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	detach(c)
	return c.Start()
}

// ---------------------------------------------------------------- macOS

func runITerm(jobs []Session, _ io.Writer) error {
	s := []string{`tell application "iTerm"`, "activate", "set w to (create window with default profile)"}
	for i, j := range jobs {
		if i > 0 {
			s = append(s, "tell w to create tab with default profile")
		}
		s = append(s, "tell current session of current tab of w",
			"write text "+osa(shellLine(j)), "set name to "+osa(title(j)), "end tell")
	}
	s = append(s, "end tell")
	return runCmd("osascript", "-e", strings.Join(s, "\n"))
}

func runAppleTerminal(jobs []Session, _ io.Writer) error {
	for _, j := range jobs {
		if err := runCmd("osascript", "-e", `tell application "Terminal" to do script `+osa(shellLine(j))); err != nil {
			return err
		}
	}
	return runCmd("osascript", "-e", `tell application "Terminal" to activate`)
}

// ---------------------------------------------------------------- cross-platform

func runTmux(jobs []Session, w io.Writer) error {
	inside := os.Getenv("TMUX") != ""
	target := ""
	for _, j := range jobs {
		t := title(j)
		if r := []rune(t); len(r) > 40 {
			t = string(r[:40])
		}
		var args []string
		switch {
		case inside:
			args = []string{"new-window", "-c", j.Cwd, "-n", t}
		case target != "":
			args = []string{"new-window", "-t", target, "-c", j.Cwd, "-n", t}
		default: // not in tmux: build a detached session to attach to
			target = "claude-restore"
			args = []string{"new-session", "-d", "-s", target, "-c", j.Cwd, "-n", t}
		}
		if err := runCmd("tmux", append(args, claudeArgv(j.ID)...)...); err != nil {
			return err
		}
	}
	if target != "" {
		fmt.Fprintf(w, "Created tmux session %q. Attach with:  tmux attach -t %s\n", target, target)
	}
	return nil
}

func runWezterm(jobs []Session, _ io.Writer) error {
	inside := os.Getenv("WEZTERM_PANE") != ""
	for _, j := range jobs {
		var err error
		if inside {
			err = runCmd("wezterm", append([]string{"cli", "spawn", "--cwd", j.Cwd, "--"}, claudeArgv(j.ID)...)...)
		} else {
			err = spawn(j.Cwd, "wezterm", append([]string{"start", "--cwd", j.Cwd, "--"}, claudeArgv(j.ID)...)...)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func runKitty(jobs []Session, _ io.Writer) error {
	remote := os.Getenv("KITTY_LISTEN_ON") != "" || os.Getenv("KITTY_WINDOW_ID") != ""
	for _, j := range jobs {
		// kitty @ needs allow_remote_control; fall back to new windows when it's off.
		if remote && runCmd("kitty", append([]string{"@", "launch", "--type=tab", "--cwd", j.Cwd,
			"--tab-title", title(j)}, claudeArgv(j.ID)...)...) == nil {
			continue
		}
		if err := spawn(j.Cwd, "kitty", append([]string{"--directory", j.Cwd, "--title", title(j)},
			claudeArgv(j.ID)...)...); err != nil {
			return err
		}
	}
	return nil
}

func runGhostty(jobs []Session, _ io.Writer) error {
	for _, j := range jobs {
		args := append([]string{"--working-directory=" + j.Cwd, "-e"}, claudeArgv(j.ID)...)
		var err error
		if runtime.GOOS == "darwin" {
			err = runCmd("open", append([]string{"-na", "Ghostty", "--args"}, args...)...)
		} else {
			err = spawn(j.Cwd, "ghostty", args...)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func runAlacritty(jobs []Session, _ io.Writer) error {
	for _, j := range jobs {
		if err := spawn(j.Cwd, "alacritty", append([]string{"--working-directory", j.Cwd, "-T", title(j), "-e"},
			claudeArgv(j.ID)...)...); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- Linux

func runGnome(jobs []Session, _ io.Writer) error {
	for _, j := range jobs { // client/server: --tab lands in the most recent window
		if err := runCmd("gnome-terminal", append([]string{"--tab", "--working-directory=" + j.Cwd,
			"--title=" + title(j), "--"}, claudeArgv(j.ID)...)...); err != nil {
			return err
		}
	}
	return nil
}

func runKonsole(jobs []Session, _ io.Writer) error {
	for _, j := range jobs {
		if err := spawn(j.Cwd, "konsole", append([]string{"--new-tab", "--workdir", j.Cwd, "-e"},
			claudeArgv(j.ID)...)...); err != nil {
			return err
		}
	}
	return nil
}

func runXfce(jobs []Session, _ io.Writer) error {
	for _, j := range jobs {
		if err := spawn(j.Cwd, "xfce4-terminal", append([]string{"--tab", "--working-directory", j.Cwd,
			"-T", title(j), "-x"}, claudeArgv(j.ID)...)...); err != nil {
			return err
		}
	}
	return nil
}

func runXterm(jobs []Session, _ io.Writer) error {
	for _, j := range jobs {
		if err := spawn(j.Cwd, "xterm", append([]string{"-T", title(j), "-e"}, claudeArgv(j.ID)...)...); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- Windows

func runWindowsTerminal(jobs []Session, _ io.Writer) error {
	for _, j := range jobs { // -w 0 = current window, nt = new tab
		if err := runCmd("wt", "-w", "0", "nt", "-d", j.Cwd, "--title", title(j),
			"powershell", "-NoExit", "-Command", "claude --resume "+j.ID); err != nil {
			return err
		}
	}
	return nil
}

func runWindowsConsole(jobs []Session, _ io.Writer) error {
	for _, j := range jobs {
		if err := spawnConsole(j.Cwd, "powershell", "-NoExit", "-Command", "claude --resume "+j.ID); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- detection

// detectTerminal prefers the terminal this command runs in, then any installed one, then "print".
func detectTerminal() string {
	e := os.Getenv
	tp := strings.ToLower(e("TERM_PROGRAM"))
	switch {
	case e("TMUX") != "":
		return "tmux"
	case tp == "iterm.app":
		return "iterm"
	case tp == "apple_terminal":
		return "terminal"
	case tp == "wezterm" || e("WEZTERM_PANE") != "":
		return "wezterm"
	case tp == "ghostty" || e("GHOSTTY_RESOURCES_DIR") != "":
		return "ghostty"
	case e("KITTY_WINDOW_ID") != "":
		return "kitty"
	case e("ALACRITTY_WINDOW_ID") != "" || e("ALACRITTY_SOCKET") != "":
		return "alacritty"
	case e("WT_SESSION") != "" && runtime.GOOS == "windows":
		return "wt"
	case e("KONSOLE_VERSION") != "":
		return "konsole"
	case e("GNOME_TERMINAL_SCREEN") != "" || e("VTE_VERSION") != "":
		return "gnome-terminal"
	}
	// Not inside a recognisable terminal (VS Code, ssh, a fresh login...): pick something installed.
	switch runtime.GOOS {
	case "darwin":
		if isDir("/Applications/iTerm.app") {
			return "iterm"
		}
		return "terminal"
	case "windows":
		if onPath("wt") {
			return "wt"
		}
		return "windows-console"
	}
	gui := e("DISPLAY") != "" || e("WAYLAND_DISPLAY") != ""
	for _, t := range []string{"gnome-terminal", "konsole", "xfce4-terminal", "kitty", "wezterm", "alacritty", "xterm"} {
		if gui && onPath(t) {
			return t
		}
	}
	if onPath("tmux") {
		return "tmux"
	}
	return "print"
}
