// Package apperror defines typed application errors that the HTTP layer maps
// to status codes. Services return these; handlers never inspect raw DB errors.
package apperror

import (
	"errors"
	"net/http"
)

type Kind int

const (
	KindInternal Kind = iota
	KindValidation
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindGone
)

type Error struct {
	Kind    Kind
	Code    string // machine-readable, e.g. "user_not_found"
	Message string // human-readable
	Details any
	Err     error // wrapped cause (never exposed to clients)
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) HTTPStatus() int {
	switch e.Kind {
	case KindValidation:
		return http.StatusUnprocessableEntity
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindGone:
		return http.StatusGone
	default:
		return http.StatusInternalServerError
	}
}

func (e *Error) WithDetails(d any) *Error { c := *e; c.Details = d; return &c }

func Validation(code, msg string) *Error {
	return &Error{Kind: KindValidation, Code: code, Message: msg}
}
func Unauthorized(code, msg string) *Error {
	return &Error{Kind: KindUnauthorized, Code: code, Message: msg}
}
func Forbidden(code, msg string) *Error { return &Error{Kind: KindForbidden, Code: code, Message: msg} }
func NotFound(code, msg string) *Error  { return &Error{Kind: KindNotFound, Code: code, Message: msg} }
func Conflict(code, msg string) *Error  { return &Error{Kind: KindConflict, Code: code, Message: msg} }
func Gone(code, msg string) *Error      { return &Error{Kind: KindGone, Code: code, Message: msg} }

func Internal(err error) *Error {
	return &Error{Kind: KindInternal, Code: "internal_error", Message: "internal server error", Err: err}
}

// As extracts an *Error, wrapping unknown errors as internal errors.
func As(err error) *Error {
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	return Internal(err)
}
