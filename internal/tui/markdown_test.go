package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRepoFileURI(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a/b.go"), "x")

	uri, ok := repoFileURI(root, "a/b.go")
	if !ok || !strings.HasPrefix(uri, "file://") || !strings.HasSuffix(uri, "/a/b.go") {
		t.Errorf("existing file should resolve to a file URI; uri=%q ok=%v", uri, ok)
	}

	for _, target := range []string{"a/missing.go", "https://example.com", "/etc/passwd", "#frag", "", "a"} {
		if _, ok := repoFileURI(root, target); ok {
			t.Errorf("%q should not resolve to a repo file URI", target)
		}
	}
}

func TestRewriteFileHyperlinks(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/foo.go"), "x")
	abs := filepath.Join(root, "internal/foo.go")

	// A repo-relative OSC 8 link gets an absolute file:// target; visible text stays.
	in := "\x1b]8;id=1;internal/foo.go\x07foo.go\x1b]8;;\x07"
	out := rewriteFileHyperlinks(in, root)
	if !strings.Contains(out, "\x1b]8;id=1;file://"+abs+"\x07") {
		t.Errorf("expected absolute file:// target in OSC 8; got %q", out)
	}
	if !strings.Contains(out, "\x07foo.go\x1b]8;;\x07") {
		t.Errorf("visible link text should be unchanged; got %q", out)
	}

	// Web links and missing files are left exactly as-is.
	for _, in := range []string{
		"\x1b]8;id=2;https://example.com\x07site\x1b]8;;\x07",
		"\x1b]8;id=3;internal/missing.go\x07x\x1b]8;;\x07",
	} {
		if got := rewriteFileHyperlinks(in, root); got != in {
			t.Errorf("link should be untouched:\n in=%q\nout=%q", in, got)
		}
	}

	// No repoRoot → no rewriting at all.
	if got := rewriteFileHyperlinks(in, ""); got != in {
		t.Errorf("empty repoRoot should be a no-op; got %q", got)
	}
}

func TestRenderMarkdownLinkifiesRepoFile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "internal/mcp/tools_test.go"), "x")
	abs := filepath.Join(root, "internal/mcp/tools_test.go")
	md := "See [tools_test.go](internal/mcp/tools_test.go) for details."

	out := renderMarkdown(md, 80, root)
	if !strings.Contains(out, "file://"+abs) {
		t.Errorf("rendered link should resolve to the absolute file; got %q", out)
	}
	if !strings.Contains(out, "tools_test.go") {
		t.Errorf("link text should still render; got %q", out)
	}

	// Without a repo root the link stays relative (unchanged behaviour).
	if strings.Contains(renderMarkdown(md, 80, ""), "file://") {
		t.Error("with no repoRoot the link should not be rewritten to file://")
	}
}
