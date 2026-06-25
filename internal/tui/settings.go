package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/03-CiprianoG/jeera/internal/config"
	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/tui/theme"
)

// permissionModes are the claude permission postures Jeera offers as the global
// default, from most to least autonomous.
var permissionModes = []string{"bypassPermissions", "acceptEdits", "plan", "default"}

type settingsField int

const (
	sfProvider settingsField = iota
	sfModel
	sfEffort
	sfWorktree
	sfPermission
	sfFieldCount
)

// settingsModel is the global-defaults editor. Fields are navigated with j/k and
// changed in place with h/l, mirroring the ticket detail sidebar. Every change is
// persisted to the config file immediately (and swapped into the live store, so
// the run manager and scheduler pick it up at once).
type settingsModel struct {
	cfg   *config.Store
	theme theme.Theme
	draft config.Config
	field settingsField

	width, height int
	err           string
}

func newSettings(cfg *config.Store, th theme.Theme, w, h int) *settingsModel {
	return &settingsModel{cfg: cfg, theme: th, draft: cfg.Get(), width: w, height: h}
}

// update handles a key while settings is open and reports whether to close.
func (s *settingsModel) update(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "esc", "q":
		return true
	case "j", "down":
		s.field = (s.field + 1) % sfFieldCount
	case "k", "up":
		s.field = (s.field - 1 + sfFieldCount) % sfFieldCount
	case "l", "right":
		s.cycle(+1)
	case "h", "left":
		s.cycle(-1)
	}
	return false
}

// cycle changes the focused field by dir and persists the result.
func (s *settingsModel) cycle(dir int) {
	d := &s.draft.Defaults
	switch s.field {
	case sfProvider:
		provs := []core.Provider{core.ProviderClaude, core.ProviderCodex}
		d.Provider = provs[(indexProvider(provs, d.Provider)+dir+len(provs))%len(provs)]
		// Keep model/effort valid for the new provider.
		if !containsStr(core.ProviderModels(d.Provider), d.Model) {
			d.Model = core.DefaultAssignee(d.Provider).Model
		}
		if !core.SupportsEffort(d.Provider, d.Effort) {
			d.Effort = core.EffortMedium
		}
	case sfModel:
		models := core.ProviderModels(d.Provider)
		d.Model = cycleStr(models, d.Model, dir, core.DefaultAssignee(d.Provider).Model)
	case sfEffort:
		efforts := core.ProviderEfforts(d.Provider)
		cur := indexEffort(efforts, d.Effort)
		if cur < 0 {
			cur = 0
		} else {
			cur = (cur + dir + len(efforts)) % len(efforts)
		}
		if len(efforts) > 0 {
			d.Effort = efforts[cur]
		}
	case sfWorktree:
		d.WorktreeOn = !d.WorktreeOn
	case sfPermission:
		d.PermissionMode = cycleStr(permissionModes, d.PermissionMode, dir, permissionModes[0])
	}
	s.save()
}

func (s *settingsModel) save() {
	if err := s.cfg.Save(s.draft); err != nil {
		s.err = err.Error()
		return
	}
	s.err = ""
}

// View renders the settings panel.
func (s *settingsModel) View() string {
	t := s.theme
	d := s.draft.Defaults

	rows := []struct {
		label, value string
		c            color.Color
	}{
		{"Provider", string(d.Provider), t.P.Info},
		{"Model", d.Model, t.P.Info},
		{"Effort", string(d.Effort), t.P.Info},
		{"Worktree", onOff(d.WorktreeOn), t.P.TextPrimary},
		{"Permission", d.PermissionMode, t.P.TextPrimary},
	}
	lines := make([]string, 0, len(rows))
	for i, r := range rows {
		marker := "  "
		label := t.Label.Render(fmt.Sprintf("%-11s", r.label))
		if settingsField(i) == s.field {
			marker = t.HelpKey.Render("▸ ")
			label = t.Label.Bold(true).Render(fmt.Sprintf("%-11s", r.label))
		}
		val := lipgloss.NewStyle().Foreground(r.c).Render(r.value)
		lines = append(lines, marker+label+" "+val)
	}

	port := t.HelpDesc.Render(fmt.Sprintf("MCP port: %s   (set JEERA_MCP_PORT or mcp_port in config.toml)", portLabel(s.draft.MCPPort)))
	body := strings.Join(lines, "\n") + "\n\n" + port

	hint := t.HelpKey.Render("j/k") + " " + t.HelpDesc.Render("field") + "   " +
		t.HelpKey.Render("h/l") + " " + t.HelpDesc.Render("change") + "   " +
		t.HelpKey.Render("esc") + " " + t.HelpDesc.Render("close")
	if s.err != "" {
		hint = t.Error.Render("! "+truncate(s.err, s.width/2)) + "   " + hint
	}

	return modalShell(t, modalWidthSettings, 0, "Settings — global defaults",
		"Fallback run settings. A project or ticket can override any of these.",
		body, hint)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func portLabel(p int) string {
	if p == 0 {
		return "auto"
	}
	return fmt.Sprintf("%d", p)
}

func indexProvider(provs []core.Provider, p core.Provider) int {
	for i, v := range provs {
		if v == p {
			return i
		}
	}
	return 0
}

func indexEffort(efforts []core.Effort, e core.Effort) int {
	for i, v := range efforts {
		if v == e {
			return i
		}
	}
	return -1
}

func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// cycleStr advances within xs from cur by dir; if cur isn't in xs it lands on
// fallback.
func cycleStr(xs []string, cur string, dir int, fallback string) string {
	if len(xs) == 0 {
		return cur
	}
	idx := -1
	for i, v := range xs {
		if v == cur {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fallback
	}
	return xs[(idx+dir+len(xs))%len(xs)]
}
