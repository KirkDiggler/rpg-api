package apierrors_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/apierrors"
)

type ErrorsTestSuite struct {
	suite.Suite
}

func TestErrorsSuite(t *testing.T) {
	suite.Run(t, new(ErrorsTestSuite))
}

func (s *ErrorsTestSuite) TestNewError() {
	testCases := []struct {
		name     string
		code     apierrors.Code
		message  string
		expected string
	}{
		{
			name:     "not found error",
			code:     apierrors.CodeNotFound,
			message:  "character not found",
			expected: "NOT_FOUND: character not found",
		},
		{
			name:     "invalid argument error",
			code:     apierrors.CodeInvalidArgument,
			message:  "invalid input",
			expected: "INVALID_ARGUMENT: invalid input",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			err := apierrors.New(tc.code, tc.message)
			s.Assert().Equal(tc.expected, err.Error())
			s.Assert().Equal(tc.code, err.Code)
			s.Assert().Equal(tc.message, err.Message)
		})
	}
}

func (s *ErrorsTestSuite) TestErrorWithMeta() {
	err := apierrors.NotFound("character not found").
		WithMeta("character_id", "123").
		WithMeta("user_id", "456")

	s.Assert().Equal("123", err.Meta["character_id"])
	s.Assert().Equal("456", err.Meta["user_id"])

	// Test WithMetaMap
	err2 := apierrors.Internal("server error").
		WithMetaMap(map[string]interface{}{
			"request_id": "abc",
			"trace_id":   "xyz",
		})

	s.Assert().Equal("abc", err2.Meta["request_id"])
	s.Assert().Equal("xyz", err2.Meta["trace_id"])
}

