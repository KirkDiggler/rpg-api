package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// pathWalker walks each named monster one fixed path on its first turn and
// passes ever after -- a monster that deliberately leaves a threatened
// square, which is the one thing the shipped driver will never do.
type pathWalker struct {
	paths  map[string][]spatial.Position
	walked map[string]bool
}

func (p *pathWalker) Act(view sdk.MonsterView) (sdk.TurnIntent, error) {
	path, ok := p.paths[view.Self]
	if !ok || p.walked[view.Self] {
		return sdk.Pass{}, nil
	}
	p.walked[view.Self] = true
	return sdk.Move{Path: path}, nil
}

// hittingDice is testDice with the one face that matters here taken off the
// top: every die rolls its maximum EXCEPT a d20, which rolls 15.
//
// A NATURAL 20 IS A CRITICAL HIT, and a crit from a level-3 fighter with a
// longsword fells a skeleton outright. That is a real and correct scene --
// ruling R6, the mover falls in the cell it was leaving and its turn is over
// -- but it is a DIFFERENT scene from the design's done-when, which needs the
// struck skeleton to finish the walk it announced. 15 still beats a
// skeleton's AC by a wide margin with a +5 to hit, so the swing lands and the
// blow is survivable, which is exactly what this scene is about.
type hittingDice struct{}

func (hittingDice) Roll(_ context.Context, size int) (int, error) {
	if size == 20 {
		return 15, nil
	}
	return size, nil
}

// buildOpenRoom is one lit rectangle and nothing else: no walls, no doors, no
// props. The tomb fixture exists to prove sight and crossings; this scene is
// about who stands next to whom on one row, so every feature that could stop
// a step or a sightline is deliberately absent.
func buildOpenRoom(t *testing.T, width, height int) *tkencounter.EncounterData {
	t.Helper()

	enc, err := tkencounter.NewEncounter(&tkencounter.SetupInput{
		Initiative: orderAsGiven{},
		Retention:  tkencounter.RetentionUnbounded,
		Standing:   allStanding{},
		Sight:      allSeeing{},
		TurnDriver: tkencounter.PassDriver{},
		Striker:    tkencounter.RefusingStriker{},
		Mover:      tkencounter.RefusingMover{},
		Announcer:  tkencounter.RefusingAnnouncer{},
		Field: tkencounter.FieldInput{
			Canvas: tkencounter.CanvasInput{
				Void: tkencounter.VoidIsOpaque(), Orientation: pointy,
			},
			Regions: []tkencounter.RegionInput{{
				ID: "room", Name: "Room", Archetype: "crypt",
				Lighting: &tkencounter.Lighting{Intensity: 1},
				Cells:    rect(0, 0, width, height),
			}},
		},
		Endings: []tkencounter.EndingInput{{Key: "unused", Trigger: tkencounter.TriggerExternal{}}},
	})
	require.NoError(t, err, "building the open room")
	data := enc.ToData()
	return &data
}

// inCombat gives a stored sheet the economy of somebody in a fight, with
// reactions in hand. AFTER Join, never before -- Join writes the joined
// member's sheet from a freshly loaded character, so anything written first
// is overwritten.
func inCombat(t *testing.T, repo characterrepo.Repository, id string, reactions int) {
	t.Helper()
	ctx := context.Background()
	got, err := repo.Get(ctx, characterrepo.GetInput{ID: id})
	require.NoError(t, err)
	got.Character.Data.ActionEconomy = &tkcharacter.ActionEconomyData{
		TurnNumber: 1, ActionsRemaining: 1, BonusActionsRemaining: 1,
		ReactionsRemaining: reactions, MovementRemaining: 30,
	}
	_, err = repo.Update(ctx, characterrepo.UpdateInput{Character: got.Character})
	require.NoError(t, err)
}

// reactionsLeft reads what a member has left on the stored sheet -- the only
// place a spend is durable, and the only place a REFUND would have to show up
// if holding cost anything.
func reactionsLeft(t *testing.T, repo characterrepo.Repository, id string) int {
	t.Helper()
	sheet := storedSheetOf(t, repo, id)
	require.NotNil(t, sheet.ActionEconomy, "member %q is not in a fight", id)
	return sheet.ActionEconomy.ReactionsRemaining
}

