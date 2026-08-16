package sessionv1alpha1

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// TestStatusError_CoversEverySDKSentinel is the single test that enforces
// design rule 7: every exported sentinel in rulebooks/dnd5e/session/errors.go
// maps to a gRPC code, in this one table. If the SDK adds a sentinel, this
// test does not fail automatically -- see
// TestStatusError_UnmappedSentinelFallsBackToInternal for the safety net --
// but a new sentinel with no explicit case here should get one deliberately
// rather than silently riding the Internal default.
func TestStatusError_CoversEverySDKSentinel(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		// NOT_FOUND -- the caller named something that does not exist.
		{"ErrNoSession", sdk.ErrNoSession, codes.NotFound},
		{"ErrNoEncounter", sdk.ErrNoEncounter, codes.NotFound},
		{"ErrNoCharacter", sdk.ErrNoCharacter, codes.NotFound},
		{"ErrNoMember", sdk.ErrNoMember, codes.NotFound},
		{"ErrNoEnding", sdk.ErrNoEnding, codes.NotFound},
		{"ErrUnknownContent", sdk.ErrUnknownContent, codes.NotFound},
		{"ErrNoLoader", sdk.ErrNoLoader, codes.NotFound},
		{"ErrNoSheet", sdk.ErrNoSheet, codes.NotFound},

		// INVALID_ARGUMENT -- the request itself is malformed.
		{"ErrNilInput", sdk.ErrNilInput, codes.InvalidArgument},
		{"ErrEmptyPath", sdk.ErrEmptyPath, codes.InvalidArgument},
		{"ErrBrokenPath", sdk.ErrBrokenPath, codes.InvalidArgument},
		{"ErrNoConnection", sdk.ErrNoConnection, codes.InvalidArgument},
		{"ErrBadPosition", sdk.ErrBadPosition, codes.InvalidArgument},
		{"ErrNoRef", sdk.ErrNoRef, codes.InvalidArgument},
		{"ErrBadRef", sdk.ErrBadRef, codes.InvalidArgument},
		{"ErrNoMemberID", sdk.ErrNoMemberID, codes.InvalidArgument},
		{"ErrNoSessionID", sdk.ErrNoSessionID, codes.InvalidArgument},
		{"ErrNoEncounterID", sdk.ErrNoEncounterID, codes.InvalidArgument},
		{"ErrInvalidWorld", sdk.ErrInvalidWorld, codes.InvalidArgument},
		{"ErrNoCause", sdk.ErrNoCause, codes.InvalidArgument},

		// FAILED_PRECONDITION -- well-formed request, world state refuses it.
		{"ErrInBubble", sdk.ErrInBubble, codes.FailedPrecondition},
		{"ErrNotInFight", sdk.ErrNotInFight, codes.FailedPrecondition},
		{"ErrClosed", sdk.ErrClosed, codes.FailedPrecondition},
		{"ErrNotACharacter", sdk.ErrNotACharacter, codes.FailedPrecondition},
		{"ErrBadAttack", sdk.ErrBadAttack, codes.FailedPrecondition},

		// ALREADY_EXISTS
		{"ErrSessionExists", sdk.ErrSessionExists, codes.AlreadyExists},

		// OUT_OF_RANGE -- deterministic fix (resync from zero), not a state
		// to wait out, which is what sets this apart from FailedPrecondition.
		{"ErrStoryTrimmed", sdk.ErrStoryTrimmed, codes.OutOfRange},

		// UNAVAILABLE -- partial write; retry guidance rides the SaveReport.
		{"ErrSaveFailed", sdk.ErrSaveFailed, codes.Unavailable},

		// INTERNAL -- storage-side integrity problems, not a caller mistake.
		{"ErrBadRepository", sdk.ErrBadRepository, codes.Internal},
		{"ErrBadCharacter", sdk.ErrBadCharacter, codes.Internal},
		{"ErrInvalidSession", sdk.ErrInvalidSession, codes.Internal},
		{"ErrNilConfig", sdk.ErrNilConfig, codes.Internal},
		{"ErrIncompleteConfig", sdk.ErrIncompleteConfig, codes.Internal},

		// sdk.ErrNotFound is the SDK's repository-facing contract sentinel: the
		// Manager translates it into a caller-facing sentinel (ErrNoSession,
		// ErrNoEncounter, ...) before any verb returns, so it should never
		// reach here. Tested anyway so a regression that DID let it leak is
		// caught as Internal (a storage-layer signal), not silently
		// misread as some caller-facing code.
		{"ErrNotFound (defensive)", sdk.ErrNotFound, codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrapped exactly like the SDK wraps its own sentinels
			// ("verb: %w") so the table is proven against errors.Is chains,
			// not bare sentinel identity.
			wrapped := fmt.Errorf("move: step 1: %w", tt.err)
			got := statusError(wrapped)
			st, ok := status.FromError(got)
			require.True(t, ok, "statusError must always return a gRPC status error")
			require.Equal(t, tt.want, st.Code(), "sentinel %s", tt.name)
		})
	}

	require.Len(t, tests, sentinelCount,
		"every exported sentinel in rulebooks/dnd5e/session/errors.go must have a table row here; "+
			"update sentinelCount and add a row when the SDK adds or removes one")
}

// sentinelCount is the exact count of exported Err* sentinels in
// rulebooks/dnd5e/session/errors.go as of session v0.11.0 (verified directly
// against the pinned module, not a local checkout -- see PR discussion: an
// unmerged toolkit branch briefly made this look like 35 with an ErrNoCrossing
// that v0.11.0 does not actually have). Kept as a named constant (not a magic
// number in the assertion above) so a failing count change points straight at
// "the SDK's sentinel vocabulary moved" rather than requiring the reader to
// recount by hand.
const sentinelCount = 34

func TestStatusError_UnmappedSentinelFallsBackToInternal(t *testing.T) {
	unrecognized := fmt.Errorf("some future sentinel the table has not been updated for")
	got := statusError(unrecognized)
	st, ok := status.FromError(got)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())
}

func TestStatusError_Nil_ReturnsNil(t *testing.T) {
	require.NoError(t, statusError(nil))
}
