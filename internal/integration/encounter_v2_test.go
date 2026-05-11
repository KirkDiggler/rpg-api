// Package integration_test provides bufconn integration tests for the v1alpha2
// encounter service. These are slice-1 gate tests: when they pass, the API side
// of wave 2.5 is feature-complete.
package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// EncounterV2IntegrationSuite is the slice-1 gate test suite.
// It proves: two players in mutual LoS, one moves, both receive EntityMoved.
type EncounterV2IntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	srv    *harness.TestServer
}

func (s *EncounterV2IntegrationSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.srv, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")
}

func (s *EncounterV2IntegrationSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

// authCtx returns a context carrying dev-mode auth for the given player.
// The gRPC auth interceptor reads from incoming metadata, so we must use
// AppendToOutgoingContext (not auth.WithPlayerID) for RPCs over bufconn.
func (s *EncounterV2IntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

func (s *EncounterV2IntegrationSuite) TestCreateEncounter_Basic() {
	ctxA := s.authCtx("alice")
	resp, err := s.srv.EncounterClientV2.CreateEncounter(ctxA, &encounterv2pb.CreateEncounterRequest{
		CampaignId:  "campaign-1",
		InitialMode: encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())
	s.Require().NotEmpty(resp.GetEncounter().GetId())

	data, err := s.srv.EncRepoV2.Get(s.ctx, resp.GetEncounter().GetId())
	s.Require().NoError(err)
	s.Require().NotNil(data)
}

func (s *EncounterV2IntegrationSuite) TestMovementSliceTwoPlayers() {
	// Seed: encounter with players A and B in mutual LoS.
	// SightRange must be > 0; default 0 means neither player sees anything.
	enc := tkenc.New("enc-1", s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-B", EntityID: "char-B",
		Position:   core.Hex{Q: 1, R: -1, S: 0},
		SightRange: 10,
	}))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	ctxA := s.authCtx("player-A")
	ctxB := s.authCtx("player-B")

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxA, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-1"})
	s.Require().NoError(err)
	streamB, err := s.srv.EncounterClientV2.StreamEncounter(ctxB, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-1"})
	s.Require().NoError(err)

	// Both receive snapshot first.
	snapA, err := streamA.Recv()
	s.Require().NoError(err)
	s.Require().NotNil(snapA.GetSnapshotDelivered(), "player-A should receive SnapshotDelivered")
	snapB, err := streamB.Recv()
	s.Require().NoError(err)
	s.Require().NotNil(snapB.GetSnapshotDelivered(), "player-B should receive SnapshotDelivered")

	// A moves two hexes (start is included in path, ending at Q:2 R:-2 S:0).
	_, err = s.srv.EncounterClientV2.MoveEntity(ctxA, &encounterv2pb.MoveEntityRequest{
		EncounterId:  "enc-1",
		EntityId:     "char-A",
		ProposedPath: []*encounterv2pb.Position{{X: 0, Y: 0, Z: 0}, {X: 1, Y: -1, Z: 0}, {X: 2, Y: -2, Z: 0}},
	})
	s.Require().NoError(err)

	// Both A and B receive EntityMoved (small encounter; mutual LoS with SightRange 10).
	// Use recvUntilEntityMoved to skip over replay events (EntityAppeared + GeometryRevealed)
	// that arrive before the live EntityMoved event.
	movA := s.recvUntilEntityMoved(streamA, 500*time.Millisecond)
	s.Require().NotNil(movA, "player-A stream should receive EntityMoved")
	s.Require().Equal("char-A", movA.EntityId, "EntityMoved.EntityId should be char-A (seen by A)")
	// Per-viewer projection check: ActualPath is built from PerPlayer[viewer].SeenSegments,
	// not the raw event Path. Mutual SightRange:10 + 3-hex path entirely in range → both viewers
	// see all 3 hexes. Asserting ActualPath length proves the per-viewer slicing pipe is real,
	// not just that the broker fanned-out an event.
	s.Require().Len(movA.ActualPath, 3, "A should see all 3 path hexes in mutual LoS")

	movB := s.recvUntilEntityMoved(streamB, 500*time.Millisecond)
	s.Require().NotNil(movB, "player-B stream should receive EntityMoved")
	s.Require().Equal("char-A", movB.EntityId, "EntityMoved.EntityId should be char-A (seen by B)")
	s.Require().Len(movB.ActualPath, 3, "B should see all 3 path hexes in mutual LoS")
}

// TestMovementSlicePerViewerProjection_AsymmetricLoS exercises the harder
// per-viewer-projection case: viewer B has SightRange:1 (sees only adjacent
// hexes), the mover A walks past B's vision and out the other side.
//
// Expected wire shape — A's stream gets the full 7-hex path; B's stream gets
// EntityAppeared at A's first-visible hex, EntityMoved with B's 2-hex slice,
// EntityDisappeared at B's last-visible hex. This proves the toolkit's
// ProjectVisibilityTransition + the v2 wire pipe respect per-viewer reality
// in the asymmetric / pass-through case (mutual-LoS test alone could pass
// with broken per-viewer projection).
//
// EXPLORATION TEST: requires the toolkit's #629 work (EntityAppeared /
// EntityDisappeared events). Runs against the local replace directive
// pointing at rpg-toolkit's feat/629 branch. Findings get reported back.
func (s *EncounterV2IntegrationSuite) TestMovementSlicePerViewerProjection_AsymmetricLoS() {
	// B at origin with SightRange:1 — sees only its 6 adjacent hexes + itself.
	// A starts off B's vision, walks across B's view, exits the other side.
	enc := tkenc.New("enc-asym", s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A",
		Position:   core.Hex{Q: 5, R: -2, S: -3},
		SightRange: 10,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-B", EntityID: "char-B",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 1,
	}))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	ctxA := s.authCtx("player-A")
	ctxB := s.authCtx("player-B")

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxA, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-asym"})
	s.Require().NoError(err)
	streamB, err := s.srv.EncounterClientV2.StreamEncounter(ctxB, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-asym"})
	s.Require().NoError(err)

	// Drain snapshots.
	snapA, err := streamA.Recv()
	s.Require().NoError(err)
	s.Require().NotNil(snapA.GetSnapshotDelivered())
	snapB, err := streamB.Recv()
	s.Require().NoError(err)
	s.Require().NotNil(snapB.GetSnapshotDelivered())

	// A walks 7 hexes. Distance from B = max(|Q|,|R|,|S|).
	// (5,-2,-3) dist 5 — invisible
	// (4,-2,-2) dist 4 — invisible
	// (3,-2,-1) dist 3 — invisible
	// (2,-2,0)  dist 2 — invisible
	// (1,-1,0)  dist 1 — VISIBLE  ← B's "appeared at"
	// (0,-1,1)  dist 1 — VISIBLE  ← B's "disappeared at"
	// (-1,-1,2) dist 2 — invisible
	_, err = s.srv.EncounterClientV2.MoveEntity(ctxA, &encounterv2pb.MoveEntityRequest{
		EncounterId: "enc-asym", EntityId: "char-A",
		ProposedPath: []*encounterv2pb.Position{
			{X: 5, Y: -2, Z: -3},
			{X: 4, Y: -2, Z: -2},
			{X: 3, Y: -2, Z: -1},
			{X: 2, Y: -2, Z: 0},
			{X: 1, Y: -1, Z: 0},
			{X: 0, Y: -1, Z: 1},
			{X: -1, Y: -1, Z: 2},
		},
	})
	s.Require().NoError(err)

	// A's perspective: A sees its own full path (A's SightRange:10 covers everything).
	// Per the toolkit semantics A's stream may also receive HexRevealedEvent for newly-
	// revealed hexes from A's own move. Drain events until we see A's EntityMoved.
	movA := s.recvUntilEntityMoved(streamA, 500*time.Millisecond)
	s.Require().NotNil(movA, "A should receive EntityMoved")
	s.Require().Equal("char-A", movA.EntityId)
	s.Require().Len(movA.ActualPath, 7, "A should see all 7 hexes of own path")

	// B's perspective: pass-through. Toolkit's ProjectVisibilityTransition for the
	// (false,false) endpoints + non-empty seenSegments case emits BOTH Appeared and
	// Disappeared. Plus an EntityMoved with B's 2-hex slice.
	//
	// Order is broker-publish-order; we don't assert a specific order, just that all
	// three event kinds arrive within the window.
	//
	// Collect enough events to cover both the replay burst (EntityAppeared for char-B
	// itself + GeometryRevealed) and the 3 live broker events (EntityAppeared for
	// char-A, EntityMoved with 2-hex slice, EntityDisappeared for char-A).
	bEvents := s.collectStreamEvents(streamB, 5, 500*time.Millisecond)
	var bAppeared *encounterv2pb.EntityAppeared
	var bMoved *encounterv2pb.EntityMoved
	var bDisappeared *encounterv2pb.EntityDisappeared
	for _, ev := range bEvents {
		if a := ev.GetEntityAppeared(); a != nil {
			bAppeared = a
		}
		if m := ev.GetEntityMoved(); m != nil {
			bMoved = m
		}
		if d := ev.GetEntityDisappeared(); d != nil {
			bDisappeared = d
		}
	}

	s.Require().NotNil(bAppeared, "B should receive EntityAppeared on pass-through")
	s.Require().Equal("char-A", bAppeared.Entity.Id)
	s.Require().Equal(int32(1), bAppeared.Entity.Position.X, "B sees A appear at (1,-1,0)")
	s.Require().Equal(int32(-1), bAppeared.Entity.Position.Y)
	s.Require().Equal(int32(0), bAppeared.Entity.Position.Z)

	s.Require().NotNil(bMoved, "B should receive EntityMoved with the visible slice")
	s.Require().Equal("char-A", bMoved.EntityId)
	s.Require().Len(bMoved.ActualPath, 2, "B's ActualPath should be the 2-hex visible slice, NOT the full 7-hex path")

	s.Require().NotNil(bDisappeared, "B should receive EntityDisappeared on pass-through")
	s.Require().Equal("char-A", bDisappeared.EntityId)
	// Per-viewer last-known position: B's last-visible hex on this pass-through
	// is (0,-1,1). The wire's last_known_position field carries it; web can
	// render "freeze marker at this hex" without client-side game-state tracking.
	s.Require().NotNil(bDisappeared.LastKnownPosition, "EntityDisappeared must carry per-viewer last-known position")
	s.Require().Equal(int32(0), bDisappeared.LastKnownPosition.X)
	s.Require().Equal(int32(-1), bDisappeared.LastKnownPosition.Y)
	s.Require().Equal(int32(1), bDisappeared.LastKnownPosition.Z)
}

// TestGetEncounter_ProjectsView verifies that GetEncounter returns the encounter
// with the caller's own entity visible, and that other players within mutual LoS
// also appear in the projected snapshot.
func (s *EncounterV2IntegrationSuite) TestGetEncounter_ProjectsView() {
	enc := tkenc.New("enc-get-1", s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position:   core.Hex{Q: 1, R: -1, S: 0},
		SightRange: 10,
	}))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	ctxA := s.authCtx("alice")
	resp, err := s.srv.EncounterClientV2.GetEncounter(ctxA, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-get-1",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())
	s.Require().Equal("enc-get-1", resp.GetEncounter().GetId())

	// Both alice and bob should appear in alice's projected snapshot because
	// they are within mutual SightRange:10.
	entities := resp.GetEncounter().GetSpace().GetEntities()
	s.Require().Len(entities, 2, "alice's snapshot should include both alice and bob")

	entityIDs := make(map[string]bool, len(entities))
	for _, e := range entities {
		entityIDs[e.GetId()] = true
	}
	s.Require().True(entityIDs["char-alice"], "alice's own entity should be in the snapshot")
	s.Require().True(entityIDs["char-bob"], "bob should be visible to alice (mutual LoS, SightRange 10)")
}

