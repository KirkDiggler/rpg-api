package session_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// The kitchen passage (the-mad-king.yaml, rpg-project#958).
const (
	kitchenPurse     = "purse"
	kitchenScullion  = "scullion"
	kitchenGrate     = "grate"
	kitchenPassage   = "kitchen-passage"
	kitchenWay       = "kitchen-way"
	favourLine       = "favour" //nolint:misspell // the castle's option id, as authored
	toChapelLine     = "to-chapel"
	toTheChapelWords = "The tunnel climbs, and you come up through a flagstone behind the chapel's altar."
	theChapelRegion  = "region-chapel"
)

// passageSecrets are the strings no player-facing read may carry before the
// telling: the grate's id, the record's and the concealment's.
var passageSecrets = []string{`"` + kitchenGrate + `"`, kitchenPassage, kitchenWay}

// requireNoPassage requires none of the passage's ids in reads.
func requireNoPassage(t *testing.T, whose string, reads ...proto.Message) {
	t.Helper()
	for _, m := range reads {
		raw, err := protojson.Marshal(m)
		require.NoError(t, err)
		for _, secret := range passageSecrets {
			require.NotContains(t, string(raw), secret, "%s: %T names %s", whose, m, secret)
		}
	}
}

// playerReads is every player-facing read of member: where they are, their
// view, their Afford, their atlas.
func playerReads(t *testing.T, h *acceptanceHarness, run, member, player string) []proto.Message {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), player)
	where, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: run, Member: member})
	require.NoError(t, err)
	view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: member})
	require.NoError(t, err)
	aff, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: run, Member: member})
	require.NoError(t, err)
	atlas, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: run, Member: member})
	require.NoError(t, err)
	return []proto.Message{where, view, aff, atlas}
}

// actsOf is acts as proto messages.
func actsOf(acts []*storypb.Act) []proto.Message {
	out := make([]proto.Message, 0, len(acts))
	for _, a := range acts {
		out = append(out, a)
	}
	return out
}

// offersGrate is whether member's Speak row offers option on the grate.
func offersGrate(t *testing.T, h *acceptanceHarness, run, member, player, option string) bool {
	t.Helper()
	for _, o := range speakRowOf(t, h, run, member, player).GetSpeakOptions() {
		if o.GetPropId() == kitchenGrate && o.GetOptionId() == option {
			return true
		}
	}
	return false
}

// sayToGrate has member say option to the grate, at the revision offered.
func sayToGrate(t *testing.T, h *acceptanceHarness, run, member, player, option string) error {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), player)
	var revision uint64
	for _, o := range speakRowOf(t, h, run, member, player).GetSpeakOptions() {
		if o.GetPropId() == kitchenGrate && o.GetOptionId() == option {
			revision = o.GetRevision()
		}
	}
	_, err := h.handler.Speak(ctx, &sessionpb.SpeakRequest{
		Session: run, Member: member, Target: &sessionpb.SpeakRequest_PropId{PropId: kitchenGrate},
		OptionId: option, Revision: revision,
	})
	return err
}

// sightedProp is whether member's map -- their atlas and their view --
// carries prop.
func sightedProp(t *testing.T, h *acceptanceHarness, run, member, player, prop string) bool {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), player)
	atlas, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: run, Member: member})
	require.NoError(t, err)
	view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: member})
	require.NoError(t, err)
	for _, m := range []proto.Message{atlas, view} {
		raw, err := protojson.Marshal(m)
		require.NoError(t, err)
		if strings.Contains(string(raw), `"`+prop+`"`) {
			return true
		}
	}
	return false
}

