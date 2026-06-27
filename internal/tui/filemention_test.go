package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// --- token detection ---------------------------------------------------------

func TestActiveMention(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		col    int
		wantAt int
		wantQ  string
		wantOK bool
	}{
		{"bare @ at start", "@", 1, 0, "", true},
		{"after space", "see @det", 8, 4, "det", true},
		{"glued to word is not a trigger", "a@b", 3, 0, "", false},
		{"caret after a trailing space closes", "@det ", 5, 0, "", false},
		{"full token", "@det", 4, 0, "det", true},
		{"slashes stay in the query", "foo @a/b", 8, 4, "a/b", true},
		{"caret mid-token shrinks query", "@de", 2, 0, "d", true},
		{"@ after leading spaces", "  @", 3, 2, "", true},
		{"second @ glued to a word", "x @a@b", 6, 0, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, q, ok := activeMention([]rune(c.line), c.col)
			if at != c.wantAt || q != c.wantQ || ok != c.wantOK {
				t.Errorf("activeMention(%q,%d) = (%d,%q,%v), want (%d,%q,%v)",
					c.line, c.col, at, q, ok, c.wantAt, c.wantQ, c.wantOK)
			}
		})
	}
}

// --- ranking -----------------------------------------------------------------

func mentionFixture() []string {
	return []string{
		"internal/tui/detail.go",
		"internal/tui/detail_view.go",
		"internal/tui/detail_fields.go",
		"internal/tui/board.go",
		"internal/core/types.go",
		"README.md",
		"main.go",
	}
}

func TestRankFilesEmptyQueryIsAlphabetical(t *testing.T) {
	got := rankFiles(mentionFixture(), "")
	if len(got) != len(mentionFixture()) {
		t.Fatalf("empty query should return all %d files, got %d", len(mentionFixture()), len(got))
	}
	if got[0] != "README.md" {
		t.Errorf("empty query should sort alphabetically; got[0]=%q want README.md", got[0])
	}
}

func TestRankFilesPrefersShorterOnTie(t *testing.T) {
	got := rankFiles(mentionFixture(), "det")
	if len(got) != 3 {
		t.Fatalf("query %q should match 3 files, got %d: %v", "det", len(got), got)
	}
	if got[0] != "internal/tui/detail.go" {
		t.Errorf("shortest equally-scored path should rank first; got[0]=%q", got[0])
	}
}

func TestRankFilesSubsequence(t *testing.T) {
	got := rankFiles(mentionFixture(), "dv")
	want := []string{"internal/tui/detail_view.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("subsequence %q = %v, want %v", "dv", got, want)
	}
}

func TestRankFilesNoMatch(t *testing.T) {
	if got := rankFiles(mentionFixture(), "zzz"); len(got) != 0 {
		t.Errorf("no-match query should return empty, got %v", got)
	}
}

// --- link formatting ---------------------------------------------------------

func TestMarkdownLink(t *testing.T) {
	cases := map[string]string{
		"internal/tui/detail.go": "[detail.go](internal/tui/detail.go)",
		"README.md":              "[README.md](README.md)",
		"a b/c (x).go":           "[c (x).go](<a b/c (x).go>)", // spaces+parens use angle-bracket target
	}
	for path, want := range cases {
		if got := markdownLink(path); got != want {
			t.Errorf("markdownLink(%q) = %q, want %q", path, got, want)
		}
	}
}

// --- git enumeration ---------------------------------------------------------

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func TestListRepoFiles(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "x")
	write("sub/b.go", "y")
	write("build.log", "noise")
	write(".gitignore", "*.log\n")
	runGit(t, dir, "add", "a.go") // a.go tracked; sub/b.go + .gitignore untracked; build.log ignored

	files, err := listRepoFiles(dir)
	if err != nil {
		t.Fatalf("listRepoFiles: %v", err)
	}
	has := func(p string) bool {
		for _, f := range files {
			if f == p {
				return true
			}
		}
		return false
	}
	if !has("a.go") || !has("sub/b.go") || !has(".gitignore") {
		t.Errorf("expected tracked + untracked-non-ignored files, got %v", files)
	}
	if has("build.log") {
		t.Errorf("gitignored file should be excluded, got %v", files)
	}
}

func TestListRepoFilesErrors(t *testing.T) {
	if _, err := listRepoFiles(""); err == nil {
		t.Error("empty repo path should error")
	}
	if _, err := listRepoFiles(t.TempDir()); err == nil {
		t.Error("a non-repo directory should error")
	}
}

