package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// fileMention powers the inline "@" file picker inside the description editor.
// Typing "@" at a word boundary opens a floating dropdown that fuzzy-finds files
// in the project's git repo; accepting one splices a Markdown link into the
// description. The dropdown is a pure projection of the textarea's buffer and
// caret, so the textarea stays the single source of truth.
type fileMention struct {
	load    mentionLoad
	files   []string // repo-relative paths, loaded once per edit session
	active  bool
	query   string   // text between the triggering "@" and the caret
	matches []string // files ranked against query (best first)
	sel     int      // highlighted row in matches
}

// mentionLoad tracks the async load of the repo's file list.
type mentionLoad int

const (
	mentionIdle    mentionLoad = iota // nothing requested yet
	mentionPending                    // git enumeration in flight
	mentionReady                      // files loaded, picker armed
	mentionFailed                     // no repo / git error — "@" stays literal
)

// mentionMaxRows is the most file rows the dropdown shows at once; the rest
// scroll under the selection.
const mentionMaxRows = 6

// repoFilesLoadedMsg delivers the repo's file list (or an error) back to the
// detail view that requested it.
type repoFilesLoadedMsg struct {
	issueID int64
	files   []string
	err     error
}

// repoFileLister is the seam the detail view uses to enumerate repo files. It is
// a package var so tests can inject a fixed list instead of shelling out to git.
var repoFileLister = listRepoFiles

// listRepoFiles returns every tracked-or-untracked-but-not-ignored file in the
// git repo at repoDir, as repo-relative paths. It mirrors the plain exec.Command
// style used in internal/worktree. NUL separation (-z) keeps odd filenames
// (spaces, newlines) intact.
func listRepoFiles(repoDir string) ([]string, error) {
	if strings.TrimSpace(repoDir) == "" {
		return nil, fmt.Errorf("no repo path")
	}
	cmd := exec.Command("git", "-C", repoDir, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(out), "\x00")
	files := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		files = append(files, p)
	}
	return files, nil
}

// loadRepoFilesCmd enumerates repoDir's files off the update loop and reports the
// result as a repoFilesLoadedMsg tagged with the issue it was requested for. When
// repoDir has no usable git repo (empty, missing, or not a repo) it falls back to
// the directory Jeera was launched from — usually the repo the user is in — so the
// picker still works for projects created without a valid repo path.
func loadRepoFilesCmd(repoDir string, issueID int64) tea.Cmd {
	return func() tea.Msg {
		if files, err := repoFileLister(repoDir); err == nil {
			return repoFilesLoadedMsg{issueID: issueID, files: files}
		}
		if wd, err := os.Getwd(); err == nil && wd != repoDir {
			if files, err := repoFileLister(wd); err == nil {
				return repoFilesLoadedMsg{issueID: issueID, files: files}
			}
		}
		return repoFilesLoadedMsg{issueID: issueID, err: fmt.Errorf("no git repo for %q", repoDir)}
	}
}

// activeMention finds the "@" token the caret is sitting in on a single logical
// line. It returns the rune index of the "@", the query text between it and the
// caret (col), and whether a token is active. A token is active only when the
// "@" is at line start or follows whitespace and nothing between it and the
// caret is whitespace — so "a@b.com" never triggers, but "see @det" does.
func activeMention(line []rune, col int) (at int, query string, ok bool) {
	if col > len(line) {
		col = len(line)
	}
	if col < 0 {
		col = 0
	}
	for i := col - 1; i >= 0; i-- {
		switch {
		case line[i] == '@':
			if i == 0 || unicode.IsSpace(line[i-1]) {
				return i, string(line[i+1 : col]), true
			}
			return 0, "", false // "@" glued to a word — not a trigger
		case unicode.IsSpace(line[i]):
			return 0, "", false // whitespace before any "@"
		}
	}
	return 0, "", false
}

// rankFiles returns files matching query, best first. An empty query returns
// every file alphabetically. Otherwise it keeps files whose path contains query
// as a case-insensitive subsequence, ranked by a basename-weighted score, then
// by shorter path, then alphabetically for stability.
func rankFiles(files []string, query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		out := append([]string(nil), files...)
		sort.Strings(out)
		return out
	}
	q := strings.ToLower(query)
	type scored struct {
		path  string
		score int
	}
	matched := make([]scored, 0, len(files))
	for _, f := range files {
		if s, ok := subseqScore(f, q); ok {
			matched = append(matched, scored{f, s})
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score > matched[j].score
		}
		if len(matched[i].path) != len(matched[j].path) {
			return len(matched[i].path) < len(matched[j].path)
		}
		return matched[i].path < matched[j].path
	})
	out := make([]string, len(matched))
	for i, m := range matched {
		out[i] = m.path
	}
	return out
}

