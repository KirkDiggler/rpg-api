package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// sightingIn is viewer's sighting of subject, nil when the viewer holds none.
func sightingIn(t *testing.T, h *acceptanceHarness, run, viewer, player, subject string) *sessionpb.Sighting {
	t.Helper()
	view, err := h.handler.GetView(auth.WithPlayerID(context.Background(), player), &sessionpb.GetViewRequest{Session: run, Member: viewer})
	require.NoError(t, err)
	for _, s := range view.GetSightings() {
		if s.GetSubject() == subject {
			return s
		}
	}
	return nil
}

// requireNoGhost requires viewer to hold no position for subject: either no
// sighting, or one that is known-of and placed nowhere (rpg-project#961).
func requireNoGhost(t *testing.T, h *acceptanceHarness, run, viewer, player, subject, why string) {
	t.Helper()
	s := sightingIn(t, h, run, viewer, player, subject)
	if s == nil {
		return
	}
	require.Empty(t, s.GetCurrentVia(), "%s: %s does not see %s live", why, viewer, subject)
	require.Nil(t, s.GetSeen().GetPosition(), "%s: %s holds no ghost of %s", why, viewer, subject)
}

// TestAcceptance_AWalkOutLeavesNoGhost is rpg-project#961 on a plain walk:
// bob, on the road beside alice, watches her walk round the gatehouse out of
// his sight. The cell he last saw her on is one he can still see, so he
// holds her as known-of and placed nowhere -- no faded figure standing on a
// spot he can see is empty. The same at each their own and at the castle's
// default shared view.
func TestAcceptance_AWalkOutLeavesNoGhost(t *testing.T) {
	for name, eachOwn := range map[string]bool{"each their own": true, "shared view": false} {
		t.Run(name, func(t *testing.T) {
			const run = "ghost-run"
			h := newAcceptanceHarnessWithDice(t, standAsideDice{})
			for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
				_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
					Character: &entities.Character{Data: armedFighter(c.id, c.player)},
				})
				require.NoError(t, err)
			}
			source := madKingRaw(t)
			if eachOwn {
				source = eachTheirOwn(t, source)
			}
			dungeon, err := sessionworld.Compile(source)
			require.NoError(t, err)
			out := h.launch(t, run, dungeon.Spec, seatAt("alice", 6, 15), seatAt("bob", 7, 15))
			require.Empty(t, out.Formed, "precondition: nobody fights")
			require.NotEmpty(t, sightingIn(t, h, run, "bob", "player-bob", "alice").GetCurrentVia(), "precondition: bob sees alice")

			walk(t, h, run, toTheCaptain(), "alice walks round the gatehouse")
			require.NotNil(t, sightingIn(t, h, run, "bob", "player-bob", "alice"), "bob still knows of alice")
			requireNoGhost(t, h, run, "bob", "player-bob", "alice", name)
		})
	}
}
