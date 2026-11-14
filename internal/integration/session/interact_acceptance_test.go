package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// The things to do in the castle (the-mad-king.yaml, rpg-project#959).
const (
	toChapelDo     = "You lift the grate and take the way up."
	toChapelSeen   = "lifts the grate by the hearth and climbs down into the dark"
	theAltar       = "altar"
	examineOption  = "examine"
	theAltarMissed = "Old stone, old candles."
)

// interactsOn is every InteractAct in acts on prop, with its act.
func interactsOn(acts []*storypb.Act, prop string) []*storypb.Act {
	var out []*storypb.Act
	for _, a := range acts {
		if in := a.GetInteract(); in != nil && in.GetProp().GetId() == prop {
			out = append(out, a)
		}
	}
	return out
}

// narrations is every table answer narration in acts.
func narrations(acts []*storypb.Act) []string {
	var out []string
	for _, a := range acts {
		if n := a.GetTableAnswer().GetNarration(); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// TestAcceptance_InteractIsDoneToAThing is rpg-project#959 through the
// api's own doors, at each their own, so what each player is sent is their
// own version. Alice buys the way
// from the scullion with bob in earshot; carol comes into the kitchen after
// the telling, in sight of the grate and untold; dan stays out on the road.
// The grate is something to DO, offered by Interact and not by Speak: alice
// and bob are offered it, carol and dan are not. Alice takes the passage:
// her stream carries her InteractAct (her own words, the option) and her
// RelocateAct; bob is shown the seen version; carol, who saw it done, is
// shown the reveal and the seen version and is offered the grate after;
// dan's own acts name nothing of it. A Speak with the grate's option, and an
// Interact with a spoken line's, are refused. In the chapel alice examines
// the altar with bob watching: she gets the check and the narration, bob the
// narration, dan none of his own. Catch-up equals live.
// kitchenScene is the castle with alice holding the purse and having paid
// the scullion for the way with bob in earshot by the grate; carol came in
// after the telling and stands in sight of the grate, untold; dan is out on
// the road; alice stands beside the grate. eachOwn marks the castle
// `party: { sharing: own }`.
func kitchenScene(t *testing.T, run string, eachOwn bool) *acceptanceHarness {
	t.Helper()
	ctxOf := func(p string) context.Context { return auth.WithPlayerID(context.Background(), p) }
	dice := &gateDice{}
	dice.face.Store(1)
	h := newAcceptanceHarnessWithDice(t, dice)
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}, {"carol", "player-carol"}, {"dan", "player-dan"}} {
		_, err := h.charRepo.Create(ctxOf("player-alice"), characterrepo.CreateInput{
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
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", -22, -9), seatAt("bob", -20, -14), seatAt("carol", 8, 16), seatAt("dan", 6, 15))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	_, err = h.handler.Hold(ctxOf("player-alice"), &sessionpb.HoldRequest{Session: run, Member: "alice", Target: kitchenPurse, Range: holdReach})
	require.NoError(t, err, "alice picks up the purse")
	_, err = sayTo(t, h, run, "alice", "player-alice", kitchenScullion, favourLine)
	require.NoError(t, err, "alice pays the scullion for the way")

	// Carol comes in after the telling, in sight of the grate; alice steps
	// up beside it.
	stepInto(t, h, run, "carol", "player-carol", pbAt(-15, -13), pbAt(-16, -13))
	stepInto(t, h, run, "alice", "player-alice", pbAt(-17, -14), pbAt(-18, -14))
	return h
}

func TestAcceptance_InteractIsDoneToAThing(t *testing.T) {
	const run = "interact-run"
	ctxOf := func(p string) context.Context { return auth.WithPlayerID(context.Background(), p) }
	h := kitchenScene(t, run, true)
	var err error

	require.NotNil(t, interactOption(t, h, run, "alice", "player-alice", kitchenGrate, toChapelLine), "alice is offered the way, to do")
	bobs := interactOption(t, h, run, "bob", "player-bob", kitchenGrate, toChapelLine)
	require.NotNil(t, bobs, "bob, told, is offered it")
	require.Equal(t, toChapelDo, bobs.GetDoWords())
	require.Nil(t, interactOption(t, h, run, "carol", "player-carol", kitchenGrate, toChapelLine), "carol, untold, is not")
	require.Nil(t, interactOption(t, h, run, "dan", "player-dan", kitchenGrate, toChapelLine), "dan is not")
	requireNoPassage(t, "carol, untold", playerReads(t, h, run, "carol", "player-carol")...)

	// Alice takes the passage, watched.
	streams := map[string][]*storypb.Act{}
	var resp *sessionpb.InteractResponse
	streams["dan"] = watchActs(ctxOf("player-dan"), t, h, run, "dan", func() {
		streams["carol"] = watchActs(ctxOf("player-carol"), t, h, run, "carol", func() {
			streams["bob"] = watchActs(ctxOf("player-bob"), t, h, run, "bob", func() {
				streams["alice"] = watchActs(ctxOf("player-alice"), t, h, run, "alice", func() {
					resp, err = interactWith(t, h, run, "alice", "player-alice", kitchenGrate, toChapelLine)
					require.NoError(t, err, "alice takes the way up")
				})
			})
		})
	})
	require.Equal(t, kitchenGrate, resp.GetDone().GetProp())
	require.Equal(t, toChapelLine, resp.GetDone().GetOptionId())
	require.Nil(t, resp.GetDescriptor_(), "a prop is not a world NPC")
	for who, acts := range streams {
		requireNothingRefused(t, acts, who)
	}

	mine := interactsOn(streams["alice"], kitchenGrate)
	require.Len(t, mine, 1, "alice's stream carries her deed")
	require.Equal(t, toChapelDo, mine[0].GetInteract().GetWords(), "in her own words")
	require.Equal(t, toChapelLine, mine[0].GetInteract().GetOption())
	moved := relocationsOf(streams["alice"], "alice")
	require.Len(t, moved, 1, "and her move")
	require.Equal(t, theChapelRegion, regionAt(t, h, run, &sessionpb.Position{
		X: moved[0].GetRelocate().GetTo().GetX(), Y: moved[0].GetRelocate().GetTo().GetY(),
	}), "into the chapel")

	bobSaw := interactsOn(ownActs(streams["bob"]), kitchenGrate)
	require.Len(t, bobSaw, 1, "bob saw it done")
	require.Equal(t, toChapelSeen, bobSaw[0].GetInteract().GetWords(), "the seen version")

	carolSaw := interactsOn(ownActs(streams["carol"]), kitchenGrate)
	require.Len(t, carolSaw, 1, "carol saw it done")
	require.Equal(t, toChapelSeen, carolSaw[0].GetInteract().GetWords())
	revealed := false
	for _, a := range ownActs(streams["carol"]) {
		if a.GetPerception().GetRevealed() != nil {
			revealed = true
		}
	}
	require.True(t, revealed, "carol, who saw it done, is shown the reveal: if you saw it, you know it")
	// Afterwards, at the grate where alice stood, she is offered it.
	stepInto(t, h, run, "carol", "player-carol", pbAt(-17, -14), pbAt(-18, -14))
	require.NotNil(t, interactOption(t, h, run, "carol", "player-carol", kitchenGrate, toChapelLine), "and is offered the grate after")
	require.True(t, sightedProp(t, h, run, "carol", "player-carol", kitchenGrate), "and has it on her map")

	requireNoPassage(t, "dan's own", actsOf(ownActs(streams["dan"]))...)
	require.Nil(t, interactOption(t, h, run, "dan", "player-dan", kitchenGrate, toChapelLine), "dan, who saw none of it, is still not offered it")
	requireNoPassage(t, "dan's reads", playerReads(t, h, run, "dan", "player-dan")...)

	// The two doors do not cross.
	_, err = h.handler.Speak(ctxOf("player-carol"), &sessionpb.SpeakRequest{
		Session: run, Member: "carol", Target: &sessionpb.SpeakRequest_PropId{PropId: kitchenGrate}, OptionId: toChapelLine,
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "the grate's thing to do is not a line: %v", err)
	_, err = h.handler.Interact(ctxOf("player-carol"), &sessionpb.InteractRequest{
		Session: run, Actor: "carol", Prop: kitchenGrate, OptionId: favourLine,
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a spoken line is not a thing to do")
	require.Equal(t, "That is not something you can do here.", status.Convert(err).Message(), "the session's sentence alone")
	_, err = h.handler.Interact(ctxOf("player-carol"), &sessionpb.InteractRequest{Session: run, Actor: "carol", Prop: kitchenGrate, Target: "pedlar"})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "name one thing")

	// Bob follows; in the chapel alice examines the altar, bob watching.
	_, err = interactWith(t, h, run, "bob", "player-bob", kitchenGrate, toChapelLine)
	require.NoError(t, err, "bob follows")
	// Alice walks up to the altar from where the passage set her down.
	stepInto(t, h, run, "alice", "player-alice", cube(24, -12), cube(25, -12))
	require.NotNil(t, interactOption(t, h, run, "alice", "player-alice", theAltar, examineOption), "beside it, alice may examine the altar")
	examined := map[string][]*storypb.Act{}
	examined["dan"] = watchActs(ctxOf("player-dan"), t, h, run, "dan", func() {
		examined["bob"] = watchActs(ctxOf("player-bob"), t, h, run, "bob", func() {
			examined["alice"] = watchActs(ctxOf("player-alice"), t, h, run, "alice", func() {
				resp, err = interactWith(t, h, run, "alice", "player-alice", theAltar, examineOption)
				require.NoError(t, err, "alice examines the altar")
			})
		})
	})
	require.Equal(t, theAltar, resp.GetDone().GetProp())
	require.NotNil(t, resp.GetDone().GetApplied(), "the check was made")
	hers := interactsOn(examined["alice"], theAltar)
	require.Len(t, hers, 1)
	require.NotNil(t, hers[0].GetInteract().GetRoll(), "the doer gets the check")
	require.Contains(t, narrations(examined["alice"]), theAltarMissed, "and the narration")
	require.Contains(t, narrations(ownActs(examined["bob"])), theAltarMissed, "bob, watching, is shown the narration")
	require.Empty(t, narrations(ownActs(examined["dan"])), "dan saw none of it")

	requireCaughtUp(t, streams["alice"], storyOf(ctxOf("player-alice"), t, h.story, run, "alice"), "alice")
	requireCaughtUp(t, examined["alice"], storyOf(ctxOf("player-alice"), t, h.story, run, "alice"), "alice, examining")
	requireCaughtUp(t, streams["carol"], storyOf(ctxOf("player-carol"), t, h.story, run, "carol"), "carol")
	requireNoWanted(t, storyOf(ctxOf("player-dan"), t, h.story, run, "dan"), "dan")
}

// TestAcceptance_AFriendIsShownTheDeedThroughTheDoer is the same deed at the
// castle's default shared view (rpg-project#935, #959): bob, carol and dan
// are each shown ONE InteractAct and ONE RelocateAct for it, alice's own
// version -- her words and the option -- through alice, in place of their
// own. Knowledge is not shared: carol, who saw it done, is offered the grate
// at it afterwards; dan, who saw none of it, is not.
func TestAcceptance_AFriendIsShownTheDeedThroughTheDoer(t *testing.T) {
	const run = "interact-view-run"
	ctxOf := func(p string) context.Context { return auth.WithPlayerID(context.Background(), p) }
	h := kitchenScene(t, run, false)
	streams := map[string][]*storypb.Act{}
	streams["dan"] = watchActs(ctxOf("player-dan"), t, h, run, "dan", func() {
		streams["carol"] = watchActs(ctxOf("player-carol"), t, h, run, "carol", func() {
			streams["bob"] = watchActs(ctxOf("player-bob"), t, h, run, "bob", func() {
				_, err := interactWith(t, h, run, "alice", "player-alice", kitchenGrate, toChapelLine)
				require.NoError(t, err, "alice takes the way up")
			})
		})
	})
	for _, who := range []string{"bob", "carol", "dan"} {
		deeds := interactsOn(streams[who], kitchenGrate)
		require.Len(t, deeds, 1, "%s is shown the deed once", who)
		require.Equal(t, []string{"alice"}, viaIDs(deeds[0]), "%s: through alice", who)
		require.Equal(t, toChapelDo, deeds[0].GetInteract().GetWords(), "%s: her own words", who)
		require.Equal(t, toChapelLine, deeds[0].GetInteract().GetOption())
		moves := relocationsOf(streams[who], "alice")
		require.Len(t, moves, 1, "%s is shown her move once", who)
		require.Equal(t, []string{"alice"}, viaIDs(moves[0]))
	}
	stepInto(t, h, run, "carol", "player-carol", pbAt(-17, -14), pbAt(-18, -14))
	require.NotNil(t, interactOption(t, h, run, "carol", "player-carol", kitchenGrate, toChapelLine), "carol, who saw it, is offered the grate")
	stepInto(t, h, run, "dan", "player-dan", pbAt(-16, -14), pbAt(-17, -14))
	require.Nil(t, interactOption(t, h, run, "dan", "player-dan", kitchenGrate, toChapelLine), "dan, shown it through alice, does not know it")
}
