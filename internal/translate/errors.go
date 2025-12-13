package translate

import "errors"

var (
	// ErrCollectionNotFound is returned when a referenced collection does not exist.
	ErrCollectionNotFound = errors.New("collection not found")

	// ErrInvalidGeometry is returned when geometry conversion fails.
	ErrInvalidGeometry = errors.New("invalid geometry")

	// ErrInvalidDateTime is returned when datetime parsing fails.
	ErrInvalidDateTime = errors.New("invalid datetime format")

	// ErrUnsupportedFilter is returned when a filter expression cannot be translated.
	ErrUnsupportedFilter = errors.New("unsupported filter expression")

	// ErrInvalidCursor is returned when cursor decoding fails.
	ErrInvalidCursor = errors.New("invalid pagination cursor")
)