// TestGetEncounter_AsymmetricLoS_ExcludesHidden verifies that players outside
// the viewer's current sight range are excluded from the projected snapshot.
func (s *EncounterV2IntegrationSuite) TestGetEncounter_AsymmetricLoS_ExcludesHidden() {
	enc := tkenc.New("enc-get-asym", s.srv.BrokerV2)
	// alice at origin with SightRange:1 — can only see adjacent hexes.
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 1,
	}))
	// bob is far away (distance 5) — invisible to alice.
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position:   core.Hex{Q: 5, R: -2, S: -3},
		SightRange: 10,
	}))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	ctxA := s.authCtx("alice")
	resp, err := s.srv.EncounterClientV2.GetEncounter(ctxA, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-get-asym",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())

	// Only alice's own entity — bob is outside alice's SightRange:1.
	entities := resp.GetEncounter().GetSpace().GetEntities()
	s.Require().Len(entities, 1, "alice should only see herself, not bob (out of sight range)")
	s.Require().Equal("char-alice", entities[0].GetId())
}

// TestGetEncounter_NonMember_PermissionDenied verifies that a player not in the
// encounter receives PermissionDenied.
func (s *EncounterV2IntegrationSuite) TestGetEncounter_NonMember_PermissionDenied() {
	enc := tkenc.New("enc-get-perm", s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	// charlie is not in this encounter.
	ctxC := s.authCtx("charlie")
	_, err := s.srv.EncounterClientV2.GetEncounter(ctxC, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-get-perm",
	})
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

// recvUntilEntityMoved pulls events from the stream until an EntityMoved arrives
// or the timeout expires. Returns nil on timeout.
func (s *EncounterV2IntegrationSuite) recvUntilEntityMoved(stream encounterv2pb.EncounterService_StreamEncounterClient, timeout time.Duration) *encounterv2pb.EntityMoved {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ev, err := stream.Recv()
		if err != nil {
			return nil
		}
		if m := ev.GetEntityMoved(); m != nil {
			return m
		}
	}
	return nil
}

// collectStreamEvents pulls up to want events or until the timeout expires.
// Returns whatever it managed to collect.
//
// NOTE: gRPC's stream.Recv() is blocking — this function launches a background
// goroutine to drain the stream so the timeout is honored correctly. The
// goroutine exits when the parent test context is canceled (TearDownTest).
func (s *EncounterV2IntegrationSuite) collectStreamEvents(stream encounterv2pb.EncounterService_StreamEncounterClient, want int, timeout time.Duration) []*encounterv2pb.EncounterEvent {
	ch := make(chan *encounterv2pb.EncounterEvent, want+4)
	go func() {
		for {
			ev, err := stream.Recv()
			if err != nil {
				close(ch)
				return
			}
			ch <- ev
		}
	}()

	out := make([]*encounterv2pb.EncounterEvent, 0, want)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for len(out) < want {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timer.C:
			return out
		}
	}
	return out
}

// TestStreamEncounter_ReplaysInitialState verifies that on connect, StreamEncounter
// emits a populated SnapshotDelivered (with Encounter.Id set) followed by one
// EntityAppeared per entity visible to the connecting player, and at least one
// GeometryRevealed event for the player's revealed hex set — all BEFORE any live
// broker event.
//
// Seeding: alice and bob in mutual LoS (SightRange:10). Alice's replay delivers:
// EntityAppeared(char-alice), EntityAppeared(char-bob), GeometryRevealed — 3 events.
func (s *EncounterV2IntegrationSuite) TestStreamEncounter_ReplaysInitialState() {
	enc := tkenc.New("enc-replay-1", s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position:   core.Hex{Q: 1, R: -1, S: 0},
		SightRange: 10,
	}))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	ctxA := s.authCtx("alice")
	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxA, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-replay-1"})
	s.Require().NoError(err)

	// First event MUST be SnapshotDelivered with a populated Encounter.
	snap, err := streamA.Recv()
	s.Require().NoError(err)
	s.Require().NotNil(snap.GetSnapshotDelivered(), "first event must be SnapshotDelivered")
	sd := snap.GetSnapshotDelivered()
	s.Require().NotNil(sd.GetEncounter(), "SnapshotDelivered.Encounter must be non-nil")
	s.Require().Equal("enc-replay-1", sd.GetEncounter().GetId(), "SnapshotDelivered.Encounter.Id must be set")

	// Replay events follow immediately: 2 EntityAppeared (alice + bob) + 1 GeometryRevealed.
	// Drain all 3 explicitly — they are queued server-side before any live event fires.
	var sawBobAppeared bool
	var sawGeometryRevealed bool
	for i := 0; i < 3; i++ {
		ev, recvErr := streamA.Recv()
		s.Require().NoError(recvErr, "expected replay event %d", i+1)
		if a := ev.GetEntityAppeared(); a != nil {
			if a.GetEntity().GetId() == "char-bob" {
				sawBobAppeared = true
			}
		}
		if g := ev.GetGeometryRevealed(); g != nil {
			s.Require().NotEmpty(g.GetHexes(), "GeometryRevealed must carry non-empty hex set")
			sawGeometryRevealed = true
		}
	}

	s.Require().True(sawBobAppeared, "replay must include EntityAppeared for bob (visible to alice at SightRange:10)")
	s.Require().True(sawGeometryRevealed, "replay must include GeometryRevealed for alice's revealed hexes")
}

