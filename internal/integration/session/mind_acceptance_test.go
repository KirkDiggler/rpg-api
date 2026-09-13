package session_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	tkmonster "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// mindRecorder passes on every turn and keeps the view it was handed.
//
// A DRIVER IS THE ONLY PLACE THE MIND IS OBSERVABLE from this side of the
// stack: nothing on the wire carries it and no projection reports it -- it is
// a fact the composition hands the brain and nobody else, exactly as
// Targeting is. So the recorder stands where the shipped driver stands and
// reports what arrived there, which is the same value sdk.Minded reads to
// decide whose turn this monster spends.
type mindRecorder struct {
	mu    sync.Mutex
	views map[string]sdk.MonsterView
}

func newMindRecorder() *mindRecorder {
	return &mindRecorder{views: map[string]sdk.MonsterView{}}
}

func (r *mindRecorder) Act(view sdk.MonsterView) (sdk.TurnIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.views[view.Self] = view
	return sdk.Pass{}, nil
}

func (r *mindRecorder) viewOf(t *testing.T, member string) sdk.MonsterView {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	view, ok := r.views[member]
	require.True(t, ok, "the clock never landed on %q, so nothing was asked of its mind", member)
	return view
}

// TestAcceptance_SpawnedMonsterCarriesTheMindItsSheetNames is the API's half
// of rpg-toolkit#1725's rule A5: a mind is named on the monster DEFINITION,
// and what reaches the driver is that word and no other.
//
// This is worth a test at this layer because rpg-api builds no monster of its
// own -- it hands session.Spawn a ref and the toolkit's catalog does the rest
// (internal/orchestrators/lobby/start_encounter_session_stack.go does exactly
// this with the dungeon's authored ref). A test here fails if any link in
// that chain drops the word: the skeleton definition's SetMind, monster.Data's
// Mind, session's spawn reading sheet.Mind, the member record that persists
// it, or the MonsterView the composition fills before it asks.
//
// BOTH ANSWERS ARE PINNED, because "retaliator" alone would also pass if the
// seam hardcoded it. The goblin names no mind, and an empty Mind is what
// tells the shipped driver to fall back to the basic brain.
func TestAcceptance_SpawnedMonsterCarriesTheMindItsSheetNames(t *testing.T) {
	const sessionID = "mind-run"

	recorder := newMindRecorder()
	h := newAcceptanceHarnessWith(t, hittingDice{}, recorder)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	// The lobby's job, in-process (design rule 5: creation is the lobby's).
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: sessionID, Encounter: "room-encounter", World: buildOpenRoom(t, 12, 6),
	})
	require.NoError(t, err)

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: sessionID, Member: "alice", Position: pbAt(3, 0),
	})
	require.NoError(t, err)
	inCombat(t, h.charRepo, "alice", 1)

	// Spawned in a fixed order: initiative ties break by arrival, so this is
	// what puts both monsters after alice and in the order asserted below.
	for _, spawn := range []struct {
		id  string
		ref string
		at  spatial.Position
	}{
		{"skel-1", refs.Monsters.Skeleton().String(), at(4, 0)},
		{"gob-1", refs.Monsters.Goblin().String(), at(5, 0)},
	} {
		_, serr := h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
			Session: sessionID, ID: spawn.id, Ref: spawn.ref, Position: spawn.at,
		})
		require.NoError(t, serr)
	}

	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: sessionID, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, []string{"alice", "skel-1", "gob-1"}, turn.GetOrder(),
		"geometry gate: alice acts first and both monsters follow, so ending her turn asks each of them once")

	// Alice passes, the clock walks onto each monster in turn, and each one
	// is asked what it does -- which is when its mind crosses.
	_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
		Session: sessionID, Member: "alice",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, sessionID, "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)

	require.Equal(t, tkmonster.MindRetaliator.String(), recorder.viewOf(t, "skel-1").Mind,
		"the skeleton definition names the retaliator mind and the whole spawn path carries that word")
	require.Equal(t, tkmonster.MindUnspecified.String(), recorder.viewOf(t, "gob-1").Mind,
		"a monster whose definition names no mind crosses empty, which is what selects the basic brain")
}
