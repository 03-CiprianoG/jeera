package tui

import (
	"errors"
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

// ErrNoTerminal is returned when no terminal emulator can be found to host a
// resumed run, so the TUI can tell the user what to do rather than silently
// doing nothing.
var ErrNoTerminal = errors.New("no terminal emulator found — set $TERMINAL or install one (e.g. alacritty, kitty, gnome-terminal)")

// Indirections so detection is unit-testable without a real PATH, OS or env.
var (
	lookPath  = exec.LookPath
	currentOS = runtime.GOOS
	getenv    = os.Getenv
)

// muxLauncher opens a new window/pane inside a terminal multiplexer the user is
// already running (tmux/zellij/screen). This is the one "new terminal" that
// works over SSH and on a headless host: the new window rides the same
// connection and the same outer terminal, so it needs no display and no GUI.
// It reports ok=false when Jeera is not running inside a multiplexer.
func muxLauncher(cwd string, argv []string) (bin string, launch []string, ok bool) {
	switch {
	case getenv("TMUX") != "":
		// A fresh tmux window; tmux runs the trailing command, -c sets its dir. The
		// `--` is load-bearing: without it tmux keeps scanning for its own flags past
		// the command and would eat a leading-dash agent arg (e.g. claude's
		// --resume). `--` makes everything after it the command, verbatim.
		return "tmux", append([]string{"new-window", "-c", cwd, "--"}, argv...), true
	case getenv("ZELLIJ") != "":
		// A new zellij pane running the command.
		return "zellij", append([]string{"run", "--cwd", cwd, "--"}, argv...), true
	case getenv("STY") != "":
		// screen has no cwd flag, so wrap in a shell that cds first. -X sends the
		// command to the running session named by $STY.
		return "screen", []string{"-X", "screen", "-t", "resume", "sh", "-c",
			"cd " + shellQuote(cwd) + " && exec " + shellJoin(argv)}, true
	}
	return "", nil, false
}

// termFamily maps one or more emulator binaries to the args that make them run
// a command in a directory as a fresh window. build returns the args AFTER the
// binary; it is pure so the (load-bearing) argv shape is testable. Every form
// here was verified against current emulator docs — flag order matters (the
// `-e`/`--` command separator must come last) and only emulators whose `-e`
// forwards a real argv vector use the shell wrapper; native-cwd emulators set
// the directory with their own flag.
type termFamily struct {
	names []string
	build func(cwd string, argv []string) []string
}

// termFamilies is the probe order: native-cwd modern emulators first, then the
// xterm-style fallbacks whose `-e` reliably forwards an argv (so the shell
// wrapper is safe). GTK terminals whose `-e` takes a re-parsed string instead
// (xfce4-terminal, mate-terminal, tilix, terminator) are intentionally omitted
// — they are reachable via $TERMINAL or xdg-terminal-exec, which know their
// quirks, rather than launched with a form that would mangle the command.
var termFamilies = []termFamily{
	{[]string{"gnome-terminal"}, func(d string, a []string) []string {
		return append([]string{"--working-directory=" + d, "--"}, a...)
	}},
	{[]string{"konsole"}, func(d string, a []string) []string {
		// --separate keeps Konsole in its own foreground process; -e must be last.
		return append([]string{"--separate", "--workdir", d, "-e"}, a...)
	}},
	{[]string{"wezterm"}, func(d string, a []string) []string {
		// `start` is required; --always-new-process forces a fresh window so the
		// invocation isn't swallowed by a running instance.
		return append([]string{"start", "--always-new-process", "--cwd", d, "--"}, a...)
	}},
	{[]string{"alacritty"}, func(d string, a []string) []string {
		return append([]string{"--working-directory", d, "-e"}, a...)
	}},
	{[]string{"kitty"}, func(d string, a []string) []string {
		// kitty has no -e: the command is a trailing positional.
		return append([]string{"--directory", d}, a...)
	}},
	{[]string{"foot"}, func(d string, a []string) []string {
		// foot also takes the command as a trailing positional (it stops parsing
		// options at the first non-flag arg); its -e is only an ignored xterm
		// compat shim on newer builds, so avoid relying on it.
		return append([]string{"--working-directory=" + d}, a...)
	}},
	{[]string{"xterm", "uxterm", "urxvt", "rxvt-unicode", "rxvt", "st"}, genericShellLaunch},
}

// genericShellLaunch is the xterm-style form: `-e sh -c "cd <dir> && exec
// <cmd>"`. It works for emulators with no cwd flag whose -e forwards a real
// argv. exec replaces sh so the command owns the pty and signals directly.
func genericShellLaunch(cwd string, argv []string) []string {
	return []string{"-e", "sh", "-c", "cd " + shellQuote(cwd) + " && exec " + shellJoin(argv)}
}

// familyFor returns the launch family for an emulator binary name, or nil if it
// is not one Jeera knows how to drive natively.
func familyFor(bin string) *termFamily {
	for i := range termFamilies {
		for _, n := range termFamilies[i].names {
			if strings.EqualFold(n, bin) {
				return &termFamilies[i]
			}
		}
	}
	return nil
}

// resolveLauncher picks a terminal emulator and the args to launch argv in cwd
// as a new window. Precedence: an explicit $TERMINAL, then macOS Terminal (via
// osascript), then the freedesktop xdg-terminal-exec, then a probe of known
// emulators, then the Debian x-terminal-emulator wrapper, else ErrNoTerminal.
func resolveLauncher(cwd string, argv []string) (bin string, launch []string, err error) {
	// 0. Inside a terminal multiplexer: open a new window/pane there. This is the
	// only "new terminal" that works over SSH and on headless hosts, so it wins.
	if bin, launch, ok := muxLauncher(cwd, argv); ok && pathOK(bin) {
		return bin, launch, nil
	}
	// 1. Honour an explicit $TERMINAL: the user's chosen emulator wins. Match it
	// to a known family for correct flags, else use the generic form. Any extra
	// tokens the user put in $TERMINAL (e.g. "alacritty --class jeera") are kept
	// as launch flags, ahead of the command separator.
	if fields := strings.Fields(getenv("TERMINAL")); len(fields) > 0 {
		if name, extra := fields[0], fields[1:]; pathOK(name) {
			tail := genericShellLaunch(cwd, argv)
			if fam := familyFor(name); fam != nil {
				tail = fam.build(cwd, argv)
			}
			return name, append(append([]string{}, extra...), tail...), nil
		}
	}
	// 2. macOS has no $TERMINAL convention and no Linux emulators: drive
	// Terminal.app through osascript, which is always present.
	if currentOS == "darwin" {
		return "osascript", osascriptLaunch(cwd, argv), nil
	}
	// 3. xdg-terminal-exec abstracts each emulator's -e/-- quirk — the most
	// portable path when it is installed.
	if pathOK("xdg-terminal-exec") {
		return "xdg-terminal-exec", append([]string{"--dir=" + cwd, "--"}, argv...), nil
	}
	// 4. Probe concrete emulators in preference order, using each one's own flags.
	for i := range termFamilies {
		for _, n := range termFamilies[i].names {
			if pathOK(n) {
				return n, termFamilies[i].build(cwd, argv), nil
			}
		}
	}
	// 5. Last resort: the Debian alternatives wrapper. Its -e target is unknown,
	// so use the generic shell form as a best effort.
	if pathOK("x-terminal-emulator") {
		return "x-terminal-emulator", genericShellLaunch(cwd, argv), nil
	}
	return "", nil, ErrNoTerminal
}

func pathOK(bin string) bool { _, err := lookPath(bin); return err == nil }

// osascriptLaunch drives Terminal.app to open a new window running argv in cwd.
// `do script` runs its string as /bin/sh and has no cwd parameter, so the
// directory is embedded as `cd … && exec …`. There are two escaping layers: the
// inner string is shell-quoted for /bin/sh, then AppleScript-escaped for the
// double-quoted `do script` literal.
func osascriptLaunch(cwd string, argv []string) []string {
	inner := "cd " + shellQuote(cwd) + " && exec " + shellJoin(argv)
	script := `tell application "Terminal" to do script "` + appleScriptEscape(inner) + `"`
	return []string{"-e", script}
}

// openInTerminal opens inner (a provider command with its working directory set)
// in a new terminal window, detached from Jeera, and returns the emulator it
// used. The window is its own process group so the TUI's signals never reach it,
// and its stdio defaults to the null device so it cannot corrupt the board.
func openInTerminal(inner *exec.Cmd) (string, error) {
	bin, launch, err := resolveLauncher(inner.Dir, inner.Args)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, launch...)
	cmd.Dir = inner.Dir
	cmd.SysProcAttr = detachedSysProcAttr()
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("launch %s: %w", bin, err)
	}
	go func() { _ = cmd.Wait() }() // reap the launcher; the window outlives it
	return bin, nil
}

