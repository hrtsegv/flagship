package idgen

import "testing"

func TestNew_Unique(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		id := New()
		if len(id) != 36 {
			t.Fatalf("New() = %q, want length 36", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("New() produced duplicate id %q", id)
		}
		seen[id] = struct{}{}
	}
}
