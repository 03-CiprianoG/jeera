package tui

import (
	"path/filepath"
	"testing"

	"github.com/03-CiprianoG/jeera/internal/config"
	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/03-CiprianoG/jeera/internal/store"
)

// openProjects seeds one project and opens the projects overlay with it selected.
func openProjects(t *testing.T) (Model, *store.Store, core.Project) {
	t.Helper()
	m, st := newTestModel(t)
	p := seedProject(t, st)
	m.reload()
	m.mode = modeProjects
	m.projSel = 0
	return m, st, p
}

func TestProjectDeleteConfirmsAndRemoves(t *testing.T) {
	m, st, p := openProjects(t)
	// An issue under the project proves the store cascade actually runs.
	if _, err := st.CreateIssue(core.Issue{ProjectID: p.ID, Title: "doomed"}); err != nil {
		t.Fatal(err)
	}
	nm, _ := m.updateProjects(keyPress("x"))
	m = nm.(Model)
	if m.mode != modeConfirm {
		t.Fatalf("x should open the confirm dialog, got mode %v", m.mode)
	}
	nm, _ = m.updateConfirm(keyPress("y"))
	m = nm.(Model)
	if _, err := st.GetProject(p.ID); err == nil {
		t.Fatal("project should be gone after confirming the delete")
	}
}

func TestProjectDeleteCancelKeeps(t *testing.T) {
	m, st, p := openProjects(t)
	nm, _ := m.updateProjects(keyPress("x"))
	m = nm.(Model)
	nm, _ = m.updateConfirm(keyPress("n"))
	m = nm.(Model)
	if _, err := st.GetProject(p.ID); err != nil {
		t.Fatalf("project should survive a cancelled delete: %v", err)
	}
}

func TestProjectDeleteClearsDefaultPin(t *testing.T) {
	m, _, p := openProjects(t)
	if err := m.cfg.SetDefaultProject(p.KeyPrefix); err != nil {
		t.Fatal(err)
	}
	nm, _ := m.updateProjects(keyPress("x"))
	m = nm.(Model)
	nm, _ = m.updateConfirm(keyPress("y"))
	m = nm.(Model)
	if got := m.cfg.Get().DefaultProjectPrefix; got != "" {
		t.Fatalf("deleting the default project should clear the pin, got %q", got)
	}
}

// Deleting the project you're currently in must not leave the board pointing at a
// project that no longer exists: reload() falls back to a remaining one.
func TestProjectDeleteActiveSwitchesToRemaining(t *testing.T) {
	m, st := newTestModel(t)
	jee := seedProject(t, st) // oldest → index 0, and the active project
	abc, err := st.CreateProject(core.Project{Name: "Acme", KeyPrefix: "ABC", RepoPath: "/tmp/abc"})
	if err != nil {
		t.Fatal(err)
	}
	m.reload()
	m.mode = modeProjects
	m.projSel = 0 // JEE, which is also active
	nm, _ := m.updateProjects(keyPress("x"))
	m = nm.(Model)
	nm, _ = m.updateConfirm(keyPress("y"))
	m = nm.(Model)
	if _, err := st.GetProject(jee.ID); err == nil {
		t.Fatal("the active project should be deleted")
	}
	if m.active.ID != abc.ID {
		t.Fatalf("deleting the active project should fall back to the remaining one, got %q", m.active.KeyPrefix)
	}
}

func TestProjectEditSavesNameAndRepo(t *testing.T) {
	m, st, p := openProjects(t)
	nm, _ := m.updateProjects(keyPress("e"))
	m = nm.(Model)
	if m.mode != modeForm || m.form == nil || m.form.kind != formEditProject {
		t.Fatalf("e should open the edit-project form, mode=%v form=%v", m.mode, m.form)
	}
	if got := m.form.fields[0].value(); got != "Jeera" {
		t.Fatalf("the edit form should pre-fill the current name, got %q", got)
	}
	m.form.fields[0].SetValue("Renamed")
	m.form.fields[1].SetValue("/tmp/renamed")
	nm, _ = m.submitForm()
	m = nm.(Model)
	got, err := st.GetProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Renamed" || got.RepoPath != "/tmp/renamed" {
		t.Fatalf("edit not saved: %+v", got)
	}
	if got.KeyPrefix != "JEE" {
		t.Fatalf("the key prefix must stay immutable across an edit, got %q", got.KeyPrefix)
	}
}

func TestProjectEditRejectsEmptyName(t *testing.T) {
	m, st, p := openProjects(t)
	nm, _ := m.updateProjects(keyPress("e"))
	m = nm.(Model)
	m.form.fields[0].SetValue("") // clear the required name
	nm, cmd := m.submitForm()
	m = nm.(Model)
	if m.mode != modeForm {
		t.Fatal("an invalid edit should keep the form open")
	}
	if cmd == nil {
		t.Fatal("an invalid edit should surface an error")
	}
	if got, _ := st.GetProject(p.ID); got.Name != "Jeera" {
		t.Fatalf("a rejected edit must not change the project, got name %q", got.Name)
	}
}

func TestProjectSetDefaultPins(t *testing.T) {
	m, _, p := openProjects(t)
	nm, _ := m.updateProjects(keyPress("d"))
	m = nm.(Model)
	if got := m.cfg.Get().DefaultProjectPrefix; got != p.KeyPrefix {
		t.Fatalf("d should pin the project as default, got %q want %q", got, p.KeyPrefix)
	}
}

func TestStartupOpensDefaultProject(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// JEE is the oldest (the historical default); ABC is the one the user pinned.
	if _, err := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/jee"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProject(core.Project{Name: "Acme", KeyPrefix: "ABC", RepoPath: "/tmp/abc"}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.NewStore(filepath.Join(t.TempDir(), "config.toml"))
	if err := cfg.SetDefaultProject("ABC"); err != nil {
		t.Fatal(err)
	}
	m := New(st, nil, nil, nil, cfg)
	if m.active.KeyPrefix != "ABC" {
		t.Fatalf("startup should open the pinned default ABC, got %q", m.active.KeyPrefix)
	}
}

func TestStartupStaleDefaultFallsBackToOldest(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "jeera.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateProject(core.Project{Name: "Jeera", KeyPrefix: "JEE", RepoPath: "/tmp/jee"}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.NewStore(filepath.Join(t.TempDir(), "config.toml"))
	if err := cfg.SetDefaultProject("GONE"); err != nil {
		t.Fatal(err)
	}
	m := New(st, nil, nil, nil, cfg)
	if m.active.KeyPrefix != "JEE" {
		t.Fatalf("a stale default pin should fall back to the oldest project, got %q", m.active.KeyPrefix)
	}
}
