package core

// The model and effort catalog is reference data for the assignee picker. Neither
// the claude nor the codex CLI exposes a reliable "list models" command, so Jeera
// maintains the list here. Prefer model aliases (which the CLIs map to the latest
// concrete model) over pinned IDs so retirements don't break assignments.

// ProviderModels lists the selectable models for a provider, in display order.
func ProviderModels(p Provider) []string {
	switch p {
	case ProviderClaude:
		return []string{"opus", "sonnet", "haiku", "fable"}
	case ProviderCodex:
		return []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.2-codex"}
	}
	return nil
}

// ProviderEfforts lists the reasoning-effort levels a provider accepts, from
// least to most. The two providers support different sets (codex adds
// "minimal"; claude adds "max"), and codex coerces an unsupported level to the
// nearest one it supports.
func ProviderEfforts(p Provider) []Effort {
	switch p {
	case ProviderClaude:
		return []Effort{EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax}
	case ProviderCodex:
		return []Effort{EffortMinimal, EffortLow, EffortMedium, EffortHigh, EffortXHigh}
	}
	return nil
}

// DefaultAssignee returns the catalog's default model for a provider (the first
// listed model at medium effort), used when assigning work without a specific
// pick.
func DefaultAssignee(p Provider) Assignee {
	models := ProviderModels(p)
	if len(models) == 0 {
		return Assignee{}
	}
	return Assignee{Provider: p, Model: models[0], Effort: EffortMedium}
}

// SupportsEffort reports whether a provider accepts the given effort level.
func SupportsEffort(p Provider, e Effort) bool {
	for _, x := range ProviderEfforts(p) {
		if x == e {
			return true
		}
	}
	return false
}
