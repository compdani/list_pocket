// Package apperr provides HTTP-aware application errors without depending on Echo.
package apperr

import (
	"fmt"
	"net/http"
)

// Error is an application error with an HTTP status and message body.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// New returns an Error with the given status and message.
func New(status int, message string) *Error {
	return &Error{Status: status, Message: message}
}

func BadRequest(message string) *Error {
	return New(http.StatusBadRequest, message)
}

func Forbidden(message string) *Error {
	return New(http.StatusForbidden, message)
}

func NotFound(message string) *Error {
	return New(http.StatusNotFound, message)
}

func Internal(message string) *Error {
	return New(http.StatusInternalServerError, message)
}

func Conflict(message string) *Error {
	return New(http.StatusConflict, message)
}

// From returns e if it is already an *Error, otherwise wraps it as 500.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return Internal(err.Error())
}

// Errorf builds a BadRequest by default when used for validation-style messages.
func Errorf(status int, format string, args ...any) *Error {
	return New(status, fmt.Sprintf(format, args...))
}
