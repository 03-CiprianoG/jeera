package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestOpenCommandIncludesRef(t *testing.T) {
	cmd := openCommand("https://example.com/spec")
	if !strings.Contains(strings.Join(cmd.Args, " "), "https://example.com/spec") {
		t.Errorf("open command should include the ref: %v", cmd.Args)
	}
}

func TestOpenCommandBinaryPerOS(t *testing.T) {
	cmd := openCommand("/x/y.png")
	want := map[string]string{"darwin": "open", "windows": "rundll32"}[runtime.GOOS]
	if want == "" {
		want = "xdg-open"
	}
	if cmd.Args[0] != want {
		t.Errorf("opener on %s = %q, want %q", runtime.GOOS, cmd.Args[0], want)
	}
}

func TestSafeToOpenAllowsHTTPS(t *testing.T) {
	if err := safeToOpen("https://example.com/a"); err != nil {
		t.Errorf("https URL should be openable: %v", err)
	}
	if err := safeToOpen("http://example.com"); err != nil {
		t.Errorf("http URL should be openable: %v", err)
	}
}

func TestSafeToOpenAllowsExistingFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := safeToOpen(f); err != nil {
		t.Errorf("an existing regular file should be openable: %v", err)
	}
}

func TestSafeToOpenRejectsDangerous(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"smb://attacker/share",
		"vbscript:Execute",
		`\\attacker\share\x`,
		"/does/not/exist/anywhere.png",
		"", // empty
	}
	for _, ref := range cases {
		if err := safeToOpen(ref); err == nil {
			t.Errorf("safeToOpen(%q) should be refused", ref)
		}
	}
}

func TestSafeToOpenRejectsDirectory(t *testing.T) {
	if err := safeToOpen(t.TempDir()); err == nil {
		t.Error("a directory should not be openable")
	}
}

// --- external terminal launcher -------------------------------------------------

// fakeEnv replaces the launcher's indirections for one test: which binaries
// resolve on PATH, the GOOS, and the environment (TERMINAL/TMUX/…). It restores
// them on cleanup so tests don't bleed into each other.
func fakeEnv(t *testing.T, goos string, env map[string]string, onPath ...string) {
	t.Helper()
	origLook, origOS, origEnv := lookPath, currentOS, getenv
	t.Cleanup(func() { lookPath, currentOS, getenv = origLook, origOS, origEnv })
	avail := map[string]bool{}
	for _, b := range onPath {
		avail[b] = true
	}
	lookPath = func(bin string) (string, error) {
		if avail[bin] {
			return "/usr/bin/" + bin, nil
		}
		return "", errors.New("not found")
	}
	currentOS = goos
	getenv = func(k string) string { return env[k] }
}

// fakeTerminals is the common case of fakeEnv: just a $TERMINAL value (no
// multiplexer in the environment).
func fakeTerminals(t *testing.T, goos, termEnv string, onPath ...string) {
	t.Helper()
	fakeEnv(t, goos, map[string]string{"TERMINAL": termEnv}, onPath...)
}

func TestTerminalFamilyArgv(t *testing.T) {
	cmd := []string{"claude", "--resume", "id-9"}
	cases := map[string][]string{
		"gnome-terminal": {"--working-directory=/w", "--", "claude", "--resume", "id-9"},
		"konsole":        {"--separate", "--workdir", "/w", "-e", "claude", "--resume", "id-9"},
		"wezterm":        {"start", "--always-new-process", "--cwd", "/w", "--", "claude", "--resume", "id-9"},
		"alacritty":      {"--working-directory", "/w", "-e", "claude", "--resume", "id-9"},
		"kitty":          {"--directory", "/w", "claude", "--resume", "id-9"},
		"foot":           {"--working-directory=/w", "claude", "--resume", "id-9"},
		"xterm":          {"-e", "sh", "-c", "cd '/w' && exec 'claude' '--resume' 'id-9'"},
	}
	for bin, want := range cases {
		fam := familyFor(bin)
		if fam == nil {
			t.Errorf("%s: no family", bin)
			continue
		}
		if got := fam.build("/w", cmd); !slices.Equal(got, want) {
			t.Errorf("%s argv:\n got  %v\n want %v", bin, got, want)
		}
	}
}

