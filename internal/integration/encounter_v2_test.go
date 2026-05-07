// Package integration_test provides bufconn integration tests for the v1alpha2
// encounter service. These are slice-1 gate tests: when they pass, the API side
// of wave 2.5 is feature-complete.
package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

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
	movA, err := streamA.Recv()
	s.Require().NoError(err)
	s.Require().NotNil(movA.GetEntityMoved(), "player-A stream should receive EntityMoved")
	s.Require().Equal("char-A", movA.GetEntityMoved().EntityId, "EntityMoved.EntityId should be char-A (seen by A)")

	movB, err := streamB.Recv()
	s.Require().NoError(err)
	s.Require().NotNil(movB.GetEntityMoved(), "player-B stream should receive EntityMoved")
	s.Require().Equal("char-A", movB.GetEntityMoved().EntityId, "EntityMoved.EntityId should be char-A (seen by B)")
}

func TestEncounterV2IntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(EncounterV2IntegrationSuite))
}