func TestLoadRepoFilesFallsBackToCwd(t *testing.T) {
	old := repoFileLister
	t.Cleanup(func() { repoFileLister = old })
	// The project's path fails to enumerate, but the working directory succeeds.
	repoFileLister = func(dir string) ([]string, error) {
		if dir == "/no/such/repo" || dir == "" {
			return nil, errors.New("not a repo")
		}
		return []string{"main.go"}, nil
	}
	msg, ok := loadRepoFilesCmd("/no/such/repo", 9)().(repoFilesLoadedMsg)
	if !ok {
		t.Fatal("expected a repoFilesLoadedMsg")
	}
	if msg.err != nil {
		t.Fatalf("expected fallback to succeed, got err %v", msg.err)
	}
	if len(msg.files) != 1 || msg.files[0] != "main.go" {
		t.Fatalf("expected fallback files from cwd, got %v", msg.files)
	}
}

// --- behaviour (driving the detail editor) -----------------------------------

func setupEditing(t *testing.T, files []string) *detailModel {
	t.Helper()
	d, _, _ := newDetailForTest(t)
	d.startEditDesc()
	// Simulate the async repo load completing with a fixed list.
	d.mention = fileMention{load: mentionReady, files: files}
	return d
}

func typeString(d *detailModel, s string) {
	for _, r := range s {
		d.updateEditDesc(keyPress(string(r)))
	}
}

func TestMentionOpensAndFilters(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	typeString(d, "The layout in @det")
	if !d.mention.active {
		t.Fatal("typing @det should open the picker")
	}
	if len(d.mention.matches) != 3 {
		t.Fatalf("want 3 matches for @det, got %d: %v", len(d.mention.matches), d.mention.matches)
	}
	if !strings.Contains(stripANSI(d.View()), "detail.go") {
		t.Error("dropdown should list detail.go")
	}
}

func TestMentionAcceptInsertsLink(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	typeString(d, "see @detail_v")
	d.updateEditDesc(keyPress("enter"))
	want := "see [detail_view.go](internal/tui/detail_view.go)"
	if got := d.desc.Value(); got != want {
		t.Fatalf("after accept buffer = %q, want %q", got, want)
	}
	if d.mention.active {
		t.Error("picker should close after accepting")
	}
}

func TestMentionAcceptWithTab(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	typeString(d, "@dv")
	d.updateEditDesc(keyPress("tab"))
	if got := d.desc.Value(); got != "[detail_view.go](internal/tui/detail_view.go)" {
		t.Fatalf("tab should accept; buffer = %q", got)
	}
}

func TestMentionNavigationClamps(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	typeString(d, "@det")
	if d.mention.sel != 0 {
		t.Fatalf("selection should start at 0, got %d", d.mention.sel)
	}
	d.updateEditDesc(keyPress("down"))
	if d.mention.sel != 1 {
		t.Errorf("down should move to 1, got %d", d.mention.sel)
	}
	d.updateEditDesc(keyPress("up"))
	d.updateEditDesc(keyPress("up")) // clamp at top
	if d.mention.sel != 0 {
		t.Errorf("up should clamp at 0, got %d", d.mention.sel)
	}
}

func TestMentionEscClosesThenExits(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	typeString(d, "@det")
	d.updateEditDesc(keyPress("esc"))
	if d.mention.active {
		t.Error("esc should close the dropdown")
	}
	if d.mode != dEditDesc {
		t.Error("esc on an open dropdown must not exit edit mode")
	}
	d.updateEditDesc(keyPress("esc"))
	if d.mode != dViewing {
		t.Error("a second esc should exit edit mode")
	}
}

func TestMentionClosesOnSpaceAndBackspace(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	typeString(d, "@det")
	typeString(d, " ") // a space ends the token
	if d.mention.active {
		t.Error("a space should close the dropdown")
	}
	typeString(d, "@d")
	if !d.mention.active {
		t.Fatal("a fresh @ should reopen the dropdown")
	}
	d.updateEditDesc(keyPress("backspace")) // delete 'd'
	d.updateEditDesc(keyPress("backspace")) // delete '@'
	if d.mention.active {
		t.Error("backspacing past @ should close the dropdown")
	}
}

func TestMentionLiteralWithoutRepo(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.startEditDesc()
	d.mention.load = mentionFailed // no repo / git failed
	typeString(d, "@det")
	if d.mention.active {
		t.Error("with no repo, @ should stay literal text")
	}
	if got := d.desc.Value(); got != "@det" {
		t.Errorf("buffer should be literal %q, got %q", "@det", got)
	}
}

func TestMentionFooterHint(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	if got := stripANSI(d.View()); !strings.Contains(got, "link file") {
		t.Error("editor footer should advertise the @ picker")
	}
	typeString(d, "@det")
	if got := stripANSI(d.View()); !strings.Contains(got, "insert") {
		t.Error("an open picker should show its own controls in the footer")
	}
}

