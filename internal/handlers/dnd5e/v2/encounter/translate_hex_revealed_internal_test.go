package encounter

// translate_hex_revealed_internal_test.go — rpg-api#687: white-box tests
// for translateHexRevealedEventWithData, the data-aware translator for the
// LIVE incremental HexKnowledgeChanged event (rpg-api-protos#197 retired the
// GeometryRevealed message this used to build). These live in package
// encounter (not encounter_test) so they can call the unexported function
// directly — mirroring hydrate_players_test.go's white-box precedent for
// translateEntityAppearedEventWithData.
//
// Why this exists alongside ProjectFor's snapshot-path zone_id fix: the
// connect-time snapshot (ProjectFor) and the live per-move
// HexKnowledgeChanged event are two SEPARATE hex-projection paths
// (translateHexRevealedEvent only has the toolkit event in hand, not the
// persisted encounter.Data) — #687's Done bar ("every revealed hex carrying
// the right zone_id") covers hexes revealed AFTER connect via movement, not
// just the ones present at connect time. Without this, a hex a player
// reveals mid-session would carry zone_id="" forever until their next
// reconnect.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/events"
)

type TranslateHexRevealedInternalSuite struct {
	suite.Suite
	now time.Time
}

func (s *TranslateHexRevealedInternalSuite) SetupTest() {
	s.now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

// TestTranslateHexRevealedEventWithData_SetsZoneIdPerRegionMembership
// proves the live GeometryRevealed event gets the same zone_id passthrough
// as the connect-time snapshot: a hex tagged into a region carries that
// region's id; a hex not tagged into any region carries "".
func (s *TranslateHexRevealedInternalSuite) TestTranslateHexRevealedEventWithData_SetsZoneIdPerRegionMembership() {
	chamber2Hex := core.Hex{Q: 3, R: -3, S: 0}
	untaggedHex := core.Hex{Q: 4, R: -4, S: 0}

	data := encounter.NewData("enc-1")
	data.Space = &encounter.SpaceData{
		Width: 10, Height: 10,
		Regions: []encounter.RegionData{
			{ID: "chamber-2", Archetype: encounter.ArchetypeChamber, Hexes: core.NewHexSet(chamber2Hex)},
		},
	}

	evt := events.NewHexRevealedEvent("enc-1", uint64(5), map[core.PlayerID]events.HexRevealedSlice{
		"player-alice": {Hexes: core.NewHexSet(chamber2Hex, untaggedHex)},
	})

	out, err := translateHexRevealedEventWithData(evt, "player-alice", s.now, data)
	s.Require().NoError(err)

	changed := out.GetHexKnowledgeChanged()
	s.Require().NotNil(changed)
	s.Require().Len(changed.GetHexes(), 2)

	byPos := make(map[[3]int32]string)
	for _, h := range changed.GetHexes() {
		byPos[[3]int32{h.GetPosition().GetX(), h.GetPosition().GetY(), h.GetPosition().GetZ()}] = h.GetZoneId()
	}
	s.Require().Equal("chamber-2", byPos[[3]int32{3, -3, 0}], "chamber-2's hex must carry its region id")
	s.Require().Equal("", byPos[[3]int32{4, -4, 0}], "untagged hex must carry empty zone_id, not a derived/nearest one")
}

// TestTranslateHexRevealedEventWithData_NilData_FallsBackEmptyZoneId
// covers the defensive nil-data case (should not occur in production —
// translateForStream always loads data before calling this — but a nil
// *encounter.Data must degrade to the pre-#687 bare-hex shape, not panic).
func (s *TranslateHexRevealedInternalSuite) TestTranslateHexRevealedEventWithData_NilData_FallsBackEmptyZoneId() {
	h := core.Hex{Q: 1, R: -1, S: 0}
	evt := events.NewHexRevealedEvent("enc-1", uint64(1), map[core.PlayerID]events.HexRevealedSlice{
		"player-alice": {Hexes: core.NewHexSet(h)},
	})

	res, err := translateHexRevealedEventWithData(evt, "player-alice", s.now, nil)
	s.Require().NoError(err)
	changed := res.GetHexKnowledgeChanged()
	s.Require().Len(changed.GetHexes(), 1)
	s.Require().Empty(changed.GetHexes()[0].GetZoneId())
}

// TestTranslateHexRevealedEventWithData_ViewerNotInPerPlayer_ErrViewerSawNothing
// preserves translateHexRevealedEvent's existing error contract — the
// data-aware variant must not weaken it.
func (s *TranslateHexRevealedInternalSuite) TestTranslateHexRevealedEventWithData_ViewerNotInPerPlayer_ErrViewerSawNothing() {
	evt := events.NewHexRevealedEvent("enc-1", uint64(1), nil)
	_, err := translateHexRevealedEventWithData(evt, "player-alice", s.now, encounter.NewData("enc-1"))
	s.Require().ErrorIs(err, ErrViewerSawNothing)
}

func TestTranslateHexRevealedInternalSuite(t *testing.T) {
	suite.Run(t, new(TranslateHexRevealedInternalSuite))
}