// TestStreamEncounter_LiveEventsDoNotDuplicateReplay verifies that a live MoveEntity
// from another player after replay completes does NOT re-emit any EntityAppeared
// for an entity already delivered by the replay. The broker's per-viewer projection
// uses LoS-crossings (not every event) so an already-visible entity should not
// fire EntityAppeared again from a move that stays within the viewer's LoS.
func (s *EncounterV2IntegrationSuite) TestStreamEncounter_LiveEventsDoNotDuplicateReplay() {
	enc := tkenc.New("enc-replay-2", s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position:   core.Hex{Q: 1, R: -1, S: 0},
		SightRange: 10,
	}))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	ctxA := s.authCtx("alice")
	ctxB := s.authCtx("bob")

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxA, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-replay-2"})
	s.Require().NoError(err)

	// Drain alice's snapshot first.
	snap, err := streamA.Recv()
	s.Require().NoError(err)
	s.Require().NotNil(snap.GetSnapshotDelivered())

	// Drain exactly 3 replay events (EntityAppeared alice, EntityAppeared bob, GeometryRevealed).
	// Count EntityAppeared for bob to verify the replay includes exactly one.
	replayBobAppearedCount := 0
	for i := 0; i < 3; i++ {
		ev, recvErr := streamA.Recv()
		s.Require().NoError(recvErr, "expected replay event %d", i+1)
		if a := ev.GetEntityAppeared(); a != nil && a.GetEntity().GetId() == "char-bob" {
			replayBobAppearedCount++
		}
	}
	s.Require().Equal(1, replayBobAppearedCount, "replay should deliver exactly one EntityAppeared for bob")

	// Bob moves one hex. Both A and B are within mutual LoS (SightRange:10).
	// Bob was already visible to alice from replay, so this move should NOT
	// produce another EntityAppeared for bob on alice's stream.
	_, err = s.srv.EncounterClientV2.MoveEntity(ctxB, &encounterv2pb.MoveEntityRequest{
		EncounterId:  "enc-replay-2",
		EntityId:     "char-bob",
		ProposedPath: []*encounterv2pb.Position{{X: 1, Y: -1, Z: 0}, {X: 2, Y: -2, Z: 0}},
	})
	s.Require().NoError(err)

	// Collect post-move events from alice's stream. The move triggers at least one
	// EntityMoved; the broker may also emit HexRevealedEvent for newly-visible hexes.
	// Collect a bounded window and assert on what actually arrived.
	postMoveEvents := s.collectStreamEvents(streamA, 5, 500*time.Millisecond)

	var sawLiveMoved bool
	extraBobAppeared := 0
	for _, ev := range postMoveEvents {
		if m := ev.GetEntityMoved(); m != nil && m.GetEntityId() == "char-bob" {
			sawLiveMoved = true
		}
		if a := ev.GetEntityAppeared(); a != nil && a.GetEntity().GetId() == "char-bob" {
			extraBobAppeared++
		}
	}

	// The live EntityMoved must arrive (proves the live stream is working).
	s.Require().True(sawLiveMoved, "live EntityMoved for bob must arrive after the move")

	// The no-duplication property: bob was already visible to alice from the replay,
	// so the toolkit's ProjectVisibilityTransition (LoS-crossing semantics) must NOT
	// fire another EntityAppeared for bob from a move that stays within LoS.
	s.Require().Zero(extraBobAppeared,
		"live stream must not re-emit EntityAppeared for bob who was already visible from replay")
}

// TestInteract_OpenDoor_TwoPlayers exercises the Wave 2.7 OpenDoor flow
// end-to-end through the v2 service. Two players in mutual LoS see a closed
// door; alice opens it via Interact; both streams receive the cause event
// (DoorOpened) and the parallel effect event (GeometryRevealed) for any
// newly-revealed hexes around the door.
//
// Mirrors the toolkit's TestSlice_OpenDoor fixture (rpg-toolkit/encounter
// integration_test.go:71) so the projection layer is exercised against the
// same shape that the toolkit-level test proved.
func (s *EncounterV2IntegrationSuite) TestInteract_OpenDoor_TwoPlayers() {
	enc := tkenc.New("enc-door-1", s.srv.BrokerV2)
	// SightRange:10 ensures both alice and bob can perceive a door at
	// distance 4 (per ProjectDoorOpen's visibility check).
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position: core.Hex{Q: 1, R: -1, S: 0}, SightRange: 10,
	}))
	enc.AddDoor("door-east", core.Hex{Q: 4, R: 0, S: -4}, false)
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	ctxA := s.authCtx("alice")
	ctxB := s.authCtx("bob")

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxA, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-door-1"})
	s.Require().NoError(err)
	streamB, err := s.srv.EncounterClientV2.StreamEncounter(ctxB, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-door-1"})
	s.Require().NoError(err)

	// Drain the snapshot + replay events on each stream before firing Interact.
	// Replay shape per pat-v2-snapshot-replay-builder-pair: SnapshotDelivered +
	// EntityAppeared per visible entity + GeometryRevealed for revealed hexes.
	// Two visible entities (alice + bob) = 4 events total per stream.
	for i := 0; i < 4; i++ {
		_, recvErr := streamA.Recv()
		s.Require().NoError(recvErr, "alice replay event %d", i+1)
		_, recvErr = streamB.Recv()
		s.Require().NoError(recvErr, "bob replay event %d", i+1)
	}

	// Alice opens the door.
	resp, err := s.srv.EncounterClientV2.Interact(ctxA, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-door-1",
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Nil(resp.GetInputRequired(), "Wave 2.7 doors do not prompt for input")

	// Both alice and bob must see the DoorOpened event (toolkit emits a
	// per-viewer slice for every player that can see the door's hex). The
	// HexRevealedEvent rides as a parallel envelope (GeometryRevealed) — we
	// don't require its arrival here because some viewers may already have
	// seen the door's neighbors and have an empty reveal slice. Cause is
	// load-bearing; effect is opportunistic per viewer.
	postA := s.collectStreamEvents(streamA, 3, 500*time.Millisecond)
	postB := s.collectStreamEvents(streamB, 3, 500*time.Millisecond)

	var aDoor *encounterv2pb.DoorOpened
	for _, ev := range postA {
		if d := ev.GetDoorOpened(); d != nil {
			aDoor = d
			break
		}
	}
	s.Require().NotNil(aDoor, "alice should receive DoorOpened (she opened it)")
	s.Require().Equal("door-east", aDoor.GetDoorEntityId())
	// Cause/effect split: revealed_hexes/walls ride on the parallel
	// GeometryRevealed event, NOT on this envelope.
	s.Require().Empty(aDoor.GetRevealedHexes(), "revealed_hexes belongs on the parallel GeometryRevealed event, not DoorOpened")

	var bDoor *encounterv2pb.DoorOpened
	for _, ev := range postB {
		if d := ev.GetDoorOpened(); d != nil {
			bDoor = d
			break
		}
	}
	s.Require().NotNil(bDoor, "bob should receive DoorOpened (door is in his LoS)")
	s.Require().Equal("door-east", bDoor.GetDoorEntityId())
	s.Require().Empty(bDoor.GetRevealedHexes(), "revealed_hexes belongs on the parallel GeometryRevealed event, not DoorOpened")

	// Confirm the door is persisted as open after Interact. Use the ok-check
	// pattern so a missing entry is distinguishable from a closed door (and
	// so this won't panic if the map value type ever becomes a pointer).
	persisted, err := s.srv.EncRepoV2.Get(s.ctx, "enc-door-1")
	s.Require().NoError(err)
	door, ok := persisted.Doors[core.EntityID("door-east")]
	s.Require().True(ok, "door-east should be in persisted.Doors")
	s.Require().True(door.Open, "door must be persisted Open after Interact")
}

// TestInteract_DoorAlreadyOpen_FailedPrecondition mirrors the unit test at
// the integration level: a second Interact on an already-open door must be
// rejected by the toolkit with FailedPrecondition.
func (s *EncounterV2IntegrationSuite) TestInteract_DoorAlreadyOpen_FailedPrecondition() {
	enc := tkenc.New("enc-door-twice", s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
	}))
	enc.AddDoor("door-east", core.Hex{Q: 1, R: 0, S: -1}, true) // pre-open
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	ctxA := s.authCtx("alice")
	_, err := s.srv.EncounterClientV2.Interact(ctxA, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-door-twice",
		TargetEntityId: "door-east",
	})
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

