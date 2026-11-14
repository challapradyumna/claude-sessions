//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func osProcessName(pid int) string {
	if pid <= 0 {
		return ""
	}
	if b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm"); err == nil { // Linux
		return strings.TrimSpace(string(b))
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output() // macOS, BSD
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

func spawnConsole(string, string, ...string) error {
	return errors.New("windows-console is only available on Windows")
}
