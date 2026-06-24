package core

import "testing"

func TestEnumValidity(t *testing.T) {
	// Every value returned by the catalog helpers must report Valid, and a
	// bogus value must not. This guards against a constant being added to the
	// type but forgotten in the Valid switch (or vice-versa).
	t.Run("provider", func(t *testing.T) {
		for _, p := range Providers() {
			if !p.Valid() {
				t.Errorf("Provider %q from Providers() is not Valid()", p)
			}
		}
		if Provider("gemini").Valid() {
			t.Error("unknown provider reported Valid()")
		}
	})
	t.Run("effort", func(t *testing.T) {
		for _, e := range Efforts() {
			if !e.Valid() {
				t.Errorf("Effort %q from Efforts() is not Valid()", e)
			}
		}
		if Effort("turbo").Valid() {
			t.Error("unknown effort reported Valid()")
		}
	})
	t.Run("issueType", func(t *testing.T) {
		for _, ty := range IssueTypes() {
			if !ty.Valid() {
				t.Errorf("IssueType %q from IssueTypes() is not Valid()", ty)
			}
		}
		if IssueType("spike").Valid() {
			t.Error("unknown issue type reported Valid()")
		}
	})
	t.Run("priority", func(t *testing.T) {
		for _, p := range Priorities() {
			if !p.Valid() {
				t.Errorf("Priority %q from Priorities() is not Valid()", p)
			}
		}
		if Priority("urgent").Valid() {
			t.Error("unknown priority reported Valid()")
		}
	})
	t.Run("statusCategory", func(t *testing.T) {
		for _, c := range StatusCategories() {
			if !c.Valid() {
				t.Errorf("StatusCategory %q is not Valid()", c)
			}
		}
		if StatusCategory("backlog").Valid() {
			t.Error("unknown status category reported Valid()")
		}
	})
	t.Run("linkType", func(t *testing.T) {
		for _, l := range LinkTypes() {
			if !l.Valid() {
				t.Errorf("LinkType %q is not Valid()", l)
			}
		}
		if LinkType("clones").Valid() {
			t.Error("unknown link type reported Valid()")
		}
	})
}

func TestPriorityRank(t *testing.T) {
	// Ranks must be strictly increasing in catalog order.
	prev := -2
	for _, p := range Priorities() {
		got := p.Rank()
		if got <= prev {
			t.Errorf("Priority %q rank %d not greater than previous %d", p, got, prev)
		}
		prev = got
	}
	if PriorityHighest.Rank() != len(Priorities())-1 {
		t.Errorf("highest priority rank = %d, want %d", PriorityHighest.Rank(), len(Priorities())-1)
	}
	if Priority("nope").Rank() != -1 {
		t.Errorf("unknown priority rank = %d, want -1", Priority("nope").Rank())
	}
}

func TestLinkInverse(t *testing.T) {
	tests := []struct {
		in, want LinkType
	}{
		{LinkBlocks, LinkBlockedBy},
		{LinkBlockedBy, LinkBlocks},
		{LinkRelates, LinkRelates},
		{LinkDuplicates, LinkDuplicates},
	}
	for _, tt := range tests {
		if got := tt.in.Inverse(); got != tt.want {
			t.Errorf("%q.Inverse() = %q, want %q", tt.in, got, tt.want)
		}
		// Inverse must be an involution: applying it twice returns the original.
		if got := tt.in.Inverse().Inverse(); got != tt.in {
			t.Errorf("%q.Inverse().Inverse() = %q, want %q", tt.in, got, tt.in)
		}
	}
}

func TestRunStatusTerminal(t *testing.T) {
	terminal := map[RunStatus]bool{
		RunQueued:    false,
		RunRunning:   false,
		RunBlocked:   false,
		RunSucceeded: true,
		RunFailed:    true,
		RunCancelled: true,
	}
	for s, want := range terminal {
		if got := s.Terminal(); got != want {
			t.Errorf("%q.Terminal() = %v, want %v", s, got, want)
		}
		if !s.Valid() {
			t.Errorf("run status %q is not Valid()", s)
		}
	}
}
