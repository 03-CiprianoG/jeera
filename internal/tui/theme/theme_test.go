package theme

import (
	"testing"

	"github.com/03-CiprianoG/jeera/internal/core"
)

// TestSprintStateColor pins the intent the goldens can't prove (they strip
// colour): the live sprint gets the iris focus accent, a finished one reads as
// success, and a future one stays muted.
func TestSprintStateColor(t *testing.T) {
	th := New()
	for _, c := range []struct {
		state core.SprintState
		want  any
	}{
		{core.SprintActive, th.P.Focus},
		{core.SprintCompleted, th.P.Success},
		{core.SprintFuture, th.P.TextMuted},
	} {
		if got := th.SprintStateColor(c.state); got != c.want {
			t.Errorf("SprintStateColor(%s) = %v, want %v", c.state, got, c.want)
		}
	}
}

// TestCategoryColor pins the board-lane accents the goldens can't prove (they
// strip colour): each of the four categories maps to its intended token, and all
// four are distinct so In Review reads as its own lane rather than echoing the
// amber of In Progress.
func TestCategoryColor(t *testing.T) {
	th := New()
	cases := []struct {
		cat  core.StatusCategory
		want any
	}{
		{core.CategoryTodo, th.P.TextMuted},
		{core.CategoryInProgress, th.P.Warning},
		{core.CategoryReview, th.P.Info},
		{core.CategoryDone, th.P.Success},
	}
	seen := map[any]core.StatusCategory{}
	for _, c := range cases {
		got := th.CategoryColor(c.cat)
		if got != c.want {
			t.Errorf("CategoryColor(%s) = %v, want %v", c.cat, got, c.want)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("CategoryColor(%s) shares a colour with %s — lanes must be distinct", c.cat, other)
		}
		seen[got] = c.cat
	}
}
