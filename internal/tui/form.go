package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

type formKind int

const (
	formCreateIssue formKind = iota
	formCreateProject
	formRename
	formCreateSprint
)

// formModel is a small modal form. It is held by pointer on the root model so
// its embedded text inputs keep their cursor/focus state across updates.
type formModel struct {
	kind    formKind
	heading string
	labels  []string
	fields  []textinput.Model
	focus   int
	issueID int64 // target for formRename
}

func newField(placeholder string, width, limit int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = limit
	ti.SetWidth(width)
	return ti
}

func newCreateIssueForm() *formModel {
	return &formModel{
		kind:    formCreateIssue,
		heading: "New issue",
		labels:  []string{"Title"},
		fields:  []textinput.Model{newField("What needs doing?", 46, 200)},
	}
}

func newCreateSprintForm() *formModel {
	return &formModel{
		kind:    formCreateSprint,
		heading: "New sprint",
		labels:  []string{"Name", "Goal"},
		fields: []textinput.Model{
			newField("Sprint name", 46, 80),
			newField("Goal (optional)", 46, 200),
		},
	}
}

func newCreateProjectForm() *formModel {
	// Pre-fill the repo path with the current directory — the common case is
	// "point Jeera at the repo I'm in" — but let the user edit it.
	repo := newField("/path/to/repo", 46, 300)
	if cwd, err := os.Getwd(); err == nil {
		repo.SetValue(cwd)
	}
	return &formModel{
		kind:    formCreateProject,
		heading: "New project",
		labels:  []string{"Name", "Key prefix", "Repo path"},
		fields: []textinput.Model{
			newField("Project name", 46, 80),
			newField("KEY, e.g. JEE", 46, 10),
			repo,
		},
	}
}

func newRenameForm(iss core.Issue) *formModel {
	f := newField("Title", 46, 200)
	f.SetValue(iss.Title)
	return &formModel{
		kind:    formRename,
		heading: "Rename " + iss.Key,
		labels:  []string{"Title"},
		fields:  []textinput.Model{f},
		issueID: iss.ID,
	}
}

// focusCmd focuses the first field.
func (f *formModel) focusCmd() tea.Cmd {
	f.focus = 0
	return f.fields[0].Focus()
}

func (f *formModel) focusNext() tea.Cmd { return f.moveFocus(1) }
func (f *formModel) focusPrev() tea.Cmd { return f.moveFocus(-1) }

func (f *formModel) moveFocus(d int) tea.Cmd {
	f.fields[f.focus].Blur()
	f.focus = (f.focus + d + len(f.fields)) % len(f.fields)
	return f.fields[f.focus].Focus()
}

// update routes a message to the focused field.
func (f *formModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.fields[f.focus], cmd = f.fields[f.focus].Update(msg)
	return cmd
}

func (f *formModel) values() []string {
	v := make([]string, len(f.fields))
	for i := range f.fields {
		v[i] = strings.TrimSpace(f.fields[i].Value())
	}
	return v
}

func (f *formModel) View(t theme.Theme) string {
	var b strings.Builder
	b.WriteString(t.Title.Render(f.heading))
	b.WriteString("\n\n")
	for i, fld := range f.fields {
		b.WriteString(t.Label.Render(f.labels[i]))
		b.WriteString("\n")
		b.WriteString(fld.View())
		b.WriteString("\n\n")
	}
	b.WriteString(t.HelpDesc.Render("enter submit · tab next · esc cancel"))
	return t.Modal.Width(54).Render(b.String())
}

// updateForm handles keys while a form is open.
func (m Model) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.form = nil
		return m, nil
	case "enter":
		return m.submitForm()
	case "tab", "down":
		return m, m.form.focusNext()
	case "shift+tab", "up":
		return m, m.form.focusPrev()
	}
	return m, m.form.update(msg)
}

// submitForm applies the form against the store. On error it keeps the form
// open and surfaces the message; on success it closes and selects the result.
func (m Model) submitForm() (tea.Model, tea.Cmd) {
	if m.form == nil {
		m.mode = modeNormal
		return m, nil
	}
	vals := m.form.values()

	switch m.form.kind {
	case formCreateProject:
		repo := vals[2]
		if repo == "" {
			repo, _ = os.Getwd()
		}
		p, err := m.store.CreateProject(core.Project{Name: vals[0], KeyPrefix: vals[1], RepoPath: repo})
		if err != nil {
			return m, reportErr(err)
		}
		m.active = p
		m.closeForm()
		return m, toast("created project " + p.KeyPrefix)

	case formCreateIssue:
		if vals[0] == "" {
			return m, reportErr(fmt.Errorf("title is required"))
		}
		iss, err := m.store.CreateIssue(core.Issue{ProjectID: m.active.ID, Title: vals[0], Type: core.TypeTask})
		if err != nil {
			return m, reportErr(err)
		}
		m.closeForm()
		m.selectIssueByID(iss.ID)
		return m, toast("created " + iss.Key)

	case formCreateSprint:
		if vals[0] == "" {
			return m, reportErr(fmt.Errorf("sprint name is required"))
		}
		if _, err := m.store.CreateSprint(core.Sprint{ProjectID: m.active.ID, Name: vals[0], Goal: vals[1]}); err != nil {
			return m, reportErr(err)
		}
		m.closeForm()
		return m, toast("created sprint " + vals[0])

	case formRename:
		iss, err := m.store.GetIssue(m.form.issueID)
		if err != nil {
			return m, reportErr(err)
		}
		if vals[0] == "" {
			return m, reportErr(fmt.Errorf("title is required"))
		}
		iss.Title = vals[0]
		if err := m.store.UpdateIssue(iss); err != nil {
			return m, reportErr(err)
		}
		m.closeForm()
		m.selectIssueByID(iss.ID)
		return m, toast("renamed " + iss.Key)
	}

	m.closeForm()
	return m, nil
}

func (m *Model) closeForm() {
	m.mode = modeNormal
	m.form = nil
	m.reload()
}
