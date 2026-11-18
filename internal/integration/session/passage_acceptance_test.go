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
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
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

// interactOption is the thing member's Speak row offers to do on prop,
// nil when none is offered (rpg-project#959).
func interactOption(t *testing.T, h *acceptanceHarness, run, member, player, prop, option string) *sessionpb.InteractOption {
	t.Helper()
	for _, o := range speakRowOf(t, h, run, member, player).GetInteractOptions() {
		if o.GetPropId() == prop && o.GetOptionId() == option {
			return o
		}
	}
	return nil
}

// interactWith has member do option to prop through Interact, at the
// revision offered.
func interactWith(t *testing.T, h *acceptanceHarness, run, member, player, prop, option string) (*sessionpb.InteractResponse, error) {
	t.Helper()
	var revision uint64
	if o := interactOption(t, h, run, member, player, prop, option); o != nil {
		revision = o.GetRevision()
	}
	return h.handler.Interact(auth.WithPlayerID(context.Background(), player), &sessionpb.InteractRequest{
		Session: run, Actor: member, Prop: prop, OptionId: option, Revision: revision,
	})
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
