package tui

import (
	"strings"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// TestDetailHLOnNoFieldTabsDoesNotMutate guards the gating that keeps h/l from
// cycling a (stale) field while on a tab that shows no fields.
func TestDetailHLOnNoFieldTabsDoesNotMutate(t *testing.T) {
	for _, tab := range []detailTab{tabRelations, tabFiles, tabActivity} {
		d, st, iss := newDetailForTest(t)
		d.tab = tab
		d.field = dfStatus // a leftover field value from another tab
		before, _ := st.GetIssue(iss.ID)

		d.updateViewing(keyPress2("l"))
		d.updateViewing(keyPress2("h"))

		after, _ := st.GetIssue(iss.ID)
		if after.StatusID != before.StatusID || after.Priority != before.Priority {
			t.Errorf("h/l on tab %d must not cycle or persist a field", tab)
		}
	}
}

func TestDetailAgentActionsGated(t *testing.T) {
	// s starts a run only on Agent. With no run manager, startRun records an error —
	// a clean observable proxy for "the action fired".
	onAgent, _, _ := newDetailForTest(t)
	onAgent.tab = tabAgent
	onAgent.updateViewing(keyPress2("s"))
	if onAgent.err == "" {
		t.Error("s on Agent should attempt to start a run")
	}
	onOver, _, _ := newDetailForTest(t)
	onOver.tab = tabOverview
	onOver.updateViewing(keyPress2("s"))
	if onOver.err != "" {
		t.Errorf("s on Overview should do nothing, got err %q", onOver.err)
	}

	// S opens the cron input only on Agent.
	sAgent, _, _ := newDetailForTest(t)
	sAgent.tab = tabAgent
	sAgent.updateViewing(keyPress2("S"))
	if sAgent.mode != dInput || sAgent.inputKind != ikCron {
		t.Error("S on Agent should open the schedule input")
	}
	sFiles, _, _ := newDetailForTest(t)
	sFiles.tab = tabFiles
	sFiles.updateViewing(keyPress2("S"))
	if sFiles.mode == dInput {
		t.Error("S on Files should not open the schedule input")
	}

	// w toggles the worktree override only on Agent.
	wAgent, st, iss := newDetailForTest(t)
	wAgent.tab = tabAgent
	wAgent.updateViewing(keyPress2("w"))
	if got, _ := st.GetIssue(iss.ID); got.WorktreeOn == nil {
		t.Error("w on Agent should set the worktree override")
	}
}

func TestDetailAgentFieldNav(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.tab = tabAgent
	d.field = dfProvider
	for _, want := range []detailField{dfModel, dfEffort, dfProvider} { // wraps after Effort
		d.updateViewing(keyPress2("j"))
		if d.field != want {
			t.Errorf("j on Agent → %d, want %d", d.field, want)
		}
	}
}

func TestDetailSwitchTabResetsOverviewField(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.tab = tabAgent
	d.field = dfEffort
	for d.tab != tabOverview { // tab forward all the way around
		d.updateViewing(keyPress2("tab"))
	}
	if d.field != dfStatus {
		t.Errorf("returning to Overview should reset the field cursor to Status, got %d", d.field)
	}
}

func TestDetailSelectedAttachment(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.inputKind = ikAttach
	d.submitInput("https://a.example/1")
	d.inputKind = ikAttach
	d.submitInput("https://b.example/2")
	d.reload()

	// Newest first: [b, a]. The cursor at index 1 selects the older one — proving
	// open acts on the selection, not always the first row.
	d.attachSel = 1
	a, ok := d.selectedAttachment()
	if !ok || a.Path != "https://a.example/1" {
		t.Errorf("selectedAttachment should return the cursor's attachment, got %q (ok=%v)", a.Path, ok)
	}
}

func TestDetailRelationsShowsParent(t *testing.T) {
	d, st, iss := newDetailForTest(t)
	parent, _ := st.CreateIssue(core.Issue{ProjectID: iss.ProjectID, Title: "Parent epic", Type: core.TypeEpic})
	iss.ParentID = &parent.ID
	st.UpdateIssue(iss)
	d.reload()
	d.tab = tabRelations

	out := stripANSI(d.View())
	if !strings.Contains(out, parent.Key) {
		t.Errorf("the Relations tab should show the parent %s", parent.Key)
	}
}

func TestDetailNarrowWidthKeepsFooter(t *testing.T) {
	d, _, _ := newDetailForTest(t)
	d.setSize(40, 24) // narrow enough that the tab strip wraps to multiple rows
	out := stripANSI(d.View())

	if !strings.Contains(out, "tabs") { // the "tab tabs" footer hint
		t.Error("the footer keybar must stay on screen at narrow widths")
	}
	if lines := strings.Count(out, "\n") + 1; lines > 24 {
		t.Errorf("view rendered %d lines, exceeding the terminal height 24", lines)
	}
}