// TestInteract_LockedDoor_PromptAndResolve_TwoPlayers is the Wave 2.9 gate
// test. It exercises the full locked-door flow end-to-end:
//
//   - Two players in mutual LoS see a locked door.
//   - alice calls Interact on the locked door — the response carries
//     InputRequired{skill_check} (caller-private); bob's stream sees nothing
//     for the prompt itself.
//   - alice calls SubmitCheck with a passing roll — both alice and bob
//     receive DoorOpened; the door is persisted as open + unlocked.
//
// The failure-path subtest exercises the same flow with a roll guaranteed to
// fall short of the DC: the prompt clears, no events fire, and the door
// stays closed + locked.
func (s *EncounterV2IntegrationSuite) TestInteract_LockedDoor_PromptAndResolve_TwoPlayers() {
	encID := "enc-locked-1"

	// Seed: alice + bob in mutual LoS; one locked door at distance 4 from
	// both (matches the unlocked-door test fixture so projection logic is
	// the same).
	enc := tkenc.New(core.EncounterID(encID), s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position: core.Hex{Q: 1, R: -1, S: 0}, SightRange: 10,
	}))
	enc.AddDoor("door-east", core.Hex{Q: 4, R: 0, S: -4}, false)
	data := enc.ToData()
	door := data.Doors[core.EntityID("door-east")]
	door.Locked = true
	door.LockDC = 10
	door.LockAbility = "DEX"
	door.LockTool = "dnd5e:item:thieves-tools"
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, data))

	ctxA := s.authCtx("alice")
	ctxB := s.authCtx("bob")

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxA,
		&encounterv2pb.StreamEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)
	streamB, err := s.srv.EncounterClientV2.StreamEncounter(ctxB,
		&encounterv2pb.StreamEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)

	// Drain replay (Snapshot + 2 EntityAppeared + 1 GeometryRevealed = 4 events
	// per stream, matching TestInteract_OpenDoor_TwoPlayers).
	for i := 0; i < 4; i++ {
		_, recvErr := streamA.Recv()
		s.Require().NoError(recvErr, "alice replay event %d", i+1)
		_, recvErr = streamB.Recv()
		s.Require().NoError(recvErr, "bob replay event %d", i+1)
	}

	// --- Step 1: alice attempts to open the locked door. ---
	resp, err := s.srv.EncounterClientV2.Interact(ctxA, &encounterv2pb.InteractRequest{
		EncounterId:    encID,
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	prompt := resp.GetInputRequired().GetSkillCheck()
	s.Require().NotNil(prompt, "locked door must return InputRequired{skill_check}")
	s.Require().Equal(int32(10), prompt.GetDc())
	s.Require().Equal("DEX", prompt.GetAbility())
	s.Require().NotNil(prompt.GetTool())
	s.Require().Equal("dnd5e", prompt.GetTool().GetModule())
	s.Require().Equal("thieves-tools", prompt.GetTool().GetId())

	// --- Step 2: alice submits a passing roll (20 vs DC 10). ---
	// We drain bob's stream once after the SubmitCheck below; the cumulative
	// window covers both Step 1 (which must be silent for bob since the
	// prompt is caller-private) and Step 2 (which must publish DoorOpened
	// to both viewers since the door is in their shared LoS). Two
	// collectStreamEvents calls on the same stream would race on Recv.
	subResp, err := s.srv.EncounterClientV2.SubmitCheck(ctxA, &encounterv2pb.SubmitCheckRequest{
		EncounterId: encID,
		EntityId:    "char-alice",
		Roll:        20,
	})
	s.Require().NoError(err)
	s.Require().True(subResp.GetSuccess())
	s.Require().Equal(int32(20), subResp.GetTotal())

	// Both alice and bob now receive DoorOpened (door is in both LoS).
	postA := s.collectStreamEvents(streamA, 3, 500*time.Millisecond)
	postB := s.collectStreamEvents(streamB, 3, 500*time.Millisecond)

	var aDoor, bDoor *encounterv2pb.DoorOpened
	for _, ev := range postA {
		if d := ev.GetDoorOpened(); d != nil {
			aDoor = d
			break
		}
	}
	for _, ev := range postB {
		if d := ev.GetDoorOpened(); d != nil {
			bDoor = d
			break
		}
	}
	s.Require().NotNil(aDoor, "alice should receive DoorOpened (she resolved the prompt)")
	s.Require().Equal("door-east", aDoor.GetDoorEntityId())
	s.Require().NotNil(bDoor, "bob should receive DoorOpened (door is in his LoS)")
	s.Require().Equal("door-east", bDoor.GetDoorEntityId())

	// The prompt was caller-private: walk bob's drained event list and
	// confirm no extra envelopes arrived ahead of DoorOpened (geometry
	// events trail DoorOpened in the toolkit's emit order).
	for _, ev := range postB {
		if d := ev.GetDoorOpened(); d != nil {
			break
		}
		s.Failf("bob received an unexpected event before DoorOpened",
			"event %s arrived ahead of DoorOpened", ev.String())
	}

	// Door is persisted as open + unlocked, prompt cleared.
	persisted, err := s.srv.EncRepoV2.Get(s.ctx, encID)
	s.Require().NoError(err)
	pdoor, ok := persisted.Doors[core.EntityID("door-east")]
	s.Require().True(ok)
	s.Require().True(pdoor.Open)
	s.Require().False(pdoor.Locked, "successful unlock clears Locked")
	s.Require().Empty(persisted.PendingPrompts, "prompt cleared after successful resolve")
}

// TestInteract_LockedDoor_FailedRoll_NoEvents covers the failure path of the
// Wave 2.9 flow: alice issues the prompt, submits a roll that can't beat the
// DC, and bob's stream sees nothing while alice's response reports failure.
// The door remains closed + locked; the prompt is cleared so alice can try
// again from scratch.
func (s *EncounterV2IntegrationSuite) TestInteract_LockedDoor_FailedRoll_NoEvents() {
	encID := "enc-locked-fail"

	enc := tkenc.New(core.EncounterID(encID), s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position: core.Hex{Q: 1, R: -1, S: 0}, SightRange: 10,
	}))
	enc.AddDoor("door-east", core.Hex{Q: 4, R: 0, S: -4}, false)
	data := enc.ToData()
	door := data.Doors[core.EntityID("door-east")]
	door.Locked = true
	door.LockDC = 30 // unbeatable with d20 + zero modifiers
	door.LockAbility = "DEX"
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, data))

	ctxA := s.authCtx("alice")
	ctxB := s.authCtx("bob")

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxA,
		&encounterv2pb.StreamEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)
	streamB, err := s.srv.EncounterClientV2.StreamEncounter(ctxB,
		&encounterv2pb.StreamEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)
	for i := 0; i < 4; i++ {
		_, recvErr := streamA.Recv()
		s.Require().NoError(recvErr, "alice replay event %d", i+1)
		_, recvErr = streamB.Recv()
		s.Require().NoError(recvErr, "bob replay event %d", i+1)
	}

	_, err = s.srv.EncounterClientV2.Interact(ctxA, &encounterv2pb.InteractRequest{
		EncounterId:    encID,
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)

	subResp, err := s.srv.EncounterClientV2.SubmitCheck(ctxA, &encounterv2pb.SubmitCheckRequest{
		EncounterId: encID,
		EntityId:    "char-alice",
		Roll:        1, // guaranteed failure vs DC 30
	})
	s.Require().NoError(err)
	s.Require().False(subResp.GetSuccess(), "roll 1 vs DC 30 must fail")
	s.Require().Equal(int32(1), subResp.GetTotal())

	// Drain each stream once with a settle window. Failure path must not
	// publish any post-replay events on either stream.
	silentA := s.collectStreamEvents(streamA, 1, 250*time.Millisecond)
	s.Require().Empty(silentA, "alice must not see DoorOpened on failed unlock")
	silentB := s.collectStreamEvents(streamB, 1, 250*time.Millisecond)
	s.Require().Empty(silentB, "bob must not see anything on failed unlock")

	persisted, err := s.srv.EncRepoV2.Get(s.ctx, encID)
	s.Require().NoError(err)
	pdoor, ok := persisted.Doors[core.EntityID("door-east")]
	s.Require().True(ok)
	s.Require().False(pdoor.Open, "door stays closed on failed unlock")
	s.Require().True(pdoor.Locked, "door stays locked on failed unlock")
	s.Require().Empty(persisted.PendingPrompts, "prompt cleared even on failure")
}

