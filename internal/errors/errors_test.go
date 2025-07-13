package errors_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/errors"
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
		code     errors.Code
		message  string
		expected string
	}{
		{
			name:     "not found error",
			code:     errors.CodeNotFound,
			message:  "character not found",
			expected: "NOT_FOUND: character not found",
		},
		{
			name:     "invalid argument error",
			code:     errors.CodeInvalidArgument,
			message:  "invalid input",
			expected: "INVALID_ARGUMENT: invalid input",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			err := errors.New(tc.code, tc.message)
			s.Assert().Equal(tc.expected, err.Error())
			s.Assert().Equal(tc.code, err.Code)
			s.Assert().Equal(tc.message, err.Message)
		})
	}
}

func (s *ErrorsTestSuite) TestErrorWithMeta() {
	err := errors.NotFound("character not found").
		WithMeta("character_id", "123").
		WithMeta("user_id", "456")

	s.Assert().Equal("123", err.Meta["character_id"])
	s.Assert().Equal("456", err.Meta["user_id"])

	// Test WithMetaMap
	err2 := errors.Internal("server error").
		WithMetaMap(map[string]interface{}{
			"request_id": "abc",
			"trace_id":   "xyz",
		})

	s.Assert().Equal("abc", err2.Meta["request_id"])
	s.Assert().Equal("xyz", err2.Meta["trace_id"])
}

