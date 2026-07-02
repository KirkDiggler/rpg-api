package encounter_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/events"
)

type TranslateSuite struct {
	suite.Suite
	now time.Time
}

func (s *TranslateSuite) SetupTest() {
	s.now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func (s *TranslateSuite) TestHexToPosition_CubeMapping() {
	got := v2encounter.HexToPosition(core.Hex{Q: 1, R: -2, S: 1})
	s.Require().Equal(int32(1), got.X)
	s.Require().Equal(int32(-2), got.Y)
	s.Require().Equal(int32(1), got.Z)
}

func (s *TranslateSuite) TestTranslateEvent_MoveEvent_FullPath() {
	// NewMoveEvent signature (events/move.go:43): (encID, seq, mover, from, path, perPlayer)
	evt := events.NewMoveEvent(
		"enc-1",
		uint64(1),
		"char-A",
		core.Hex{Q: 0, R: 0, S: 0},
		[]core.Hex{{Q: 0, R: 0, S: 0}, {Q: 1, R: -1, S: 0}, {Q: 2, R: -2, S: 0}},
		map[core.PlayerID]events.MovePlayerSlice{
			"player-B": {SeenSegments: []core.Hex{{Q: 0, R: 0, S: 0}, {Q: 1, R: -1, S: 0}, {Q: 2, R: -2, S: 0}}},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	moved := out.GetEntityMoved()
	s.Require().NotNil(moved)
	s.Require().Equal("char-A", moved.EntityId)
	s.Require().Len(moved.ActualPath, 3)
	s.Require().True(proto.Equal(moved.From, moved.ActualPath[0]))
	s.Require().True(proto.Equal(moved.To, moved.ActualPath[2]))
}

func (s *TranslateSuite) TestTranslateEvent_MoveEvent_EmptySliceReturnsErrViewerSawNothing() {
	evt := events.NewMoveEvent(
		"enc-1", uint64(1), "char-A",
		core.Hex{Q: 0, R: 0, S: 0},
		[]core.Hex{{Q: 0, R: 0, S: 0}},
		map[core.PlayerID]events.MovePlayerSlice{
			"player-B": {SeenSegments: nil},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_MoveEvent_ViewerNotInPerPlayerReturnsErrViewerSawNothing() {
	evt := events.NewMoveEvent("enc-1", uint64(1), "char-A", core.Hex{}, nil, nil)
	_, err := v2encounter.TranslateEvent(evt, "player-X", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_HexRevealedEvent_HappyPath() {
	evt := events.NewHexRevealedEvent(
		"enc-1",
		uint64(2),
		map[core.PlayerID]events.HexRevealedSlice{
			"player-A": {Hexes: core.HexSet{{Q: 1, R: 0, S: -1}: {}, {Q: 0, R: 1, S: -1}: {}}},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	revealed := out.GetGeometryRevealed()
	s.Require().NotNil(revealed)
	s.Require().Len(revealed.Hexes, 2)
}

func (s *TranslateSuite) TestTranslateEvent_HexRevealedEvent_EmptySliceReturnsErrViewerSawNothing() {
	evt := events.NewHexRevealedEvent(
		"enc-1",
		uint64(2),
		map[core.PlayerID]events.HexRevealedSlice{
			"player-A": {Hexes: core.HexSet{}},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_HexRevealedEvent_ViewerNotInPerPlayerReturnsErrViewerSawNothing() {
	evt := events.NewHexRevealedEvent("enc-1", uint64(2), nil)
	_, err := v2encounter.TranslateEvent(evt, "player-X", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_Sequence_SetCorrectly() {
	evt := events.NewMoveEvent(
		"enc-1",
		uint64(42),
		"char-A",
		core.Hex{Q: 0, R: 0, S: 0},
		[]core.Hex{{Q: 0, R: 0, S: 0}},
		map[core.PlayerID]events.MovePlayerSlice{
			"player-B": {SeenSegments: []core.Hex{{Q: 0, R: 0, S: 0}}},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().NoError(err)
	s.Require().Equal(int64(42), out.Sequence)
}

func (s *TranslateSuite) TestTranslateEvent_EntityAppearedEvent_HappyPath() {
	evt := events.NewEntityAppearedEvent(
		"enc-1", uint64(7), "char-A",
		core.Hex{Q: 3, R: -1, S: -2},
		map[core.PlayerID]struct{}{"player-B": {}},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().NoError(err)
	app := out.GetEntityAppeared()
	s.Require().NotNil(app)
	s.Require().Equal("char-A", app.Entity.Id)
	s.Require().Equal(int32(3), app.Entity.Position.X)
	s.Require().Equal(int32(-1), app.Entity.Position.Y)
	s.Require().Equal(int32(-2), app.Entity.Position.Z)
	s.Require().Equal("entered LOS", app.Reason)
}

func (s *TranslateSuite) TestTranslateEvent_EntityAppearedEvent_ViewerNotInPerPlayerReturnsErrViewerSawNothing() {
	evt := events.NewEntityAppearedEvent("enc-1", uint64(7), "char-A", core.Hex{}, nil)
	_, err := v2encounter.TranslateEvent(evt, "player-X", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_EntityDisappearedEvent_HappyPath() {
	evt := events.NewEntityDisappearedEvent(
		"enc-1", uint64(8), "char-A",
		map[core.PlayerID]core.Hex{"player-B": {Q: 5, R: -2, S: -3}},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().NoError(err)
	dis := out.GetEntityDisappeared()
	s.Require().NotNil(dis)
	s.Require().Equal("char-A", dis.EntityId)
	s.Require().Equal("left LOS", dis.Reason)
	// Per-viewer last-known position: B's PerPlayer entry was (5,-2,-3),
	// proves the translator picks the viewer's own last-known hex.
	s.Require().NotNil(dis.LastKnownPosition)
	s.Require().Equal(int32(5), dis.LastKnownPosition.X)
	s.Require().Equal(int32(-2), dis.LastKnownPosition.Y)
	s.Require().Equal(int32(-3), dis.LastKnownPosition.Z)
}

func (s *TranslateSuite) TestTranslateEvent_EntityDisappearedEvent_ViewerNotInPerPlayerReturnsErrViewerSawNothing() {
	evt := events.NewEntityDisappearedEvent("enc-1", uint64(8), "char-A", nil)
	_, err := v2encounter.TranslateEvent(evt, "player-X", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_UnknownEventTypeReturnsErrUnknownEventType() {
	// The events.EncounterEvent interface is sealed (unexported marker method);
	// external packages cannot implement it, so we cannot synthesize a "novel"
	// concrete type for the default branch. Wave 2.7 mapped DoorOpenedEvent
	// (the previous unknown-type fixture), so all current concretes route to
	// real translators. Passing a nil events.EncounterEvent exercises the
	// default branch — a future toolkit event lacking a translator case will
	// reproduce this same shape until its case is added.
	_, err := v2encounter.TranslateEvent(nil, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrUnknownEventType))
}

func (s *TranslateSuite) TestTranslateEvent_DoorOpenedEvent_HappyPath() {
	evt := events.NewDoorOpenedEvent(
		"enc-1", uint64(11), "door-east", "char-alice",
		map[core.PlayerID]events.DoorOpenedPlayerSlice{
			"player-A": {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	door := out.GetDoorOpened()
	s.Require().NotNil(door)
	s.Require().Equal("door-east", door.GetDoorEntityId())
	// Cause/effect split: DoorOpened envelope carries only door identity;
	// HexRevealedEvent translation (GeometryRevealed) carries the geometry
	// delta as a parallel event. revealed_hexes / revealed_walls /
	// removed_walls must be empty here.
	s.Require().Empty(door.GetRevealedHexes(), "revealed_hexes belongs on the parallel GeometryRevealed event")
	s.Require().Empty(door.GetRevealedWalls(), "revealed_walls deferred — not modeled in v0.2.0")
	s.Require().Empty(door.GetRemovedWalls(), "removed_walls deferred — not modeled in v0.2.0")
	s.Require().Equal(int64(11), out.Sequence)
}

func (s *TranslateSuite) TestTranslateEvent_DoorOpenedEvent_NotVisible_ReturnsErrViewerSawNothing() {
	evt := events.NewDoorOpenedEvent(
		"enc-1", uint64(11), "door-east", "char-alice",
		map[core.PlayerID]events.DoorOpenedPlayerSlice{
			// Defensive: even if the toolkit changed to populate Visible:false
			// for non-visible viewers, the translator must skip them.
			"player-A": {Visible: false},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_DoorOpenedEvent_ViewerNotInPerPlayer_ReturnsErrViewerSawNothing() {
	evt := events.NewDoorOpenedEvent(
		"enc-1", uint64(11), "door-east", "char-alice",
		map[core.PlayerID]events.DoorOpenedPlayerSlice{
			"player-A": {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, "player-X", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

// TestTranslateSnapshot_EmptyEncounter verifies that TranslateSnapshot with a
// nil pbEncounter still produces a valid SnapshotDelivered envelope with an
// empty (nil) Encounter field — guards against panics on nil input.
func (s *TranslateSuite) TestTranslateSnapshot_EmptyEncounter() {
	out := v2encounter.TranslateSnapshot(nil, s.now)
	s.Require().NotNil(out)
	sd := out.GetSnapshotDelivered()
	s.Require().NotNil(sd)
	s.Require().Nil(sd.GetEncounter(), "nil pbEncounter → nil Encounter field")
	s.Require().Equal(int64(0), out.Sequence, "snapshot events are sequence 0 (pre-history)")
}

// TestTranslateSnapshot_PopulatedEncounter verifies that TranslateSnapshot wraps
// a populated proto Encounter inside SnapshotDelivered.
func (s *TranslateSuite) TestTranslateSnapshot_PopulatedEncounter() {
	pbEnc := &encounterv2pb.Encounter{
		Id:   "enc-unit-1",
		Mode: encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
		Space: &encounterv2pb.Space{
			Entities: []*encounterv2pb.Entity{{Id: "char-A", Position: &encounterv2pb.Position{X: 0, Y: 0, Z: 0}}},
		},
	}
	out := v2encounter.TranslateSnapshot(pbEnc, s.now)
	s.Require().NotNil(out)
	sd := out.GetSnapshotDelivered()
	s.Require().NotNil(sd)
	s.Require().NotNil(sd.GetEncounter())
	s.Require().Equal("enc-unit-1", sd.GetEncounter().GetId())
	s.Require().Equal(int64(0), out.Sequence)
}

// TestBuildReplayEvents_NilEncounter verifies that a nil pbEncounter produces
// an empty replay slice.
func (s *TranslateSuite) TestBuildReplayEvents_NilEncounter() {
	out := v2encounter.BuildReplayEvents(nil, s.now)
	s.Require().Empty(out, "nil encounter → no replay events")
}

// TestBuildReplayEvents_EntitiesAndHexes verifies that BuildReplayEvents emits one
// EntityAppeared per entity in Space.Entities and one GeometryRevealed carrying
// all of Space.Hexes. ProjectFor is responsible for sorting; BuildReplayEvents
// is a pure pass-through.
func (s *TranslateSuite) TestBuildReplayEvents_EntitiesAndHexes() {
	pbEnc := &encounterv2pb.Encounter{
		Id: "enc-unit-2",
		Space: &encounterv2pb.Space{
			Entities: []*encounterv2pb.Entity{
				{Id: "char-alice", Position: &encounterv2pb.Position{X: 0, Y: 0, Z: 0}},
				{Id: "char-bob", Position: &encounterv2pb.Position{X: 1, Y: -1, Z: 0}},
			},
			Hexes: []*encounterv2pb.Hex{
				{Position: &encounterv2pb.Position{X: -1, Y: 1, Z: 0}},
				{Position: &encounterv2pb.Position{X: 0, Y: 0, Z: 0}},
				{Position: &encounterv2pb.Position{X: 1, Y: -1, Z: 0}},
			},
		},
	}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)

	// Expect: 2 EntityAppeared + 1 GeometryRevealed = 3 events total.
	s.Require().Len(out, 3, "2 entities + 1 geometry event")

	var appearedIDs []string
	var geoRevealed *encounterv2pb.GeometryRevealed
	for _, ev := range out {
		if a := ev.GetEntityAppeared(); a != nil {
			appearedIDs = append(appearedIDs, a.GetEntity().GetId())
			s.Require().Equal("initial state", a.GetReason())
			s.Require().Equal(int64(0), ev.Sequence, "replay events use sequence 0")
		}
		if g := ev.GetGeometryRevealed(); g != nil {
			geoRevealed = g
		}
	}
	s.Require().ElementsMatch([]string{"char-alice", "char-bob"}, appearedIDs,
		"both entities must appear in replay")
	s.Require().NotNil(geoRevealed, "GeometryRevealed must be present")
	s.Require().Len(geoRevealed.GetHexes(), 3, "all 3 hexes must be included")
}

// TestBuildReplayEvents_NoHexes verifies that an encounter with entities but no
// hexes emits only EntityAppeared events (no GeometryRevealed).
func (s *TranslateSuite) TestBuildReplayEvents_NoHexes() {
	pbEnc := &encounterv2pb.Encounter{
		Space: &encounterv2pb.Space{
			Entities: []*encounterv2pb.Entity{
				{Id: "char-alice", Position: &encounterv2pb.Position{}},
			},
		},
	}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)
	s.Require().Len(out, 1, "1 entity → 1 EntityAppeared, no GeometryRevealed")
	s.Require().NotNil(out[0].GetEntityAppeared())
}

// TestBuildReplayEvents_NoEntitiesWithHexes verifies that an encounter with
// hexes but no entities emits only GeometryRevealed (no EntityAppeared).
func (s *TranslateSuite) TestBuildReplayEvents_NoEntitiesWithHexes() {
	pbEnc := &encounterv2pb.Encounter{
		Space: &encounterv2pb.Space{
			Hexes: []*encounterv2pb.Hex{
				{Position: &encounterv2pb.Position{X: 0, Y: 0, Z: 0}},
			},
		},
	}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)
	s.Require().Len(out, 1, "no entities → only GeometryRevealed")
	s.Require().NotNil(out[0].GetGeometryRevealed())
}

// TestBuildReplayEvents_HexesPreserveInputOrder verifies that GeometryRevealed
// carries pbEncounter.Space.Hexes verbatim. Sort responsibility lives in
// ProjectFor; BuildReplayEvents must not reorder.
func (s *TranslateSuite) TestBuildReplayEvents_HexesPreserveInputOrder() {
	hexes := []*encounterv2pb.Hex{
		{Position: &encounterv2pb.Position{X: -1, Y: 0, Z: 1}},
		{Position: &encounterv2pb.Position{X: 0, Y: 0, Z: 0}},
		{Position: &encounterv2pb.Position{X: 1, Y: -1, Z: 0}},
		{Position: &encounterv2pb.Position{X: 2, Y: -1, Z: -1}},
	}
	pbEnc := &encounterv2pb.Encounter{Space: &encounterv2pb.Space{Hexes: hexes}}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)
	s.Require().Len(out, 1)
	got := out[0].GetGeometryRevealed().GetHexes()
	s.Require().Len(got, 4)
	for i := range got {
		s.Require().True(proto.Equal(got[i], hexes[i]), "hex %d must match input", i)
	}
}

// TestTranslateEvent_ResourceChangedEvent verifies that a ResourceChangedEvent
// translates to a proto ResourceChanged envelope for a viewer with visibility.
func (s *TranslateSuite) TestTranslateEvent_ResourceChangedEvent_VisibleViewer() {
	viewerID := core.PlayerID("player-A")
	evt := events.NewResourceChangedEvent(
		"enc-1", 3,
		core.EntityID("char-bob"),
		"rage_charges",
		1, 2,
		map[core.PlayerID]events.ResourceChangedSlice{
			viewerID: {Visible: true},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, viewerID, s.now)
	s.Require().NoError(err)
	rc := out.GetResourceChanged()
	s.Require().NotNil(rc)
	s.Equal("char-bob", rc.GetEntityId())
	s.Equal(int32(1), rc.GetNewCurrent())
	s.Equal(int32(2), rc.GetMax())
	s.NotNil(rc.GetResourceRef())
}

// TestTranslateEvent_ResourceChangedEvent_ViewerSawNothing verifies that a viewer
// absent from the audience gets ErrViewerSawNothing.
func (s *TranslateSuite) TestTranslateEvent_ResourceChangedEvent_ViewerSawNothing() {
	viewerID := core.PlayerID("player-A")
	other := core.PlayerID("player-B")
	evt := events.NewResourceChangedEvent(
		"enc-1", 4,
		core.EntityID("char-bob"),
		"rage_charges",
		1, 2,
		map[core.PlayerID]events.ResourceChangedSlice{
			other: {Visible: true},
		},
	)
	_, err := v2encounter.TranslateEvent(evt, viewerID, s.now)
	s.Require().Error(err)
	s.True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

// Compile-time check: ensure the package exports used below are accessible.
var _ = (*encounterv2pb.EncounterEvent)(nil)

func TestTranslateSuite(t *testing.T) {
	suite.Run(t, new(TranslateSuite))
}
