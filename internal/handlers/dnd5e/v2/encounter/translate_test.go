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
	// NewMoveEvent signature (events/move.go:34): (encID, seq, mover, path, perPlayer)
	evt := events.NewMoveEvent(
		"enc-1",
		uint64(1),
		"char-A",
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
	evt := events.NewMoveEvent("enc-1", uint64(1), "char-A", nil, nil)
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
		[]core.Hex{{Q: 0, R: 0, S: 0}},
		map[core.PlayerID]events.MovePlayerSlice{
			"player-B": {SeenSegments: []core.Hex{{Q: 0, R: 0, S: 0}}},
		},
	)
	out, err := v2encounter.TranslateEvent(evt, "player-B", s.now)
	s.Require().NoError(err)
	s.Require().Equal(int64(42), out.Sequence)
}

func (s *TranslateSuite) TestTranslateEvent_UnknownEventTypeReturnsErrUnknownEventType() {
	// DoorOpenedEvent implements events.EncounterEvent but TranslateEvent has no
	// mapping for it — exercises the default branch (ErrUnknownEventType).
	evt := events.NewDoorOpenedEvent("enc-1", uint64(3), "door-1", "char-A", nil)
	_, err := v2encounter.TranslateEvent(evt, "player-A", s.now)
	s.Require().Error(err)
	s.Require().True(errors.Is(err, v2encounter.ErrUnknownEventType))
}

// Compile-time check: ensure the package exports used below are accessible.
var _ = (*encounterv2pb.EncounterEvent)(nil)

func TestTranslateSuite(t *testing.T) {
	suite.Run(t, new(TranslateSuite))
}
