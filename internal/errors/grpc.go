package errors

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ToGRPCError converts an error to a gRPC status error
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}

	// Check if it's already a gRPC status error
	if _, ok := status.FromError(err); ok {
		return err
	}

	// Check if it's our custom error
	var customErr *Error
	if As(err, &customErr) {
		st := status.New(customErr.Code.GRPCCode(), customErr.Message)

		// Add metadata if present
		if len(customErr.Meta) > 0 {
			details := &ErrorDetails{
				Code:    string(customErr.Code),
				Message: customErr.Message,
				Meta:    customErr.Meta,
			}
			st, _ = st.WithDetails(details)
		}

		return st.Err()
	}

	// Default to internal error
	return status.Error(codes.Internal, err.Error())
}

// FromGRPCError converts a gRPC error to our custom error
func FromGRPCError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	// Map gRPC code to our code
	code := grpcCodeToCode(st.Code())

	// Create base error
	customErr := &Error{
		Code:    code,
		Message: st.Message(),
	}

	// Extract details if present
	for _, detail := range st.Details() {
		if errDetails, ok := detail.(*ErrorDetails); ok {
			customErr.Meta = errDetails.Meta
			break
		}
	}

	return customErr
}

// GRPCStatus returns the gRPC status for any error
func GRPCStatus(err error) *status.Status {
	if err == nil {
		return status.New(codes.OK, "")
	}

	// Check if it's already a gRPC status
	if st, ok := status.FromError(err); ok {
		return st
	}

	// Check if it's our custom error
	var customErr *Error
	if As(err, &customErr) {
		return status.New(customErr.Code.GRPCCode(), customErr.Message)
	}

	// Default to internal error
	return status.New(codes.Internal, err.Error())
}

// GRPCCode returns the corresponding gRPC code
func (c Code) GRPCCode() codes.Code {
	switch c {
	case CodeOK:
		return codes.OK
	case CodeCanceled:
		return codes.Canceled
	case CodeInvalidArgument:
		return codes.InvalidArgument
	case CodeDeadlineExceeded:
		return codes.DeadlineExceeded
	case CodeNotFound:
		return codes.NotFound
	case CodeAlreadyExists:
		return codes.AlreadyExists
	case CodePermissionDenied:
		return codes.PermissionDenied
	case CodeResourceExhausted:
		return codes.ResourceExhausted
	case CodeFailedPrecondition:
		return codes.FailedPrecondition
	case CodeAborted:
		return codes.Aborted
	case CodeOutOfRange:
		return codes.OutOfRange
	case CodeUnimplemented:
		return codes.Unimplemented
	case CodeInternal:
		return codes.Internal
	case CodeUnavailable:
		return codes.Unavailable
	case CodeDataLoss:
		return codes.DataLoss
	case CodeUnauthenticated:
		return codes.Unauthenticated
	default:
		return codes.Unknown
	}
}

// grpcCodeToCode converts a gRPC code to our error code
func grpcCodeToCode(grpcCode codes.Code) Code {
	switch grpcCode {
	case codes.OK:
		return CodeOK
	case codes.Canceled:
		return CodeCanceled
	case codes.InvalidArgument:
		return CodeInvalidArgument
	case codes.DeadlineExceeded:
		return CodeDeadlineExceeded
	case codes.NotFound:
		return CodeNotFound
	case codes.AlreadyExists:
		return CodeAlreadyExists
	case codes.PermissionDenied:
		return CodePermissionDenied
	case codes.ResourceExhausted:
		return CodeResourceExhausted
	case codes.FailedPrecondition:
		return CodeFailedPrecondition
	case codes.Aborted:
		return CodeAborted
	case codes.OutOfRange:
		return CodeOutOfRange
	case codes.Unimplemented:
		return CodeUnimplemented
	case codes.Internal:
		return CodeInternal
	case codes.Unavailable:
		return CodeUnavailable
	case codes.DataLoss:
		return CodeDataLoss
	case codes.Unauthenticated:
		return CodeUnauthenticated
	default:
		return CodeInternal
	}
}

// ErrorDetails is a protobuf-compatible structure for error metadata
// This would typically be generated from a .proto file
type ErrorDetails struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Meta    map[string]interface{} `json:"meta,omitempty"`
}

// Reset implements proto.Message (stub for now)
func (e *ErrorDetails) Reset() {}

// String implements proto.Message (stub for now)
func (e *ErrorDetails) String() string {
	return e.Message
}

// ProtoMessage implements proto.Message (stub for now)
func (e *ErrorDetails) ProtoMessage() {}
