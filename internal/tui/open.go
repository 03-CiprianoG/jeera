package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// safeToOpen validates a reference before it is handed to the OS opener.
// Attachment refs can be supplied by an agent over MCP, so Jeera opens only what
// it trusts: an http/https URL, or a path to an existing regular local file.
// Everything else — other URI schemes (file://, javascript:, smb://…), UNC paths,
// or strings with shell metacharacters — is refused, since the opener would
// otherwise dispatch it to a registered scheme handler or (on Windows) let it be
// re-parsed by a shell.
func safeToOpen(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("nothing to open")
	}
	if strings.HasPrefix(ref, `\\`) || strings.HasPrefix(ref, "//") {
		return fmt.Errorf("refusing to open a UNC path")
	}
	if core.ClassifyAttachment(ref).IsURL() {
		return nil // a plain http/https URL
	}
	// Anything that isn't an http/https URL must be an existing regular file. This
	// rejects every other URI scheme by construction: file://, javascript:, smb://
	// and the like don't stat as a real file.
	info, err := os.Stat(ref)
	if err != nil {
		return fmt.Errorf("cannot open %q: %w", ref, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to open %q: not a regular file", ref)
	}
	return nil
}

// openCommand builds the platform command that opens a (validated) file or URL in
// the user's default application/browser. It is split out (and pure) so the
// argument shape is testable without launching anything.
func openCommand(ref string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", ref)
	case "windows":
		// rundll32 hands the ref to the shell's file/URL handler WITHOUT routing
		// through cmd.exe, so the ref is never re-parsed for shell metacharacters
		// (which `cmd /c start` would be — see go doc exec.Command).
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", ref)
	default:
		return exec.Command("xdg-open", ref)
	}
}

// openExternal validates ref and opens it in the default application, detached
// from the TUI. It reaps the launcher process so it doesn't linger as a zombie.
func openExternal(ref string) error {
	if err := safeToOpen(ref); err != nil {
		return err
	}
	cmd := openCommand(ref)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap the launcher without blocking the TUI
	return nil
}
