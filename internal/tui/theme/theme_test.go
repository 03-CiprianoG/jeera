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
