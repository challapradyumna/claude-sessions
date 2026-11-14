//go:build windows

package main

import (
	"encoding/csv"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const createNewConsole = 0x00000010

// osProcessName uses tasklist. Never probe liveness with os.FindProcess+Signal on Windows:
// signalling there means TerminateProcess.
func osProcessName(pid int) string {
	if pid <= 0 {
		return ""
	}
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return ""
	}
	rec, err := csv.NewReader(strings.NewReader(strings.TrimSpace(string(out)))).Read()
	if err != nil || len(rec) < 2 || rec[1] != strconv.Itoa(pid) { // "INFO: No tasks..." when absent
		return ""
	}
	return rec[0]
}

func detach(c *exec.Cmd) {}

func spawnConsole(dir, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	return c.Start()
}