// subseqScore scores a case-insensitive subsequence match of q (already
// lowercased) against path, rewarding matches in the basename, at segment
// boundaries, and in contiguous runs. It returns false when q is not a
// subsequence of path at all.
func subseqScore(path, q string) (int, bool) {
	runes := []rune(strings.ToLower(path))
	baseStart := 0
	for i, r := range runes {
		if r == '/' {
			baseStart = i + 1
		}
	}
	qr := []rune(q)
	score, qi, prev := 0, 0, -2
	for i := 0; i < len(runes) && qi < len(qr); i++ {
		if runes[i] != qr[qi] {
			continue
		}
		score++
		if i >= baseStart {
			score += 3 // hits in the filename matter most
		}
		if i == baseStart {
			score += 5 // right at the start of the filename
		}
		if i == prev+1 {
			score += 4 // contiguous with the previous hit
		}
		if i > 0 && isBoundary(runes[i-1]) {
			score += 2 // start of a path segment / word
		}
		prev = i
		qi++
	}
	if qi < len(qr) {
		return 0, false
	}
	return score, true
}

func isBoundary(r rune) bool {
	return r == '/' || r == '_' || r == '-' || r == '.' || r == ' '
}

// markdownLink renders a repo-relative path as a Markdown link whose text is the
// filename and whose target is the full path. Paths containing spaces or parens
// use the CommonMark angle-bracket target form so the link stays valid.
func markdownLink(path string) string {
	target := path
	if strings.ContainsAny(path, " ()") {
		target = "<" + path + ">"
	}
	return "[" + filepath.Base(path) + "](" + target + ")"
}

// move shifts the selection by delta, clamped to the match list.
func (fm *fileMention) move(delta int) {
	if len(fm.matches) == 0 {
		return
	}
	fm.sel += delta
	if fm.sel < 0 {
		fm.sel = 0
	}
	if fm.sel >= len(fm.matches) {
		fm.sel = len(fm.matches) - 1
	}
}

// view renders the dropdown box at the given interior content width. Every span
// carries the overlay background so the box is fully opaque when composited over
// the panels beneath it.
func (fm fileMention) view(t theme.Theme, innerW int) string {
	if innerW < 16 {
		innerW = 16
	}
	bg := t.P.BgOverlay
	paint := func(st lipgloss.Style) lipgloss.Style { return st.Background(bg) }
	fill := func(n int) string {
		if n <= 0 {
			return ""
		}
		return lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", n))
	}
	line := func(s string) string { return s + fill(innerW-lipgloss.Width(s)) }

	// Header: the live query on the left, a match count on the right.
	q := fm.query
	if q == "" {
		q = paint(t.HelpDesc).Render("type to filter…")
	} else {
		q = paint(t.StatusText).Render(truncate(q, innerW-18))
	}
	head := paint(t.HelpKey).Render(iconSearch+" ") + q
	count := paint(t.HelpDesc).Render(fmt.Sprintf("%d of %d", len(fm.matches), len(fm.files)))
	gap := innerW - lipgloss.Width(head) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	rows := []string{head + fill(gap) + count}

	switch {
	case fm.load == mentionPending:
		rows = append(rows, line(paint(t.HelpDesc).Render("  Loading files…")))
	case len(fm.matches) == 0:
		rows = append(rows, line(paint(t.HelpDesc).Render("  No files match")))
	default:
		start, end := scrollWindow(fm.sel, len(fm.matches), mentionMaxRows)
		nameW := 0
		for i := start; i < end; i++ {
			if w := lipgloss.Width(filepath.Base(fm.matches[i])); w > nameW {
				nameW = w
			}
		}
		if nameW > innerW-12 {
			nameW = innerW - 12
		}
		for i := start; i < end; i++ {
			rows = append(rows, line(fm.row(t, paint, fill, nameW, innerW, i)))
		}
	}
	rows = append(rows, line(paint(t.HelpDesc).Render("↑↓ move · ↵ insert · esc dismiss")))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.P.FocusGlow).
		BorderBackground(bg).
		Background(bg).
		Padding(0, 1).
		Render(strings.Join(rows, "\n"))
}

// row renders one file row: a caret + filename, then its directory, following
// the single-choice list idiom from picker.go.
func (fm fileMention) row(t theme.Theme, paint func(lipgloss.Style) lipgloss.Style, fill func(int) string, nameW, innerW, i int) string {
	p := fm.matches[i]
	base := truncate(filepath.Base(p), nameW)
	dir := filepath.Dir(p)
	if dir == "." {
		dir = ""
	}

	caret := fill(2)
	nameStyle := paint(t.StatusText)
	if i == fm.sel {
		caret = paint(t.HelpKey).Render("▸ ")
		nameStyle = paint(t.CardTitle)
	}
	cell := caret + nameStyle.Render(base) + fill(nameW-lipgloss.Width(base))
	if dir != "" {
		dirW := innerW - lipgloss.Width(cell) - 2
		if dirW > 0 {
			cell += "  " + paint(t.CardMeta).Render(truncate(dir, dirW))
		}
	}
	return cell
}
