package session_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/KirkDiggler/rpg-toolkit/core"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// inspirationRef and inspiredRef are the two halves of the bard's level-1
// row: the feature she activates, and the condition the ally then holds.
const (
	inspirationRef = "dnd5e:features:bardic_inspiration"
	inspiredRef    = "dnd5e:conditions:inspired"
)

// bardSessionID keeps these scenes off every other suite's session key.
const bardSessionID = "bard-run"

// levelOneBard is the sheet a finalized bard draft produces, written directly
// so these scenes do not depend on the creation flow: the feature on the
// sheet, and a long-rest pool with `uses` left in it.
//
// NO SPELLS AND NO SLOTS, and that is the design's own ruling (rpg-project#397
// R6) rather than an omission in the fixture. Slice one gives the bard one die
// to grant and nothing to cast, so a slot here would be a number nothing in
// the stack can spend.
func levelOneBard(id, playerID string, uses int) *tkcharacter.Data {
	feature, err := json.Marshal(map[string]any{
		"ref":   refs.Features.BardicInspiration(),
		"id":    refs.Features.BardicInspiration().ID,
		"name":  conditions.InspiredName,
		"level": 1,
	})
	if err != nil {
		panic(err)
	}
	return &tkcharacter.Data{
		ID: id, PlayerID: playerID, Name: id, Level: 1,
		ClassID: classes.Bard, RaceID: races.Human,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8, abilities.DEX: 14, abilities.CON: 12,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 16,
		},
		HitPoints: 9, MaxHitPoints: 9, ArmorClass: 12, ProficiencyBonus: 2,
		Features: []json.RawMessage{feature},
		Resources: map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
			resources.Inspiration: {
				Current: uses, Maximum: uses, ResetType: coreResources.ResetLongRest,
			},
		},
	}
}

// bardScene is the whole fixture: a bard and a fighter standing together with
// one skeleton to swing at, on a turn clock in that order.
//
// THE MONSTER NEVER ACTS. pathWalker with no paths passes on every turn, so
// nothing a skeleton decides can reach the numbers these scenes assert. That
// is the same fake interrupt_acceptance_test.go wires and for the same
// reason: the scripted driver decides nothing about reactions.
func bardScene(t *testing.T) (*acceptanceHarness, context.Context, context.Context) {
	t.Helper()

	h := newAcceptanceHarnessWith(t, hittingDice{}, &pathWalker{
		paths:  map[string][]spatial.Position{},
		walked: map[string]bool{},
	})
	bardCtx := auth.WithPlayerID(context.Background(), "player-bella")
	fighterCtx := auth.WithPlayerID(context.Background(), "player-alice")

	for _, sheet := range []*tkcharacter.Data{
		levelOneBard("bella", "player-bella", 2),
		armedFighter("alice", "player-alice"),
	} {
		_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
			Character: &entities.Character{Data: sheet},
		})
		require.NoError(t, err)
	}

	// The lobby's job, in-process (design rule 5: creation is the lobby's).
	_, err := h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: bardSessionID, Encounter: "room-encounter", World: buildOpenRoom(t, 12, 6),
	})
	require.NoError(t, err)

	// EVERYBODY TIES ON INITIATIVE HERE -- the roller gives every d20 the
	// same face and all three carry the same DEX -- so the order falls to the
	// seam's own tie-break, and the assertion below is what pins it. The
	// fighter acts first, which is why she passes her turn before the bard
	// grants: the die has to be in her hand BEFORE the swing that offers it.
	_, err = h.handler.Join(bardCtx, &sessionpb.JoinRequest{
		Session: bardSessionID, Member: "bella", Position: pbAt(2, 0),
	})
	require.NoError(t, err)
	_, err = h.handler.Join(fighterCtx, &sessionpb.JoinRequest{
		Session: bardSessionID, Member: "alice", Position: pbAt(3, 0),
	})
	require.NoError(t, err)
	inCombat(t, h.charRepo, "bella", 1)
	inCombat(t, h.charRepo, "alice", 1)

	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: bardSessionID, ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: at(4, 0),
	})
	require.NoError(t, err)

	turn, err := h.handler.Turn(fighterCtx, &sessionpb.TurnRequest{
		Session: bardSessionID, Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"alice", "bella", "skel-1"}, turn.GetOrder(),
		"geometry gate: the fighter acts first, then the bard, then the skeleton -- every step below reads off this")

	return h, bardCtx, fighterCtx
}