func TestMultiplexerLaunch(t *testing.T) {
	cmd := []string{"claude", "--resume", "id-9"}
	cases := []struct {
		name, envKey, bin string
		want              []string
	}{
		{"tmux", "TMUX", "tmux", []string{"new-window", "-c", "/w", "--", "claude", "--resume", "id-9"}},
		{"zellij", "ZELLIJ", "zellij", []string{"run", "--cwd", "/w", "--", "claude", "--resume", "id-9"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeEnv(t, "linux", map[string]string{c.envKey: "1"}, c.bin)
			bin, argv, err := resolveLauncher("/w", cmd)
			if err != nil || bin != c.bin || !slices.Equal(argv, c.want) {
				t.Fatalf("got %q %v err=%v, want %q %v", bin, argv, err, c.bin, c.want)
			}
		})
	}
}

func TestMultiplexerScreenWrapsShell(t *testing.T) {
	fakeEnv(t, "linux", map[string]string{"STY": "123.pts-0"}, "screen")
	bin, argv, err := resolveLauncher("/w", []string{"claude", "--resume", "id"})
	if err != nil || bin != "screen" {
		t.Fatalf("got %q err=%v, want screen", bin, err)
	}
	if argv[0] != "-X" || argv[len(argv)-1] != `cd '/w' && exec 'claude' '--resume' 'id'` {
		t.Errorf("screen should cd via a wrapped shell: %v", argv)
	}
}

