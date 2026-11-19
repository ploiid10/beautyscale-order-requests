// Package order validates input and runs the accept and reject transactions.
package order

import (
	"errors"
	"fmt"
)

// Sentinel errors for HTTP status mapping. Use errors.Is on the returned error.
var (
	ErrInvalid  = errors.New("invalid")
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// Error is a client-facing failure with a status kind.
type Error struct {
	Kind    error
	Message string
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Unwrap() error { return e.Kind }

// Invalid reports bad input.
func Invalid(message string) error {
	return &Error{Kind: ErrInvalid, Message: message}
}

// NotFound reports a row the caller cannot see.
func NotFound(message string) error {
	return &Error{Kind: ErrNotFound, Message: message}
}

// Conflict reports a request that cannot change, or stock that is too low.
func Conflict(message string) error {
	return &Error{Kind: ErrConflict, Message: message}
}

// Decision is the body returned after a reject.
type Decision struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func fmtStock(name string) string {
	return fmt.Sprintf("not enough stock for %s", name)
}
