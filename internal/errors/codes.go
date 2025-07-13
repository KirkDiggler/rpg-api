package errors

import "net/http"

// Code represents an error code
type Code string

// Error codes
const (
	CodeOK                 Code = "OK"
	CodeCanceled           Code = "CANCELED"
	CodeInvalidArgument    Code = "INVALID_ARGUMENT"
	CodeDeadlineExceeded   Code = "DEADLINE_EXCEEDED"
	CodeNotFound           Code = "NOT_FOUND"
	CodeAlreadyExists      Code = "ALREADY_EXISTS"
	CodePermissionDenied   Code = "PERMISSION_DENIED"
	CodeResourceExhausted  Code = "RESOURCE_EXHAUSTED"
	CodeFailedPrecondition Code = "FAILED_PRECONDITION"
	CodeAborted            Code = "ABORTED"
	CodeOutOfRange         Code = "OUT_OF_RANGE"
	CodeUnimplemented      Code = "UNIMPLEMENTED"
	CodeInternal           Code = "INTERNAL"
	CodeUnavailable        Code = "UNAVAILABLE"
	CodeDataLoss           Code = "DATA_LOSS"
	CodeUnauthenticated    Code = "UNAUTHENTICATED"
)

// String returns the string representation of the code
func (c Code) String() string {
	return string(c)
}

// HTTPStatus returns the corresponding HTTP status code
func (c Code) HTTPStatus() int {
	switch c {
	case CodeOK:
		return http.StatusOK
	case CodeCanceled:
		return http.StatusRequestTimeout
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeDeadlineExceeded:
		return http.StatusGatewayTimeout
	case CodeNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists:
		return http.StatusConflict
	case CodePermissionDenied:
		return http.StatusForbidden
	case CodeResourceExhausted:
		return http.StatusTooManyRequests
	case CodeFailedPrecondition:
		return http.StatusPreconditionFailed
	case CodeAborted:
		return http.StatusConflict
	case CodeOutOfRange:
		return http.StatusBadRequest
	case CodeUnimplemented:
		return http.StatusNotImplemented
	case CodeInternal:
		return http.StatusInternalServerError
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	case CodeDataLoss:
		return http.StatusInternalServerError
	case CodeUnauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
