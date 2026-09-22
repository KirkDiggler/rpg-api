package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// driver_acceptance_test.go is what the PRODUCTION driver wiring does through
// the whole stack (rpg-project#465).
//
// WHAT USED TO BE HERE, AND WHY IT IS NOT. This file pinned rpg-toolkit#1725's
// rule A5: a MIND was named on a monster's definition, and the test followed
// that word from the skeleton's SetMind, through monster.Data, session's spawn,
// the persisted member record and the MonsterView, to the driver that read it.
// Every link in that chain is deleted. A creature's policy is the TABLE its
// author wrote for it, laid over the rulebook's default for its kind, loaded by
// its temperament — and a table is content a streamer can open, which is what
// the mind never was. The word is gone, so the test that carried it is gone
// rather than rewritten to assert something else; what a table does is proved
// in the toolkit that evaluates it, and what the CONTENT says is proved against
// the shipped files in internal/sessionworld.

// TestAcceptance_TwoSessionsEachDriveTheirOwnMonsters runs the PRODUCTION
// driver wiring -- no scripted driver, so sdk.Driver() is what answers -- for
// two sessions in one process.
//
// WHAT IT PROVES AND WHAT IT DOES NOT, because the difference matters. It
// proves the wiring serves every session rather than only the first: each
// session's EndTurn walks the clock across its own skeleton and comes back,
// and a driver that failed to resolve would fail the verb outright (the SDK
// never falls back).
//
// ONE DRIVER SERVES BOTH NOW, and that is the change rather than an accident
// of this test. sdk.Minded was stateful -- it remembered which Go preset each
// member had been given -- so two sessions in one process had to be handed two
// instances or two parties in the same tomb shared one skeleton's brain
// (rpg-toolkit#1734). The table remembers nothing: a creature's whole memory is
// its own holdings, which live in the world. So the per-session cache this test
// was written against is deleted, and what it now guards is that removing it
// did not break the second session.
//
// THE SAME MEMBER ID IN BOTH IS STILL THE POINT. Ids are authored per dungeon
// rather than minted per run, so the two runs below hold a skeleton of the same
// name -- which is exactly the collision a stateful driver keyed by member id
// could not survive and a stateless one never sees.
func TestAcceptance_TwoSessionsEachDriveTheirOwnMonsters(t *testing.T) {
	h := newAcceptanceHarness(t)

	for _, run := range []struct {
		session string
		player  string
		fighter string
	}{
		{"mind-run-a", "player-alice", "alice"},
		{"mind-run-b", "player-bob", "bob"},
	} {
		t.Run(run.session, func(t *testing.T) {
			ctx := auth.WithPlayerID(context.Background(), run.player)

			_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
				Character: &entities.Character{Data: armedFighter(run.fighter, run.player)},
			})
			require.NoError(t, err)

			_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
				Session: run.session, Encounter: run.session + "-encounter", World: buildOpenRoom(t, 12, 6),
			})
			require.NoError(t, err)

			_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
				Session: run.session, Member: run.fighter, Position: pbAt(3, 0),
			})
			require.NoError(t, err)
			inCombat(t, h.charRepo, run.fighter, 1)

			_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
				Session: run.session, ID: "skel-1", Ref: refs.Monsters.Skeleton().String(), Position: at(4, 0),
			})
			require.NoError(t, err)

			turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: run.session, Member: run.fighter})
			require.NoError(t, err)
			require.Equal(t, []string{run.fighter, "skel-1"}, turn.GetOrder(),
				"geometry gate: the fighter acts first, so ending the turn asks the skeleton once")

			_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
				Session: run.session, Member: run.fighter,
				DeclarationId: currentDeclarationID(
					ctx, t, h.handler, run.session, run.fighter, sessionpb.Verb_VERB_END_TURN),
			})
			require.NoError(t, err, "this session's driver answered for its own skeleton")

			after, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: run.session, Member: run.fighter})
			require.NoError(t, err)
			require.Equal(t, run.fighter, after.GetActive(),
				"the skeleton's turn was taken and ended, so the clock is back on the player")
		})
	}
}
