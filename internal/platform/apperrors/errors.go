package apperrors

import "fmt"

type Kind string

const (
	KindValidation Kind = "validation"
	KindNotFound   Kind = "not_found"
	KindConflict   Kind = "conflict"
	KindInternal   Kind = "internal"
)

type Error struct {
	Kind    Kind
	Message string
	Cause   error
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func Validation(message string) error {
	return &Error{Kind: KindValidation, Message: message}
}

func Validationf(format string, args ...any) error {
	return Validation(fmt.Sprintf(format, args...))
}

func NotFound(message string) error {
	return &Error{Kind: KindNotFound, Message: message}
}

func NotFoundf(format string, args ...any) error {
	return NotFound(fmt.Sprintf(format, args...))
}

func Conflict(message string) error {
	return &Error{Kind: KindConflict, Message: message}
}

func Conflictf(format string, args ...any) error {
	return Conflict(fmt.Sprintf(format, args...))
}

func Internal(message string, cause error) error {
	return &Error{Kind: KindInternal, Message: message, Cause: cause}
}
