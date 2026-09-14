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
// seam hardcoded it. The ghoul names no mind, and an empty Mind is what
// tells the shipped driver to fall back to the basic brain.
//
// It used to be the goblin, until rpg-toolkit#1745 cast the goblin as a
// coward and this test caught it. The control has to be a definition that
// names NOTHING, so it moved to one that still does: same DEX modifier, so
// the initiative tie still breaks by arrival and the order below is
// unchanged. Every monster the toolkit casts is one fewer candidate, which
// is a thing to notice rather than a problem -- the day none is left, an
// empty Mind is no longer a shipped monster's answer and this half of the
// proof needs a definition of its own.
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
		{"ghoul-1", refs.Monsters.Ghoul().String(), at(5, 0)},
	} {
		_, serr := h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
			Session: sessionID, ID: spawn.id, Ref: spawn.ref, Position: spawn.at,
		})
		require.NoError(t, serr)
	}

	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: sessionID, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, []string{"alice", "skel-1", "ghoul-1"}, turn.GetOrder(),
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
	require.Equal(t, tkmonster.MindUnspecified.String(), recorder.viewOf(t, "ghoul-1").Mind,
		"a monster whose definition names no mind crosses empty, which is what selects the basic brain")
}

// TestAcceptance_TwoSessionsEachDriveTheirOwnMonsters runs the PRODUCTION
// driver wiring -- no scripted driver, so the orchestrator's per-session cache
// is what answers -- for two sessions in one process.
//
// WHAT IT PROVES AND WHAT IT DOES NOT, because the difference matters. It
// proves the cache serves every session rather than only the first: the second
// session's EndTurn walks the clock across its own skeleton and comes back,
// which a source that answered for one session and failed for the next would
// break (a resolver error fails the verb; the SDK never falls back).
//
// It does NOT show two DISTINCT drivers, and cannot from here: nothing on the
// wire or in any projection reports which driver took a turn. That claim is
// proved where it is observable -- turnDriverCache's own test one package over,
// and rpg-toolkit's TestEachSessionDrivesItsOwnTurnsWithItsOwnDriver, which
// drives two sessions through one Manager and shows a member recorded in one
// is unknown in the other.
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

			// The SAME member id in both sessions, which is the caveat this
			// whole wave is about: ids are authored per dungeon, not minted
			// per run, so two parties in the same tomb hold a skeleton of the
			// same name.
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