func TestMentionAcceptAlsoAttaches(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "internal/mcp/tools_test.go"), "hello")

	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := st.CreateProject(core.Project{Name: "J", KeyPrefix: "JEE", RepoPath: repo})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "X", Type: core.TypeStory})

	d := newDetail(st, nil, nil, theme.New(), iss.ID, 100, 30)
	d.startEditDesc()
	d.mention = fileMention{load: mentionReady, files: []string{"internal/mcp/tools_test.go"}}

	typeString(d, "@tools")
	d.updateEditDesc(keyPress("enter"))

	if !strings.Contains(d.desc.Value(), "[tools_test.go](internal/mcp/tools_test.go)") {
		t.Fatalf("link not inserted: %q", d.desc.Value())
	}
	atts, _ := st.ListAttachments(iss.ID)
	if len(atts) != 1 {
		t.Fatalf("accepting a file should create one attachment, got %d", len(atts))
	}
	abs := filepath.Join(repo, "internal/mcp/tools_test.go")
	if atts[0].Path != abs {
		t.Errorf("attachment path = %q, want absolute %q", atts[0].Path, abs)
	}
	if atts[0].Filename != "tools_test.go" {
		t.Errorf("attachment filename = %q, want tools_test.go", atts[0].Filename)
	}
	if atts[0].Size != int64(len("hello")) {
		t.Errorf("attachment size = %d, want %d", atts[0].Size, len("hello"))
	}
	if len(d.attachments) != 1 {
		t.Errorf("Relations & Files panel not refreshed: %d attachments", len(d.attachments))
	}
}

func TestMentionAcceptDedupsAttachment(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "a.go"), "x")

	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := st.CreateProject(core.Project{Name: "J", KeyPrefix: "JEE", RepoPath: repo})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "X", Type: core.TypeStory})

	d := newDetail(st, nil, nil, theme.New(), iss.ID, 100, 30)
	d.startEditDesc()
	d.mention = fileMention{load: mentionReady, files: []string{"a.go"}}

	typeString(d, "@a")
	d.updateEditDesc(keyPress("enter"))
	typeString(d, " and again @a")
	d.updateEditDesc(keyPress("enter"))

	if atts, _ := st.ListAttachments(iss.ID); len(atts) != 1 {
		t.Fatalf("mentioning the same file twice should attach it once, got %d", len(atts))
	}
}

func TestAttachmentURI(t *testing.T) {
	if got := attachmentURI(core.Attachment{Path: "/abs/foo.go"}); got != "file:///abs/foo.go" {
		t.Errorf("file attachment uri = %q, want file:///abs/foo.go", got)
	}
	urlAtt := core.Attachment{Path: "https://example.com", MIME: core.AttachmentURIMIME}
	if got := attachmentURI(urlAtt); got != "https://example.com" {
		t.Errorf("url attachment uri = %q, want the URL itself", got)
	}
	if got := attachmentURI(core.Attachment{}); got != "" {
		t.Errorf("pathless attachment should have no uri, got %q", got)
	}
}

func TestAttachmentRowIsClickable(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "internal/mcp/tools_test.go"), "hello")

	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := st.CreateProject(core.Project{Name: "J", KeyPrefix: "JEE", RepoPath: repo})
	iss, _ := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "X", Type: core.TypeStory})

	d := newDetail(st, nil, nil, theme.New(), iss.ID, 100, 30)
	d.startEditDesc()
	d.mention = fileMention{load: mentionReady, files: []string{"internal/mcp/tools_test.go"}}
	typeString(d, "@tools")
	d.updateEditDesc(keyPress("enter"))

	abs := filepath.Join(repo, "internal/mcp/tools_test.go")
	raw := d.View() // unstripped: assert the OSC 8 hyperlink wraps the row
	if !strings.Contains(raw, "\x1b]8;;file://"+abs+"\x07") {
		t.Errorf("attachment row should be an OSC 8 hyperlink to file://%s", abs)
	}
	if !strings.Contains(stripANSI(raw), "tools_test.go") {
		t.Error("attachment filename should still be visible")
	}
}

// --- golden ------------------------------------------------------------------

func detailRender(d *detailModel) string {
	return strings.TrimRight(stripANSI(d.View()), "\n ")
}

func TestGoldenDetailMention(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	typeString(d, "The bento layout in @det")
	if !d.mention.active {
		t.Fatal("expected the picker open")
	}
	goldenFile(t, "detail_mention", detailRender(d))
}

func TestGoldenDetailMentionEmpty(t *testing.T) {
	d := setupEditing(t, mentionFixture())
	typeString(d, "@zzz")
	if !d.mention.active {
		t.Fatal("expected the picker open even with no matches")
	}
	goldenFile(t, "detail_mention_empty", detailRender(d))
}
