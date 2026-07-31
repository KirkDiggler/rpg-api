package encounter

// out_of_range_mapping_internal_test.go — rpg-api#747: white-box tests
// proving the toolkit's ErrOutOfRange sentinel (rpg-toolkit#864's action
// range/reach gate) maps to codes.FailedPrecondition, not the generic
// codes.Internal fallback, at all three status-mapping call sites
// (takeActionStatusError, endTurnStatusError, submitCheckStatusError).
//
// Package encounter (not encounter_test) so these can call the unexported
// mapper functions directly — matching translate_move_internal_test.go's
// precedent for white-box tests of private functions in this package.
//
// Each error is wrapped (not passed bare) to also prove the mapping survives
// realistic wrapping: takeActionStatusError/submitCheckStatusError see
// ErrOutOfRange surfaced unwrapped by their orchestrators (a single %w), but
// endTurnStatusError sees it behind driveNPCChain's double-%w ErrNPCAct wrap
// (fmt.Errorf("%w %q: %w", ErrNPCAct, ..., actErr)) — Go's errors.Is
// traverses every wrapped error in the tree, not just the first %w, so the
// more specific ErrOutOfRange case still matches ahead of the generic
// ErrNPCAct/Internal fallback. See endTurnStatusError's doc comment for why
// this path is defense-in-depth rather than a live bug today: toolkit#868's
// gate-review fix made an out-of-reach NPCAct target a no-op (skip), not an
// error, so ErrOutOfRange shouldn't actually propagate out of NPCAct anymore
// — this test guards against a future regression reintroducing it.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

func TestTakeActionStatusError_OutOfRange_FailedPrecondition(t *testing.T) {
	err := fmt.Errorf("target out of range: 3 hexes, reach 1: %w", tkenc.ErrOutOfRange)
	mapped := takeActionStatusError(err)
	st, ok := status.FromError(mapped)
	require.True(t, ok)
	require.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestEndTurnStatusError_OutOfRange_FailedPrecondition(t *testing.T) {
	// Mirrors driveNPCChain's actual wrap shape: ErrNPCAct AND the original
	// toolkit error are both preserved via a double-%w fmt.Errorf.
	rangeErr := fmt.Errorf("target out of range: 2 hexes, reach 1: %w", tkenc.ErrOutOfRange)
	wrapped := fmt.Errorf("%w %q: %w", encounterorch.ErrNPCAct, "goblin-1", rangeErr)

	require.True(t, errors.Is(wrapped, encounterorch.ErrNPCAct), "test premise: the error is also ErrNPCAct")
	require.True(t, errors.Is(wrapped, tkenc.ErrOutOfRange), "test premise: ErrOutOfRange survives the wrap")

	mapped := endTurnStatusError(wrapped)
	st, ok := status.FromError(mapped)
	require.True(t, ok)
	require.Equal(t, codes.FailedPrecondition, st.Code(),
		"ErrOutOfRange must be classified ahead of the generic ErrNPCAct/Internal fallback")
}

func TestSubmitCheckStatusError_OutOfRange_FailedPrecondition(t *testing.T) {
	err := fmt.Errorf("door %q: target out of range: 4 hexes, reach 1: %w", "door-1", tkenc.ErrOutOfRange)
	mapped := submitCheckStatusError(err)
	st, ok := status.FromError(mapped)
	require.True(t, ok)
	require.Equal(t, codes.FailedPrecondition, st.Code())
}
