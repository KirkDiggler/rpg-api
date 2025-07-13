package errors

import (
	"errors"
)

// As is a wrapper around errors.As that works with our Error type
func As(err error, target **Error) bool {
	return errors.As(err, target)
}

// Is checks if an error is of a specific type
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// GetCode extracts the error code from an error
func GetCode(err error) Code {
	if err == nil {
		return CodeOK
	}

	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr.Code
	}

	return CodeInternal
}

// GetMeta extracts metadata from an error
func GetMeta(err error) map[string]interface{} {
	if err == nil {
		return nil
	}

	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr.Meta
	}

	return nil
}

// GetMessage extracts the user-friendly message from an error
func GetMessage(err error) string {
	if err == nil {
		return ""
	}

	var customErr *Error
	if errors.As(err, &customErr) {
		return customErr.Message
	}

	return err.Error()
}

// Type checking helpers

// IsNotFound checks if an error is a not found error
func IsNotFound(err error) bool {
	return GetCode(err) == CodeNotFound
}

// IsInvalidArgument checks if an error is an invalid argument error
func IsInvalidArgument(err error) bool {
	return GetCode(err) == CodeInvalidArgument
}

// IsAlreadyExists checks if an error is an already exists error
func IsAlreadyExists(err error) bool {
	return GetCode(err) == CodeAlreadyExists
}

// IsPermissionDenied checks if an error is a permission denied error
func IsPermissionDenied(err error) bool {
	return GetCode(err) == CodePermissionDenied
}

// IsInternal checks if an error is an internal error
func IsInternal(err error) bool {
	return GetCode(err) == CodeInternal
}

// IsUnavailable checks if an error is an unavailable error
func IsUnavailable(err error) bool {
	return GetCode(err) == CodeUnavailable
}

// IsUnauthenticated checks if an error is an unauthenticated error
func IsUnauthenticated(err error) bool {
	return GetCode(err) == CodeUnauthenticated
}

// IsResourceExhausted checks if an error is a resource exhausted error
func IsResourceExhausted(err error) bool {
	return GetCode(err) == CodeResourceExhausted
}

// IsFailedPrecondition checks if an error is a failed precondition error
func IsFailedPrecondition(err error) bool {
	return GetCode(err) == CodeFailedPrecondition
}

// IsAborted checks if an error is an aborted error
func IsAborted(err error) bool {
	return GetCode(err) == CodeAborted
}

// IsOutOfRange checks if an error is an out of range error
func IsOutOfRange(err error) bool {
	return GetCode(err) == CodeOutOfRange
}

// IsUnimplemented checks if an error is an unimplemented error
func IsUnimplemented(err error) bool {
	return GetCode(err) == CodeUnimplemented
}

// IsDataLoss checks if an error is a data loss error
func IsDataLoss(err error) bool {
	return GetCode(err) == CodeDataLoss
}

// IsCanceled checks if an error is a canceled error
func IsCanceled(err error) bool {
	return GetCode(err) == CodeCanceled
}

// IsDeadlineExceeded checks if an error is a deadline exceeded error
func IsDeadlineExceeded(err error) bool {
	return GetCode(err) == CodeDeadlineExceeded
}
