// Package errors provides a comprehensive error handling solution for the rpg-api project.
//
// This package is inspired by the goaterr pattern and provides:
//   - Structured errors with codes, messages, and metadata
//   - Seamless gRPC integration with bidirectional conversion
//   - User-friendly error messages
//   - Error context preservation through wrapping
//   - Validation error helpers
//   - Type-safe error checking
//
// # Basic Usage
//
// Creating errors:
//
//	err := errors.NotFound("character not found")
//	err := errors.InvalidArgumentf("invalid ability score: %d", score)
//
// Adding metadata:
//
//	err := errors.NotFound("character not found").
//	    WithMeta("character_id", charID).
//	    WithMeta("user_id", userID)
//
// Wrapping errors:
//
//	if err := repo.Get(id); err != nil {
//	    return errors.Wrap(err, "failed to get character")
//	}
//
// Changing error semantics:
//
//	if err := db.Query(); err != nil {
//	    if isNotFound(err) {
//	        return errors.WrapWithCode(err, errors.CodeNotFound, "character not found")
//	    }
//	    return errors.Wrap(err, "database error")
//	}
//
// # Error Checking
//
// Type checking:
//
//	if errors.IsNotFound(err) {
//	    // Handle not found case
//	}
//
// Extracting information:
//
//	code := errors.GetCode(err)
//	message := errors.GetMessage(err)
//	meta := errors.GetMeta(err)
//
// # Validation Errors
//
// Using the validation builder:
//
//	vb := errors.NewValidationBuilder()
//	errors.ValidateRequired("name", input.Name, vb)
//	errors.ValidateRange("level", input.Level, 1, 20, vb)
//	if err := vb.Build(); err != nil {
//	    return err
//	}
//
// # gRPC Integration
//
// Converting to gRPC:
//
//	func (s *Server) GetCharacter(ctx context.Context, req *pb.GetCharacterRequest) (*pb.Character, error) {
//	    char, err := s.service.GetCharacter(ctx, req.Id)
//	    if err != nil {
//	        return nil, errors.ToGRPCError(err)
//	    }
//	    return char.ToProto(), nil
//	}
//
// Converting from gRPC:
//
//	char, err := client.GetCharacter(ctx, req)
//	if err != nil {
//	    return nil, errors.FromGRPCError(err)
//	}
//
// # Layer-Specific Guidelines
//
// Repository layer:
//   - Return domain-specific errors (NotFound, AlreadyExists)
//   - Include relevant IDs in metadata
//   - Wrap database errors with context
//
// Service/Orchestrator layer:
//   - Validate inputs and return InvalidArgument errors
//   - Check preconditions and return FailedPrecondition errors
//   - Wrap repository errors with business context
//
// Handler layer:
//   - Convert errors to gRPC format
//   - Extract user-friendly messages
//   - Log internal errors for debugging
//
// # Error Codes
//
// The following error codes are available:
//   - NotFound: Resource not found
//   - InvalidArgument: Invalid input provided
//   - AlreadyExists: Resource already exists
//   - PermissionDenied: Insufficient permissions
//   - Internal: Internal server error
//   - Unavailable: Service temporarily unavailable
//   - Unauthenticated: Authentication required
//   - ResourceExhausted: Rate limit or quota exceeded
//   - FailedPrecondition: Operation requirements not met
//   - Aborted: Operation aborted
//   - OutOfRange: Value out of valid range
//   - Unimplemented: Feature not implemented
//   - DataLoss: Unrecoverable data loss
//   - Canceled: Operation canceled
//   - DeadlineExceeded: Operation timeout
package errors
