package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// pickerItem is one choice in a picker overlay.
type pickerItem struct {
	id    int64
	label string
	meta  string
}

// pickerModel is a modal single-choice list: highlight an item, press enter, and
// onPick fires with its id. It backs "assign issue to sprint" and "add issue to
// sprint" — the two planning flows that need the user to choose one thing.
type pickerModel struct {
	title  string
	items  []pickerItem
	empty  string
	sel    int
	onPick func(id int64) tea.Cmd
}

func (p *pickerModel) clamp() {
	if p.sel >= len(p.items) {
		p.sel = len(p.items) - 1
	}
	if p.sel < 0 {
		p.sel = 0
	}
}

// updatePicker drives the picker overlay, returning to the active view on esc or
// after a choice is made.
func (m Model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.picker == nil {
		m.mode = modeNormal
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Up):
		m.picker.sel--
		m.picker.clamp()
	case key.Matches(msg, m.keys.Down):
		m.picker.sel++
		m.picker.clamp()
	case key.Matches(msg, m.keys.Enter):
		var cmd tea.Cmd
		if m.picker.sel >= 0 && m.picker.sel < len(m.picker.items) && m.picker.onPick != nil {
			cmd = m.picker.onPick(m.picker.items[m.picker.sel].id)
		}
		m.picker = nil
		m.mode = modeNormal
		m.reload()
		return m, cmd
	default:
		if msg.String() == "esc" {
			m.picker = nil
			m.mode = modeNormal
		}
	}
	return m, nil
}

func (p *pickerModel) View(t theme.Theme) string {
	if len(p.items) == 0 {
		return modalShell(t, modalWidthList, 3, p.title, "", t.HelpDesc.Render(p.empty), modalHint(t, "esc close"))
	}
	// The inner width is the modal width less its border (2) and padding (4).
	// Truncate each label to what's left after the cursor lead and the meta chip,
	// so every item stays on one line and the cursor indent and meta stay aligned.
	const inner = modalWidthList - 6
	var b strings.Builder
	for i, it := range p.items {
		cursor := "  "
		labelStyle := t.StatusText
		if i == p.sel {
			cursor = t.HelpKey.Render("▸ ")
			labelStyle = t.CardTitle
		}
		metaW := 0
		if it.meta != "" {
			metaW = lipgloss.Width(it.meta) + 2
		}
		labelW := inner - 2 - metaW
		if labelW < 8 {
			labelW = 8
		}
		row := cursor + labelStyle.Render(truncate(it.label, labelW))
		if it.meta != "" {
			row += "  " + t.CardMeta.Render(it.meta)
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(row)
	}
	return modalShell(t, modalWidthList, 5, p.title, "", b.String(), modalHint(t, "↑/↓ select · enter choose · esc close"))
}

// openSprintPicker opens a chooser of the project's sprints; choosing one assigns
// the given issue to it. Used from the Backlog and to move an issue between sprints.
func (m Model) openSprintPicker(iss core.Issue) (tea.Model, tea.Cmd) {
	sprints, err := m.store.ListSprints(m.active.ID)
	if err != nil {
		return m, reportErr(err)
	}
	items := make([]pickerItem, 0, len(sprints))
	for _, sp := range sprints {
		if iss.SprintID != nil && *iss.SprintID == sp.ID {
			continue // it's already here; offer the others
		}
		items = append(items, pickerItem{id: sp.ID, label: sp.Name, meta: string(sp.State)})
	}
	st := m.store
	m.picker = &pickerModel{
		title: "Assign " + iss.Key + " to sprint",
		items: items,
		empty: "No other sprints yet — close and add one in Sprints.",
		onPick: func(sprintID int64) tea.Cmd {
			id := sprintID
			if err := st.AddIssueToSprint(iss.ID, &id); err != nil {
				return reportErr(err)
			}
			return toast("assigned " + iss.Key)
		},
	}
	m.mode = modePicker
	return m, nil
}

// openBacklogPicker opens a chooser of the project's unsprinted issues; choosing
// one adds it to the given sprint. Used from the Sprints view.
func (m Model) openBacklogPicker(sprintID int64, sprintName string) (tea.Model, tea.Cmd) {
	issues, err := m.store.ListIssues(issueFilterUnsprinted(m.active.ID))
	if err != nil {
		return m, reportErr(err)
	}
	items := make([]pickerItem, 0, len(issues))
	for _, iss := range issues {
		items = append(items, pickerItem{id: iss.ID, label: iss.Key + "  " + iss.Title, meta: string(iss.Type)})
	}
	st := m.store
	m.picker = &pickerModel{
		title: "Add an issue to " + sprintName,
		items: items,
		empty: "The backlog is empty — nothing to add.",
		onPick: func(issueID int64) tea.Cmd {
			id := sprintID
			if err := st.AddIssueToSprint(issueID, &id); err != nil {
				return reportErr(err)
			}
			return toast("added to " + sprintName)
		},
	}
	m.mode = modePicker
	return m, nil
}