// inspirationRow is the bard's own Bardic Inspiration declaration, read
// through the Afford surface a client uses. Tests never mint a selector.
func inspirationRow(
	ctx context.Context, t *testing.T, h *acceptanceHarness,
) *sessionpb.Declaration {
	t.Helper()
	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{
		Session: bardSessionID, Member: "bella",
	})
	require.NoError(t, err)
	for _, declaration := range out.GetDeclarations() {
		if declaration.GetVerb() == sessionpb.Verb_VERB_ACTIVATE &&
			declaration.GetAbility().GetRef() == inspirationRef {
			return declaration
		}
	}
	require.FailNow(t, "the bard was offered no Bardic Inspiration row")
	return nil
}

// poolLeft reads the bard's uses off the STORED sheet -- the only place a
// spend is durable, and the only place a refund would have to show up if
// declining cost anything.
func poolLeft(t *testing.T, repo characterrepo.Repository, id string) int {
	t.Helper()
	pool, ok := storedSheetOf(t, repo, id).Resources[resources.Inspiration]
	require.True(t, ok, "member %q carries no inspiration pool", id)
	return pool.Current
}

// holdsTheDie reads whether a stored sheet still carries a Bardic Inspiration
// die.
//
// Decoded LENIENTLY, and not through conditionRefsOf beside it: a sheet
// carries several conditions and they do not all spell their ref the same
// way, so a strict decode of the whole list would fail on a neighbor rather
// than answer the one question asked here.
func holdsTheDie(t *testing.T, repo characterrepo.Repository, id string) bool {
	t.Helper()
	for _, raw := range storedSheetOf(t, repo, id).Conditions {
		var peek struct {
			Ref *core.Ref `json:"ref"`
		}
		if json.Unmarshal(raw, &peek) != nil || peek.Ref == nil {
			continue
		}
		if peek.Ref.ID == refs.Conditions.Inspired().ID {
			return true
		}
	}
	return false
}

// rollWindowsIn returns every ROLL_WINDOW_OPENED body in a captured slice.
func rollWindowsIn(events []*sessionpb.Event) []*sessionpb.RollWindowOpened {
	var out []*sessionpb.RollWindowOpened
	for _, evt := range events {
		if evt.GetKind() == sessionpb.EventKind_EVENT_KIND_ROLL_WINDOW_OPENED {
			out = append(out, evt.GetRollWindowOpened())
		}
	}
	return out
}

// outcomesIn counts the struck-and-missed beats in a captured slice. ONE
// SWING MUST PRODUCE ONE OUTCOME across the pause and the answer, which is
// the whole risk of resuming a machine: a resume that re-folded the attack
// would record the attempt twice.
func outcomesIn(events []*sessionpb.Event) int {
	count := 0
	for _, evt := range events {
		switch evt.GetKind() {
		case sessionpb.EventKind_EVENT_KIND_STRUCK, sessionpb.EventKind_EVENT_KIND_MISSED:
			count++
		default:
		}
	}
	return count
}

// totalOfOutcome reads the total off the one struck-or-missed beat.
func totalOfOutcome(t *testing.T, events []*sessionpb.Event) int32 {
	t.Helper()
	for _, evt := range events {
		if struck := evt.GetStruck(); struck != nil {
			return struck.GetTotal()
		}
		if missed := evt.GetMissed(); missed != nil {
			return missed.GetTotal()
		}
	}
	require.FailNow(t, "no outcome beat in the captured stream")
	return 0
}

// grantAndSwing plays the shared half of both branches: the bard grants her
// die to the fighter, ends her turn, and the fighter swings at the skeleton.
// It returns the paused response, the fighter's own open REACT row, and the
// window beat the stream carried.
type posedSwing struct {
	h       *acceptanceHarness
	bardCtx context.Context
	fighter context.Context
	stream  *recordingStream
	cancel  context.CancelFunc
	done    chan error

	attack *sessionpb.AttackResponse
	row    *sessionpb.Declaration
}

