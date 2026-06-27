package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

type formKind int

const (
	formCreateIssue formKind = iota
	formCreateProject
	formEditProject
	formRename
	formCreateSprint
)

// fieldType is how a form field is edited: free text, a date, or a value cycled
// between fixed options with the arrow keys.
type fieldType int

const (
	fieldText fieldType = iota
	fieldChoice
	fieldDate
)

// formField is one input in a form. Text and date fields carry a live text
// input; a choice field carries its options and the selected index.
type formField struct {
	label   string
	ftype   fieldType
	input   textinput.Model
	options []string
	choice  int
}

// SetValue sets a text/date field's value. It is a thin shim so tests (and the
// rename pre-fill) can seed a field by name without reaching into the input.
func (f *formField) SetValue(s string) { f.input.SetValue(s) }

func (f formField) value() string {
	if f.ftype == fieldChoice {
		if f.choice >= 0 && f.choice < len(f.options) {
			return f.options[f.choice]
		}
		return ""
	}
	return strings.TrimSpace(f.input.Value())
}

// formModel is Jeera's modal form. Focus walks the fields and then the two
// buttons (Create/Save, Cancel); choice fields cycle with ←/→, text fields edit
// in place, and Enter submits from anywhere but the Cancel button.
type formModel struct {
	kind      formKind
	heading   string
	sub       string
	submit    string // the primary button's label ("Create" / "Save")
	fields    []formField
	focus     int
	issueID   int64  // target for formRename
	projectID int64  // target for formEditProject
	statusID  int64  // target column for a new issue (0 → the project's first status)
	sprintID  *int64 // sprint a new issue joins (set when creating on the board; nil → backlog)
}

func newTextField(label, placeholder string, limit int) formField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = limit
	ti.Prompt = ""
	ti.SetWidth(52)
	return formField{label: label, ftype: fieldText, input: ti}
}

func newDateField(label string) formField {
	ti := textinput.New()
	ti.Placeholder = "YYYY-MM-DD"
	ti.CharLimit = 10
	ti.Prompt = ""
	ti.SetWidth(24)
	return formField{label: label, ftype: fieldDate, input: ti}
}

func newChoiceField(label string, options []string, def int) formField {
	return formField{label: label, ftype: fieldChoice, options: options, choice: def}
}

// newCreateIssueForm captures a new issue: a required title plus the two
// decisions worth making up front (type and priority) and an optional
// description. statusID targets the column it lands in (0 → the first status).
func newCreateIssueForm(statusID int64) *formModel {
	types := enumStrings(core.IssueTypes())
	prios := enumStrings(core.Priorities())
	return &formModel{
		kind:     formCreateIssue,
		heading:  "New issue",
		sub:      "Title is all you need now — everything else is refinable.",
		submit:   "Create",
		statusID: statusID,
		fields: []formField{
			newTextField("Title", "What needs doing?", 200),
			newChoiceField("Type", types, indexOf(types, string(core.TypeTask))),
			newChoiceField("Priority", prios, indexOf(prios, string(core.PriorityMedium))),
			newTextField("Description", "Optional — add detail", 1000),
		},
	}
}

// newCreateSprintForm plans a sprint: a name, an optional goal, and the window
// it runs. Dates are typed as YYYY-MM-DD and may be left blank.
func newCreateSprintForm() *formModel {
	return &formModel{
		kind:    formCreateSprint,
		heading: "New sprint",
		sub:     "Time-box a set of issues. Dates are optional.",
		submit:  "Create",
		fields: []formField{
			newTextField("Name", "Sprint name", 80),
			newTextField("Goal", "Goal (optional)", 200),
			newDateField("Start"),
			newDateField("End"),
		},
	}
}

func newCreateProjectForm() *formModel {
	// Pre-fill the repo path with the current directory — the common case is
	// "point Jeera at the repo I'm in" — but let the user edit it.
	repo := newTextField("Repo path", "/path/to/repo", 300)
	if cwd, err := os.Getwd(); err == nil {
		repo.input.SetValue(cwd)
	}
	return &formModel{
		kind:    formCreateProject,
		heading: "New project",
		sub:     "A board bound to a git repository.",
		submit:  "Create",
		fields: []formField{
			newTextField("Name", "Project name", 80),
			newTextField("Key", "KEY, e.g. JEE", 10),
			repo,
		},
	}
}

