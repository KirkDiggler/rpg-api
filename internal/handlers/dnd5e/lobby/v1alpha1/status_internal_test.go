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
// plus the default Internal fallback — one exhaustive table so adding a new
// case can't perturb any existing mapping.
func TestLobbyStatusError_Table(t *testing.T) {
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
