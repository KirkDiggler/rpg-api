package encounter

// translate_entity_disappeared_internal_test.go — playtest follow-up, same
// fix as rpg-api#737/the move-restatement Entities fix: white-box tests for
// translateEntityDisappearedEventWithData's REMEMBERED branch.
//
// The bug (Kirk playtest): a monster VANISHED the instant it left sight,
// then reappeared as a proper shadow one step later. Root cause was this
// function's REMEMBERED branch emitting a bare {Position, State: Remembered}
// shell — no Contents, Edges, or ZoneID — which the client's
// replace-wholesale merge rule (HexKnowledgeChanged's doc) took as "this hex
// is now known to be empty," wiping the frozen placement. The very next full
// move restatement re-read the hex from the toolkit's own Memory (which DOES
// have the placement) and restored it — the one-step vanish-then-return this
// tests against.
//
// rpg-toolkit#851 made the real fix possible: perception.View.Memory now
// persists the full frozen observation from the last time a hex was
// genuinely visible, so this branch can carry it instead of asserting
// emptiness. These tests build that Memory entry directly (bypassing a real
// toolkit Move/reveal cycle) to isolate the translator in white-box fashion,
// the same pattern translate_hex_revealed_internal_test.go and
// hydrate_players_test.go already establish for this package's other
// data-aware translators.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/events"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
)

type TranslateEntityDisappearedInternalSuite struct {
	suite.Suite
	now time.Time
}