// TestCombatSlice_TakeActionAndEndTurn_NPCDispatch is the wave-2.8 gate test.
// It exercises the full attack → endturn → npc-act → endturn loop on bufconn:
//
//   - Two players + one monster, all in mutual LoS, mode set to TURN_BASED on
//     seed (initiative is rolled by the toolkit).
//   - Both players subscribe via StreamEncounter and drain replay events.
//   - The active player attacks goblin-1; the toolkit resolves the attack
//     with a real d20, so EntityDamaged is published only on hit. The test
//     asserts that IF EntityDamaged arrives, it is shaped correctly
//     (target=goblin-1, source=attacker); a miss-only run is still a valid
//     pass. AttackResolvedEvent is suppressed at the translator and must
//     not appear on either stream.
//   - The active player ends turn; the orchestrator's NPC dispatch loop runs
//     NPCAct + EndTurn for the goblin server-side until the next active actor
//     is a player. Both streams observe TurnEnded → (NPC events) → TurnStarted.
//
// This is the integration-test face of pat-v2-npc-turn-dispatch and
// pat-v2-combat-event-translator.
func (s *EncounterV2IntegrationSuite) TestCombatSlice_TakeActionAndEndTurn_NPCDispatch() {
	enc := tkenc.New("enc-combat", s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14, AttackBonus: 4, DamageDice: "1d8+2", DamageType: "slashing",
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position: core.Hex{Q: 1, R: -1, S: 0}, SightRange: 10,
		HP: 10, MaxHP: 10, AC: 13, AttackBonus: 3, DamageDice: "1d6+1", DamageType: "piercing",
	}))
	// Goblin HP intentionally bumped to 30 (vs canonical 7) so the one-shot
	// path is statistically impossible. Player damage including crits caps
	// at 1d8+2 → 18 max (alice, nat-20 doubles the d8) and 1d6+1 → 13 max
	// (bob, nat-20 doubles the d6) — both well under 30. Pre-Wave-2.10 a
	// kill here was harmless — the encounter just had no monsters and
	// EndTurn still worked. Wave 2.10 added the ErrEncounterEnded
	// terminal-state guard, so a kill here now flips EndTurn to
	// FailedPrecondition mid-test. The test was never trying to exercise
	// lethality (Wave 2.10's death suite covers that with killGoblinFixture);
	// bumping HP keeps the attack/endturn/npc-dispatch loop the actual
	// subject under test. Closes #520.
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID: "goblin-1", Position: core.Hex{Q: 2, R: -1, S: -1},
		HP: 30, MaxHP: 30, AC: 15, Speed: 6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4, DamageDice: "1d6+2", DamageType: "slashing",
	}))
	s.Require().NoError(enc.SetMode(core.ModeTurnBased))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	// Walk initiative until a player is active so both TakeAction and
	// EndTurn run in their happy path. Done outside the streams so the
	// initial-state replay is what each viewer sees on connect.
	s.advanceUntilPlayerActiveByDirectToolkit(enc)

	// Reload to discover whose turn it is now.
	data, err := s.srv.EncRepoV2.Get(s.ctx, "enc-combat")
	s.Require().NoError(err)
	activeEntity := string(data.Initiative[data.ActiveIdx])
	var activePlayer string
	for pid, pd := range data.Players {
		if string(pd.EntityID) == activeEntity {
			activePlayer = string(pid)
			break
		}
	}
	s.Require().NotEmpty(activePlayer, "an active player must be found before TakeAction")

	ctxAlice := s.authCtx("alice")
	ctxBob := s.authCtx("bob")
	ctxActive := s.authCtx(activePlayer)

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxAlice, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-combat"})
	s.Require().NoError(err)
	streamB, err := s.srv.EncounterClientV2.StreamEncounter(ctxBob, &encounterv2pb.StreamEncounterRequest{EncounterId: "enc-combat"})
	s.Require().NoError(err)

	// Both streams use a single long-running background drainer started
	// here. Each drainer collects all events into its own slice with a mutex.
	// Tests then read from the slice with a polling helper after firing the
	// active RPC. This avoids racy multi-call collectStreamEvents patterns.
	aSink := newEventSink()
	bSink := newEventSink()
	go aSink.drain(streamA)
	go bSink.drain(streamB)

	// Wait until both sinks have observed the full replay set: SnapshotDelivered
	// + at least one EntityAppeared (the viewer's own entity) + GeometryRevealed
	// (the viewer's revealed-hexes set). All replay events ship with Sequence==0
	// (pre-history); live events start at Sequence>=1. The replay-complete
	// signal is robust against slow CI without timing assumptions — we only
	// watermark once both viewers have received their replay envelopes.
	s.Require().Eventually(func() bool {
		return aSink.hasReplayComplete() && bSink.hasReplayComplete()
	}, 2*time.Second, 25*time.Millisecond,
		"both streams must complete replay (snapshot + entity-appeared + geometry-revealed) before the test fires the active RPC")
	aSink.snapshot() // capture replay watermark; subsequent reads start AFTER this
	bSink.snapshot()

	// Active player attacks goblin-1.
	_, err = s.srv.EncounterClientV2.TakeAction(ctxActive, &encounterv2pb.TakeActionRequest{
		EncounterId:   "enc-combat",
		ActorEntityId: activeEntity,
		ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: "goblin-1"},
		},
	})
	s.Require().NoError(err)

	// Active player ends turn — fires the NPC dispatch loop server-side.
	_, err = s.srv.EncounterClientV2.EndTurn(ctxActive, &encounterv2pb.EndTurnRequest{
		EncounterId: "enc-combat", EntityId: activeEntity,
	})
	s.Require().NoError(err)

	// Wait for the broker to fan out the events triggered by both RPCs.
	// TakeAction emits AttackResolvedEvent (suppressed) + DamageDealtEvent (on hit).
	// EndTurn emits TurnEnded(activeEntity) + (NPC chain if applicable) +
	// TurnStarted(nextActor).
	s.Require().Eventually(func() bool {
		return aSink.hasTurnEndedFor(activeEntity) && bSink.hasTurnEndedFor(activeEntity)
	}, 2*time.Second, 50*time.Millisecond,
		"both streams must see TurnEnded for the active entity")

	aPost := aSink.eventsAfterSnapshot()
	bPost := bSink.eventsAfterSnapshot()

	// AttackResolved is suppressed at the translator. Verify no events with
	// a nil oneof slipped through (every event must carry a known envelope).
	s.Require().True(eventsHaveNoUnmappedEnvelope(aPost),
		"alice's stream must not surface unmapped envelopes")
	s.Require().True(eventsHaveNoUnmappedEnvelope(bPost),
		"bob's stream must not surface unmapped envelopes")

	// On hit, EntityDamaged should target goblin-1 with the active entity as source.
	if dmg := findEntityDamaged(aPost); dmg != nil {
		s.Require().Equal("goblin-1", dmg.GetEntityId(), "alice's damage event targets goblin-1")
		s.Require().Equal(activeEntity, dmg.GetSourceEntityId(),
			"alice's damage event source is the attacking entity")
	}
	if dmg := findEntityDamaged(bPost); dmg != nil {
		s.Require().Equal("goblin-1", dmg.GetEntityId(), "bob's damage event targets goblin-1")
	}

	// Reload final state — active actor must be a player after the loop.
	finalData, err := s.srv.EncRepoV2.Get(s.ctx, "enc-combat")
	s.Require().NoError(err)
	finalActive := finalData.Initiative[finalData.ActiveIdx]
	finalIsPlayer := false
	for _, pd := range finalData.Players {
		if pd.EntityID == finalActive {
			finalIsPlayer = true
			break
		}
	}
	s.Require().True(finalIsPlayer,
		"after EndTurn + NPC dispatch loop, active actor must be a player, got %q", finalActive)
}

