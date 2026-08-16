// Package errors defines stable errors shared by Soro's non-HTTP layers.
package errors

import (
	"errors"
	"fmt"
)

// Code identifies a category of framework error without exposing its cause.
type Code string

const (
	CodeNotFound        Code = "not_found"
	CodeValidation      Code = "validation_failed"
	CodeConflict        Code = "conflict"
	CodeForbidden       Code = "forbidden"
	CodeUnauthorized    Code = "unauthorized"
	CodeInternal        Code = "internal_error"
	CodeInvalidArgument Code = "invalid_argument"
)

// Error is safe to serialize. Cause is deliberately omitted from JSON.
type Error struct {
	Code      Code                `json:"code"`
	Message   string              `json:"message"`
	RequestID string              `json:"request_id,omitempty"`
	Fields    map[string][]string `json:"fields,omitempty"`
	cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Message
}

// Unwrap preserves the internal cause for errors.Is/errors.As and logging.
func (e *Error) Unwrap() error { return e.cause }

// WithCause attaches an internal error without making it serializable.
func (e *Error) WithCause(cause error) *Error {
	e.cause = cause
	return e
}

func NotFound(resource string) *Error {
	return &Error{Code: CodeNotFound, Message: resource + " not found"}
}

func Validation(fields map[string][]string) *Error {
	return &Error{Code: CodeValidation, Message: "Validation failed", Fields: fields}
}

func Conflict(message string) *Error {
	return &Error{Code: CodeConflict, Message: message}
}

func Forbidden(message string) *Error {
	return &Error{Code: CodeForbidden, Message: message}
}

func Unauthorized(message string) *Error {
	return &Error{Code: CodeUnauthorized, Message: message}
}

func Internal(cause error) *Error {
	return (&Error{Code: CodeInternal, Message: "Internal server error"}).WithCause(cause)
}

func InvalidArgument(message string) *Error {
	return &Error{Code: CodeInvalidArgument, Message: message}
}

// IsCode reports whether err contains a Soro error with code.
func IsCode(err error, code Code) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}

// FieldError adds one normalized message for a field.
func FieldError(field, message string) *Error {
	return Validation(map[string][]string{field: {message}})
}

// Wrap annotates an internal cause while retaining a safe public message.
func Wrap(code Code, message string, cause error) *Error {
	if cause == nil {
		cause = fmt.Errorf("%s", message)
	}
	return (&Error{Code: code, Message: message}).WithCause(cause)
}