// TestAcceptance_TheKitchenPassage is the kitchen's secret passage
// (rpg-project#958) through the api's own doors. Alice, beside the purse,
// picks it up and pays the scullion for the way with bob by the grate in
// earshot and carol out on the road. Before the telling nobody's reads name
// the grate, the record or the concealment. After it alice and bob are
// offered the grate's lines and see it; carol is offered nothing and sees
// nothing. Alice takes the way to the chapel: her stream carries her
// RelocateAct with both ends and the authored words, bob's shows her going,
// and her GetWhere is in the chapel. Bob follows and lands on another cell.
// A catch-up read returns what was live, and nothing of carol's names the
// passage.
func TestAcceptance_TheKitchenPassage(t *testing.T) {
	const run = "passage-run"
	aliceCtx := auth.WithPlayerID(context.Background(), "player-alice")
	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	carolCtx := auth.WithPlayerID(context.Background(), "player-carol")
	dice := &gateDice{}
	dice.face.Store(1)
	h := newAcceptanceHarnessWithDice(t, dice)
	for _, c := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}, {"carol", "player-carol"}} {
		_, err := h.charRepo.Create(aliceCtx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(c.id, c.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile(madKingRaw(t))
	require.NoError(t, err)
	// The encounter's own passage scene's cells: alice beside the purse and
	// two cells from the scullion; bob west of the grate; carol on the road.
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", -22, -9), seatAt("bob", -20, -14), seatAt("carol", 8, 16))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	for _, p := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}, {"carol", "player-carol"}} {
		requireNoPassage(t, p.id+", before", playerReads(t, h, run, p.id, p.player)...)
		requireNoPassage(t, p.id+", before", actsOf(storyOf(auth.WithPlayerID(context.Background(), p.player), t, h.story, run, p.id))...)
	}

	_, err = h.handler.Hold(aliceCtx, &sessionpb.HoldRequest{Session: run, Member: "alice", Target: kitchenPurse, Range: holdReach})
	require.NoError(t, err, "alice picks up the purse")

	carolLive := watchActs(carolCtx, t, h, run, "carol", func() {
		_, sayErr := sayTo(t, h, run, "alice", "player-alice", kitchenScullion, favourLine)
		require.NoError(t, sayErr, "alice pays the scullion for the way")
	})

	require.True(t, offersGrate(t, h, run, "bob", "player-bob", toChapelLine), "bob, by the grate and in earshot, is offered the way")
	require.False(t, offersGrate(t, h, run, "carol", "player-carol", toChapelLine), "carol is not")
	require.True(t, sightedProp(t, h, run, "alice", "player-alice", kitchenGrate), "the grate is on alice's view")
	require.True(t, sightedProp(t, h, run, "bob", "player-bob", kitchenGrate), "and bob's")
	require.False(t, sightedProp(t, h, run, "carol", "player-carol", kitchenGrate), "not carol's")

	// Alice beside the grate: east of it, where the encounter's scene stands
	// its second traveler.
	stepInto(t, h, run, "alice", "player-alice", pbAt(-17, -14), pbAt(-18, -14))
	require.True(t, offersGrate(t, h, run, "alice", "player-alice", toChapelLine), "beside it, alice is offered the way")

	var aliceLive []*storypb.Act
	bobLive := watchActs(bobCtx, t, h, run, "bob", func() {
		aliceLive = watchActs(aliceCtx, t, h, run, "alice", func() {
			require.NoError(t, sayToGrate(t, h, run, "alice", "player-alice", toChapelLine), "alice takes the way up")
		})
	})
	requireNothingRefused(t, aliceLive, "alice")
	requireNothingRefused(t, bobLive, "bob")
	mine := relocationsOf(aliceLive, "alice")
	require.Len(t, mine, 1, "alice is shown her move")
	require.Equal(t, storypb.RelocateEnd_RELOCATE_END_BOTH, mine[0].GetRelocate().GetEnd())
	require.Equal(t, toTheChapelWords, mine[0].GetRelocate().GetWords())
	seen := relocationsOf(bobLive, "alice")
	require.Len(t, seen, 1, "bob is shown alice going")
	where, err := h.handler.GetWhere(aliceCtx, &sessionpb.GetWhereRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	to := mine[0].GetRelocate().GetTo()
	require.Equal(t, [2]float64{to.GetX(), to.GetY()}, [2]float64{where.GetPosition().GetX(), where.GetPosition().GetY()}, "she is where she was set down")
	require.Equal(t, theChapelRegion, regionAt(t, h, run, where.GetPosition()), "in the chapel")

	require.NoError(t, sayToGrate(t, h, run, "bob", "player-bob", toChapelLine), "bob follows")
	aliceAt := worldMember(t, h, run, "alice").Cell
	bobAt := worldMember(t, h, run, "bob").Cell
	require.NotEqual(t, *aliceAt, *bobAt, "bob lands on a different cell")

	requireCaughtUp(t, aliceLive, storyOf(aliceCtx, t, h.story, run, "alice"), "alice")
	requireCaughtUp(t, bobLive, storyOf(bobCtx, t, h.story, run, "bob"), "bob")
	// Carol's own: at the castle's shared view she is SHOWN what alice and
	// bob perceived, marked via them (rpg-project#935); her own acts and her
	// reads -- her map, her options -- name nothing of the passage.
	carolStory := storyOf(carolCtx, t, h.story, run, "carol")
	requireNoPassage(t, "carol's own, live", actsOf(ownActs(carolLive))...)
	requireNoPassage(t, "carol's own, after", actsOf(ownActs(carolStory))...)
	requireNoPassage(t, "carol, after", playerReads(t, h, run, "carol", "player-carol")...)
}

// regionAt is the region the stored world holds pos in.
func regionAt(t *testing.T, h *acceptanceHarness, run string, pos *sessionpb.Position) string {
	t.Helper()
	data, err := sessionorch.NewEncounterRepository(h.redis, 0).GetEncounter(context.Background(), run)
	require.NoError(t, err)
	caps := tkencounter.RefusingCapabilities()
	caps.Participation, caps.Sheets = allStanding{}, standStillSheets{}
	world, err := tkencounter.LoadEncounter(&tkencounter.LoadEncounterInput{Data: *data, Capabilities: caps, Sites: h.manager.Sites})
	require.NoError(t, err)
	region, ok := world.RegionAt(spatial.Position{X: pos.GetX(), Y: pos.GetY()})
	require.True(t, ok, "%v is on the floor", pos)
	return region
}