// newEditProjectForm edits a project's mutable fields, pre-filled with its
// current values. The key prefix is deliberately absent: issue keys depend on it,
// so the store treats it as immutable — only the name and repo path can change.
func newEditProjectForm(p core.Project) *formModel {
	name := newTextField("Name", "Project name", 80)
	name.input.SetValue(p.Name)
	repo := newTextField("Repo path", "/path/to/repo", 300)
	repo.input.SetValue(p.RepoPath)
	return &formModel{
		kind:      formEditProject,
		heading:   "Edit " + p.KeyPrefix,
		sub:       "Rename it or change its repo path.",
		submit:    "Save",
		fields:    []formField{name, repo},
		projectID: p.ID,
	}
}

func newRenameForm(iss core.Issue) *formModel {
	f := newTextField("Title", "Title", 200)
	f.input.SetValue(iss.Title)
	return &formModel{
		kind:    formRename,
		heading: "Rename " + iss.Key,
		submit:  "Save",
		fields:  []formField{f},
		issueID: iss.ID,
	}
}

// focusCount is the number of focus stops: every field plus the two buttons.
func (f *formModel) focusCount() int { return len(f.fields) + 2 }
func (f *formModel) onSubmit() bool  { return f.focus == len(f.fields) }
func (f *formModel) onCancel() bool  { return f.focus == len(f.fields)+1 }

// curField returns the focused field, or nil when a button is focused.
func (f *formModel) curField() *formField {
	if f.focus >= 0 && f.focus < len(f.fields) {
		return &f.fields[f.focus]
	}
	return nil
}

// focusCmd focuses the first field.
func (f *formModel) focusCmd() tea.Cmd {
	f.focus = 0
	return f.syncFocus()
}

// syncFocus focuses the live input under the cursor and blurs the rest, so only
// the active text field shows a cursor.
func (f *formModel) syncFocus() tea.Cmd {
	var cmd tea.Cmd
	for i := range f.fields {
		if i == f.focus && f.fields[i].ftype != fieldChoice {
			cmd = f.fields[i].input.Focus()
		} else {
			f.fields[i].input.Blur()
		}
	}
	return cmd
}

func (f *formModel) moveFocus(d int) tea.Cmd {
	f.focus = (f.focus + d + f.focusCount()) % f.focusCount()
	return f.syncFocus()
}

// update routes a message (typing, cursor blink) to the focused text field.
func (f *formModel) update(msg tea.Msg) tea.Cmd {
	fld := f.curField()
	if fld == nil || fld.ftype == fieldChoice {
		return nil
	}
	var cmd tea.Cmd
	fld.input, cmd = fld.input.Update(msg)
	return cmd
}

func (f *formModel) View(t theme.Theme) string {
	const labelW = 14
	rows := make([]string, 0, len(f.fields))
	for i := range f.fields {
		fld := &f.fields[i]
		focused := f.focus == i
		labelStyle := t.Label
		if focused {
			labelStyle = lipgloss.NewStyle().Foreground(t.P.FocusGlow).Bold(true)
		}
		label := labelStyle.Render(fmt.Sprintf("%-*s", labelW, fld.label))

		// All controls share one left edge (label + a 2-cell gap). A focused choice
		// spends that gap on a ◀ chevron and trails a ▶, so cycling is advertised
		// without shifting the value column.
		var control string
		switch fld.ftype {
		case fieldChoice:
			raw := fld.value()
			disp := titleFirst(raw)
			vs := lipgloss.NewStyle().Foreground(t.P.TextPrimary)
			if fld.label == "Priority" {
				p := core.Priority(raw)
				vs = vs.Foreground(t.PriorityColor(p))
				disp = theme.PriorityGlyph(p) + " " + disp
			}
			control = cycler(t, disp, vs, focused)
		default:
			control = "  " + fld.input.View()
		}
		rows = append(rows, label+control)
	}

	// Fields breathe a row apart, then the action row sits below its own gap — the
	// extra air is what makes the bigger frame read as deliberate, not empty.
	body := strings.Join(rows, "\n\n") +
		"\n\n\n" + buttonRow(t, []string{f.submit, "Cancel"}, f.buttonFocus())
	hint := modalHint(t, "enter "+strings.ToLower(f.submit)+" · tab next · ←/→ change · esc cancel")
	return modalShell(t, modalWidthForm, 0, f.heading, f.sub, body, hint)
}

// buttonFocus maps the focus index to the focused button (0 submit, 1 cancel),
// or -1 when a field is focused.
func (f *formModel) buttonFocus() int {
	switch {
	case f.onSubmit():
		return 0
	case f.onCancel():
		return 1
	default:
		return -1
	}
}

