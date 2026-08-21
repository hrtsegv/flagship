package domain

import (
	"fmt"
	"strings"
)

// ValidateKey enforces the minimal invariant that a key is present. Key
// naming restrictions are an open question in docs/domain-model.md and are
// deliberately not resolved here.
func ValidateKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%w: key is required", ErrValidation)
	}
	return nil
}

// ValidateName enforces the minimal invariant that a name is present.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	return nil
}
