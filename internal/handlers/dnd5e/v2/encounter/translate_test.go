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
	// NewMoveEvent signature (events/move.go): (encID, seq, mover, from, path,
	// perPlayer, moverPlayerID, moverKnownHexes) — the last two (rpg-api#737)
	// are irrelevant to the bare TranslateEvent path this test exercises.
	evt := events.NewMoveEvent(
		"enc-1",
		uint64(1),
		"char-A",
		core.Hex{Q: 0, R: 0, S: 0},
		[]core.Hex{{Q: 0, R: 0, S: 0}, {Q: 1, R: -1, S: 0}, {Q: 2, R: -2, S: 0}},
		map[core.PlayerID]events.MovePlayerSlice{
			"player-B": {SeenSegments: []core.Hex{{Q: 0, R: 0, S: 0}, {Q: 1, R: -1, S: 0}, {Q: 2, R: -2, S: 0}}},
		},
		"", nil,
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
		"", nil,
	)
	_, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrViewerSawNothing))
}

func (s *TranslateSuite) TestTranslateEvent_MoveEvent_ViewerNotInPerPlayerReturnsErrViewerSawNothing() {
	evt := events.NewMoveEvent("enc-1", uint64(1), "char-A", core.Hex{}, nil, nil, "", nil)
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

	changed := out.GetHexKnowledgeChanged()
	s.Require().NotNil(changed)
	s.Require().Len(changed.Hexes, 2)
	for _, h := range changed.Hexes {
		s.Require().Equal(encounterv2pb.HexState_HEX_STATE_VISIBLE, h.GetState())
		s.Require().Empty(h.GetContents(), "a bare HexRevealedEvent names no occupant")
	}
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
		"", nil,
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
	changed := out.GetHexKnowledgeChanged()
	s.Require().NotNil(changed)

	s.Require().Len(changed.Entities, 1)
	s.Require().Equal("char-A", changed.Entities[0].GetId())

	s.Require().Len(changed.Hexes, 1)
	hex := changed.Hexes[0]
	s.Require().Equal(encounterv2pb.HexState_HEX_STATE_VISIBLE, hex.GetState())
	s.Require().Equal(int32(3), hex.GetPosition().GetX())
	s.Require().Equal(int32(-1), hex.GetPosition().GetY())
	s.Require().Equal(int32(-2), hex.GetPosition().GetZ())
	s.Require().Len(hex.GetContents(), 1)
	s.Require().Equal("char-A", hex.GetContents()[0].GetEntityId())
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
	changed := out.GetHexKnowledgeChanged()
	s.Require().NotNil(changed)
	s.Require().Empty(changed.Entities, "no entity is disclosed on a disappearance — see the translator's doc")

	s.Require().Len(changed.Hexes, 1)
	hex := changed.Hexes[0]
	s.Require().Equal(encounterv2pb.HexState_HEX_STATE_REMEMBERED, hex.GetState())
	s.Require().Empty(hex.GetContents(), "we do not know the entity is still there — contents must stay empty")
	// Per-viewer last-known position: B's PerPlayer entry was (5,-2,-3),
	// proves the translator picks the viewer's own last-known hex.
	s.Require().Equal(int32(5), hex.GetPosition().GetX())
	s.Require().Equal(int32(-2), hex.GetPosition().GetY())
	s.Require().Equal(int32(-3), hex.GetPosition().GetZ())
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
	// Cause/effect split (rpg-api-protos#197): DoorOpened is now a pure
	// notification carrying only door_entity_id — no other fields exist on
	// the message to carry geometry. The parallel HexRevealedEvent
	// (translated to HexKnowledgeChanged) carries the geometry/visibility
	// delta as a separate event.
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
			Entities: []*encounterv2pb.Entity{{Id: "char-A"}},
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

// TestBuildReplayEvents_EntitiesAndHexes verifies that BuildReplayEvents emits a
// single HexKnowledgeChanged carrying all of Space.Entities and all of
// Space.Hexes verbatim (rpg-api-protos#197 retired the per-entity
// EntityAppeared / whole-set GeometryRevealed pair this used to synthesize).
// ProjectFor is responsible for sorting; BuildReplayEvents is a pure
// pass-through.
func (s *TranslateSuite) TestBuildReplayEvents_EntitiesAndHexes() {
	pbEnc := &encounterv2pb.Encounter{
		Id: "enc-unit-2",
		Space: &encounterv2pb.Space{
			Entities: []*encounterv2pb.Entity{
				{Id: "char-alice"},
				{Id: "char-bob"},
			},
			Hexes: []*encounterv2pb.HexRecord{
				{Position: &encounterv2pb.Position{X: -1, Y: 1, Z: 0}},
				{Position: &encounterv2pb.Position{X: 0, Y: 0, Z: 0}},
				{Position: &encounterv2pb.Position{X: 1, Y: -1, Z: 0}},
			},
		},
	}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)

	s.Require().Len(out, 1, "full hydration is one HexKnowledgeChanged")
	changed := out[0].GetHexKnowledgeChanged()
	s.Require().NotNil(changed)
	s.Require().Equal(int64(0), out[0].Sequence, "replay events use sequence 0")

	appearedIDs := make([]string, 0, len(changed.GetEntities()))
	for _, e := range changed.GetEntities() {
		appearedIDs = append(appearedIDs, e.GetId())
	}
	s.Require().ElementsMatch([]string{"char-alice", "char-bob"}, appearedIDs,
		"both entities must appear in replay")
	s.Require().Len(changed.GetHexes(), 3, "all 3 hexes must be included")
}