// reactRow is the member's own open REACT declaration as the WIRE carries it,
// or nil when nothing is being asked of them.
//
// READ THROUGH Afford, never minted: the selector is the server's, and a test
// that built one would be asserting against its own arithmetic.
func reactRow(
	ctx context.Context, t *testing.T, h *acceptanceHarness, session, member string,
) *sessionpb.Declaration {
	t.Helper()
	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: session, Member: member})
	require.NoError(t, err)
	for _, declaration := range out.GetDeclarations() {
		if declaration.GetVerb() == sessionpb.Verb_VERB_REACT {
			return declaration
		}
	}
	return nil
}

// windowOpenedIn returns every WINDOW_OPENED event in a captured slice.
func windowOpenedIn(events []*sessionpb.Event) []*sessionpb.WindowOpened {
	var out []*sessionpb.WindowOpened
	for _, evt := range events {
		if evt.GetKind() == sessionpb.EventKind_EVENT_KIND_WINDOW_OPENED {
			out = append(out, evt.GetWindowOpened())
		}
	}
	return out
}

// TestAcceptance_ReactionWindowCrossesTheWire is rung 3 of rpg-project#316
// through the real, wired stack: proto request in, proto response out, the
// real session.Manager over real Redis-backed repositories, and the real
// StreamEvents subscriber -- the production path minus the network hop.
//
// WHY A SCRIPTED DRIVER. The shipped monster brain (sdk.Behavior()) attacks
// the closest standing player and otherwise closes the distance; it will
// never walk OUT of a fighter's reach, so no play walk can provoke the one
// thing this slice exists to handle. The driver is the only fake in the
// scene, and it decides nothing about reactions -- it only says which cells a
// skeleton walks.
//
// The scene is the design's own done-when: a fighter between two skeletons,
// each walking out of her reach on its own turn. She declines the first swing
// and takes the second, and the reaction she kept is the one she spends.
func TestAcceptance_ReactionWindowCrossesTheWire(t *testing.T) {
	const sessionID = "interrupt-run"

	h := newAcceptanceHarnessWith(t, hittingDice{}, &pathWalker{
		paths: map[string][]spatial.Position{
			"skel-1": {at(5, 0), at(6, 0)},
			"skel-2": {at(1, 0), at(0, 0)},
		},
		walked: map[string]bool{},
	})
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)
	// bob exists but never joins. He is the non-audience caller below: the
	// ownership gate needs a character this player owns, and React's own
	// refusal is about whose window it is, not who is in the fight.
	_, err = h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("bob", "player-bob")},
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

	// Spawned in a fixed order, because initiative ties break by arrival and
	// the whole scene's arithmetic below reads off the resulting order.
	for _, spawn := range []struct {
		id string
		at spatial.Position
	}{{"skel-1", at(4, 0)}, {"skel-2", at(2, 0)}} {
		_, serr := h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
			Session: sessionID, ID: spawn.id, Ref: refs.Monsters.Skeleton().String(), Position: spawn.at,
		})
		require.NoError(t, serr)
	}

	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: sessionID, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, "alice", turn.GetActive())
	require.Equal(t, []string{"alice", "skel-1", "skel-2"}, turn.GetOrder(),
		"geometry gate: alice acts first and the skeletons follow in spawn order -- every step below reads off this")

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newRecordingStream(streamCtx)
	done := make(chan error, 1)
	go func() {
		done <- h.handler.StreamEvents(&sessionpb.StreamEventsRequest{Session: sessionID, Member: "alice"}, stream)
	}()
	waitForLive(t, h.manager.Broker, sessionID, "alice", stream)
	baseline := len(stream.snapshot())

	// -- alice passes her turn, and the first skeleton's driven walk stops to
	// ask her, on the live stream, before it takes the step --
	_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
		Session: sessionID, Member: "alice",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, sessionID, "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)

	firstPass := waitForQuiescence(t, stream, 2*time.Second)[baseline:]
	opened := windowOpenedIn(firstPass)
	require.Len(t, opened, 1, "the first skeleton's first step leaves reach and asks")
	require.Equal(t, "skel-1", opened[0].GetMover())
	require.Equal(t, []string{"alice"}, opened[0].GetAudience())
	require.Equal(t, oaRef, opened[0].GetReaction().GetRef())
	require.Equal(t, "Opportunity Attack", opened[0].GetReaction().GetName())
	// The step is ANNOUNCED and NOT TAKEN: the skeleton still stands on
	// `from`, which is the whole reason reach can still be checked against it.
	require.Equal(t, at(4, 0).X, opened[0].GetFrom().GetX())
	require.Equal(t, at(5, 0).X, opened[0].GetTo().GetX())
	require.Equal(t, at(4, 0), whereIs(t, h, sessionID, "skel-1"),
		"the announced step is not taken while the question stands")

	// -- the dock is told, before any click, exactly what it may do --
	row := reactRow(ctx, t, h, sessionID, "alice")
	require.NotNil(t, row, "alice must be offered the window she is being asked about")
	require.NotEmpty(t, row.GetId())
	require.True(t, row.GetAvailable(), "every gate a reaction has was passed before the question was worth asking")
	require.Equal(t, sessionpb.Slot_SLOT_REACTION, row.GetSlot())
	require.Equal(t, oaRef, row.GetReaction().GetRef())
	require.Equal(t, "Opportunity Attack", row.GetReaction().GetName())
	require.Len(t, row.GetCandidates(), 1, "one window, one mover")
	require.Equal(t, "skel-1", row.GetCandidates()[0].GetMember())

	// -- and every OTHER row says why it is dark, in the one word only the
	// freeze can produce. Without this the dock greys out with no reason and
	// a player is told nothing at the moment they most need telling. --
	requireFrozenRows(ctx, t, h, sessionID, "alice")

	// -- the freeze is real, not merely announced --
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: sessionID, Member: "alice", DeclarationId: "anything",
		Path: []*sessionpb.Position{pbAt(3, 1)},
	})
	requireGRPCCode(t, err, codes.FailedPrecondition)

	// -- an answer that names no choice is refused, and refused DIFFERENTLY
	// from a window that will not take it: "the client forgot to say" and
	// "the player chose to hold" must not be the same bytes --
	_, err = h.handler.React(ctx, &sessionpb.ReactRequest{
		Session: sessionID, Member: "alice", DeclarationId: row.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_UNSPECIFIED,
	})
	requireGRPCCode(t, err, codes.InvalidArgument)

	// -- somebody else's window is a permission refusal, not a stale one --
	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	_, err = h.handler.React(bobCtx, &sessionpb.ReactRequest{
		Session: sessionID, Member: "bob", DeclarationId: row.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_STRIKE,
	})
	requireGRPCCode(t, err, codes.PermissionDenied)

	// -- and the window survives all three refusals --
	require.Equal(t, row.GetId(), reactRow(ctx, t, h, sessionID, "alice").GetId(),
		"a refused answer must not close the question")

	// -- HOLD. The skeleton finishes the walk it announced, and the reaction
	// is still in her hand for the next one. --
	baseline = len(stream.snapshot())
	holdResp, err := h.handler.React(ctx, &sessionpb.ReactRequest{
		Session: sessionID, Member: "alice", DeclarationId: row.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_HOLD,
	})
	require.NoError(t, err)
	require.NotEmpty(t, holdResp.GetSaved().GetWritten(), "answering a window is a write")

	afterHold := waitForQuiescence(t, stream, 2*time.Second)[baseline:]
	require.Contains(t, movedCellsOf(afterHold, "skel-1"), at(6, 0),
		"the resumed turn walks the rest of the path it had announced")
	require.Equal(t, at(6, 0), whereIs(t, h, sessionID, "skel-1"))
	require.Empty(t, struckWithReaction(afterHold), "holding swings at nobody")
	require.Equal(t, 1, reactionsLeft(t, h.charRepo, "alice"), "a reaction nobody took costs nothing")

	// -- THE SECOND PASS ASKS AGAIN, posed during the resume that finished
	// the first skeleton's turn, with nobody calling a verb for it --
	second := windowOpenedIn(afterHold)
	require.Len(t, second, 1, "the second skeleton's own turn asks in its own right")
	require.Equal(t, "skel-2", second[0].GetMover())
	require.Equal(t, []string{"alice"}, second[0].GetAudience())

	secondRow := reactRow(ctx, t, h, sessionID, "alice")
	require.NotNil(t, secondRow)
	require.NotEqual(t, row.GetId(), secondRow.GetId(), "a window id is never reused, so neither is its selector")
	require.Len(t, secondRow.GetCandidates(), 1)
	require.Equal(t, "skel-2", secondRow.GetCandidates()[0].GetMember())

	// -- STRIKE. The swing reaches the story saying WHAT it was taken as,
	// which is the half of the done-when the window alone does not meet. --
	baseline = len(stream.snapshot())
	_, err = h.handler.React(ctx, &sessionpb.ReactRequest{
		Session: sessionID, Member: "alice", DeclarationId: secondRow.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_STRIKE,
	})
	require.NoError(t, err)

	afterStrike := waitForQuiescence(t, stream, 2*time.Second)[baseline:]
	swings := struckWithReaction(afterStrike)
	require.Len(t, swings, 1, "one swing, at the skeleton she chose")
	require.Equal(t, "alice", swings[0].GetAttacker())
	require.Equal(t, "skel-2", swings[0].GetTarget())
	require.Equal(t, oaRef, swings[0].GetReaction().GetRef())
	require.Equal(t, "Opportunity Attack", swings[0].GetReaction().GetName())

	require.Equal(t, at(0, 0), whereIs(t, h, sessionID, "skel-2"),
		"the second skeleton's turn finishes from where it stopped -- the blow it took on the way out did not stop it")
	require.Equal(t, 0, reactionsLeft(t, h.charRepo, "alice"), "spent once, by the swing she took")
	require.Nil(t, reactRow(ctx, t, h, sessionID, "alice"), "nothing is being asked any more")

	backToAlice, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: sessionID, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, "alice", backToAlice.GetActive(), "and the fight is back on alice's own turn")

	cancel()
	require.NoError(t, <-done)
}

