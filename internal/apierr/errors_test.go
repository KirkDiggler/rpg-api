package apierr_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
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
		code     apierr.Code
		message  string
		expected string
	}{
		{
			name:     "not found error",
			code:     apierr.CodeNotFound,
			message:  "character not found",
			expected: "NOT_FOUND: character not found",
		},
		{
			name:     "invalid argument error",
			code:     apierr.CodeInvalidArgument,
			message:  "invalid input",
			expected: "INVALID_ARGUMENT: invalid input",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			err := apierr.New(tc.code, tc.message)
			s.Assert().Equal(tc.expected, err.Error())
			s.Assert().Equal(tc.code, err.Code)
			s.Assert().Equal(tc.message, err.Message)
		})
	}
}

func (s *ErrorsTestSuite) TestErrorWithMeta() {
	err := apierr.NotFound("character not found").
		WithMeta("character_id", "123").
		WithMeta("user_id", "456")

	s.Assert().Equal("123", err.Meta["character_id"])
	s.Assert().Equal("456", err.Meta["user_id"])

	// Test WithMetaMap
	err2 := apierr.Internal("server error").
		WithMetaMap(map[string]interface{}{
			"request_id": "abc",
			"trace_id":   "xyz",
		})

	s.Assert().Equal("abc", err2.Meta["request_id"])
	s.Assert().Equal("xyz", err2.Meta["trace_id"])
}