func (s *ErrorsTestSuite) TestWrap() {
	baseErr := fmt.Errorf("database connection failed")
	wrapped := errors.Wrap(baseErr, "failed to get character")

	s.Assert().Equal(errors.CodeInternal, wrapped.Code)
	s.Assert().Equal("failed to get character", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapPreservesCode() {
	baseErr := errors.NotFound("record not found")
	wrapped := errors.Wrap(baseErr, "character not found")

	s.Assert().Equal(errors.CodeNotFound, wrapped.Code)
	s.Assert().Equal("character not found", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapWithCode() {
	baseErr := fmt.Errorf("connection timeout")
	wrapped := errors.WrapWithCode(baseErr, errors.CodeUnavailable, "service unavailable")

	s.Assert().Equal(errors.CodeUnavailable, wrapped.Code)
	s.Assert().Equal("service unavailable", wrapped.Message)
	s.Assert().Equal(baseErr, wrapped.Unwrap())
}

func (s *ErrorsTestSuite) TestWrapNil() {
	s.Assert().Nil(errors.Wrap(nil, "should be nil"))
	s.Assert().Nil(errors.WrapWithCode(nil, errors.CodeNotFound, "should be nil"))
}

func (s *ErrorsTestSuite) TestConstructorFunctions() {
	testCases := []struct {
		name        string
		constructor func() *errors.Error
		code        errors.Code
	}{
		{"NotFound", func() *errors.Error { return errors.NotFound("test") }, errors.CodeNotFound},
		{"InvalidArgument", func() *errors.Error { return errors.InvalidArgument("test") }, errors.CodeInvalidArgument},
		{"AlreadyExists", func() *errors.Error { return errors.AlreadyExists("test") }, errors.CodeAlreadyExists},
		{"PermissionDenied", func() *errors.Error { return errors.PermissionDenied("test") }, errors.CodePermissionDenied},
		{"Internal", func() *errors.Error { return errors.Internal("test") }, errors.CodeInternal},
		{"Unavailable", func() *errors.Error { return errors.Unavailable("test") }, errors.CodeUnavailable},
		{"Unauthenticated", func() *errors.Error { return errors.Unauthenticated("test") }, errors.CodeUnauthenticated},
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
	err := errors.NotFoundf("character %s not found", "123")
	s.Assert().Equal(errors.CodeNotFound, err.Code)
	s.Assert().Equal("character 123 not found", err.Message)

	err2 := errors.InvalidArgumentf("invalid level: %d", 25)
	s.Assert().Equal(errors.CodeInvalidArgument, err2.Code)
	s.Assert().Equal("invalid level: 25", err2.Message)
}

func (s *ErrorsTestSuite) TestErrorIs() {
	err1 := errors.NotFound("test")
	err2 := errors.NotFound("test")
	err3 := errors.InvalidArgument("test")

	s.Assert().True(err1.Is(err2))
	s.Assert().False(err1.Is(err3))
}

func (s *ErrorsTestSuite) TestHelperFunctions() {
	notFoundErr := errors.NotFound("test")
	invalidErr := errors.InvalidArgument("test")
	wrappedErr := errors.Wrap(notFoundErr, "wrapped")

	s.Assert().True(errors.IsNotFound(notFoundErr))
	s.Assert().True(errors.IsNotFound(wrappedErr))
	s.Assert().False(errors.IsNotFound(invalidErr))

	s.Assert().True(errors.IsInvalidArgument(invalidErr))
	s.Assert().False(errors.IsInvalidArgument(notFoundErr))
}

func (s *ErrorsTestSuite) TestGetCode() {
	err := errors.NotFound("test")
	wrapped := errors.Wrap(err, "wrapped")

	s.Assert().Equal(errors.CodeNotFound, errors.GetCode(err))
	s.Assert().Equal(errors.CodeNotFound, errors.GetCode(wrapped))
	s.Assert().Equal(errors.CodeInternal, errors.GetCode(fmt.Errorf("standard error")))
	s.Assert().Equal(errors.CodeOK, errors.GetCode(nil))
}

func (s *ErrorsTestSuite) TestGetMeta() {
	err := errors.NotFound("test").WithMeta("key", "value")
	wrapped := errors.Wrap(err, "wrapped")

	s.Assert().Equal("value", errors.GetMeta(err)["key"])
	s.Assert().Equal("value", errors.GetMeta(wrapped)["key"])
	s.Assert().Nil(errors.GetMeta(fmt.Errorf("standard error")))
}

func (s *ErrorsTestSuite) TestGetMessage() {
	err := errors.NotFound("user friendly message")
	wrapped := errors.Wrap(err, "wrapped message")
	stdErr := fmt.Errorf("standard error")

	s.Assert().Equal("user friendly message", errors.GetMessage(err))
	s.Assert().Equal("wrapped message", errors.GetMessage(wrapped))
	s.Assert().Equal("standard error", errors.GetMessage(stdErr))
}

func (s *ErrorsTestSuite) TestHTTPStatus() {
	testCases := []struct {
		code     errors.Code
		expected int
	}{
		{errors.CodeOK, 200},
		{errors.CodeNotFound, 404},
		{errors.CodeInvalidArgument, 400},
		{errors.CodeAlreadyExists, 409},
		{errors.CodePermissionDenied, 403},
		{errors.CodeUnauthenticated, 401},
		{errors.CodeInternal, 500},
		{errors.CodeUnavailable, 503},
	}

	for _, tc := range testCases {
		s.Run(string(tc.code), func() {
			s.Assert().Equal(tc.expected, tc.code.HTTPStatus())
		})
	}
}

func (s *ErrorsTestSuite) TestGRPCConversion() {
	// Test ToGRPCError
	err := errors.NotFound("character not found").
		WithMeta("character_id", "123")

	grpcErr := errors.ToGRPCError(err)
	st, ok := status.FromError(grpcErr)
	s.Require().True(ok)
	s.Assert().Equal(codes.NotFound, st.Code())
	s.Assert().Equal("character not found", st.Message())

	// Test FromGRPCError
	grpcErr2 := status.Error(codes.InvalidArgument, "invalid input")
	err2 := errors.FromGRPCError(grpcErr2)
	s.Assert().Equal(errors.CodeInvalidArgument, errors.GetCode(err2))
	s.Assert().Equal("invalid input", errors.GetMessage(err2))
}

func (s *ErrorsTestSuite) TestGRPCCodeMapping() {
	testCases := []struct {
		code     errors.Code
		expected codes.Code
	}{
		{errors.CodeNotFound, codes.NotFound},
		{errors.CodeInvalidArgument, codes.InvalidArgument},
		{errors.CodeAlreadyExists, codes.AlreadyExists},
		{errors.CodePermissionDenied, codes.PermissionDenied},
		{errors.CodeInternal, codes.Internal},
		{errors.CodeUnavailable, codes.Unavailable},
		{errors.CodeUnauthenticated, codes.Unauthenticated},
	}

	for _, tc := range testCases {
		s.Run(string(tc.code), func() {
			s.Assert().Equal(tc.expected, tc.code.GRPCCode())
		})
	}
}
