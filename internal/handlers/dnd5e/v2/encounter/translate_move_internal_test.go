package encounter

// translate_move_internal_test.go — rpg-api#737 (companion fix:
// KirkDiggler/rpg-toolkit#862): white-box tests for
// translateMoveEventWithData, proving it no longer depends on a
// stream-handler-side repository read for the mover's own knowledge
// restatement.
//
// The bug (caught live in a Kirk playtest, root-caused via dual-poll
// client-vs-redis comparison and raw stream capture): the toolkit's own
// Move() publishes the MoveEvent SYNCHRONOUSLY, from INSIDE Move()'s own
// call stack, before returning to the rpg-api orchestrator — which only
// calls persistWithCharacterData() AFTER Move() returns. The old
// translateMoveEventWithData reacted to that same event by doing its OWN
// encRepo.Get()+LoadFromData()+KnownHexes(viewer) re-read, keyed off the
// mover's CURRENT position in that freshly-loaded data. Since KnownHexes
// derives VISIBLE/REMEMBERED at read time from perception.VisibleHexesAt(
// data.Players[viewer].View.Position, ...) — and a pre-persist read still
// has that Position at the mover's OLD hex — the origin hex was guaranteed
// to compute as VISIBLE (distance 0 from itself) with the mover still
// listed on it, every single time this event fired, not as a rare timing
// accident.
//
// rpg-toolkit#862 closed the class at the source: MoveEvent now carries
// MoverPlayerID and MoverKnownHexes, computed by the toolkit at the exact
// moment of publish — immediately after the mover's own position/view
// mutation, the ONE moment guaranteed to reflect this exact move. The fixed
// translateMoveEventWithData takes no ctx/data/broker at all anymore: there
// is no repository seam left through which a stale read could enter.
// Before the fix, an equivalent test driving the old
// LoadFromData+KnownHexes(viewer) path with a stale `data` snapshot (mover
// still at the origin, origin hex still VISIBLE with the mover on it)
// failed exactly this way — the assertions below on the happy-path case
// reproduce that same origin-hex expectation and are what caught it.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/events"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
)

type TranslateMoveInternalSuite struct {
	suite.Suite
	now time.Time
}