func TestMultiplexerBeatsGUI(t *testing.T) {
	// Inside tmux *and* with a GUI emulator available, the multiplexer wins: it
	// works over SSH, where the GUI window cannot.
	fakeEnv(t, "linux", map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0", "TERMINAL": "alacritty"}, "tmux", "alacritty")
	bin, _, err := resolveLauncher("/w", []string{"claude", "--resume", "id"})
	if err != nil || bin != "tmux" {
		t.Fatalf("got %q err=%v, want tmux to win over the GUI emulator", bin, err)
	}
}

func TestResolveLauncherPrecedence(t *testing.T) {
	cmd := []string{"claude", "--resume", "id-9"}

	t.Run("TERMINAL known emulator wins", func(t *testing.T) {
		fakeTerminals(t, "linux", "alacritty", "alacritty", "kitty")
		bin, argv, err := resolveLauncher("/w", cmd)
		if err != nil || bin != "alacritty" || !slices.Contains(argv, "--working-directory") {
			t.Fatalf("got %q %v err=%v, want alacritty with its native flags", bin, argv, err)
		}
	})

	t.Run("TERMINAL unknown emulator uses generic form", func(t *testing.T) {
		fakeTerminals(t, "linux", "myterm", "myterm")
		bin, argv, err := resolveLauncher("/w", cmd)
		if err != nil || bin != "myterm" || !slices.Equal(argv, genericShellLaunch("/w", cmd)) {
			t.Fatalf("got %q %v err=%v, want myterm with the generic shell form", bin, argv, err)
		}
	})

	t.Run("TERMINAL not on PATH falls through to probe", func(t *testing.T) {
		fakeTerminals(t, "linux", "ghost-term", "kitty") // $TERMINAL set but not installed
		bin, _, err := resolveLauncher("/w", cmd)
		if err != nil || bin != "kitty" {
			t.Fatalf("got %q err=%v, want fall-through to kitty", bin, err)
		}
	})

	t.Run("macOS uses osascript", func(t *testing.T) {
		fakeTerminals(t, "darwin", "") // no emulators on PATH; osascript is implicit
		bin, argv, err := resolveLauncher("/w", cmd)
		if err != nil || bin != "osascript" || !strings.Contains(strings.Join(argv, " "), "do script") {
			t.Fatalf("got %q %v err=%v, want osascript do-script", bin, argv, err)
		}
	})

	t.Run("xdg-terminal-exec preferred over raw probe", func(t *testing.T) {
		fakeTerminals(t, "linux", "", "xdg-terminal-exec", "xterm")
		bin, argv, err := resolveLauncher("/w", cmd)
		if err != nil || bin != "xdg-terminal-exec" || argv[0] != "--dir=/w" {
			t.Fatalf("got %q %v err=%v, want xdg-terminal-exec", bin, argv, err)
		}
	})

	t.Run("probe order prefers native-cwd emulator", func(t *testing.T) {
		fakeTerminals(t, "linux", "", "xterm", "kitty") // both present
		bin, _, err := resolveLauncher("/w", cmd)
		if err != nil || bin != "kitty" {
			t.Fatalf("got %q err=%v, want kitty (native cwd) before xterm", bin, err)
		}
	})

	t.Run("x-terminal-emulator is the last resort", func(t *testing.T) {
		fakeTerminals(t, "linux", "", "x-terminal-emulator")
		bin, argv, err := resolveLauncher("/w", cmd)
		if err != nil || bin != "x-terminal-emulator" || !slices.Equal(argv, genericShellLaunch("/w", cmd)) {
			t.Fatalf("got %q %v err=%v, want x-terminal-emulator generic form", bin, argv, err)
		}
	})

	t.Run("nothing found is a typed error", func(t *testing.T) {
		fakeTerminals(t, "linux", "")
		if _, _, err := resolveLauncher("/w", cmd); !errors.Is(err, ErrNoTerminal) {
			t.Fatalf("err = %v, want ErrNoTerminal", err)
		}
	})
}

func TestResolveLauncherKeepsTerminalArgs(t *testing.T) {
	fakeTerminals(t, "linux", "alacritty --class jeera", "alacritty")
	bin, argv, err := resolveLauncher("/w", []string{"claude", "--resume", "id"})
	if err != nil || bin != "alacritty" {
		t.Fatalf("got %q err=%v, want alacritty", bin, err)
	}
	// The user's extra flags survive, ahead of alacritty's own.
	if !slices.Equal(argv[:3], []string{"--class", "jeera", "--working-directory"}) {
		t.Errorf("extra $TERMINAL args not preserved: %v", argv)
	}
}

func TestOpenInTerminalNoTerminal(t *testing.T) {
	fakeTerminals(t, "linux", "") // empty PATH, no $TERMINAL
	inner := exec.Command("claude", "--resume", "id")
	inner.Dir = "/w"
	if _, err := openInTerminal(inner); !errors.Is(err, ErrNoTerminal) {
		t.Errorf("openInTerminal with no emulator = %v, want ErrNoTerminal", err)
	}
}

func TestOpenInTerminalSpawns(t *testing.T) {
	// Point $TERMINAL at `true`: a real binary that ignores its args and exits 0,
	// so the detached-spawn path runs end to end without opening a window.
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("`true` not available")
	}
	fakeTerminals(t, "linux", "true", "true")
	inner := exec.Command("claude", "--resume", "id")
	inner.Dir = t.TempDir()
	bin, err := openInTerminal(inner)
	if err != nil || bin != "true" {
		t.Fatalf("openInTerminal = %q, %v; want it to spawn `true`", bin, err)
	}
}

func TestCommandLineEmbedsDir(t *testing.T) {
	inner := exec.Command("claude", "--resume", "x y")
	inner.Dir = "/a b"
	if got := commandLine(inner); got != `cd '/a b' && 'claude' '--resume' 'x y'` {
		t.Errorf("commandLine = %q", got)
	}
}

func TestLaunchInTerminalOrCopyCopiesWhenNoTerminal(t *testing.T) {
	fakeEnv(t, "linux", map[string]string{}) // no multiplexer, no $TERMINAL, nothing on PATH
	inner := exec.Command("claude", "--resume", "id-9")
	inner.Dir = "/work dir"
	out := launchInTerminalOrCopy(inner, "resuming id-9")
	if out.Err != nil {
		t.Fatalf("no terminal must not error (it copies instead), got %v", out.Err)
	}
	if out.Copy == "" {
		t.Fatal("no terminal must yield a paste-ready command, got none")
	}
	// The copied command cds into the run's dir and runs the verbatim argv.
	if !strings.Contains(out.Copy, `cd '/work dir' && 'claude' '--resume' 'id-9'`) {
		t.Errorf("copied command missing cd+argv: %q", out.Copy)
	}
}

