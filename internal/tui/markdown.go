package tui

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"charm.land/glamour/v2"
)

// renderMarkdown renders Markdown to styled terminal text wrapped to width,
// using a fixed dark style so output is deterministic (and golden-testable). On
// any error it falls back to the raw text rather than failing the view. When
// repoRoot is set, relative file links (e.g. those inserted by the "@" picker)
// are rewritten to absolute file:// hyperlinks so they're actually clickable.
func renderMarkdown(md string, width int, repoRoot string) string {
	if strings.TrimSpace(md) == "" {
		md = "_No description yet. Press e to add one._"
	}
	if width < 10 {
		width = 10
	}
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width))
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return rewriteFileHyperlinks(strings.TrimRight(out, "\n"), repoRoot)
}

// osc8Re matches an OSC 8 hyperlink-open sequence: ESC ] 8 ; params ; uri (BEL|ST).
// Glamour emits these for every Markdown link; the visible link text follows and
// is left untouched.
var osc8Re = regexp.MustCompile("\x1b]8;([^;]*);([^\x07\x1b]*)(?:\x07|\x1b\\\\)")

// rewriteFileHyperlinks rewrites the target of every OSC 8 hyperlink that points
// at a real, repo-relative file into an absolute file:// URI, so the rendered
// link opens the actual file when clicked. Links that aren't relative repo files
// (web URLs, anchors, missing paths) are left exactly as glamour rendered them,
// as is the visible link text.
func rewriteFileHyperlinks(s, repoRoot string) string {
	if repoRoot == "" || !strings.Contains(s, "\x1b]8;") {
		return s
	}
	return osc8Re.ReplaceAllStringFunc(s, func(m string) string {
		g := osc8Re.FindStringSubmatch(m)
		uri, ok := repoFileURI(repoRoot, g[2])
		if !ok {
			return m
		}
		return "\x1b]8;" + g[1] + ";" + uri + "\x07"
	})
}

// repoFileURI resolves a hyperlink target against repoRoot and, when it names an
// existing file in the repo, returns its absolute file:// URI. It returns ok ==
// false for web URLs, absolute paths, anchors and anything that isn't a regular
// file, leaving those links unchanged.
func repoFileURI(repoRoot, target string) (string, bool) {
	if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "#") {
		return "", false
	}
	rel, err := url.PathUnescape(target)
	if err != nil {
		rel = target
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if fi, err := os.Stat(abs); err != nil || fi.IsDir() {
		return "", false
	}
	return fileURI(abs), true
}

// fileURI returns the file:// URI for an absolute path.
func fileURI(abs string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

// osc8 wraps text in an OSC 8 terminal hyperlink pointing at uri, so the terminal
// makes it clickable — the same mechanism glamour uses for description links. An
// empty uri returns text unchanged.
func osc8(uri, text string) string {
	if uri == "" {
		return text
	}
	return "\x1b]8;;" + uri + "\x07" + text + "\x1b]8;;\x07"
}
