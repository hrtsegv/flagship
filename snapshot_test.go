package flagship

import "testing"

func TestNewSnapshot_CopiesFlags(t *testing.T) {
	flags := []Flag{{Key: "a", Enabled: true}}
	snap := NewSnapshot("production", 1, flags)

	flags[0].Enabled = false // mutating the caller's slice after construction

	enabled, ok := snap.flag("a")
	if !ok || !enabled {
		t.Errorf("flag(a) = (%v, %v), want (true, true); snapshot must not be affected by later mutation of the input slice", enabled, ok)
	}
}

func TestSnapshot_EnvironmentAndVersion(t *testing.T) {
	snap := NewSnapshot("production", 42, nil)

	if got := snap.Environment(); got != "production" {
		t.Errorf("Environment() = %q, want %q", got, "production")
	}
	if got := snap.Version(); got != 42 {
		t.Errorf("Version() = %d, want 42", got)
	}
}

func TestSnapshot_FlagLookup(t *testing.T) {
	snap := NewSnapshot("production", 1, []Flag{
		{Key: "new-checkout", Enabled: true},
		{Key: "new-search", Enabled: false},
	})

	if enabled, ok := snap.flag("new-checkout"); !ok || !enabled {
		t.Errorf("flag(new-checkout) = (%v, %v), want (true, true)", enabled, ok)
	}
	if enabled, ok := snap.flag("new-search"); !ok || enabled {
		t.Errorf("flag(new-search) = (%v, %v), want (false, true)", enabled, ok)
	}
	if _, ok := snap.flag("does-not-exist"); ok {
		t.Errorf("flag(does-not-exist) ok = true, want false")
	}
}
