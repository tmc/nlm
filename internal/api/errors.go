package api

import (
    "errors"
    "fmt"
)

// Common errors returned by the API
var (
    ErrNotFound     = errors.New("not found")
    ErrUnauthorized = errors.New("unauthorized")
    ErrInvalidInput = errors.New("invalid input")
    ErrNotReady     = errors.New("not ready")
)

// NotFoundError wraps ErrNotFound with context
type NotFoundError struct {
    ResourceType string
    ID          string
}

func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s not found: %s", e.ResourceType, e.ID)
}

func (e *NotFoundError) Unwrap() error {
    return ErrNotFound
}