// Wave 2.10 integration tests: death + encounter end ----------------------
//
// Two scenarios verify the full death-resolution slice end-to-end:
//
//   1. Last-hostile kill: 2 players + 1 goblin. Active player attacks the
//      goblin to HP=0; both streams must receive EntityDied + EntityRemoved
//      + EncounterEnded. A subsequent TakeAction must return
//      FailedPrecondition (mapped from tkenc.ErrEncounterEnded).
//
//   2. Mid-fight kill (no end): 2 players + 2 goblins. Active player kills
//      goblin-1; both streams must receive EntityDied + EntityRemoved for
//      goblin-1 only — no EncounterEnded. The active player ends turn; the
//      NPC dispatch loop skips dead goblin-1 (now spliced out of
//      initiative) and brings combat back around. Active player kills
//      goblin-2; EncounterEnded fires.
//
// Determinism: monsters are seeded with HP=1 and the killing player has
// AttackBonus=20 (so any d20 roll except nat-1 hits AC=10). Damage dice
// "1d6" guarantees >=1 dmg on hit, killing HP=1 monster. Tests loop
// TakeAction up to maxKillAttempts to absorb the rare nat-1 miss without
// flakiness.

// maxKillAttempts caps the number of TakeAction calls used to drive a
// goblin (HP=1, AC=10) to zero HP from a player with AttackBonus=20.
// Each attack misses only on nat-1 (5% chance), so 20 attempts give a
// failure probability of (1/20)^20 ≈ 9.5e-27 — well below CI flakiness
// floors.
const maxKillAttempts = 20

// killGoblinFixturePlayerInput returns a PlayerInput tuned for the death
// integration tests: AttackBonus=20 + DamageDice "1d6" makes the player a
// reliable HP=1 monster killer (assuming AC=10 monsters).
func killGoblinFixturePlayerInput(playerID core.PlayerID, entityID core.EntityID, pos core.Hex) tkenc.PlayerInput {
	return tkenc.PlayerInput{
		PlayerID: playerID, EntityID: entityID,
		Position: pos, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14,
		AttackBonus: 20,
		DamageDice:  "1d6",
		DamageType:  "slashing",
	}
}

// killGoblinFixtureMonsterInput returns a MonsterInput tuned for the death
// integration tests: HP=1 + AC=10 goes down to any d20-roll-except-nat-1
// + 1+ damage attack from the killGoblinFixturePlayerInput player.
func killGoblinFixtureMonsterInput(id core.EntityID, pos core.Hex) tkenc.MonsterInput {
	return tkenc.MonsterInput{
		ID: id, Position: pos,
		HP: 1, MaxHP: 1, AC: 10, Speed: 6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4, DamageDice: "1d6+2", DamageType: "slashing",
	}
}

// killMonsterViaTakeActionLoop calls TakeAction up to maxKillAttempts times
// against monsterID using the active player's context. Returns when the
// goblin HP reaches 0 (verified via repo read after each call) or after the
// cap. Stops early if TakeAction errors with FailedPrecondition (e.g., the
// encounter ended).
func (s *EncounterV2IntegrationSuite) killMonsterViaTakeActionLoop(
	ctx context.Context,
	encID, actorEntityID, monsterID string,
) {
	for attempt := 0; attempt < maxKillAttempts; attempt++ {
		_, err := s.srv.EncounterClientV2.TakeAction(ctx, &encounterv2pb.TakeActionRequest{
			EncounterId:   encID,
			ActorEntityId: actorEntityID,
			ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
			Target: &encounterv2pb.ActionTarget{
				Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: monsterID},
			},
		})
		if err != nil {
			// FailedPrecondition surfaces when the encounter ended (last
			// hostile dropped). That's the success exit for the last-monster
			// case — the kill landed on the prior call and this attempt
			// raced past the terminal-state gate.
			if st, ok := status.FromError(err); ok && st.Code() == codes.FailedPrecondition {
				return
			}
			s.Require().NoError(err, "TakeAction attempt %d failed unexpectedly", attempt+1)
		}
		// Read state to see if the monster's gone (killEntity removes it
		// from data.Monsters once HP hits zero).
		data, err := s.srv.EncRepoV2.Get(s.ctx, encID)
		s.Require().NoError(err)
		if _, stillAlive := data.Monsters[core.EntityID(monsterID)]; !stillAlive {
			return
		}
	}
	s.Require().Failf("kill loop exhausted",
		"failed to kill %q in %d attempts", monsterID, maxKillAttempts)
}

// TestCombatSlice_KillLastHostile_FiresDeathChainAndEnds verifies the
// Wave 2.10 happy path: 2 players + 1 goblin, active player kills the
// goblin via attack loop. Both streams must receive EntityDied (with
// killer attribution) + EntityRemoved (broadcast) + EncounterEnded
// (broadcast). A follow-up TakeAction returns FailedPrecondition.
func (s *EncounterV2IntegrationSuite) TestCombatSlice_KillLastHostile_FiresDeathChainAndEnds() {
	encID := "enc-kill-last"

	enc := tkenc.New(core.EncounterID(encID), s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(killGoblinFixturePlayerInput(
		"alice", "char-alice", core.Hex{Q: 0, R: 0, S: 0})))
	s.Require().NoError(enc.AddPlayer(killGoblinFixturePlayerInput(
		"bob", "char-bob", core.Hex{Q: 1, R: -1, S: 0})))
	s.Require().NoError(enc.AddMonster(killGoblinFixtureMonsterInput(
		"goblin-1", core.Hex{Q: 2, R: -1, S: -1})))
	s.Require().NoError(enc.SetMode(core.ModeTurnBased))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	// Cycle past any leading NPC turn so a player is active and TakeAction
	// runs in the happy path.
	s.advanceUntilPlayerActiveByDirectToolkit(enc)

	// Discover the active player (initiative is rolled randomly).
	data, err := s.srv.EncRepoV2.Get(s.ctx, encID)
	s.Require().NoError(err)
	activeEntity := string(data.Initiative[data.ActiveIdx])
	var activePlayer string
	for pid, pd := range data.Players {
		if string(pd.EntityID) == activeEntity {
			activePlayer = string(pid)
			break
		}
	}
	s.Require().NotEmpty(activePlayer)

	ctxAlice := s.authCtx("alice")
	ctxBob := s.authCtx("bob")
	ctxActive := s.authCtx(activePlayer)

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxAlice,
		&encounterv2pb.StreamEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)
	streamB, err := s.srv.EncounterClientV2.StreamEncounter(ctxBob,
		&encounterv2pb.StreamEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)

	aSink := newEventSink()
	bSink := newEventSink()
	go aSink.drain(streamA)
	go bSink.drain(streamB)

	// Wait for replay to complete on both streams; watermark.
	s.Require().Eventually(func() bool {
		return aSink.hasReplayComplete() && bSink.hasReplayComplete()
	}, 2*time.Second, 25*time.Millisecond,
		"both streams must complete replay before the kill chain begins")
	aSink.snapshot()
	bSink.snapshot()

	// Kill the goblin. The loop bounds the rare-nat-1 miss case.
	s.killMonsterViaTakeActionLoop(ctxActive, encID, activeEntity, "goblin-1")

	// Wait for the death chain to fan out to both streams.
	// The toolkit publishes Died → Removed → Ended in that order; the
	// broker queue preserves it per-stream.
	s.Require().Eventually(func() bool {
		return aSink.hasEncounterEnded() && bSink.hasEncounterEnded()
	}, 2*time.Second, 50*time.Millisecond,
		"both streams must receive EncounterEnded after the last hostile dies")

	// Both streams must have seen the full chain.
	s.Require().True(aSink.hasEntityDiedFor("goblin-1"),
		"alice's stream must include EntityDied for goblin-1")
	s.Require().True(aSink.hasEntityRemovedFor("goblin-1"),
		"alice's stream must include EntityRemoved for goblin-1")
	s.Require().True(aSink.hasEncounterEnded(),
		"alice's stream must include EncounterEnded")

	s.Require().True(bSink.hasEntityDiedFor("goblin-1"),
		"bob's stream must include EntityDied for goblin-1")
	s.Require().True(bSink.hasEntityRemovedFor("goblin-1"),
		"bob's stream must include EntityRemoved for goblin-1")
	s.Require().True(bSink.hasEncounterEnded(),
		"bob's stream must include EncounterEnded")

	// Killer attribution: EntityDied for goblin-1 should carry the killer
	// entity id. The killer is whichever player landed the last attack —
	// both players in the fixture use identical kill-tuned stats, so the
	// killer is the active player.
	died := aSink.findEntityDied("goblin-1")
	s.Require().NotNil(died)
	s.Require().Equal(activeEntity, died.GetKillerEntityId(),
		"EntityDied.KillerEntityId must be the active attacker")

	// EncounterEnded reason must be the toolkit's documented value for
	// "all hostiles defeated."
	end := aSink.findEncounterEnded()
	s.Require().NotNil(end)
	s.Require().Equal("all_hostiles_defeated", end.GetReason(),
		"EncounterEnded.Reason should attribute to all-hostiles-defeated")

	// Persisted state: monsters table empty, mode is ModeEnded.
	finalData, err := s.srv.EncRepoV2.Get(s.ctx, encID)
	s.Require().NoError(err)
	s.Require().Empty(finalData.Monsters,
		"persisted monsters table must be empty post-kill")
	s.Require().Equal(core.ModeEnded, finalData.Mode,
		"persisted Mode must be ModeEnded")

	// A follow-up TakeAction must return FailedPrecondition (terminal state).
	_, err = s.srv.EncounterClientV2.TakeAction(ctxActive, &encounterv2pb.TakeActionRequest{
		EncounterId:   encID,
		ActorEntityId: activeEntity,
		ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: "goblin-1"},
		},
	})
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.FailedPrecondition, st.Code(),
		"TakeAction post-end must return FailedPrecondition, got: %v", err)

	// Same for EndTurn.
	_, err = s.srv.EncounterClientV2.EndTurn(ctxActive, &encounterv2pb.EndTurnRequest{
		EncounterId: encID, EntityId: activeEntity,
	})
	s.Require().Error(err)
	st, ok = status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.FailedPrecondition, st.Code(),
		"EndTurn post-end must return FailedPrecondition, got: %v", err)
}

