package tui

import "testing"

func TestViewSwitchingCycles(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m)
	if m.view != viewBoard {
		t.Fatalf("should start on the board, got view %d", m.view)
	}

	for _, want := range []view{viewBacklog, viewSprints, viewRuns, viewBoard} {
		next, _ := m.Update(keyPress("tab"))
		m = next.(Model)
		if m.view != want {
			t.Errorf("tab → view %d, want %d", m.view, want)
		}
	}

	next, _ := m.Update(keyPress("shift+tab"))
	m = next.(Model)
	if m.view != viewRuns {
		t.Errorf("shift+tab from board wraps → runs, got %d", m.view)
	}
}

func TestGlobalKeysReachableFromSprints(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m) // leaves the model on the Sprints view

	for _, tc := range []struct {
		key  string
		want mode
	}{
		{"p", modeProjects},
		{",", modeSettings},
		{"?", modeHelp},
		{"m", modeMCP},
	} {
		next, _ := m.Update(keyPress(tc.key))
		if got := next.(Model).mode; got != tc.want {
			t.Errorf("%q from the Sprints view: mode = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestShiftTabFromSprints(t *testing.T) {
	m, _ := newTestModel(t)
	seedSprints(t, &m) // leaves the model on the Sprints view
	next, _ := m.Update(keyPress("shift+tab"))
	if got := next.(Model).view; got != viewBacklog {
		t.Errorf("shift+tab from Sprints should step back to Backlog, got view %d", got)
	}
}

func TestViewSwitchPreservesBoardSelection(t *testing.T) {
	m, _ := newTestModel(t)
	seedBoard(t, &m) // first column holds two issues
	m.colIdx, m.cardIdx = 0, 0

	next, _ := m.Update(keyPress("j")) // move down one card
	m = next.(Model)
	want, ok := m.selectedIssue()
	if !ok {
		t.Fatal("expected a selected issue after moving down")
	}

	for i := 0; i < int(viewCount); i++ { // a full lap returns to the board
		next, _ = m.Update(keyPress("tab"))
		m = next.(Model)
	}
	if m.view != viewBoard {
		t.Fatalf("expected to land back on the board, got view %d", m.view)
	}
	got, _ := m.selectedIssue()
	if got.ID != want.ID {
		t.Errorf("selection drifted across a view round-trip: got %d want %d", got.ID, want.ID)
	}
}
