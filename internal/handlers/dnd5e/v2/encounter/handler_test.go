package encounter_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
)

type HandlerSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc // canceled in TearDownTest to stop streaming goroutines

	broker  *tkenc.Broker
	repo    encountersv2.Repository
	handler *v2encounter.Handler
	fixed   time.Time
}

func (s *HandlerSuite) SetupTest() {
	s.fixed = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.ctx = auth.WithPlayerID(base, "player-A")
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()
	// Capture s.fixed by value (not pointer) to avoid a data race: the streaming
	// goroutines spawned in tests call h.now() concurrently with the next test's
	// SetupTest overwriting s.fixed.
	fixedNow := s.fixed
	h, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: s.broker, Repo: s.repo, Now: func() time.Time { return fixedNow },
	})
	s.Require().NoError(err)
	s.handler = h
}

// TearDownTest cancels the test context, which causes any streaming goroutines
// started by the test to exit their select loop and return.
func (s *HandlerSuite) TearDownTest() {
	s.cancel()
}

func (s *HandlerSuite) TestMoveEntity_HappyPath_LoadsCallsMoveSaves() {
	// Seed encounter with player-A controlling char-A at (0,0,0).
	enc := tkenc.New(context.Background(), "enc-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId:  "enc-1",
		EntityId:     "char-A",
		ProposedPath: []*encounterv2pb.Position{{X: 0, Y: 0, Z: 0}, {X: 1, Y: -1, Z: 0}},
	})
	s.Require().NoError(err)

	// Verify the encounter was saved post-move and that player-A's position
	// reflects the move destination (0,0,0) → (1,-1,0).
	loaded, err := s.repo.Get(s.ctx, "enc-1")
	s.Require().NoError(err)
	s.Require().NotNil(loaded)
	s.Require().Equal(core.EncounterID("enc-1"), loaded.ID)
	s.Require().NotNil(loaded.Players)
	s.Require().Contains(loaded.Players, core.PlayerID("player-A"))
	s.Require().Equal(core.Hex{Q: 1, R: -1, S: 0}, loaded.Players["player-A"].View.Position)
}