// oaRef is the one reaction that can reach a movement fold today.
const oaRef = "dnd5e:conditions:opportunity_attack"

// whereIs reads a member's cell straight off the Manager. Monsters have no
// owning player, so this cannot go through GetWhere's ownership gate.
func whereIs(t *testing.T, h *acceptanceHarness, session, member string) spatial.Position {
	t.Helper()
	out, err := h.manager.Manager.Where(context.Background(), &sdk.WhereInput{
		Session: session, Member: member,
	})
	require.NoError(t, err)
	return out.Position
}

// requireFrozenRows asserts every non-REACT row Afford returns is unavailable
// for the freeze and for nothing else.
func requireFrozenRows(ctx context.Context, t *testing.T, h *acceptanceHarness, session, member string) {
	t.Helper()
	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: session, Member: member})
	require.NoError(t, err)

	others := 0
	for _, declaration := range out.GetDeclarations() {
		if declaration.GetVerb() == sessionpb.Verb_VERB_REACT {
			continue
		}
		others++
		require.False(t, declaration.GetAvailable(), "verb %s must be frozen", declaration.GetVerb())
		require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_WINDOW_OPEN,
			declaration.GetWhy().GetReason(),
			"verb %s must say the window is why, not a clock or a budget", declaration.GetVerb())
	}
	require.NotZero(t, others, "the frozen panel must still carry the verbs it is refusing")
}

// movedCellsOf is every cell one member was seen stepping onto.
func movedCellsOf(events []*sessionpb.Event, member string) []spatial.Position {
	var out []spatial.Position
	for _, evt := range events {
		moved := evt.GetMoved()
		if evt.GetKind() != sessionpb.EventKind_EVENT_KIND_MOVED || moved.GetMember() != member {
			continue
		}
		out = append(out, spatial.Position{X: moved.GetTo().GetX(), Y: moved.GetTo().GetY()})
	}
	return out
}

// struckWithReaction is every landed blow that names what it was taken AS.
// An ordinary swing carries no reaction, so this is not merely "every
// struck": it is the false-vs-absent half of the field.
func struckWithReaction(events []*sessionpb.Event) []*sessionpb.Struck {
	var out []*sessionpb.Struck
	for _, evt := range events {
		if struck := evt.GetStruck(); struck != nil && struck.GetReaction() != nil {
			out = append(out, struck)
		}
	}
	return out
}
