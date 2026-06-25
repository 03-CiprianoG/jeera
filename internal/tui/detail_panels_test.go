package tui

import (
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// --- focus model -------------------------------------------------------------

func TestDetailPanelCycling(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	if d.focus != panelDescription {
		t.Fatalf("detail should open focused on Description, got %d", d.focus)
	}
	for _, want := range []detailPanel{panelProperties, panelAgent, panelRelations, panelActivity, panelDescription} {
		d.updateViewing(keyPress2("tab"))
		if d.focus != want {
			t.Errorf("tab → panel %d, want %d", d.focus, want)
		}
	}
	d.updateViewing(keyPress2("shift+tab"))
	if d.focus != panelActivity {
		t.Errorf("shift+tab from Description should wrap to Activity, got %d", d.focus)
	}
}

func TestDetailFocusLandsCursor(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.field = dfProvider // not a Properties field
	d.focus = panelDescription
	d.updateViewing(keyPress2("tab")) // → Properties
	if d.focus != panelProperties {
		t.Fatalf("expected Properties, got %d", d.focus)
	}
	if idxOf(propertyFields(), d.field) < 0 {
		t.Errorf("focusing Properties should land on a property field, got %d", d.field)
	}

	d.agentSel = 99                   // out of range
	d.updateViewing(keyPress2("tab")) // → Agent
	if d.agentSel != 0 || d.field != dfProvider {
		t.Errorf("focusing Agent should reset the cursor and sync the field, got sel=%d field=%d", d.agentSel, d.field)
	}
}

// --- Properties --------------------------------------------------------------

func TestDetailPropertiesNavAndCycle(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	d.focus = panelProperties
	d.field = dfStatus

	d.updateViewing(keyPress2("j"))
	if d.field != dfType {
		t.Errorf("j should move to the next property (Type), got %d", d.field)
	}
	d.updateViewing(keyPress2("k"))
	if d.field != dfStatus {
		t.Errorf("k should move back to Status, got %d", d.field)
	}
	d.updateViewing(keyPress2("l")) // cycle Status forward → transition
	if got, _ := st.GetIssue(iss.ID); got.StatusID != d.statuses[1].ID {
		t.Errorf("→ on Status should transition to the second column")
	}
}

func TestDetailPropertiesEnterEditsPoints(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.focus = panelProperties
	d.field = dfPoints
	d.updateViewing(keyPress2("enter"))
	if d.mode != dInput || d.inputKind != ikPoints {
		t.Error("enter on Points should open the points input")
	}
}

// --- Agent -------------------------------------------------------------------

func TestDetailAgentRunButton(t *testing.T) {
	d, _, _ := newDetailForTest(t) // no run manager
	d.focus = panelAgent
	d.agentSel = agRun
	d.updateViewing(keyPress2("enter"))
	if d.err == "" {
		t.Error("enter on the Run button (no manager) should surface an error")
	}
}

func TestDetailAgentScheduleButton(t *testing.T) {
	d, _, _ := detailWithScheduler(t)
	d.focus = panelAgent
	d.agentSel = agSchedule
	d.updateViewing(keyPress2("enter"))
	if d.mode != dInput || d.inputKind != ikCron {
		t.Error("enter on the Schedule button should open the cron input")
	}
}

func TestDetailAgentWorktreeToggle(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	d.focus = panelAgent
	d.agentSel = agWorktree
	d.updateViewing(keyPress2("right"))
	if got, _ := st.GetIssue(iss.ID); got.WorktreeOn == nil {
		t.Error("→ on Worktree should set the override")
	}
}

func TestDetailAgentFieldNavSyncs(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.focus = panelAgent
	d.agentSel = agProvider
	d.updateViewing(keyPress2("right")) // cycle provider → sets an assignee
	if d.issue.Assignee.IsZero() {
		t.Fatal("cycling Provider should set an assignee")
	}
	d.updateViewing(keyPress2("j")) // → Model row
	if d.agentSel != agModel || d.field != dfModel {
		t.Errorf("down should move to Model and sync the field, got sel=%d field=%d", d.agentSel, d.field)
	}
}

// --- Relations & Files -------------------------------------------------------

func TestDetailRelationsAttachButton(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.inputKind = ikAttach
	d.submitInput("https://a.example/1")
	d.reload()
	d.focus = panelRelations
	d.attachSel = 0

	d.updateViewing(keyPress2("j")) // → the "+ Attach" slot (index 1 = len)
	if d.attachSel != 1 {
		t.Fatalf("down should reach the attach slot, got %d", d.attachSel)
	}
	d.updateViewing(keyPress2("enter"))
	if d.mode != dInput || d.inputKind != ikAttach {
		t.Error("enter on the attach slot should open the attach input")
	}
}

func TestDetailSelectedAttachment(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.inputKind = ikAttach
	d.submitInput("https://a.example/1")
	d.inputKind = ikAttach
	d.submitInput("https://b.example/2")
	d.reload()
	// Newest first: [b, a]. The cursor at index 1 selects the older one.
	d.attachSel = 1
	a, ok := d.selectedAttachment()
	if !ok || a.Path != "https://a.example/1" {
		t.Errorf("selectedAttachment should follow the cursor, got %q (ok=%v)", a.Path, ok)
	}
}

func TestDetailRelationsShowsParent(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	parent, _ := st.CreateIssue(core.Issue{ProjectID: iss.ProjectID, Title: "Parent epic", Type: core.TypeEpic})
	iss.ParentID = &parent.ID
	st.UpdateIssue(iss)
	d.reload()
	d.focus = panelRelations
	if out := stripANSI(d.View()); !strings.Contains(out, parent.Key) {
		t.Errorf("Relations panel should show the parent %s", parent.Key)
	}
}

// --- Activity & Description --------------------------------------------------

func TestDetailActivityComment(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.focus = panelActivity
	d.updateViewing(keyPress2("enter"))
	if d.mode != dInput || d.inputKind != ikComment {
		t.Error("enter on the Activity panel should open the comment input")
	}
}

func TestDetailDescriptionEdit(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.focus = panelDescription
	d.updateViewing(keyPress2("enter"))
	if d.mode != dEditDesc {
		t.Error("enter on the Description panel should start editing")
	}
}

// --- layout ------------------------------------------------------------------

func TestDetailNarrowSingleColumnFits(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.setSize(56, 28) // below the two-column threshold
	out := stripANSI(d.View())
	for _, line := range strings.Split(out, "\n") {
		if w := lineWidth(line); w > 56 {
			t.Fatalf("narrow detail overflows width 56: a line is %d cells", w)
		}
	}
	if lines := strings.Count(out, "\n") + 1; lines > 28 {
		t.Errorf("detail rendered %d lines, exceeding height 28", lines)
	}
	if !strings.Contains(out, "esc back") {
		t.Error("the footer should stay on screen at narrow widths")
	}
}

func TestDetailWideUsesFullWidth(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.setSize(120, 40)
	out := stripANSI(d.View())
	for _, line := range strings.Split(out, "\n") {
		if w := lineWidth(line); w != 0 && w != 120 {
			t.Fatalf("wide detail line is %d cells, want full width 120: %q", w, line)
		}
	}
}

// --- goldens (the full bento, focused on different panels) -------------------

func TestGoldenDetailAgent(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	iss.Assignee = core.Assignee{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortHigh}
	st.UpdateIssue(iss)
	// Runs without StartedAt keep the golden timezone-independent.
	st.CreateRun(core.Run{IssueID: iss.ID, Version: 1, Provider: core.ProviderClaude, Model: "opus", Status: core.RunSucceeded})
	st.CreateRun(core.Run{IssueID: iss.ID, Version: 2, Provider: core.ProviderClaude, Status: core.RunRunning})
	d.reload()
	d.focus = panelAgent
	d.agentSel = agRun
	goldenFile(t, "detail_agent", stripANSI(d.View()))
}

func TestGoldenDetailRelations(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	epic, _ := st.CreateIssue(core.Issue{ProjectID: iss.ProjectID, Title: "Board epic", Type: core.TypeEpic})
	iss.EpicID = &epic.ID
	st.UpdateIssue(iss)
	st.CreateIssue(core.Issue{ProjectID: iss.ProjectID, Title: "Render the columns", Type: core.TypeTask, ParentID: &iss.ID})
	other, _ := st.CreateIssue(core.Issue{ProjectID: iss.ProjectID, Title: "Theme tokens", Type: core.TypeTask})
	st.CreateLink(core.IssueLink{SourceID: iss.ID, TargetID: other.ID, Type: core.LinkBlocks})
	d.reload()
	d.focus = panelRelations
	goldenFile(t, "detail_relations", stripANSI(d.View()))
}

func TestGoldenDetailFiles(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.inputKind = ikAttach
	d.submitInput("https://example.com/spec")
	d.inputKind = ikAttach
	d.submitInput("/tmp/jeera-diagram.png")
	d.reload()
	d.focus = panelRelations
	d.attachSel = 0
	goldenFile(t, "detail_files", stripANSI(d.View()))
}

func TestGoldenDetailActivity(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	st.AddComment(core.Comment{IssueID: iss.ID, Body: "Kicked off the work."})
	st.AddComment(core.Comment{IssueID: iss.ID, Author: "claude", Body: "Moved it to In Progress."})
	d.reload()
	d.focus = panelActivity
	goldenFile(t, "detail_activity", stripANSI(d.View()))
}