func TestLaunchInTerminalOrCopyOpensWhenTerminal(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("`true` not available to stand in for a terminal")
	}
	fakeTerminals(t, "linux", "true", "true")
	inner := exec.Command("claude", "--resume", "id")
	inner.Dir = t.TempDir()
	out := launchInTerminalOrCopy(inner, "resuming id")
	if out.Err != nil || out.Copy != "" {
		t.Fatalf("a terminal is available, it must open not copy: %+v", out)
	}
	if !strings.Contains(out.Msg, "in true") {
		t.Errorf("success message should name the terminal: %q", out.Msg)
	}
}

func TestRunsWindowKeepsCursorVisible(t *testing.T) {
	cases := []struct {
		cursor, n, visible, wantStart, wantEnd int
	}{
		{0, 2, 30, 0, 2},    // everything fits
		{0, 50, 5, 0, 5},    // top of a long list
		{4, 50, 5, 0, 5},    // last row of the first window
		{5, 50, 5, 1, 6},    // scrolled by one
		{49, 50, 5, 45, 50}, // bottom of a long list
	}
	for _, c := range cases {
		start, end := runsWindow(c.cursor, c.n, c.visible)
		if start != c.wantStart || end != c.wantEnd {
			t.Errorf("runsWindow(%d,%d,%d) = (%d,%d), want (%d,%d)",
				c.cursor, c.n, c.visible, start, end, c.wantStart, c.wantEnd)
		}
		if start < 0 || end > c.n || c.cursor < start || c.cursor >= end {
			t.Errorf("runsWindow(%d,%d,%d) = (%d,%d) does not contain the cursor in range",
				c.cursor, c.n, c.visible, start, end)
		}
	}
}

func TestDetachedSysProcAttrIsolatesGroup(t *testing.T) {
	// On unix the launched window must be in its own process group so the TUI's
	// Ctrl-C/Ctrl-Z never reaches it; elsewhere a nil attr is fine.
	got := detachedSysProcAttr()
	if runtime.GOOS == "windows" || runtime.GOOS == "js" {
		if got != nil {
			t.Errorf("non-unix detachedSysProcAttr = %+v, want nil", got)
		}
		return
	}
	if got == nil || !got.Setpgid {
		t.Errorf("unix detachedSysProcAttr = %+v, want Setpgid", got)
	}
}

func TestShellQuoteAndJoin(t *testing.T) {
	if got := shellQuote(`/a b/c`); got != `'/a b/c'` {
		t.Errorf("shellQuote spaces = %q", got)
	}
	// A single quote inside is closed, escaped and reopened.
	if got := shellQuote(`it's`); got != `'it'\''s'` {
		t.Errorf("shellQuote apostrophe = %q", got)
	}
	if got := shellJoin([]string{"claude", "--resume", "a b"}); got != `'claude' '--resume' 'a b'` {
		t.Errorf("shellJoin = %q", got)
	}
}

func TestAppleScriptEscape(t *testing.T) {
	if got := appleScriptEscape(`say "hi"\bye`); got != `say \"hi\"\\bye` {
		t.Errorf("appleScriptEscape = %q", got)
	}
}

func TestOsascriptLaunchEmbedsDir(t *testing.T) {
	argv := osascriptLaunch("/repo path", []string{"claude", "--resume", "x"})
	joined := strings.Join(argv, " ")
	if argv[0] != "-e" || !strings.Contains(joined, "do script") {
		t.Fatalf("osascript argv malformed: %v", argv)
	}
	// The cwd is embedded (shell-quoted) since `do script` has no cwd parameter.
	if !strings.Contains(joined, `cd '/repo path'`) {
		t.Errorf("osascript should embed a cd into the working dir: %v", argv)
	}
}
