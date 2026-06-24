//go:build !unix

package tui

import "syscall"

// detachedSysProcAttr is a no-op on platforms without Unix process groups;
// callers tolerate a nil attr.
func detachedSysProcAttr() *syscall.SysProcAttr { return nil }