func (s *HandlerSuite) TestMoveEntity_NoPlayerID_Unauthenticated() {
	ctx := context.Background() // no auth
	_, err := s.handler.MoveEntity(ctx, &encounterv2pb.MoveEntityRequest{EncounterId: "enc-1"})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestMoveEntity_MissingEncounter_NotFound() {
	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId: "missing", EntityId: "char-A",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestMoveEntity_PlayerNotInEncounter_PermissionDenied() {
	// Seed encounter with no players; auth as player-A (the suite default).
	enc := tkenc.New(context.Background(), "enc-noplayer", s.broker)
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId: "enc-noplayer", EntityId: "char-A",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestMoveEntity_EntityIDMismatch_PermissionDenied() {
	enc := tkenc.New(context.Background(), "enc-2", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId: "enc-2", EntityId: "char-IMPOSTER",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestMoveEntity_EmptyPath_InvalidArgument() {
	enc := tkenc.New(context.Background(), "enc-3", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId: "enc-3", EntityId: "char-A",
		ProposedPath: nil, // empty path is the one true argument-shaped error
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestStreamEncounter_SendsSnapshotFirst() {
	enc := tkenc.New(context.Background(), "enc-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	stream := newCapturingStream(s.ctx)
	go func() {
		_ = s.handler.StreamEncounter(&encounterv2pb.StreamEncounterRequest{
			EncounterId: "enc-1",
		}, stream)
	}()

	first := stream.WaitForSend(s.T(), 2*time.Second)
	s.Require().NotNil(first.GetSnapshotDelivered(), "first event should be SnapshotDelivered")
}

func (s *HandlerSuite) TestStreamEncounter_ForwardsBrokerEvents() {
	enc := tkenc.New(context.Background(), "enc-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	stream := newCapturingStream(s.ctx)
	go func() {
		_ = s.handler.StreamEncounter(&encounterv2pb.StreamEncounterRequest{
			EncounterId: "enc-1",
		}, stream)
	}()

	// Drain the snapshot and the replay event before firing the live move.
	// rpg-api-protos#197 collapsed the old per-entity EntityAppeared + whole-set
	// GeometryRevealed replay burst into a single HexKnowledgeChanged carrying
	// both (BuildReplayEvents' doc) -- one replay envelope now, not two.
	_ = stream.WaitForSend(s.T(), 2*time.Second) // SnapshotDelivered
	_ = stream.WaitForSend(s.T(), 2*time.Second) // HexKnowledgeChanged (char-A self + origin hex)

	// Move via the handler — broker emits MoveEvent.
	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId:  "enc-1",
		EntityId:     "char-A",
		ProposedPath: []*encounterv2pb.Position{{X: 0, Y: 0, Z: 0}, {X: 1, Y: -1, Z: 0}},
	})
	s.Require().NoError(err)

	// Stream should receive an EntityMoved.
	got := stream.WaitForSend(s.T(), 2*time.Second)
	s.Require().NotNil(got.GetEntityMoved())
}

// TestStreamEncounter_MoveOutOfSight_RestatesOriginHexAsRemembered proves the
// rpg-api#733 fix: a viewer moving out of sight of a previously-visible hex
// must have that hex reach them on the wire as HEX_STATE_REMEMBERED. Before
// this fix, the live path only ever translated HexRevealedEvent — which
// fires exclusively on vision GAIN (its own doc) — so a move that only lost
// visibility (exactly what happens here: SightRange defaults to zero, so
// stepping one hex away drops the origin hex out of range with nothing new
// coming into view) published no hex-knowledge event at all. The client kept
// rendering the origin hex as VISIBLE forever, which was the playtest
// symptom ("stepping behind a pillar... does not" turn the area remembered).
//
// This test failing is the bug: run it against translateForStream before the
// translateMoveEventWithData branch existed and the second WaitForSend below
// times out because no second envelope is ever sent for this move.
func (s *HandlerSuite) TestStreamEncounter_MoveOutOfSight_RestatesOriginHexAsRemembered() {
	enc := tkenc.New(context.Background(), "enc-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	stream := newCapturingStream(s.ctx)
	go func() {
		_ = s.handler.StreamEncounter(&encounterv2pb.StreamEncounterRequest{
			EncounterId: "enc-1",
		}, stream)
	}()

	_ = stream.WaitForSend(s.T(), 2*time.Second) // SnapshotDelivered
	_ = stream.WaitForSend(s.T(), 2*time.Second) // HexKnowledgeChanged replay (origin hex, VISIBLE)

	// One-hex move. SightRange is unset (zero), so the destination's visible
	// set is just itself — the origin hex (distance 1 away) falls out of
	// range in this same move, with no new hex revealed to ride a
	// HexRevealedEvent on.
	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId:  "enc-1",
		EntityId:     "char-A",
		ProposedPath: []*encounterv2pb.Position{{X: 0, Y: 0, Z: 0}, {X: 1, Y: -1, Z: 0}},
	})
	s.Require().NoError(err)

	moved := stream.WaitForSend(s.T(), 2*time.Second)
	s.Require().NotNil(moved.GetEntityMoved(), "the mover's own EntityMoved must still be sent, unchanged")

	restated := stream.WaitForSend(s.T(), 2*time.Second)
	changed := restated.GetHexKnowledgeChanged()
	s.Require().NotNil(changed, "the mover's own move must carry a supplemental HexKnowledgeChanged restating their full current knowledge")

	var origin *encounterv2pb.HexRecord
	for _, h := range changed.GetHexes() {
		if h.GetPosition().GetX() == 0 && h.GetPosition().GetY() == 0 && h.GetPosition().GetZ() == 0 {
			origin = h
			break
		}
	}
	s.Require().NotNil(origin, "the origin hex must still be present — nothing is ever forgotten, only re-stated")
	s.Require().Equal(encounterv2pb.HexState_HEX_STATE_REMEMBERED, origin.GetState(),
		"the origin hex must flip to REMEMBERED once out of sight — this is the transition the bug dropped")
}

func (s *HandlerSuite) TestGetEncounter_NoAuth_Unauthenticated() {
	ctx := context.Background() // no auth
	_, err := s.handler.GetEncounter(ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-1",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestGetEncounter_EmptyEncounterID_InvalidArgument() {
	_, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestGetEncounter_UnknownID_NotFound() {
	_, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "does-not-exist",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestGetEncounter_NonMember_PermissionDenied() {
	// Encounter seeded with player-B only; auth context is player-A.
	enc := tkenc.New(context.Background(), "enc-nonmember", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-B", EntityID: "char-B", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-nonmember",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestGetEncounter_Success_ReturnsEncounterWithID() {
	enc := tkenc.New(context.Background(), "enc-get-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	resp, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-get-1",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())
	s.Require().Equal("enc-get-1", resp.GetEncounter().GetId())
}

// TestGetEncounter_ProjectsOptionalPlacementFacing exercises the real handler
// and projection path. The toolkit owns authored-facing validation and values;
// the API only carries pointer presence onto the v1alpha2 Placement wire field.
func (s *HandlerSuite) TestGetEncounter_ProjectsOptionalPlacementFacing() {
	facingEast := uint32(0)
	facingNortheast := uint32(1)
	facingNorthwest := uint32(2)
	facingWest := uint32(3)
	facingSouthwest := uint32(4)
	facingSoutheast := uint32(5)

	testCases := []struct {
		id     string
		facing *uint32
	}{
		{id: "prop-absent", facing: nil},
		{id: "prop-east", facing: &facingEast},
		{id: "prop-northeast", facing: &facingNortheast},
		{id: "prop-northwest", facing: &facingNorthwest},
		{id: "prop-west", facing: &facingWest},
		{id: "prop-southwest", facing: &facingSouthwest},
		{id: "prop-southeast", facing: &facingSoutheast},
	}

	base := tkenc.New(context.Background(), "enc-optional-facing", s.broker)
	s.Require().NoError(base.InitRoom(20, 20, environments.PatternEmpty))
	data := base.ToData()
	data.Space.Obstacles = make([]tkenc.ObstacleData, 0, len(testCases))
	for index, tc := range testCases {
		data.Space.Obstacles = append(data.Space.Obstacles, tkenc.ObstacleData{
			ID:       core.EntityID(tc.id),
			Ref:      "dnd5e:props:statue-reaper",
			Position: core.Hex{Q: index + 1, R: -index - 1, S: 0},
			Facing:   tc.facing,
		})
	}

	enc, err := tkenc.LoadFromData(context.Background(), data, s.broker)
	s.Require().NoError(err)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{}, SightRange: 10,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	resp, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-optional-facing",
	})
	s.Require().NoError(err)

	placements := make(map[string]*encounterv2pb.Placement, len(testCases))
	for _, record := range resp.GetEncounter().GetSpace().GetHexes() {
		for _, placement := range record.GetContents() {
			placements[placement.GetEntityId()] = placement
		}
	}
	for _, tc := range testCases {
		placement := placements[tc.id]
		s.Require().NotNil(placement, "%s must be visible on its hex record", tc.id)
		if tc.facing == nil {
			s.Require().Nil(placement.Facing, "%s must remain absent, not inferred as E", tc.id)
			continue
		}
		s.Require().NotNil(placement.Facing, "%s must retain explicit facing presence", tc.id)
		s.Require().Equal(*tc.facing, *placement.Facing, "%s must retain its toolkit-owned direction index", tc.id)
	}
}

// TestGetEncounter_ProjectsPlacementOffsetsThroughAuthorizedSnapshot exercises
// repository reload plus the real GetEncounter projection. Fog suppression and
// omission/explicit-zero/signed axes are asserted on the same production path.
func (s *HandlerSuite) TestGetEncounter_ProjectsPlacementOffsetsThroughAuthorizedSnapshot() {
	zero := core.PlacementOffset{0, 0, 0}
	signed := core.PlacementOffset{0.125, -2.5, 3.75}
	hidden := core.PlacementOffset{9, 8, 7}

	base := tkenc.New(context.Background(), "enc-placement-offset", s.broker)
	s.Require().NoError(base.InitRoom(30, 30, environments.PatternEmpty))
	data := base.ToData()
	data.Space.Obstacles = []tkenc.ObstacleData{
		{ID: "prop-absent", Ref: "dnd5e:props:bookcase", Position: core.Hex{Q: 1, R: -1}},
		{ID: "prop-zero", Ref: "dnd5e:props:bookcase", Position: core.Hex{Q: 2, R: -2}, Offset: &zero},
	}

	enc, err := tkenc.LoadFromData(context.Background(), data, s.broker)
	s.Require().NoError(err)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{}, SightRange: 10,
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID: "monster-signed", MonsterRef: "dnd5e:monsters:skeleton-captain",
		Position: core.Hex{Q: 3, R: -3}, HP: 10, MaxHP: 10, Offset: &signed,
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID: "monster-hidden", MonsterRef: "dnd5e:monsters:skeleton",
		Position: core.Hex{Q: 20, R: -20}, HP: 10, MaxHP: 10, Offset: &hidden,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	resp, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{EncounterId: "enc-placement-offset"})
	s.Require().NoError(err)
	placements := make(map[string]*encounterv2pb.Placement)
	for _, record := range resp.GetEncounter().GetSpace().GetHexes() {
		for _, placement := range record.GetContents() {
			placements[placement.GetEntityId()] = placement
		}
	}

	s.Require().Nil(placements["prop-absent"].GetOffset())
	s.Require().NotNil(placements["prop-zero"].GetOffset())
	s.Require().Zero(placements["prop-zero"].GetOffset().GetX())
	s.Require().Zero(placements["prop-zero"].GetOffset().GetY())
	s.Require().Zero(placements["prop-zero"].GetOffset().GetZ())
	s.Require().Equal(0.125, placements["monster-signed"].GetOffset().GetX())
	s.Require().Equal(-2.5, placements["monster-signed"].GetOffset().GetY())
	s.Require().Equal(3.75, placements["monster-signed"].GetOffset().GetZ())
	s.Require().NotContains(placements, "monster-hidden", "fog must suppress unauthorized placement and offset")
}

// --- Interact (Wave 2.7) -----------------------------------------------------

func (s *HandlerSuite) TestInteract_NoPlayerID_Unauthenticated() {
	ctx := context.Background() // no auth
	_, err := s.handler.Interact(ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-1",
		TargetEntityId: "door-east",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestInteract_EmptyEncounterID_InvalidArgument() {
	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "",
		TargetEntityId: "door-east",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestInteract_EmptyTargetEntityID_InvalidArgument() {
	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-1",
		TargetEntityId: "",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestInteract_MissingEncounter_NotFound() {
	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "missing",
		TargetEntityId: "door-east",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestInteract_MissingDoorTarget_NotFound() {
	enc := tkenc.New(context.Background(), "enc-no-door", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	// Wave 2.7 only supports door interactions; a target not in data.Doors
	// must be NotFound (not InvalidArgument — the request was syntactically
	// fine, the world just doesn't have an interactable target by that id).
	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-no-door",
		TargetEntityId: "not-a-door",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestInteract_DoorAlreadyOpen_FailedPrecondition() {
	enc := tkenc.New(context.Background(), "enc-open-door", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	// Pre-seed the door as already open.
	enc.AddDoor("door-east", core.Hex{Q: 1, R: 0, S: -1}, true)
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-open-door",
		TargetEntityId: "door-east",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	// Toolkit OpenDoor refuses an already-open door — state-dependent
	// failure, FailedPrecondition per pat-v2-status-code-mapping.
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestInteract_PlayerNotInEncounter_PermissionDenied() {
	// Encounter has a door but no player-A. The #582 orchestrator carve added
	// the upfront membership check to the shared load path, so Interact now maps
	// player-not-in-encounter to PermissionDenied — consistent with MoveEntity,
	// EndTurn, and the orchestrator's other carved verbs (all of which already do
	// this). Previously Interact was the outlier, falling through to the toolkit's
	// FailedPrecondition.
	enc := tkenc.New(context.Background(), "enc-no-player", s.broker)
	enc.AddDoor("door-east", core.Hex{Q: 1, R: 0, S: -1}, false)
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-no-player",
		TargetEntityId: "door-east",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestInteract_OpenDoor_HappyPath() {
	enc := tkenc.New(context.Background(), "enc-interact-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	// Door at adjacent hex so player-A's SightRange:4 can perceive it
	// (toolkit ProjectDoorOpen requires the viewer to see the door's hex
	// before publishing a per-viewer slice).
	enc.AddDoor("door-east", core.Hex{Q: 1, R: 0, S: -1}, false)
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	resp, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-interact-1",
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp, "response is empty by proto design but must be non-nil")
	s.Require().Nil(resp.GetInputRequired(), "Wave 2.7 doors don't prompt; InputRequired is for Wave 2.10 locked doors")

	// Verify the door is now open in the persisted data.
	loaded, err := s.repo.Get(s.ctx, "enc-interact-1")
	s.Require().NoError(err)
	s.Require().NotNil(loaded)
	door, ok := loaded.Doors[core.EntityID("door-east")]
	s.Require().True(ok, "door-east must remain in persisted Doors map")
	s.Require().True(door.Open, "door-east must be persisted as Open after Interact")
}

// --------------------------------------------------------------------
// Wave 2.9: locked-door Interact + SubmitCheck
// --------------------------------------------------------------------

// seedLockedDoorEncounter persists an encounter with player-A standing
// adjacent to a locked door whose lock parameters are taken from the
// (dc, ability, tool) arguments. Tests that need finer control over the
// fixture should construct the Data themselves rather than extending this
// helper — it intentionally takes no extra knobs to keep call sites readable.
func (s *HandlerSuite) seedLockedDoorEncounter(encID, doorID string, dc int, ability, tool string) {
	enc := tkenc.New(context.Background(), core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   "player-A",
		EntityID:   "char-A",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 4,
	}))
	enc.AddDoor(core.EntityID(doorID), core.Hex{Q: 1, R: 0, S: -1}, false)
	data := enc.ToData()
	door := data.Doors[core.EntityID(doorID)]
	door.Locked = true
	door.LockDC = dc
	door.LockAbility = ability
	door.LockTool = tool
	s.Require().NoError(s.repo.Save(s.ctx, data))
}

func (s *HandlerSuite) TestInteract_LockedDoor_ReturnsSkillCheckPrompt() {
	s.seedLockedDoorEncounter("enc-locked-1", "door-east", 15, "DEX", "dnd5e:item:thieves-tools")

	resp, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-locked-1",
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	prompt := resp.GetInputRequired().GetSkillCheck()
	s.Require().NotNil(prompt, "InputRequired{skill_check} must be populated for locked door")
	s.Require().Equal(int32(15), prompt.GetDc())
	s.Require().Equal("DEX", prompt.GetAbility())
	s.Require().NotNil(prompt.GetTool())
	s.Require().Equal("dnd5e", prompt.GetTool().GetModule())
	s.Require().Equal("item", prompt.GetTool().GetType())
	s.Require().Equal("thieves-tools", prompt.GetTool().GetId())

	// Persisted prompt survives the call so SubmitCheck can resolve it.
	loaded, err := s.repo.Get(s.ctx, "enc-locked-1")
	s.Require().NoError(err)
	s.Require().NotNil(loaded.PendingPrompts)
	s.Require().Contains(loaded.PendingPrompts, core.PlayerID("player-A"))

	// Door state remains locked + closed until SubmitCheck resolves the prompt.
	door, ok := loaded.Doors[core.EntityID("door-east")]
	s.Require().True(ok)
	s.Require().True(door.Locked)
	s.Require().False(door.Open)
}

func (s *HandlerSuite) TestInteract_LockedDoor_OmitsToolWhenEmpty() {
	// LockTool is empty (no tool proficiency required) — proto Tool field
	// must be nil rather than a zero-valued Ref.
	s.seedLockedDoorEncounter("enc-locked-no-tool", "door-east", 12, "STR", "")

	resp, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-locked-no-tool",
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)
	prompt := resp.GetInputRequired().GetSkillCheck()
	s.Require().NotNil(prompt)
	s.Require().Equal(int32(12), prompt.GetDc())
	s.Require().Equal("STR", prompt.GetAbility())
	s.Require().Nil(prompt.GetTool(), "tool ref must be nil when LockTool is empty")
}

func (s *HandlerSuite) TestInteract_LockedDoor_PendingPromptCollision_FailedPrecondition() {
	s.seedLockedDoorEncounter("enc-locked-collide", "door-east", 15, "DEX", "")

	// First call issues the prompt.
	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-locked-collide",
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)

	// Second call (same player, prompt outstanding) must be rejected — the
	// "at most one pending prompt per player" invariant is enforced server
	// side, never gated in the web client.
	_, err = s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-locked-collide",
		TargetEntityId: "door-east",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestSubmitCheck_NoAuth_Unauthenticated() {
	ctx := context.Background()
	_, err := s.handler.SubmitCheck(ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "enc-1", EntityId: "char-A", Roll: 10,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestSubmitCheck_EmptyEncounterID_InvalidArgument() {
	_, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "", EntityId: "char-A", Roll: 10,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestSubmitCheck_EmptyEntityID_InvalidArgument() {
	_, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "enc-1", EntityId: "", Roll: 10,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestSubmitCheck_RollOutOfRange_InvalidArgument() {
	tests := []int32{0, 21, -1, 100}
	for _, roll := range tests {
		_, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
			EncounterId: "enc-1", EntityId: "char-A", Roll: roll,
		})
		s.Require().Error(err, "roll=%d", roll)
		st, _ := status.FromError(err)
		s.Require().Equal(codes.InvalidArgument, st.Code(), "roll=%d", roll)
	}
}

func (s *HandlerSuite) TestSubmitCheck_MissingEncounter_NotFound() {
	_, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "missing", EntityId: "char-A", Roll: 10,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestSubmitCheck_NonMember_PermissionDenied() {
	// Encounter exists with a different player (not the caller).
	enc := tkenc.New(context.Background(), "enc-non-member", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "other-player", EntityID: "other-char",
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	// player-A (the auth context's player) is not a member.
	_, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "enc-non-member", EntityId: "char-A", Roll: 10,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestSubmitCheck_EntityIDMismatch_PermissionDenied() {
	// Caller is in the encounter but the request's entity_id does not match
	// their controlled entity (player-A controls char-A).
	enc := tkenc.New(context.Background(), "enc-wrong-entity", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A",
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "enc-wrong-entity", EntityId: "char-someone-else", Roll: 10,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestSubmitCheck_NoPendingPrompt_FailedPrecondition() {
	// Encounter exists with player-A but no prompt outstanding.
	enc := tkenc.New(context.Background(), "enc-no-prompt", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "enc-no-prompt", EntityId: "char-A", Roll: 15,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestSubmitCheck_SuccessPath_OpensDoorAndEmitsEvents() {
	s.seedLockedDoorEncounter("enc-resolve-1", "door-east", 10, "DEX", "")

	// Issue the prompt first via Interact (puts a PendingPrompt on the encounter).
	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-resolve-1",
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)

	// Subscribe before resolving so we capture the DoorOpened event.
	sub, err := s.broker.Subscribe(core.EncounterID("enc-resolve-1"), core.PlayerID("player-A"))
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	// Roll 20 — well above DC 10 even with stub resolver returning zero mods.
	resp, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "enc-resolve-1", EntityId: "char-A", Roll: 20,
	})
	s.Require().NoError(err)
	s.Require().True(resp.GetSuccess())
	s.Require().Equal(int32(20), resp.GetTotal())

	// Drain broker events with a short timeout. We expect at least one
	// DoorOpenedEvent emitted by the toolkit's OpenDoor dispatch.
	var sawDoorOpened bool
	timer := time.After(200 * time.Millisecond)
	for !sawDoorOpened {
		select {
		case evt, ok := <-sub.Events():
			if !ok {
				s.FailNow("subscription closed before DoorOpened arrived")
			}
			// We don't import the toolkit event types here; reflect on the
			// concrete type name to keep this test free of new imports. The
			// integration test below verifies the wire shape end-to-end.
			if evt != nil {
				if t := fmt.Sprintf("%T", evt); strings.Contains(t, "DoorOpened") {
					sawDoorOpened = true
				}
			}
		case <-timer:
			s.FailNow("timed out waiting for DoorOpenedEvent on success path")
		}
	}

	// Door is now open + unlocked, prompt cleared.
	loaded, err := s.repo.Get(s.ctx, "enc-resolve-1")
	s.Require().NoError(err)
	door, ok := loaded.Doors[core.EntityID("door-east")]
	s.Require().True(ok)
	s.Require().True(door.Open)
	s.Require().False(door.Locked, "successful unlock clears Locked")
	s.Require().Empty(loaded.PendingPrompts, "prompt cleared after successful SubmitCheck")
}

func (s *HandlerSuite) TestSubmitCheck_FailurePath_NoEventsPromptCleared() {
	s.seedLockedDoorEncounter("enc-resolve-fail", "door-east", 30, "DEX", "")

	_, err := s.handler.Interact(s.ctx, &encounterv2pb.InteractRequest{
		EncounterId:    "enc-resolve-fail",
		TargetEntityId: "door-east",
	})
	s.Require().NoError(err)

	sub, err := s.broker.Subscribe(core.EncounterID("enc-resolve-fail"), core.PlayerID("player-A"))
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	// Roll 1 against DC 30 — guaranteed failure.
	resp, err := s.handler.SubmitCheck(s.ctx, &encounterv2pb.SubmitCheckRequest{
		EncounterId: "enc-resolve-fail", EntityId: "char-A", Roll: 1,
	})
	s.Require().NoError(err)
	s.Require().False(resp.GetSuccess())
	s.Require().Equal(int32(1), resp.GetTotal())

	// No DoorOpened/HexRevealed events should fire on failure. Wait briefly
	// to give a misbehaving toolkit a chance to publish; absence is the assertion.
	select {
	case evt := <-sub.Events():
		if evt != nil {
			s.Failf("unexpected event on failure path", "%T", evt)
		}
	case <-time.After(100 * time.Millisecond):
		// expected: no events
	}

	// Door remains closed + locked (failure doesn't unlock), prompt cleared.
	loaded, err := s.repo.Get(s.ctx, "enc-resolve-fail")
	s.Require().NoError(err)
	door, ok := loaded.Doors[core.EntityID("door-east")]
	s.Require().True(ok)
	s.Require().False(door.Open)
	s.Require().True(door.Locked, "failed unlock leaves door locked")
	s.Require().Empty(loaded.PendingPrompts, "prompt cleared even on failure")
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}

// capturingStream satisfies grpc.ServerStreamingServer[encounterv2pb.EncounterEvent]
// (i.e., encounterv2pb.EncounterService_StreamEncounterServer) for unit tests.
// It records Send calls in a buffered channel so tests can assert on them.
type capturingStream struct {
	grpc.ServerStream // embed for unused methods (SetHeader, SendHeader, SetTrailer, etc.)
	ctx               context.Context
	sent              chan *encounterv2pb.EncounterEvent
}

func newCapturingStream(ctx context.Context) *capturingStream {
	return &capturingStream{ctx: ctx, sent: make(chan *encounterv2pb.EncounterEvent, 16)}
}

// Context returns the stream's context; satisfies grpc.ServerStream.
func (s *capturingStream) Context() context.Context { return s.ctx }

// Send records the event for later assertion. Non-blocking: if the
// 16-slot buffer fills (more events than the test drains), Send returns
// an error rather than deadlocking the streaming goroutine.
func (s *capturingStream) Send(evt *encounterv2pb.EncounterEvent) error {
	select {
	case s.sent <- evt:
		return nil
	default:
		return fmt.Errorf("capturingStream buffer full (16 events undrained); test should drain or grow buffer")
	}
}

// WaitForSend blocks until an event arrives or timeout expires.
func (s *capturingStream) WaitForSend(t *testing.T, timeout time.Duration) *encounterv2pb.EncounterEvent {
	t.Helper()
	select {
	case evt := <-s.sent:
		return evt
	case <-time.After(timeout):
		t.Fatalf("no event received within %s", timeout)
		return nil
	}
}
