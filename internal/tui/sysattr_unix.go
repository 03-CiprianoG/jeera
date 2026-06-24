//go:build unix

package tui

import "syscall"

// detachedSysProcAttr puts a launched terminal in its own process group, so a
// Ctrl-C or Ctrl-Z aimed at Jeera's foreground group never reaches (or
// suspends) the external window we just spawned.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