func (s *ErrorsTestSuite) TestWrap() {
	baseErr := fmt.Errorf("database connection failed")
	wrapped := apierr.Wrap(baseErr, "failed to get character")

	s.Assert().Equal(apierr.CodeInternal, wrapped.Code)
	s.Assert().Equal("failed to get character", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapPreservesCode() {
	baseErr := apierr.NotFound("record not found")
	wrapped := apierr.Wrap(baseErr, "character not found")

	s.Assert().Equal(apierr.CodeNotFound, wrapped.Code)
	s.Assert().Equal("character not found", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapDoesNotShareMetadata() {
	// Create base error with metadata
	baseErr := apierr.NotFound("record not found").
		WithMeta("original", "value")

	// Wrap the error
	wrapped := apierr.Wrap(baseErr, "wrapped error")

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
	wrapped := apierr.WrapWithCode(baseErr, apierr.CodeUnavailable, "service unavailable")

	s.Assert().Equal(apierr.CodeUnavailable, wrapped.Code)
	s.Assert().Equal("service unavailable", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapNil() {
	s.Assert().Nil(apierr.Wrap(nil, "should be nil"))
	s.Assert().Nil(apierr.WrapWithCode(nil, apierr.CodeNotFound, "should be nil"))
	s.Assert().Nil(apierr.Wrapf(nil, "should be nil: %s", "test"))
	s.Assert().Nil(apierr.WrapWithCodef(nil, apierr.CodeNotFound, "should be nil: %s", "test"))
}

func (s *ErrorsTestSuite) TestWrapfFormatting() {
	baseErr := fmt.Errorf("base error")
	wrapped := apierr.Wrapf(baseErr, "failed to process %s with id %d", "character", 123)

	s.Assert().Equal(apierr.CodeInternal, wrapped.Code)
	s.Assert().Equal("failed to process character with id 123", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapWithCodefFormatting() {
	baseErr := fmt.Errorf("timeout")
	wrapped := apierr.WrapWithCodef(
		baseErr,
		apierr.CodeDeadlineExceeded,
		"operation %s timed out after %d seconds",
		"save",
		30,
	)

	s.Assert().Equal(apierr.CodeDeadlineExceeded, wrapped.Code)
	s.Assert().Equal("operation save timed out after 30 seconds", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestConstructorFunctions() {
	testCases := []struct {
		name        string
		constructor func() *apierr.Error
		code        apierr.Code
	}{
		{"NotFound", func() *apierr.Error { return apierr.NotFound("test") }, apierr.CodeNotFound},
		{"InvalidArgument", func() *apierr.Error { return apierr.InvalidArgument("test") }, apierr.CodeInvalidArgument},
		{"AlreadyExists", func() *apierr.Error { return apierr.AlreadyExists("test") }, apierr.CodeAlreadyExists},
		{"PermissionDenied", func() *apierr.Error { return apierr.PermissionDenied("test") }, apierr.CodePermissionDenied},
		{"Internal", func() *apierr.Error { return apierr.Internal("test") }, apierr.CodeInternal},
		{"Unavailable", func() *apierr.Error { return apierr.Unavailable("test") }, apierr.CodeUnavailable},
		{"Unauthenticated", func() *apierr.Error { return apierr.Unauthenticated("test") }, apierr.CodeUnauthenticated},
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
	err := apierr.NotFoundf("character %s not found", "123")
	s.Assert().Equal(apierr.CodeNotFound, err.Code)
	s.Assert().Equal("character 123 not found", err.Message)

	err2 := apierr.InvalidArgumentf("invalid level: %d", 25)
	s.Assert().Equal(apierr.CodeInvalidArgument, err2.Code)
	s.Assert().Equal("invalid level: 25", err2.Message)
}

func (s *ErrorsTestSuite) TestErrorIs() {
	err1 := apierr.NotFound("test")
	err2 := apierr.NotFound("test")
	err3 := apierr.InvalidArgument("test")

	s.Assert().True(err1.Is(err2))
	s.Assert().False(err1.Is(err3))
}

func (s *ErrorsTestSuite) TestHelperFunctions() {
	notFoundErr := apierr.NotFound("test")
	invalidErr := apierr.InvalidArgument("test")
	wrappedErr := apierr.Wrap(notFoundErr, "wrapped")

	s.Assert().True(apierr.IsNotFound(notFoundErr))
	s.Assert().True(apierr.IsNotFound(wrappedErr))
	s.Assert().False(apierr.IsNotFound(invalidErr))

	s.Assert().True(apierr.IsInvalidArgument(invalidErr))
	s.Assert().False(apierr.IsInvalidArgument(notFoundErr))
}

func (s *ErrorsTestSuite) TestGetCode() {
	err := apierr.NotFound("test")
	wrapped := apierr.Wrap(err, "wrapped")

	s.Assert().Equal(apierr.CodeNotFound, apierr.GetCode(err))
	s.Assert().Equal(apierr.CodeNotFound, apierr.GetCode(wrapped))
	s.Assert().Equal(apierr.CodeInternal, apierr.GetCode(fmt.Errorf("standard error")))
	s.Assert().Equal(apierr.CodeOK, apierr.GetCode(nil))
}

func (s *ErrorsTestSuite) TestGetMeta() {
	err := apierr.NotFound("test").WithMeta("key", "value")
	wrapped := apierr.Wrap(err, "wrapped")

	s.Assert().Equal("value", apierr.GetMeta(err)["key"])
	s.Assert().Equal("value", apierr.GetMeta(wrapped)["key"])
	s.Assert().Nil(apierr.GetMeta(fmt.Errorf("standard error")))
}

func (s *ErrorsTestSuite) TestGetMessage() {
	err := apierr.NotFound("user friendly message")
	wrapped := apierr.Wrap(err, "wrapped message")
	stdErr := fmt.Errorf("standard error")

	s.Assert().Equal("user friendly message", apierr.GetMessage(err))
	s.Assert().Equal("wrapped message", apierr.GetMessage(wrapped))
	s.Assert().Equal("standard error", apierr.GetMessage(stdErr))
}

func (s *ErrorsTestSuite) TestHTTPStatus() {
	testCases := []struct {
		code     apierr.Code
		expected int
	}{
		{apierr.CodeOK, 200},
		{apierr.CodeNotFound, 404},
		{apierr.CodeInvalidArgument, 400},
		{apierr.CodeAlreadyExists, 409},
		{apierr.CodePermissionDenied, 403},
		{apierr.CodeUnauthenticated, 401},
		{apierr.CodeInternal, 500},
		{apierr.CodeUnavailable, 503},
	}

	for _, tc := range testCases {
		s.Run(string(tc.code), func() {
			s.Assert().Equal(tc.expected, tc.code.HTTPStatus())
		})
	}
}

func (s *ErrorsTestSuite) TestGRPCConversion() {
	// Test ToGRPCError
	err := apierr.NotFound("character not found").
		WithMeta("character_id", "123")

	grpcErr := apierr.ToGRPCError(err)
	st, ok := status.FromError(grpcErr)
	s.Require().True(ok)
	s.Assert().Equal(codes.NotFound, st.Code())
	s.Assert().Equal("character not found", st.Message())

	// Test FromGRPCError
	grpcErr2 := status.Error(codes.InvalidArgument, "invalid input")
	err2 := apierr.FromGRPCError(grpcErr2)
	s.Assert().Equal(apierr.CodeInvalidArgument, apierr.GetCode(err2))
	s.Assert().Equal("invalid input", apierr.GetMessage(err2))
}

func (s *ErrorsTestSuite) TestGRPCCodeMapping() {
	testCases := []struct {
		code     apierr.Code
		expected codes.Code
	}{
		{apierr.CodeNotFound, codes.NotFound},
		{apierr.CodeInvalidArgument, codes.InvalidArgument},
		{apierr.CodeAlreadyExists, codes.AlreadyExists},
		{apierr.CodePermissionDenied, codes.PermissionDenied},
		{apierr.CodeInternal, codes.Internal},
		{apierr.CodeUnavailable, codes.Unavailable},
		{apierr.CodeUnauthenticated, codes.Unauthenticated},
	}

	for _, tc := range testCases {
		s.Run(string(tc.code), func() {
			s.Assert().Equal(tc.expected, tc.code.GRPCCode())
		})
	}
}