// TestCombatSlice_KillOneOfTwoHostiles_NoEncounterEnd verifies the
// Wave 2.10 mid-fight slice: 2 players + 2 goblins. Active player kills
// goblin-1 only; both streams receive EntityDied + EntityRemoved for
// goblin-1, but NOT EncounterEnded. After EndTurn, the NPC dispatch
// loop runs goblin-2's turn (skipping the dead goblin-1, which is now
// spliced out of initiative) and lands turn back on a player. A second
// kill targeting goblin-2 finally fires EncounterEnded.
func (s *EncounterV2IntegrationSuite) TestCombatSlice_KillOneOfTwoHostiles_NoEncounterEnd() {
	encID := "enc-kill-one-of-two"

	enc := tkenc.New(core.EncounterID(encID), s.srv.BrokerV2)
	s.Require().NoError(enc.AddPlayer(killGoblinFixturePlayerInput(
		"alice", "char-alice", core.Hex{Q: 0, R: 0, S: 0})))
	s.Require().NoError(enc.AddPlayer(killGoblinFixturePlayerInput(
		"bob", "char-bob", core.Hex{Q: 1, R: -1, S: 0})))
	s.Require().NoError(enc.AddMonster(killGoblinFixtureMonsterInput(
		"goblin-1", core.Hex{Q: 2, R: -1, S: -1})))
	s.Require().NoError(enc.AddMonster(killGoblinFixtureMonsterInput(
		"goblin-2", core.Hex{Q: 3, R: -2, S: -1})))
	s.Require().NoError(enc.SetMode(core.ModeTurnBased))
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))

	s.advanceUntilPlayerActiveByDirectToolkit(enc)

	data, err := s.srv.EncRepoV2.Get(s.ctx, encID)
	s.Require().NoError(err)
	activeEntity := string(data.Initiative[data.ActiveIdx])
	var activePlayer string
	for pid, pd := range data.Players {
		if string(pd.EntityID) == activeEntity {
			activePlayer = string(pid)
			break
		}
	}
	s.Require().NotEmpty(activePlayer)

	ctxAlice := s.authCtx("alice")
	ctxBob := s.authCtx("bob")
	ctxActive := s.authCtx(activePlayer)

	streamA, err := s.srv.EncounterClientV2.StreamEncounter(ctxAlice,
		&encounterv2pb.StreamEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)
	streamB, err := s.srv.EncounterClientV2.StreamEncounter(ctxBob,
		&encounterv2pb.StreamEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)

	aSink := newEventSink()
	bSink := newEventSink()
	go aSink.drain(streamA)
	go bSink.drain(streamB)

	s.Require().Eventually(func() bool {
		return aSink.hasReplayComplete() && bSink.hasReplayComplete()
	}, 2*time.Second, 25*time.Millisecond, "replay must complete on both streams")
	aSink.snapshot()
	bSink.snapshot()

	// --- Step 1: kill goblin-1 only. ---
	s.killMonsterViaTakeActionLoop(ctxActive, encID, activeEntity, "goblin-1")

	// Wait for goblin-1's death chain on both streams. We do NOT expect
	// EncounterEnded here — goblin-2 is still alive.
	s.Require().Eventually(func() bool {
		return aSink.hasEntityRemovedFor("goblin-1") && bSink.hasEntityRemovedFor("goblin-1")
	}, 2*time.Second, 50*time.Millisecond,
		"both streams must receive EntityRemoved for goblin-1")

	// Verify EntityDied + EntityRemoved fired only for goblin-1 (not
	// goblin-2). And NOT EncounterEnded.
	s.Require().True(aSink.hasEntityDiedFor("goblin-1"))
	s.Require().False(aSink.hasEntityDiedFor("goblin-2"),
		"goblin-2 is still alive; EntityDied must NOT fire for it")
	s.Require().False(aSink.hasEncounterEnded(),
		"EncounterEnded must NOT fire while goblin-2 is alive")
	s.Require().False(bSink.hasEncounterEnded(),
		"EncounterEnded must NOT fire (bob's view) while goblin-2 is alive")

	// Persisted: goblin-1 gone, goblin-2 still present, mode unchanged.
	midData, err := s.srv.EncRepoV2.Get(s.ctx, encID)
	s.Require().NoError(err)
	_, goblin1Gone := midData.Monsters[core.EntityID("goblin-1")]
	s.Require().False(goblin1Gone, "goblin-1 must be removed from monsters table")
	_, goblin2Alive := midData.Monsters[core.EntityID("goblin-2")]
	s.Require().True(goblin2Alive, "goblin-2 must still be in monsters table")
	s.Require().Equal(core.ModeTurnBased, midData.Mode,
		"Mode must remain TurnBased — encounter is not over")

	// --- Step 2: end the active player's turn. The NPC dispatch loop
	// must skip the dead goblin-1 (no longer in initiative) and run
	// goblin-2's turn server-side. After the loop, control should land
	// back on a player. ---
	_, err = s.srv.EncounterClientV2.EndTurn(ctxActive, &encounterv2pb.EndTurnRequest{
		EncounterId: encID, EntityId: activeEntity,
	})
	s.Require().NoError(err)

	// Reload to find whoever's active now (likely a player after the
	// dispatch loop).
	postEndData, err := s.srv.EncRepoV2.Get(s.ctx, encID)
	s.Require().NoError(err)
	postEndActive := string(postEndData.Initiative[postEndData.ActiveIdx])
	// Initiative must NOT contain goblin-1 (it was spliced out).
	for _, id := range postEndData.Initiative {
		s.Require().NotEqual(core.EntityID("goblin-1"), id,
			"dead goblin-1 must be spliced out of initiative")
	}
	// Active should be a player (NPC dispatch loop runs goblin-2 then
	// lands on a player). With 2 players + 1 NPC, this is guaranteed
	// per the loop's roster cap.
	var nextActivePlayer string
	for pid, pd := range postEndData.Players {
		if string(pd.EntityID) == postEndActive {
			nextActivePlayer = string(pid)
			break
		}
	}
	s.Require().NotEmpty(nextActivePlayer,
		"after EndTurn + NPC dispatch loop, active actor must be a player")

	// --- Step 3: kill goblin-2. EncounterEnded must now fire on both
	// streams. ---
	ctxNextActive := s.authCtx(nextActivePlayer)
	s.killMonsterViaTakeActionLoop(ctxNextActive, encID, postEndActive, "goblin-2")

	s.Require().Eventually(func() bool {
		return aSink.hasEncounterEnded() && bSink.hasEncounterEnded()
	}, 2*time.Second, 50*time.Millisecond,
		"both streams must receive EncounterEnded after goblin-2 dies")

	s.Require().True(aSink.hasEntityDiedFor("goblin-2"),
		"alice's stream must include EntityDied for goblin-2")
	s.Require().True(aSink.hasEntityRemovedFor("goblin-2"),
		"alice's stream must include EntityRemoved for goblin-2")
}

