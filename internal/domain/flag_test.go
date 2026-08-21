package domain

import (
	"testing"
	"time"
)

func TestFlag_Apply(t *testing.T) {
	newName := "New Name"
	newDescription := "New description"
	enabled := true

	t.Run("updates only provided fields and bumps version", func(t *testing.T) {
		created := time.Now().UTC().Add(-time.Hour)
		f := Flag{
			Key:         "new-checkout",
			Name:        "Old Name",
			Description: "Old description",
			Enabled:     false,
			Version:     1,
			CreatedAt:   created,
			UpdatedAt:   created,
		}

		now := time.Now().UTC()
		f.Apply(FlagPatch{Name: &newName, Enabled: &enabled}, now)

		if f.Name != newName {
			t.Errorf("Name = %q, want %q", f.Name, newName)
		}
		if f.Description != "Old description" {
			t.Errorf("Description = %q, want unchanged", f.Description)
		}
		if !f.Enabled {
			t.Errorf("Enabled = false, want true")
		}
		if f.Version != 2 {
			t.Errorf("Version = %d, want 2", f.Version)
		}
		if !f.UpdatedAt.Equal(now) {
			t.Errorf("UpdatedAt = %v, want %v", f.UpdatedAt, now)
		}
		if !f.CreatedAt.Equal(created) {
			t.Errorf("CreatedAt changed, want unchanged")
		}
	})

	t.Run("empty patch still increments version", func(t *testing.T) {
		f := Flag{Version: 5}
		f.Apply(FlagPatch{}, time.Now().UTC())
		if f.Version != 6 {
			t.Errorf("Version = %d, want 6", f.Version)
		}
	})

	t.Run("description can be applied independently", func(t *testing.T) {
		f := Flag{Version: 1, Description: "old"}
		f.Apply(FlagPatch{Description: &newDescription}, time.Now().UTC())
		if f.Description != newDescription {
			t.Errorf("Description = %q, want %q", f.Description, newDescription)
		}
	})
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "new-checkout", false},
		{"empty key", "", true},
		{"whitespace only", "   ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid name", "New Checkout", false},
		{"empty name", "", true},
		{"whitespace only", "   ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