// updateForm handles keys while a form is open. tab walks focus, ←/→ cycles a
// choice (or hops between buttons), Enter submits unless the Cancel button is
// focused, and esc closes.
func (m Model) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := m.form
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.form = nil
		return m, nil
	case "tab", "down":
		return m, f.moveFocus(1)
	case "shift+tab", "up":
		return m, f.moveFocus(-1)
	case "left", "right":
		dir := 1
		if msg.String() == "left" {
			dir = -1
		}
		if fld := f.curField(); fld != nil {
			if fld.ftype == fieldChoice {
				fld.choice = wrap(fld.choice+dir, len(fld.options))
				return m, nil
			}
			return m, f.update(msg) // text/date: move the cursor
		}
		// A button is focused: ←/→ hops between Submit and Cancel.
		if f.onSubmit() && dir > 0 {
			f.focus = len(f.fields) + 1
		} else if f.onCancel() && dir < 0 {
			f.focus = len(f.fields)
		}
		return m, f.syncFocus()
	case "enter":
		if f.onCancel() {
			m.mode = modeNormal
			m.form = nil
			return m, nil
		}
		return m.submitForm()
	}
	return m, f.update(msg)
}

// submitForm applies the form against the store. On error it keeps the form open
// and surfaces the message; on success it closes and selects the result.
func (m Model) submitForm() (tea.Model, tea.Cmd) {
	if m.form == nil {
		m.mode = modeNormal
		return m, nil
	}
	f := m.form

	switch f.kind {
	case formCreateProject:
		repo := f.fields[2].value()
		if repo == "" {
			repo, _ = os.Getwd()
		}
		p, err := m.store.CreateProject(core.Project{Name: f.fields[0].value(), KeyPrefix: f.fields[1].value(), RepoPath: repo})
		if err != nil {
			return m, reportErr(err)
		}
		m.active = p
		m.closeForm()
		return m, toast("created project " + p.KeyPrefix)

	case formEditProject:
		p, err := m.store.GetProject(f.projectID)
		if err != nil {
			return m, reportErr(err)
		}
		// Carry the name and repo path over the loaded project so its prefix and
		// defaults are preserved; Validate (in UpdateProject) rejects an empty name
		// or repo path and keeps the form open with the message.
		p.Name = f.fields[0].value()
		p.RepoPath = f.fields[1].value()
		if err := m.store.UpdateProject(p); err != nil {
			return m, reportErr(err)
		}
		m.closeForm()
		return m, toast("updated " + p.KeyPrefix)

	case formCreateIssue:
		if f.fields[0].value() == "" {
			return m, reportErr(fmt.Errorf("title is required"))
		}
		iss, err := m.store.CreateIssue(core.Issue{
			ProjectID:   m.active.ID,
			Title:       f.fields[0].value(),
			Type:        core.IssueType(f.fields[1].value()),
			Priority:    core.Priority(f.fields[2].value()),
			Description: f.fields[3].value(),
			StatusID:    f.statusID,
			SprintID:    f.sprintID,
		})
		if err != nil {
			return m, reportErr(err)
		}
		m.closeForm()
		m.selectIssueByID(iss.ID)
		return m, toast("created " + iss.Key)

	case formCreateSprint:
		if f.fields[0].value() == "" {
			return m, reportErr(fmt.Errorf("sprint name is required"))
		}
		start, err := parseDate(f.fields[2].value())
		if err != nil {
			return m, reportErr(fmt.Errorf("start date — %w", err))
		}
		end, err := parseDate(f.fields[3].value())
		if err != nil {
			return m, reportErr(fmt.Errorf("end date — %w", err))
		}
		sp := core.Sprint{ProjectID: m.active.ID, Name: f.fields[0].value(), Goal: f.fields[1].value(), StartAt: start, EndAt: end}
		if err := sp.Validate(); err != nil {
			return m, reportErr(err)
		}
		if _, err := m.store.CreateSprint(sp); err != nil {
			return m, reportErr(err)
		}
		m.closeForm()
		return m, toast("created sprint " + sp.Name)

	case formRename:
		iss, err := m.store.GetIssue(f.issueID)
		if err != nil {
			return m, reportErr(err)
		}
		if f.fields[0].value() == "" {
			return m, reportErr(fmt.Errorf("title is required"))
		}
		iss.Title = f.fields[0].value()
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

// parseDate reads a YYYY-MM-DD date in the local zone; an empty string is a
// valid "unset" and returns nil.
func parseDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	tm, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil, fmt.Errorf("use YYYY-MM-DD")
	}
	return &tm, nil
}

// enumStrings converts a slice of ~string enum values to plain strings.
func enumStrings[T ~string](xs []T) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = string(x)
	}
	return out
}

// titleFirst upper-cases the first rune for display, so a lowercase enum value
// like "task" reads as "Task" without changing what's stored.
func titleFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// indexOf returns the position of v in xs, or 0 when absent (a safe default for
// a choice field).
func indexOf(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return 0
}