func (s *TranslateMoveInternalSuite) SetupTest() {
	s.now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

// TestTranslateMoveEventWithData_RestatesFromEventKnowledge_NotAStaleRead is
// the rpg-api#737 regression test. The event's own MoverKnownHexes already
// carries the correct post-move truth (origin demoted to REMEMBERED with no
// occupant, destination VISIBLE with the mover on it) — exactly what
// rpg-toolkit's applyAndPublishMove computes at publish time, after the
// mover's position/view mutation. The translator must project this
// verbatim; it has no data/broker parameter left to go stale instead.
func (s *TranslateMoveInternalSuite) TestTranslateMoveEventWithData_RestatesFromEventKnowledge_NotAStaleRead() {
	origin := core.Hex{Q: 0, R: 0, S: 0}
	dest := core.Hex{Q: 1, R: -1, S: 0}
	const mover = core.EntityID("char-A")
	const viewer = core.PlayerID("player-A")

	moverKnownHexes := []events.KnownHex{
		{Position: origin, State: int(perception.KnowledgeStateRemembered)},
		{
			Position: dest,
			State:    int(perception.KnowledgeStateVisible),
			Contents: []events.KnownHexPlacement{{EntityID: mover}},
		},
	}

	evt := events.NewMoveEvent(
		"enc-1", uint64(1), mover, origin,
		[]core.Hex{dest},
		map[core.PlayerID]events.MovePlayerSlice{
			viewer: {SeenSegments: []core.Hex{dest}},
		},
		viewer, moverKnownHexes,
	)

	out, err := translateMoveEventWithData(evt, viewer, s.now)
	s.Require().NoError(err)

	var moved *encounterv2pb.EntityMoved
	var changed *encounterv2pb.HexKnowledgeChanged
	for _, e := range out {
		if m := e.GetEntityMoved(); m != nil {
			moved = m
		}
		if hk := e.GetHexKnowledgeChanged(); hk != nil {
			changed = hk
		}
	}
	s.Require().NotNil(moved, "the bare EntityMoved envelope must still be sent, unchanged")
	s.Require().NotNil(changed, "the mover's own move must carry a supplemental HexKnowledgeChanged restatement")
	s.Require().Len(changed.GetHexes(), 2)

	var originRecord, destRecord *encounterv2pb.HexRecord
	for _, h := range changed.GetHexes() {
		switch {
		case h.GetPosition().GetX() == 0 && h.GetPosition().GetY() == 0 && h.GetPosition().GetZ() == 0:
			originRecord = h
		case h.GetPosition().GetX() == 1 && h.GetPosition().GetY() == -1 && h.GetPosition().GetZ() == 0:
			destRecord = h
		}
	}
	s.Require().NotNil(originRecord, "origin hex must still be present — nothing is ever forgotten, only re-stated")
	s.Require().Equal(encounterv2pb.HexState_HEX_STATE_REMEMBERED, originRecord.GetState(),
		"origin hex must restate as REMEMBERED per the event's own MoverKnownHexes")
	s.Require().Empty(originRecord.GetContents(),
		"the mover must not be reported standing on the hex they just left")

	s.Require().NotNil(destRecord)
	s.Require().Equal(encounterv2pb.HexState_HEX_STATE_VISIBLE, destRecord.GetState())
	s.Require().Len(destRecord.GetContents(), 1)
	s.Require().Equal(string(mover), destRecord.GetContents()[0].GetEntityId(),
		"the mover must be reported on the hex they just arrived at")
}

// TestTranslateMoveEventWithData_OtherViewer_NoSupplementalRestatement proves
// the gate is scoped to the mover's own viewer only: a bystander who merely
// saw a segment of this move gets the plain EntityMoved and nothing else —
// their own geometry knowledge didn't change because they didn't move.
func (s *TranslateMoveInternalSuite) TestTranslateMoveEventWithData_OtherViewer_NoSupplementalRestatement() {
	origin := core.Hex{Q: 0, R: 0, S: 0}
	dest := core.Hex{Q: 1, R: -1, S: 0}
	const mover = core.EntityID("char-A")

	evt := events.NewMoveEvent(
		"enc-1", uint64(1), mover, origin,
		[]core.Hex{dest},
		map[core.PlayerID]events.MovePlayerSlice{
			"player-A": {SeenSegments: []core.Hex{dest}},
			"player-B": {SeenSegments: []core.Hex{dest}},
		},
		"player-A", []events.KnownHex{{Position: dest, State: int(perception.KnowledgeStateVisible)}},
	)

	out, err := translateMoveEventWithData(evt, "player-B", s.now)
	s.Require().NoError(err)
	s.Require().Len(out, 1, "a bystander viewer gets only the bare EntityMoved envelope")
	s.Require().NotNil(out[0].GetEntityMoved())
	s.Require().Nil(out[0].GetHexKnowledgeChanged())
}

// TestTranslateMoveEventWithData_ViewerNotInPerPlayer_ErrViewerSawNothing
// preserves translateMoveEvent's existing error contract — the data-aware
// variant must not weaken it, even for a viewer who happens to be the mover
// (defensive: should not occur, PerPlayer always includes the mover's own
// seat when MoverPlayerID is set).
func (s *TranslateMoveInternalSuite) TestTranslateMoveEventWithData_ViewerNotInPerPlayer_ErrViewerSawNothing() {
	evt := events.NewMoveEvent("enc-1", uint64(1), "char-A", core.Hex{}, nil, nil, "", nil)
	_, err := translateMoveEventWithData(evt, "player-X", s.now)
	s.Require().ErrorIs(err, ErrViewerSawNothing)
}

func TestTranslateMoveInternalSuite(t *testing.T) {
	suite.Run(t, new(TranslateMoveInternalSuite))
}
