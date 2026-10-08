// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package session_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// TestAcceptance_OneUnreadableSheetRefusesAffordNamingThatSheet pins how a
// session with one sheet it cannot read reaches the wire (rpg-project#538,
// rpg-toolkit#1973): the session answers every member's speed and sight from
// the sheets at use, so bob's unreadable sheet refuses ALICE's Afford too.
// The refusal is INTERNAL (session.ErrBadCharacter is stored content the host
// wrote, not a caller mistake) and names bob, so the host can repair the named
// sheet. No fallback: alice is not offered a partial menu.
func TestAcceptance_OneUnreadableSheetRefusesAffordNamingThatSheet(t *testing.T) {
	alice := dualWieldingFighter(t, "alice", "player-alice", false)
	h, aliceCtx := adjacentOffHandFight(t, alice)

	bob := dualWieldingFighter(t, "bob", "player-bob", false)
	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: bob},
	})
	require.NoError(t, err)
	bobCtx := auth.WithPlayerID(context.Background(), bob.PlayerID)
	_, err = h.handler.Join(bobCtx, &sessionpb.JoinRequest{
		Session: "off-hand-run", Member: bob.ID, Position: pbAt(17, 3),
	})
	require.NoError(t, err)

	_, err = h.handler.Afford(aliceCtx, &sessionpb.AffordRequest{Session: "off-hand-run", Member: "alice"})
	require.NoError(t, err, "with every sheet readable, alice is offered her turn")

	// A condition no catalog knows: the stored sheet parses as JSON and
	// cannot be loaded.
	bob.Conditions = []json.RawMessage{json.RawMessage(`{"ref":{"module":"dnd5e","type":"conditions","id":"no-such-condition"}}`)}
	_, err = h.charRepo.Update(context.Background(), characterrepo.UpdateInput{
		Character: &entities.Character{Data: bob},
	})
	require.NoError(t, err)

	out, err := h.handler.Afford(aliceCtx, &sessionpb.AffordRequest{Session: "off-hand-run", Member: "alice"})
	require.Nil(t, out, "no partial menu")
	st, ok := status.FromError(err)
	require.True(t, ok, "%v", err)
	require.Equal(t, codes.Internal, st.Code(), st.Message())
	require.Contains(t, st.Message(), `"bob"`, "the refusal names the sheet to repair")
}