// advanceUntilPlayerActiveByDirectToolkit drives toolkit verbs directly to
// cycle past any leading NPC turns in initiative. Used in test setup so the
// integration test starts with a player active and TakeAction runs in its
// happy path without needing a SetMode RPC.
func (s *EncounterV2IntegrationSuite) advanceUntilPlayerActiveByDirectToolkit(enc *tkenc.Encounter) {
	for {
		data := enc.ToData()
		if len(data.Initiative) == 0 {
			s.Require().FailNow("seeded encounter has empty initiative; SetMode may have failed")
		}
		active := data.Initiative[data.ActiveIdx]
		isPlayer := false
		for _, pd := range data.Players {
			if pd.EntityID == active {
				isPlayer = true
				break
			}
		}
		if isPlayer {
			break
		}
		// Active is the goblin — run NPCAct + EndTurn to cycle.
		//
		// Reload with a StandIn resolver: encounter v0.7.0 requires WithCombatResolver
		// before NPCAct. The stand-in is sufficient here because we only need to
		// advance past the NPC turn — the attack outcome is not asserted.
		var reloadErr error
		enc, reloadErr = tkenc.LoadFromData(data, s.srv.BrokerV2,
			tkenc.WithCombatResolver(testStandInResolver{}))
		s.Require().NoError(reloadErr)
		s.Require().NoError(enc.NPCAct(s.ctx, active))
		_, _, err := enc.EndTurn(active)
		s.Require().NoError(err)
	}
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, enc.ToData()))
}

// testStandInResolver is a minimal CombatResolver for test setup helpers that
// need to call NPCAct (which requires a wired resolver since encounter v0.7.0)
// without asserting on specific attack outcomes. It mirrors the snapshot-based
// math of the handler's StandInCombatResolver without importing handler packages.
type testStandInResolver struct{}

func (testStandInResolver) ResolveAttack(input tkenc.AttackInput) (*tkenc.AttackOutcome, error) {
	// Deterministic: always hit with AttackBonus+10 (well above any AC in
	// test fixtures) so NPC turns advance reliably without random d20 misses.
	return &tkenc.AttackOutcome{
		Hit:         true,
		AttackRoll:  15,
		AttackBonus: input.AttackerAttackBonus,
		TargetAC:    input.TargetAC,
		Damage:      1,
		DamageType:  input.AttackerDamageType,
	}, nil
}

// eventSink consumes a stream's events into a thread-safe slice. snapshot()
// captures a watermark; eventsAfterSnapshot() returns everything after the
// last snapshot. Used by the combat integration test to separate replay
// events from the events fired by an active RPC.
type eventSink struct {
	mu        sync.Mutex
	events    []*encounterv2pb.EncounterEvent
	watermark int
}

func newEventSink() *eventSink {
	return &eventSink{events: make([]*encounterv2pb.EncounterEvent, 0, 32)}
}

// drain runs in a background goroutine, appending every event from stream
// into the sink until stream.Recv errors (typically EOF / context cancel).
func (s *eventSink) drain(stream encounterv2pb.EncounterService_StreamEncounterClient) {
	for {
		ev, err := stream.Recv()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.events = append(s.events, ev)
		s.mu.Unlock()
	}
}

// snapshot records the current event count as the watermark. Subsequent
// eventsAfterSnapshot calls return only events that arrived after this
// point.
func (s *eventSink) snapshot() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watermark = len(s.events)
}

// eventsAfterSnapshot returns the events that arrived after the most recent
// snapshot() call. Safe to call concurrently with drain().
func (s *eventSink) eventsAfterSnapshot() []*encounterv2pb.EncounterEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*encounterv2pb.EncounterEvent, len(s.events)-s.watermark)
	copy(out, s.events[s.watermark:])
	return out
}

// hasTurnEndedFor reports whether the sink has observed a TurnEnded
// envelope with the given entity id, AFTER the most recent snapshot.
func (s *eventSink) hasTurnEndedFor(entityID string) bool {
	for _, ev := range s.eventsAfterSnapshot() {
		if te := ev.GetTurnEnded(); te != nil && te.GetEntityId() == entityID {
			return true
		}
	}
	return false
}

// hasReplayComplete reports whether the sink has observed all expected
// replay events: SnapshotDelivered + at least one EntityAppeared + at least
// one GeometryRevealed. Replay events are sent synchronously by the handler
// before it enters the live forward loop; once all three kinds are present
// the replay phase is guaranteed complete (per the BuildReplayEvents output
// shape — see translate.go). Used by tests as the watermark signal.
func (s *eventSink) hasReplayComplete() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sawSnap, sawAppeared, sawGeo bool
	for _, ev := range s.events {
		if ev.GetSnapshotDelivered() != nil {
			sawSnap = true
		}
		if ev.GetEntityAppeared() != nil {
			sawAppeared = true
		}
		if ev.GetGeometryRevealed() != nil {
			sawGeo = true
		}
	}
	return sawSnap && sawAppeared && sawGeo
}

// hasEntityDiedFor reports whether the sink has observed an EntityDied
// envelope with the given entity id, AFTER the most recent snapshot.
func (s *eventSink) hasEntityDiedFor(entityID string) bool {
	for _, ev := range s.eventsAfterSnapshot() {
		if d := ev.GetEntityDied(); d != nil && d.GetEntityId() == entityID {
			return true
		}
	}
	return false
}

// hasEntityRemovedFor reports whether the sink has observed an
// EntityRemoved envelope with the given entity id, AFTER the most recent
// snapshot.
func (s *eventSink) hasEntityRemovedFor(entityID string) bool {
	for _, ev := range s.eventsAfterSnapshot() {
		if r := ev.GetEntityRemoved(); r != nil && r.GetEntityId() == entityID {
			return true
		}
	}
	return false
}

// hasEncounterEnded reports whether the sink has observed any
// EncounterEnded envelope AFTER the most recent snapshot.
func (s *eventSink) hasEncounterEnded() bool {
	for _, ev := range s.eventsAfterSnapshot() {
		if ev.GetEncounterEnded() != nil {
			return true
		}
	}
	return false
}

// findEntityDied returns the first EntityDied event in the sink's events
// after snapshot with a matching entity id, or nil.
func (s *eventSink) findEntityDied(entityID string) *encounterv2pb.EntityDied {
	for _, ev := range s.eventsAfterSnapshot() {
		if d := ev.GetEntityDied(); d != nil && d.GetEntityId() == entityID {
			return d
		}
	}
	return nil
}

// findEncounterEnded returns the first EncounterEnded event in the
// sink's events after snapshot, or nil.
func (s *eventSink) findEncounterEnded() *encounterv2pb.EncounterEnded {
	for _, ev := range s.eventsAfterSnapshot() {
		if e := ev.GetEncounterEnded(); e != nil {
			return e
		}
	}
	return nil
}

// findEntityDamaged returns the first EntityDamaged event in the slice, or
// nil if none is present.
func findEntityDamaged(events []*encounterv2pb.EncounterEvent) *encounterv2pb.EntityDamaged {
	for _, ev := range events {
		if d := ev.GetEntityDamaged(); d != nil {
			return d
		}
	}
	return nil
}

// eventsHaveNoUnmappedEnvelope asserts every event in the slice has a
// non-nil oneof envelope. Translator suppression should drop events at the
// stream loop (continue without sending), not surface a nil-oneof event.
func eventsHaveNoUnmappedEnvelope(events []*encounterv2pb.EncounterEvent) bool {
	for _, ev := range events {
		if ev.GetEvent() == nil {
			return false
		}
	}
	return true
}

func TestEncounterV2IntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(EncounterV2IntegrationSuite))
}
