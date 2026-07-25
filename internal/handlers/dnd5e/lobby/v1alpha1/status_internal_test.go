// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package lobby

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// TestLobbyStatusError_Table covers every sentinel lobbyStatusError maps,
// plus the two Task E3 additions (ErrUnknownDungeonKey, *DisabledDungeonKeyError)
// and the default Internal fallback — one exhaustive table so adding the
// two new cases can't perturb any existing mapping.
func TestLobbyStatusError_Table(t *testing.T) {
	cause := errors.New("must have at least 2 rooms, got 1")
	disabledErr := &lobbyorch.DisabledDungeonKeyError{Key: "broken-dungeon", Cause: cause}

	cases := []struct {
		name        string
		err         error
		wantCode    codes.Code
		wantMessage string // exact message when set
		msgContains string // substring check when exact isn't meaningful
	}{
		{name: "lobby not found", err: lobbyorch.ErrLobbyNotFound, wantCode: codes.NotFound, wantMessage: "lobby not found"},
		{name: "character not found", err: lobbyorch.ErrCharacterNotFound, wantCode: codes.NotFound, wantMessage: "character not found"},
		{name: "player not in lobby", err: lobbyorch.ErrPlayerNotInLobby, wantCode: codes.PermissionDenied, wantMessage: "player is not a member of this lobby"},
		{name: "not host", err: lobbyorch.ErrNotHost, wantCode: codes.PermissionDenied, wantMessage: "only the host can start the encounter"},
		{name: "character ownership mismatch", err: lobbyorch.ErrCharacterOwnershipMismatch, wantCode: codes.PermissionDenied, wantMessage: "character_id does not belong to the authenticated player"},
		{name: "lobby already started", err: lobbyorch.ErrLobbyAlreadyStarted, wantCode: codes.FailedPrecondition, wantMessage: "lobby has already started"},
		{name: "lobby full", err: lobbyorch.ErrLobbyFull, wantCode: codes.FailedPrecondition, wantMessage: "lobby is full"},
		{name: "not all ready", err: lobbyorch.ErrNotAllReady, wantCode: codes.FailedPrecondition, wantMessage: "not all members are ready"},
		{name: "lobby not started", err: lobbyorch.ErrLobbyNotStarted, wantCode: codes.FailedPrecondition, wantMessage: "lobby has not started an encounter"},
		{name: "encounter already ended", err: lobbyorch.ErrEncounterAlreadyEnded, wantCode: codes.FailedPrecondition, wantMessage: "encounter has already ended"},

		// Task E3 additions.
		{name: "unknown dungeon key", err: lobbyorch.ErrUnknownDungeonKey, wantCode: codes.NotFound, wantMessage: "unknown dungeon key"},
		{
			name: "disabled content key carries the CAUSE, not Error()'s wrapped prefix",
			err:  disabledErr, wantCode: codes.InvalidArgument, wantMessage: cause.Error(),
		},
		{
			// nil-Cause guard: DisabledDungeonKeyError is an exported type --
			// a future construction site could build one with Cause left
			// nil. Calling Cause.Error() on a nil error interface panics; a
			// handler panicking on a malformed error is worse than falling
			// back to Error()'s own wrapped message, so that's the fallback.
			name: "disabled content key with nil Cause falls back to Error(), never panics",
			err:  &lobbyorch.DisabledDungeonKeyError{Key: "no-cause"}, wantCode: codes.InvalidArgument,
			wantMessage: (&lobbyorch.DisabledDungeonKeyError{Key: "no-cause"}).Error(),
		},

		{name: "unclassified error falls through to Internal", err: errors.New("boom"), wantCode: codes.Internal, msgContains: "boom"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := lobbyStatusError(tc.err)
			st, ok := status.FromError(got)
			require.True(t, ok, "lobbyStatusError must always return a gRPC status error")
			assert.Equal(t, tc.wantCode, st.Code())
			if tc.wantMessage != "" {
				assert.Equal(t, tc.wantMessage, st.Message())
			}
			if tc.msgContains != "" {
				assert.Contains(t, st.Message(), tc.msgContains)
			}
		})
	}
}

// TestLobbyStatusError_DisabledContentKey_NeverLeaksOrchestratorPrefix is a
// dedicated regression pin (not just a table row): the client-facing
// message must be exactly the stored validation cause, never
// DisabledDungeonKeyError.Error()'s own "lobby orchestrator: dungeon key
// ... is disabled:" wrapping prefix.
func TestLobbyStatusError_DisabledContentKey_NeverLeaksOrchestratorPrefix(t *testing.T) {
	cause := errors.New("must have at least 2 rooms, got 1")
	err := &lobbyorch.DisabledDungeonKeyError{Key: "broken-dungeon", Cause: cause}

	got := lobbyStatusError(err)
	st, ok := status.FromError(got)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, cause.Error(), st.Message())
	assert.NotContains(t, st.Message(), "lobby orchestrator", "must never leak the internal error-taxonomy prefix to the client")
}