// TestBuildReplayEvents_NoHexes verifies that an encounter with entities but no
// hexes still emits one HexKnowledgeChanged carrying just the entities.
func (s *TranslateSuite) TestBuildReplayEvents_NoHexes() {
	pbEnc := &encounterv2pb.Encounter{
		Space: &encounterv2pb.Space{
			Entities: []*encounterv2pb.Entity{
				{Id: "char-alice"},
			},
		},
	}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)
	s.Require().Len(out, 1, "1 entity → 1 HexKnowledgeChanged")
	changed := out[0].GetHexKnowledgeChanged()
	s.Require().NotNil(changed)
	s.Require().Len(changed.GetEntities(), 1)
	s.Require().Empty(changed.GetHexes())
}

// TestBuildReplayEvents_NoEntitiesWithHexes verifies that an encounter with
// hexes but no entities still emits one HexKnowledgeChanged carrying just the
// hexes.
func (s *TranslateSuite) TestBuildReplayEvents_NoEntitiesWithHexes() {
	pbEnc := &encounterv2pb.Encounter{
		Space: &encounterv2pb.Space{
			Hexes: []*encounterv2pb.HexRecord{
				{Position: &encounterv2pb.Position{X: 0, Y: 0, Z: 0}},
			},
		},
	}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)
	s.Require().Len(out, 1, "no entities → still one HexKnowledgeChanged")
	changed := out[0].GetHexKnowledgeChanged()
	s.Require().NotNil(changed)
	s.Require().Empty(changed.GetEntities())
	s.Require().Len(changed.GetHexes(), 1)
}

// TestBuildReplayEvents_NoHexesNoEntities verifies that a Space with neither
// hexes nor entities produces no replay event at all — no empty envelope for
// the client to process.
func (s *TranslateSuite) TestBuildReplayEvents_NoHexesNoEntities() {
	pbEnc := &encounterv2pb.Encounter{Space: &encounterv2pb.Space{}}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)
	s.Require().Empty(out)
}

// TestBuildReplayEvents_HexesPreserveInputOrder verifies that the replayed
// HexKnowledgeChanged carries pbEncounter.Space.Hexes verbatim. Sort
// responsibility lives in ProjectFor; BuildReplayEvents must not reorder.
func (s *TranslateSuite) TestBuildReplayEvents_HexesPreserveInputOrder() {
	hexes := []*encounterv2pb.HexRecord{
		{Position: &encounterv2pb.Position{X: -1, Y: 0, Z: 1}},
		{Position: &encounterv2pb.Position{X: 0, Y: 0, Z: 0}},
		{Position: &encounterv2pb.Position{X: 1, Y: -1, Z: 0}},
		{Position: &encounterv2pb.Position{X: 2, Y: -1, Z: -1}},
	}
	pbEnc := &encounterv2pb.Encounter{Space: &encounterv2pb.Space{Hexes: hexes}}
	out := v2encounter.BuildReplayEvents(pbEnc, s.now)
	s.Require().Len(out, 1)
	got := out[0].GetHexKnowledgeChanged().GetHexes()
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
