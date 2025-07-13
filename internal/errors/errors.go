package errors

import (
	"errors"
	"fmt"
)

// Error represents a structured error with code, message, and metadata
type Error struct {
	Code    Code                   `json:"code"`
	Message string                 `json:"message"`
	Cause   error                  `json:"-"`
	Meta    map[string]interface{} `json:"meta,omitempty"`
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error
func (e *Error) Unwrap() error {
	return e.Cause
}

// Is checks if the target error is of the same type
func (e *Error) Is(target error) bool {
	var targetErr *Error
	if errors.As(target, &targetErr) {
		return e.Code == targetErr.Code
	}
	return false
}

// WithMeta adds metadata to the error
func (e *Error) WithMeta(key string, value interface{}) *Error {
	if e.Meta == nil {
		e.Meta = make(map[string]interface{})
	}
	e.Meta[key] = value
	return e
}

// WithMetaMap adds multiple metadata entries
func (e *Error) WithMetaMap(meta map[string]interface{}) *Error {
	if e.Meta == nil {
		e.Meta = make(map[string]interface{})
	}
	for k, v := range meta {
		e.Meta[k] = v
	}
	return e
}

// New creates a new error with the given code and message
func New(code Code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Newf creates a new error with a formatted message
func Newf(code Code, format string, args ...interface{}) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// Wrap wraps an existing error, preserving its code if it's an Error
func Wrap(err error, message string) *Error {
	if err == nil {
		return nil
	}

	var existingErr *Error
	if errors.As(err, &existingErr) {
		return &Error{
			Code:    existingErr.Code,
			Message: message,
			Cause:   err,
			Meta:    existingErr.Meta,
		}
	}

	return &Error{
		Code:    CodeInternal,
		Message: message,
		Cause:   err,
	}
}

// Wrapf wraps an error with a formatted message
func Wrapf(err error, format string, args ...interface{}) *Error {
	return Wrap(err, fmt.Sprintf(format, args...))
}

// WrapWithCode wraps an error with a specific code
func WrapWithCode(err error, code Code, message string) *Error {
	if err == nil {
		return nil
	}

	var existingErr *Error
	meta := make(map[string]interface{})
	if errors.As(err, &existingErr) && existingErr.Meta != nil {
		for k, v := range existingErr.Meta {
			meta[k] = v
		}
	}

	return &Error{
		Code:    code,
		Message: message,
		Cause:   err,
		Meta:    meta,
	}
}

// WrapWithCodef wraps an error with a specific code and formatted message
func WrapWithCodef(err error, code Code, format string, args ...interface{}) *Error {
	return WrapWithCode(err, code, fmt.Sprintf(format, args...))
}

// Constructor functions for common error types

// NotFound creates a not found error
func NotFound(message string) *Error {
	return New(CodeNotFound, message)
}

// NotFoundf creates a not found error with formatted message
func NotFoundf(format string, args ...interface{}) *Error {
	return Newf(CodeNotFound, format, args...)
}

// InvalidArgument creates an invalid argument error
func InvalidArgument(message string) *Error {
	return New(CodeInvalidArgument, message)
}

// InvalidArgumentf creates an invalid argument error with formatted message
func InvalidArgumentf(format string, args ...interface{}) *Error {
	return Newf(CodeInvalidArgument, format, args...)
}

// AlreadyExists creates an already exists error
func AlreadyExists(message string) *Error {
	return New(CodeAlreadyExists, message)
}

// AlreadyExistsf creates an already exists error with formatted message
func AlreadyExistsf(format string, args ...interface{}) *Error {
	return Newf(CodeAlreadyExists, format, args...)
}

// PermissionDenied creates a permission denied error
func PermissionDenied(message string) *Error {
	return New(CodePermissionDenied, message)
}

// PermissionDeniedf creates a permission denied error with formatted message
func PermissionDeniedf(format string, args ...interface{}) *Error {
	return Newf(CodePermissionDenied, format, args...)
}

// Internal creates an internal error
func Internal(message string) *Error {
	return New(CodeInternal, message)
}

// Internalf creates an internal error with formatted message
func Internalf(format string, args ...interface{}) *Error {
	return Newf(CodeInternal, format, args...)
}

// Unavailable creates an unavailable error
func Unavailable(message string) *Error {
	return New(CodeUnavailable, message)
}

// Unavailablef creates an unavailable error with formatted message
func Unavailablef(format string, args ...interface{}) *Error {
	return Newf(CodeUnavailable, format, args...)
}

// Unauthenticated creates an unauthenticated error
func Unauthenticated(message string) *Error {
	return New(CodeUnauthenticated, message)
}

// Unauthenticatedf creates an unauthenticated error with formatted message
func Unauthenticatedf(format string, args ...interface{}) *Error {
	return Newf(CodeUnauthenticated, format, args...)
}

// ResourceExhausted creates a resource exhausted error
func ResourceExhausted(message string) *Error {
	return New(CodeResourceExhausted, message)
}

// ResourceExhaustedf creates a resource exhausted error with formatted message
func ResourceExhaustedf(format string, args ...interface{}) *Error {
	return Newf(CodeResourceExhausted, format, args...)
}

// FailedPrecondition creates a failed precondition error
func FailedPrecondition(message string) *Error {
	return New(CodeFailedPrecondition, message)
}

// FailedPreconditionf creates a failed precondition error with formatted message
func FailedPreconditionf(format string, args ...interface{}) *Error {
	return Newf(CodeFailedPrecondition, format, args...)
}

// Aborted creates an aborted error
func Aborted(message string) *Error {
	return New(CodeAborted, message)
}

// Abortedf creates an aborted error with formatted message
func Abortedf(format string, args ...interface{}) *Error {
	return Newf(CodeAborted, format, args...)
}

// OutOfRange creates an out of range error
func OutOfRange(message string) *Error {
	return New(CodeOutOfRange, message)
}

// OutOfRangef creates an out of range error with formatted message
func OutOfRangef(format string, args ...interface{}) *Error {
	return Newf(CodeOutOfRange, format, args...)
}

// Unimplemented creates an unimplemented error
func Unimplemented(message string) *Error {
	return New(CodeUnimplemented, message)
}

// Unimplementedf creates an unimplemented error with formatted message
func Unimplementedf(format string, args ...interface{}) *Error {
	return Newf(CodeUnimplemented, format, args...)
}

// DataLoss creates a data loss error
func DataLoss(message string) *Error {
	return New(CodeDataLoss, message)
}

// DataLossf creates a data loss error with formatted message
func DataLossf(format string, args ...interface{}) *Error {
	return Newf(CodeDataLoss, format, args...)
}

// Canceled creates a canceled error
func Canceled(message string) *Error {
	return New(CodeCanceled, message)
}

// Canceledf creates a canceled error with formatted message
func Canceledf(format string, args ...interface{}) *Error {
	return Newf(CodeCanceled, format, args...)
}

// DeadlineExceeded creates a deadline exceeded error
func DeadlineExceeded(message string) *Error {
	return New(CodeDeadlineExceeded, message)
}

// DeadlineExceededf creates a deadline exceeded error with formatted message
func DeadlineExceededf(format string, args ...interface{}) *Error {
	return Newf(CodeDeadlineExceeded, format, args...)
}