func grantAndSwing(t *testing.T) *posedSwing {
	t.Helper()
	h, bardCtx, fighterCtx := bardScene(t)

	// -- the fighter opens the round and passes, so the bard can hand her the
	// die before she swings --
	_, err := h.handler.EndTurn(fighterCtx, &sessionpb.EndTurnRequest{
		Session: bardSessionID, Member: "alice",
		DeclarationId: currentDeclarationID(
			fighterCtx, t, h.handler, bardSessionID, "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)

	// -- the grant: a bonus action and one use, charged at the door --
	row := inspirationRow(bardCtx, t, h)
	require.True(t, row.GetAvailable(), "the bard can afford her own level-1 feature")
	require.Equal(t, sessionpb.Slot_SLOT_BONUS, row.GetSlot())
	require.Equal(t, 2, poolLeft(t, h.charRepo, "bella"))

	_, err = h.handler.Activate(bardCtx, &sessionpb.ActivateRequest{
		Session: bardSessionID, Member: "bella", Target: "alice", DeclarationId: row.GetId(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, poolLeft(t, h.charRepo, "bella"), "the use is spent by the grant")
	require.True(t, holdsTheDie(t, h.charRepo, "alice"), "the die is on the ALLY's sheet")

	// The bard ends her turn; the skeleton passes; the clock comes back round
	// to the fighter, who now holds a die she did not have when she last had
	// the initiative.
	_, err = h.handler.EndTurn(bardCtx, &sessionpb.EndTurnRequest{
		Session: bardSessionID, Member: "bella",
		DeclarationId: currentDeclarationID(
			bardCtx, t, h.handler, bardSessionID, "bella", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)

	// -- the stream, opened on the fighter: she is the audience, so hers is
	// the one that must carry the question --
	streamCtx, cancel := context.WithCancel(fighterCtx)
	stream := newRecordingStream(streamCtx)
	done := make(chan error, 1)
	go func() {
		done <- h.handler.StreamEvents(
			&sessionpb.StreamEventsRequest{Session: bardSessionID, Member: "alice"}, stream)
	}()
	waitForLive(t, h.manager.Broker, bardSessionID, "alice", stream)
	baseline := len(stream.snapshot())

	// -- the swing stops after the d20 --
	attack, err := h.handler.Attack(fighterCtx, &sessionpb.AttackRequest{
		Session: bardSessionID, Attacker: "alice", Target: "skel-1",
		DeclarationId: currentDeclarationID(
			fighterCtx, t, h.handler, bardSessionID, "alice", sessionpb.Verb_VERB_ATTACK),
	})
	require.NoError(t, err)

	// Roll and total are the two numbers that ARE answers. Everything else on
	// this response is zero because nothing has landed: the AC has
	// deliberately not been shown, which is the whole decision.
	require.Equal(t, int32(15), attack.GetRoll(), "the d20 the player is deciding about")
	require.NotZero(t, attack.GetTotal())
	require.Zero(t, attack.GetAgainst(), "the AC is not shown before the choice")
	require.False(t, attack.GetHit())
	require.Zero(t, attack.GetDamage())

	posed := waitForQuiescence(t, stream, 2*time.Second)[baseline:]
	require.Zero(t, outcomesIn(posed), "no outcome beat: there is no outcome yet")

	windows := rollWindowsIn(posed)
	require.Len(t, windows, 1, "one d20, one window")
	require.Equal(t, "alice", windows[0].GetAudience(), "the audience is the roller and nobody else")
	require.Equal(t, inspiredRef, windows[0].GetOffer().GetRef())
	require.Equal(t, conditions.InspiredName, windows[0].GetOffer().GetName(),
		"the server authors the label the dock's buttons are named for")
	require.Equal(t, attack.GetRoll(), windows[0].GetRoll())
	require.Equal(t, attack.GetTotal(), windows[0].GetTotal())

	// -- the dock is told, before any click, exactly what it may do. There is
	// no step and nobody to aim at, so this row carries no candidates and
	// costs no reaction -- both of which a movement window does the opposite
	// of, and both of which a client arms its UI off. --
	reaction := reactRow(fighterCtx, t, h, bardSessionID, "alice")
	require.NotNil(t, reaction, "the roller must be offered the window she is being asked about")
	require.True(t, reaction.GetAvailable())
	require.Equal(t, inspiredRef, reaction.GetReaction().GetRef())
	require.Equal(t, conditions.InspiredName, reaction.GetReaction().GetName())
	require.Empty(t, reaction.GetCandidates(), "there is no step and nobody to aim at")
	require.Equal(t, sessionpb.Slot_SLOT_NONE, reaction.GetSlot(), "answering costs no reaction")

	// -- and the table is frozen on her answer, with the one word that says
	// why. The bard is not the audience and holds no window of her own. --
	requireFrozenRows(bardCtx, t, h, bardSessionID, "bella")
	require.Nil(t, reactRow(bardCtx, t, h, bardSessionID, "bella"),
		"the bard granted the die; she is not the one being asked about it")

	return &posedSwing{
		h: h, bardCtx: bardCtx, fighter: fighterCtx,
		stream: stream, cancel: cancel, done: done,
		attack: attack, row: reaction,
	}
}

// TestAcceptance_BardicInspirationSpendCrossesTheWire is rpg-project#397 and
// #398 through the real, wired stack: proto request in, proto response out,
// the real session.Manager over real Redis-backed repositories, and the real
// StreamEvents subscriber -- the production path minus the network hop.
//
// The scene is the design's own walk, spend branch: a level-1 bard inspires
// the fighter as a bonus action, the fighter swings, the swing stops to ask
// her, and she takes the die.
func TestAcceptance_BardicInspirationSpendCrossesTheWire(t *testing.T) {
	scene := grantAndSwing(t)
	defer scene.cancel()

	// -- somebody else's window is a permission refusal, not a stale one.
	// The bard passes the ownership gate on her own member and is refused for
	// the only reason that matters here: it is not her question. --
	_, err := scene.h.handler.React(scene.bardCtx, &sessionpb.ReactRequest{
		Session: bardSessionID, Member: "bella", DeclarationId: scene.row.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_STRIKE,
	})
	requireGRPCCode(t, err, codes.PermissionDenied)
	require.Equal(t, scene.row.GetId(),
		reactRow(scene.fighter, t, scene.h, bardSessionID, "alice").GetId(),
		"a refused answer must not close the question")

	// -- STRIKE. The die joins the total, once, and is gone. --
	baseline := len(scene.stream.snapshot())
	_, err = scene.h.handler.React(scene.fighter, &sessionpb.ReactRequest{
		Session: bardSessionID, Member: "alice", DeclarationId: scene.row.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_STRIKE,
	})
	require.NoError(t, err)

	after := waitForQuiescence(t, scene.stream, 2*time.Second)[baseline:]
	require.Equal(t, 1, outcomesIn(after),
		"exactly one outcome beat across the pause and the answer -- a resume that re-folded would record two")

	total := totalOfOutcome(t, after)
	require.Equal(t, scene.attack.GetTotal()+6, total,
		"a d6 rolling its own face joined the total the window showed, and the d20 was not re-rolled")

	require.False(t, holdsTheDie(t, scene.h.charRepo, "alice"), "the die is spent when it is TAKEN")
	require.Equal(t, 1, poolLeft(t, scene.h.charRepo, "bella"),
		"the spend is the fighter's die, not a second charge on the bard")
	require.Nil(t, reactRow(scene.fighter, t, scene.h, bardSessionID, "alice"),
		"nothing is being asked any more")

	scene.cancel()
	require.NoError(t, <-scene.done)
}

// TestAcceptance_BardicInspirationHoldKeepsTheDie is the other branch, and
// the reason declining is worth offering at all: the swing still finishes,
// the total carries no die, and the fighter still holds one afterwards.
func TestAcceptance_BardicInspirationHoldKeepsTheDie(t *testing.T) {
	scene := grantAndSwing(t)
	defer scene.cancel()

	baseline := len(scene.stream.snapshot())
	_, err := scene.h.handler.React(scene.fighter, &sessionpb.ReactRequest{
		Session: bardSessionID, Member: "alice", DeclarationId: scene.row.GetId(),
		Choice: sessionpb.ReactChoice_REACT_CHOICE_HOLD,
	})
	require.NoError(t, err)

	after := waitForQuiescence(t, scene.stream, 2*time.Second)[baseline:]
	require.Equal(t, 1, outcomesIn(after), "holding still finishes the swing")
	require.Equal(t, scene.attack.GetTotal(), totalOfOutcome(t, after), "no face joined it")

	require.True(t, holdsTheDie(t, scene.h.charRepo, "alice"), "declining costs nothing")
	require.Nil(t, reactRow(scene.fighter, t, scene.h, bardSessionID, "alice"),
		"the question is answered even though the die was kept")

	scene.cancel()
	require.NoError(t, <-scene.done)
}
