package tui

import (
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

func TestDetailTabSwitching(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	if d.tab != tabOverview {
		t.Fatalf("detail should open on Overview, got %d", d.tab)
	}
	for _, want := range []detailTab{tabAgent, tabRelations, tabFiles, tabActivity, tabOverview} {
		d.updateViewing(keyPress2("tab"))
		if d.tab != want {
			t.Errorf("tab → %d, want %d", d.tab, want)
		}
	}
	d.updateViewing(keyPress2("shift+tab"))
	if d.tab != tabActivity {
		t.Errorf("shift+tab from Overview should wrap to Activity, got %d", d.tab)
	}
}

func TestDetailTabSwitchResetsFieldCursor(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.field = dfTags // the last Overview field
	d.updateViewing(keyPress2("tab"))
	if d.tab != tabAgent {
		t.Fatalf("expected the Agent tab, got %d", d.tab)
	}
	if d.field != dfProvider {
		t.Errorf("switching to Agent should land on its first field (Provider), got %d", d.field)
	}
}

func TestDetailFieldNavStaysWithinTab(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	fields := tabOverview.fields()
	d.field = fields[0]
	for range fields { // a full lap
		d.updateViewing(keyPress2("j"))
	}
	if d.field != fields[0] {
		t.Errorf("j a full lap should wrap to the first field, got %d", d.field)
	}
	d.updateViewing(keyPress2("j"))
	if idxOf(fields, d.field) < 0 {
		t.Errorf("the field cursor left the Overview field set: %d", d.field)
	}
}

func TestDetailActionsGatedByTab(t *testing.T) {
	// e edits the description — Overview only.
	over, _, _ := newDetailForTest(t)
	over.tab = tabOverview
	over.updateViewing(keyPress2("e"))
	if over.mode != dEditDesc {
		t.Error("e on Overview should start editing the description")
	}
	agent, _, _ := newDetailForTest(t)
	agent.tab = tabAgent
	agent.updateViewing(keyPress2("e"))
	if agent.mode == dEditDesc {
		t.Error("e on Agent should do nothing")
	}

	// c comments — Activity only.
	act, _, _ := newDetailForTest(t)
	act.tab = tabActivity
	act.updateViewing(keyPress2("c"))
	if act.mode != dInput || act.inputKind != ikComment {
		t.Error("c on Activity should open the comment input")
	}
	notAct, _, _ := newDetailForTest(t)
	notAct.tab = tabOverview
	notAct.updateViewing(keyPress2("c"))
	if notAct.mode == dInput {
		t.Error("c on Overview should do nothing")
	}

	// A attaches — Files only.
	files, _, _ := newDetailForTest(t)
	files.tab = tabFiles
	files.updateViewing(keyPress2("A"))
	if files.mode != dInput || files.inputKind != ikAttach {
		t.Error("A on Files should open the attach input")
	}
}

func TestDetailFilesCursor(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.inputKind = ikAttach
	d.submitInput("https://a.example/1")
	d.inputKind = ikAttach
	d.submitInput("https://b.example/2")
	d.reload()
	d.tab = tabFiles
	d.attachSel = 0

	d.updateViewing(keyPress2("j"))
	if d.attachSel != 1 {
		t.Errorf("j on Files should move the attachment cursor, got %d", d.attachSel)
	}
	d.updateViewing(keyPress2("j")) // wrap past the end (2 attachments)
	if d.attachSel != 0 {
		t.Errorf("j past the end should wrap, got %d", d.attachSel)
	}
}

func TestGoldenDetailAgent(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	iss.Assignee = core.Assignee{Provider: core.ProviderClaude, Model: "opus", Effort: core.EffortHigh}
	st.UpdateIssue(iss)
	// Runs without StartedAt keep the golden timezone-independent.
	st.CreateRun(core.Run{IssueID: iss.ID, Version: 1, Provider: core.ProviderClaude, Model: "opus", Status: core.RunSucceeded})
	st.CreateRun(core.Run{IssueID: iss.ID, Version: 2, Provider: core.ProviderClaude, Status: core.RunRunning})
	d.reload()
	d.tab = tabAgent
	d.field = dfProvider
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
	d.tab = tabRelations
	goldenFile(t, "detail_relations", stripANSI(d.View()))
}

func TestGoldenDetailFiles(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.inputKind = ikAttach
	d.submitInput("https://example.com/spec")
	d.inputKind = ikAttach
	d.submitInput("/tmp/jeera-diagram.png")
	d.reload()
	d.tab = tabFiles
	goldenFile(t, "detail_files", stripANSI(d.View()))
}

func TestGoldenDetailActivity(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	st.AddComment(core.Comment{IssueID: iss.ID, Body: "Kicked off the work."})
	st.AddComment(core.Comment{IssueID: iss.ID, Author: "claude", Body: "Moved it to In Progress."})
	d.reload()
	d.tab = tabActivity
	goldenFile(t, "detail_activity", stripANSI(d.View()))
}