func (s *TranslateEntityDisappearedInternalSuite) SetupTest() {
	s.now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

// TestTranslateEntityDisappearedEventWithData_OutOfSight_CarriesFrozenPlacement
// is the regression test: a viewer who has NEVER previously seen this
// monster loses sight of the WHOLE hex it was standing on (not just the
// entity walking away — the hex itself falls out of view), and the emitted
// REMEMBERED record must carry the frozen placement AND a matching Entities
// disclosure, not an empty shell. Proved to fail against the pre-fix shape
// (temporarily reverted the fix, confirmed Contents/Entities came back
// empty, restored it).
func (s *TranslateEntityDisappearedInternalSuite) TestTranslateEntityDisappearedEventWithData_OutOfSight_CarriesFrozenPlacement() {
	const viewer = core.PlayerID("player-A")
	const mover = core.EntityID("char-A")
	const monster = core.EntityID("skeleton-1")
	lastKnown := core.Hex{Q: 5, R: -5, S: 0}
	currentPos := core.Hex{Q: 0, R: 0, S: 0} // far outside sight range 1 of lastKnown

	data := encounter.NewData("enc-1")
	view := perception.NewView(viewer, currentPos, 1)
	// The frozen observation, as the toolkit's own refreshObservations would
	// have written it the last time lastKnown was genuinely visible: the
	// monster present, real edges/zone, all now stale but honest.
	frozenOffset := core.PlacementOffset{-0.25, 1.5, 2.75}
	view.Memory[lastKnown] = perception.HexObservation{
		Position: lastKnown,
		State:    perception.KnowledgeStateVisible, // stale write-time value; State is re-derived below
		ZoneID:   "sentry",
		Contents: []perception.Placement{{EntityID: monster, Offset: &frozenOffset}},
	}
	data.Players[viewer] = &encounter.PlayerData{ID: viewer, EntityID: mover, View: view, HP: 16, MaxHP: 16}
	currentOffset := core.PlacementOffset{99, 98, 97}
	data.Monsters[monster] = &encounter.MonsterData{
		ID: monster, Position: lastKnown, HP: 11, MaxHP: 11, AC: 13,
		MonsterRef: "dnd5e:monsters:skeleton", Offset: &currentOffset,
	}

	evt := events.NewEntityDisappearedEvent("enc-1", uint64(1), monster, map[core.PlayerID]core.Hex{
		viewer: lastKnown,
	})

	out, err := translateEntityDisappearedEventWithData(context.Background(), nil, evt, viewer, s.now, data, nil)
	s.Require().NoError(err)

	changed := out.GetHexKnowledgeChanged()
	s.Require().NotNil(changed)
	s.Require().Len(changed.GetHexes(), 1)

	record := changed.GetHexes()[0]
	s.Require().Equal(encounterv2pb.HexState_HEX_STATE_REMEMBERED, record.GetState())
	s.Require().Equal("sentry", record.GetZoneId(), "the frozen record must carry the memory's own zone_id, not lose it")

	var monsterPlacement *encounterv2pb.Placement
	for _, p := range record.GetContents() {
		if p.GetEntityId() == string(monster) {
			monsterPlacement = p
		}
	}
	s.Require().NotNil(monsterPlacement,
		"the REMEMBERED record must carry the frozen placement, not an empty shell — "+
			"an empty Contents here is exactly the vanish-then-return bug")
	s.Require().Equal(-0.25, monsterPlacement.GetOffset().GetX())
	s.Require().Equal(1.5, monsterPlacement.GetOffset().GetY())
	s.Require().Equal(2.75, monsterPlacement.GetOffset().GetZ(),
		"REMEMBERED must project the frozen offset, never the different current data")

	var disclosed *encounterv2pb.Entity
	for _, e := range changed.GetEntities() {
		if e.GetId() == string(monster) {
			disclosed = e
		}
	}
	s.Require().NotNil(disclosed,
		"a viewer who has never seen this monster before needs its identity in THIS SAME envelope "+
			"for the frozen placement to resolve client-side")
	s.Require().Equal(encounterv2pb.EntityType_ENTITY_TYPE_MONSTER, disclosed.GetType())
}

// TestTranslateEntityDisappearedEventWithData_StillVisible_RebuildsFromCurrentData
// proves the OTHER branch (hex stays visible, only the entity itself left)
// is unaffected: State stays VISIBLE and Contents/Entities come from
// disclosedEntities' current authorization, not frozen Memory.
func (s *TranslateEntityDisappearedInternalSuite) TestTranslateEntityDisappearedEventWithData_StillVisible_RebuildsFromCurrentData() {
	const viewer = core.PlayerID("player-A")
	const mover = core.EntityID("char-A")
	const bystander = core.EntityID("skeleton-2")
	sharedHex := core.Hex{Q: 1, R: -1, S: 0}
	viewerPos := core.Hex{Q: 0, R: 0, S: 0} // adjacent, well within sight range 5

	data := encounter.NewData("enc-1")
	view := perception.NewView(viewer, viewerPos, 5)
	staleOffset := core.PlacementOffset{9, 8, 7}
	view.Memory[sharedHex] = perception.HexObservation{
		Position: sharedHex,
		State:    perception.KnowledgeStateVisible,
		Contents: []perception.Placement{{EntityID: bystander, Offset: &staleOffset}},
	}
	data.Players[viewer] = &encounter.PlayerData{ID: viewer, EntityID: mover, View: view, HP: 16, MaxHP: 16}
	// bystander is STILL standing on sharedHex even after the departed
	// entity (whichever one triggered this event) left it.
	currentOffset := core.PlacementOffset{0.125, -2.5, 3.75}
	data.Monsters[bystander] = &encounter.MonsterData{
		ID: bystander, Position: sharedHex, HP: 5, MaxHP: 5, AC: 12,
		MonsterRef: "dnd5e:monsters:skeleton", Offset: &currentOffset,
	}

	evt := events.NewEntityDisappearedEvent("enc-1", uint64(1), "skeleton-1", map[core.PlayerID]core.Hex{
		viewer: sharedHex,
	})

	out, err := translateEntityDisappearedEventWithData(context.Background(), nil, evt, viewer, s.now, data, nil)
	s.Require().NoError(err)

	changed := out.GetHexKnowledgeChanged()
	s.Require().NotNil(changed)
	record := changed.GetHexes()[0]
	s.Require().Equal(encounterv2pb.HexState_HEX_STATE_VISIBLE, record.GetState(),
		"the hex itself is still visible; only the departed entity left")

	var bystanderPlacement *encounterv2pb.Placement
	for _, p := range record.GetContents() {
		if p.GetEntityId() == string(bystander) {
			bystanderPlacement = p
		}
	}
	s.Require().NotNil(bystanderPlacement, "another entity still genuinely standing there must not vanish too")
	s.Require().Equal(0.125, bystanderPlacement.GetOffset().GetX())
	s.Require().Equal(-2.5, bystanderPlacement.GetOffset().GetY())
	s.Require().Equal(3.75, bystanderPlacement.GetOffset().GetZ(),
		"VISIBLE current truth must win over a different remembered offset")

	var disclosed *encounterv2pb.Entity
	for _, e := range changed.GetEntities() {
		if e.GetId() == string(bystander) {
			disclosed = e
		}
	}
	s.Require().NotNil(disclosed, "the still-present bystander must be disclosed too, not just placed")
}

// TestTranslateEntityDisappearedEventWithData_NoMemoryEntry_FallsBackEmpty
// covers the defensive no-fallback-to-live-data case: if the viewer's own
// Memory genuinely has no entry for lastKnown, the record degrades to the
// pre-#851 empty shell rather than reaching into current world data.
func (s *TranslateEntityDisappearedInternalSuite) TestTranslateEntityDisappearedEventWithData_NoMemoryEntry_FallsBackEmpty() {
	const viewer = core.PlayerID("player-A")
	const mover = core.EntityID("char-A")
	lastKnown := core.Hex{Q: 9, R: -9, S: 0}
	currentPos := core.Hex{Q: 0, R: 0, S: 0}

	data := encounter.NewData("enc-1")
	view := perception.NewView(viewer, currentPos, 1) // Memory left empty — no entry for lastKnown
	data.Players[viewer] = &encounter.PlayerData{ID: viewer, EntityID: mover, View: view, HP: 16, MaxHP: 16}

	evt := events.NewEntityDisappearedEvent("enc-1", uint64(1), "skeleton-1", map[core.PlayerID]core.Hex{
		viewer: lastKnown,
	})

	out, err := translateEntityDisappearedEventWithData(context.Background(), nil, evt, viewer, s.now, data, nil)
	s.Require().NoError(err)

	changed := out.GetHexKnowledgeChanged()
	record := changed.GetHexes()[0]
	s.Require().Equal(encounterv2pb.HexState_HEX_STATE_REMEMBERED, record.GetState())
	s.Require().Empty(record.GetContents())
	s.Require().Empty(changed.GetEntities())
}

// TestTranslateEntityDisappearedEventWithData_ViewerNotInPerPlayer_ErrViewerSawNothing
// preserves the existing error contract.
func (s *TranslateEntityDisappearedInternalSuite) TestTranslateEntityDisappearedEventWithData_ViewerNotInPerPlayer_ErrViewerSawNothing() {
	evt := events.NewEntityDisappearedEvent("enc-1", uint64(1), "skeleton-1", nil)
	_, err := translateEntityDisappearedEventWithData(context.Background(), nil, evt, "player-X", s.now, nil, nil)
	s.Require().ErrorIs(err, ErrViewerSawNothing)
}

func TestTranslateEntityDisappearedInternalSuite(t *testing.T) {
	suite.Run(t, new(TranslateEntityDisappearedInternalSuite))
}
