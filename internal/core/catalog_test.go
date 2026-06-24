package core

import "testing"

func TestProviderCatalog(t *testing.T) {
	for _, p := range Providers() {
		models := ProviderModels(p)
		if len(models) == 0 {
			t.Errorf("provider %q has no models", p)
		}
		efforts := ProviderEfforts(p)
		if len(efforts) == 0 {
			t.Errorf("provider %q has no efforts", p)
		}
		// Every catalog effort must be a valid Effort value.
		for _, e := range efforts {
			if !e.Valid() {
				t.Errorf("provider %q lists invalid effort %q", p, e)
			}
		}
	}
	// Unknown provider yields empty catalogs.
	if ProviderModels("nope") != nil || ProviderEfforts("nope") != nil {
		t.Error("unknown provider should have empty catalog")
	}
}

func TestDefaultAssigneeAndSupportsEffort(t *testing.T) {
	a := DefaultAssignee(ProviderClaude)
	if a.Provider != ProviderClaude || a.Model == "" || a.Effort != EffortMedium {
		t.Errorf("default claude assignee = %+v", a)
	}
	if err := a.Validate(); err != nil {
		t.Errorf("default assignee should validate: %v", err)
	}

	if !SupportsEffort(ProviderClaude, EffortMax) {
		t.Error("claude should support max effort")
	}
	if SupportsEffort(ProviderClaude, EffortMinimal) {
		t.Error("claude should not support minimal effort")
	}
	if !SupportsEffort(ProviderCodex, EffortMinimal) {
		t.Error("codex should support minimal effort")
	}
	if SupportsEffort(ProviderCodex, EffortMax) {
		t.Error("codex should not support max effort")
	}
}
