package domain

import "errors"

// Sentinel errors returned by the domain and repository layers. Callers
// should use errors.Is to test for these, since concrete errors are wrapped
// with additional context (e.g. which resource was not found).
var (
	ErrNotFound     = errors.New("resource not found")
	ErrDuplicateKey = errors.New("key already exists")
	ErrValidation   = errors.New("validation failed")
)