// commandLine renders inner as a paste-ready shell one-liner — a cd into its
// working directory followed by the shell-quoted argv. When Jeera has no
// terminal to open (a bare SSH/headless shell with no multiplexer and no GUI),
// it hands the user this exact command to run themselves rather than ever taking
// over the board.
func commandLine(inner *exec.Cmd) string {
	line := shellJoin(inner.Args)
	if inner.Dir != "" {
		line = "cd " + shellQuote(inner.Dir) + " && " + line
	}
	return line
}

// launchOutcome is the result of hosting a provider command without ever running
// it inline. Exactly one situation is described:
//   - Err != nil:            a terminal existed but the launch failed.
//   - Copy != "":            no terminal here, so Copy is a paste-ready command
//     for the user to run; Msg explains that it was copied.
//   - otherwise:             opened in a real terminal; Msg names which one.
type launchOutcome struct {
	Msg  string
	Copy string
	Err  error
}

// launchInTerminalOrCopy opens inner in a new terminal window/pane (detached,
// never inline). When no terminal can be reached it returns a paste-ready
// command in Copy instead of running anything inline — the caller copies it to
// the clipboard and tells the user. `what` is a short verb phrase for the
// success message, e.g. "resuming 93049003".
func launchInTerminalOrCopy(inner *exec.Cmd, what string) launchOutcome {
	switch bin, err := openInTerminal(inner); {
	case err == nil:
		return launchOutcome{Msg: fmt.Sprintf("%s in %s", what, bin)}
	case !errors.Is(err, ErrNoTerminal):
		return launchOutcome{Err: err} // a terminal exists but failed to launch
	default:
		return launchOutcome{Msg: "no terminal to open — command copied, paste it to run", Copy: commandLine(inner)}
	}
}

// shellQuote wraps s in single quotes, escaping any embedded single quote, so it
// survives /bin/sh word-splitting intact.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellJoin shell-quotes each arg and joins them with spaces.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return strings.Join(quoted, " ")
}

// appleScriptEscape escapes a string for an AppleScript double-quoted literal.
func appleScriptEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