func (s *ErrorsTestSuite) TestWrap() {
	baseErr := fmt.Errorf("database connection failed")
	wrapped := apierrors.Wrap(baseErr, "failed to get character")

	s.Assert().Equal(apierrors.CodeInternal, wrapped.Code)
	s.Assert().Equal("failed to get character", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapPreservesCode() {
	baseErr := apierrors.NotFound("record not found")
	wrapped := apierrors.Wrap(baseErr, "character not found")

	s.Assert().Equal(apierrors.CodeNotFound, wrapped.Code)
	s.Assert().Equal("character not found", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapDoesNotShareMetadata() {
	// Create base error with metadata
	baseErr := apierrors.NotFound("record not found").
		WithMeta("original", "value")

	// Wrap the error
	wrapped := apierrors.Wrap(baseErr, "wrapped error")

	// Modify the wrapped error's metadata
	err1 := wrapped.WithMeta("wrapped", "data")
	s.Require().Equal(wrapped, err1) // WithMeta returns the same error
	err2 := wrapped.WithMeta("original", "modified")
	s.Require().Equal(wrapped, err2) // WithMeta returns the same error

	// Verify base error's metadata is unchanged
	s.Assert().Equal("value", baseErr.Meta["original"])
	s.Assert().Nil(baseErr.Meta["wrapped"])

	// Verify wrapped error has both metadata
	s.Assert().Equal("modified", wrapped.Meta["original"])
	s.Assert().Equal("data", wrapped.Meta["wrapped"])
}

func (s *ErrorsTestSuite) TestWrapWithCode() {
	baseErr := fmt.Errorf("connection timeout")
	wrapped := apierrors.WrapWithCode(baseErr, apierrors.CodeUnavailable, "service unavailable")

	s.Assert().Equal(apierrors.CodeUnavailable, wrapped.Code)
	s.Assert().Equal("service unavailable", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapNil() {
	s.Assert().Nil(apierrors.Wrap(nil, "should be nil"))
	s.Assert().Nil(apierrors.WrapWithCode(nil, apierrors.CodeNotFound, "should be nil"))
	s.Assert().Nil(apierrors.Wrapf(nil, "should be nil: %s", "test"))
	s.Assert().Nil(apierrors.WrapWithCodef(nil, apierrors.CodeNotFound, "should be nil: %s", "test"))
}

func (s *ErrorsTestSuite) TestWrapfFormatting() {
	baseErr := fmt.Errorf("base error")
	wrapped := apierrors.Wrapf(baseErr, "failed to process %s with id %d", "character", 123)

	s.Assert().Equal(apierrors.CodeInternal, wrapped.Code)
	s.Assert().Equal("failed to process character with id 123", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapWithCodefFormatting() {
	baseErr := fmt.Errorf("timeout")
	wrapped := apierrors.WrapWithCodef(
		baseErr,
		apierrors.CodeDeadlineExceeded,
		"operation %s timed out after %d seconds",
		"save",
		30,
	)

	s.Assert().Equal(apierrors.CodeDeadlineExceeded, wrapped.Code)
	s.Assert().Equal("operation save timed out after 30 seconds", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestConstructorFunctions() {
	testCases := []struct {
		name        string
		constructor func() *apierrors.Error
		code        apierrors.Code
	}{
		{"NotFound", func() *apierrors.Error { return apierrors.NotFound("test") }, apierrors.CodeNotFound},
		{"InvalidArgument", func() *apierrors.Error { return apierrors.InvalidArgument("test") }, apierrors.CodeInvalidArgument},
		{"AlreadyExists", func() *apierrors.Error { return apierrors.AlreadyExists("test") }, apierrors.CodeAlreadyExists},
		{"PermissionDenied", func() *apierrors.Error { return apierrors.PermissionDenied("test") }, apierrors.CodePermissionDenied},
		{"Internal", func() *apierrors.Error { return apierrors.Internal("test") }, apierrors.CodeInternal},
		{"Unavailable", func() *apierrors.Error { return apierrors.Unavailable("test") }, apierrors.CodeUnavailable},
		{"Unauthenticated", func() *apierrors.Error { return apierrors.Unauthenticated("test") }, apierrors.CodeUnauthenticated},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			err := tc.constructor()
			s.Assert().Equal(tc.code, err.Code)
			s.Assert().Equal("test", err.Message)
		})
	}
}

func (s *ErrorsTestSuite) TestFormattedConstructors() {
	err := apierrors.NotFoundf("character %s not found", "123")
	s.Assert().Equal(apierrors.CodeNotFound, err.Code)
	s.Assert().Equal("character 123 not found", err.Message)

	err2 := apierrors.InvalidArgumentf("invalid level: %d", 25)
	s.Assert().Equal(apierrors.CodeInvalidArgument, err2.Code)
	s.Assert().Equal("invalid level: 25", err2.Message)
}

func (s *ErrorsTestSuite) TestErrorIs() {
	err1 := apierrors.NotFound("test")
	err2 := apierrors.NotFound("test")
	err3 := apierrors.InvalidArgument("test")

	s.Assert().True(err1.Is(err2))
	s.Assert().False(err1.Is(err3))
}

func (s *ErrorsTestSuite) TestHelperFunctions() {
	notFoundErr := apierrors.NotFound("test")
	invalidErr := apierrors.InvalidArgument("test")
	wrappedErr := apierrors.Wrap(notFoundErr, "wrapped")

	s.Assert().True(apierrors.IsNotFound(notFoundErr))
	s.Assert().True(apierrors.IsNotFound(wrappedErr))
	s.Assert().False(apierrors.IsNotFound(invalidErr))

	s.Assert().True(apierrors.IsInvalidArgument(invalidErr))
	s.Assert().False(apierrors.IsInvalidArgument(notFoundErr))
}

func (s *ErrorsTestSuite) TestGetCode() {
	err := apierrors.NotFound("test")
	wrapped := apierrors.Wrap(err, "wrapped")

	s.Assert().Equal(apierrors.CodeNotFound, apierrors.GetCode(err))
	s.Assert().Equal(apierrors.CodeNotFound, apierrors.GetCode(wrapped))
	s.Assert().Equal(apierrors.CodeInternal, apierrors.GetCode(fmt.Errorf("standard error")))
	s.Assert().Equal(apierrors.CodeOK, apierrors.GetCode(nil))
}

func (s *ErrorsTestSuite) TestGetMeta() {
	err := apierrors.NotFound("test").WithMeta("key", "value")
	wrapped := apierrors.Wrap(err, "wrapped")

	s.Assert().Equal("value", apierrors.GetMeta(err)["key"])
	s.Assert().Equal("value", apierrors.GetMeta(wrapped)["key"])
	s.Assert().Nil(apierrors.GetMeta(fmt.Errorf("standard error")))
}

func (s *ErrorsTestSuite) TestGetMessage() {
	err := apierrors.NotFound("user friendly message")
	wrapped := apierrors.Wrap(err, "wrapped message")
	stdErr := fmt.Errorf("standard error")

	s.Assert().Equal("user friendly message", apierrors.GetMessage(err))
	s.Assert().Equal("wrapped message", apierrors.GetMessage(wrapped))
	s.Assert().Equal("standard error", apierrors.GetMessage(stdErr))
}

func (s *ErrorsTestSuite) TestHTTPStatus() {
	testCases := []struct {
		code     apierrors.Code
		expected int
	}{
		{apierrors.CodeOK, 200},
		{apierrors.CodeNotFound, 404},
		{apierrors.CodeInvalidArgument, 400},
		{apierrors.CodeAlreadyExists, 409},
		{apierrors.CodePermissionDenied, 403},
		{apierrors.CodeUnauthenticated, 401},
		{apierrors.CodeInternal, 500},
		{apierrors.CodeUnavailable, 503},
	}

	for _, tc := range testCases {
		s.Run(string(tc.code), func() {
			s.Assert().Equal(tc.expected, tc.code.HTTPStatus())
		})
	}
}

func (s *ErrorsTestSuite) TestGRPCConversion() {
	// Test ToGRPCError
	err := apierrors.NotFound("character not found").
		WithMeta("character_id", "123")

	grpcErr := apierrors.ToGRPCError(err)
	st, ok := status.FromError(grpcErr)
	s.Require().True(ok)
	s.Assert().Equal(codes.NotFound, st.Code())
	s.Assert().Equal("character not found", st.Message())

	// Test FromGRPCError
	grpcErr2 := status.Error(codes.InvalidArgument, "invalid input")
	err2 := apierrors.FromGRPCError(grpcErr2)
	s.Assert().Equal(apierrors.CodeInvalidArgument, apierrors.GetCode(err2))
	s.Assert().Equal("invalid input", apierrors.GetMessage(err2))
}

func (s *ErrorsTestSuite) TestGRPCCodeMapping() {
	testCases := []struct {
		code     apierrors.Code
		expected codes.Code
	}{
		{apierrors.CodeNotFound, codes.NotFound},
		{apierrors.CodeInvalidArgument, codes.InvalidArgument},
		{apierrors.CodeAlreadyExists, codes.AlreadyExists},
		{apierrors.CodePermissionDenied, codes.PermissionDenied},
		{apierrors.CodeInternal, codes.Internal},
		{apierrors.CodeUnavailable, codes.Unavailable},
		{apierrors.CodeUnauthenticated, codes.Unauthenticated},
	}

	for _, tc := range testCases {
		s.Run(string(tc.code), func() {
			s.Assert().Equal(tc.expected, tc.code.GRPCCode())
		})
	}
}
