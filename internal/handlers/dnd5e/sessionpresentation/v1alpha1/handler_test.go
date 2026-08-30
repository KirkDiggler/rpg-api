package sessionpresentationv1alpha1

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	grpc "google.golang.org/grpc"

	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionaccess "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	orchsessionpresentation "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

type HandlerSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	ctx       context.Context
	playerID  string
	sessionID string
	memberID  string
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}

func (s *HandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.playerID = "alice"
	s.sessionID = "session-1"
	s.memberID = "char-1"
	s.ctx = auth.WithPlayerID(context.Background(), s.playerID)
}

func (s *HandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *HandlerSuite) ownedAccess(order *callOrderRecorder) *sessionaccess.Access {
	s.T().Helper()
	return s.accessForMemberOwner(s.playerID, order)
}

func (s *HandlerSuite) foreignAccess() *sessionaccess.Access {
	s.T().Helper()
	return s.accessForMemberOwner("other-player", nil)
}

func (s *HandlerSuite) accessForMemberOwner(ownerPlayerID string, order *callOrderRecorder) *sessionaccess.Access {
	s.T().Helper()

	characters := charactermock.NewMockRepository(s.ctrl)
	characters.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: s.memberID}).DoAndReturn(
		func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			if order != nil {
				order.Add("characters.Get")
			}
			return &characterrepo.GetOutput{Character: &entities.Character{Data: &tkcharacter.Data{ID: s.memberID, PlayerID: ownerPlayerID}}}, nil
		},
	).Times(1)

	access, err := sessionaccess.New(characters, &fakeRosterRepo{getFn: func(_ context.Context, encounterID string) (*rosterrepo.Data, error) {
		if order != nil {
			order.Add("roster.Get")
		}
		return &rosterrepo.Data{EncounterID: encounterID, Members: []rosterrepo.Member{{ID: s.memberID, Kind: rosterrepo.KindPlayer}}}, nil
	}})
	s.Require().NoError(err)
	return access
}

func (s *HandlerSuite) newHandler(service orchsessionpresentation.Service, access *sessionaccess.Access) *Handler {
	s.T().Helper()
	h, err := New(&HandlerConfig{Service: service, Access: access})
	s.Require().NoError(err)
	return h
}

func (s *HandlerSuite) testDomainDraft() orchsessionpresentation.Draft {
	return orchsessionpresentation.Draft{
		SchemaVersion:       1,
		PresentationID:      "present-1",
		AuthoritySeq:        42,
		Attempt:             2,
		PhysicsSchema:       orchsessionpresentation.PhysicsSchemaRapierDungeonD20V1,
		ColliderFingerprint: repeatedBytes(0x11),
		Bodies: []orchsessionpresentation.BodyInitial{
			{DieID: "attack-d20", Shape: orchsessionpresentation.ShapeD20, State: rigidBodyState(1, 2, 3)},
			{DieID: "support-d20", Shape: orchsessionpresentation.ShapeD20, State: rigidBodyState(-1, -2, -3)},
		},
		Contacts: []orchsessionpresentation.ContactCheckpoint{
			{
				Step:         1,
				PrimaryDieID: "attack-d20",
				OtherDieID:   "support-d20",
				After: []orchsessionpresentation.BodyCheckpoint{
					{DieID: "attack-d20", State: rigidBodyState(10, 20, 30)},
					{DieID: "support-d20", State: rigidBodyState(11, 21, 31)},
				},
			},
			{
				Step:         2,
				PrimaryDieID: "attack-d20",
				StaticCollider: &orchsessionpresentation.StaticColliderContact{
					Kind:       orchsessionpresentation.StaticContactKindDoor,
					ColliderID: "door:hall-1",
				},
				After: []orchsessionpresentation.BodyCheckpoint{{DieID: "attack-d20", State: rigidBodyState(12, 22, 32)}},
			},
		},
		Terminal: []orchsessionpresentation.BodyTerminal{
			{DieID: "attack-d20", Step: 3, Kind: orchsessionpresentation.TerminalKindSettled, State: rigidBodyState(13, 23, 33)},
			{DieID: "support-d20", Step: 4, Kind: orchsessionpresentation.TerminalKindOffTable, State: rigidBodyState(14, 24, 34)},
		},
	}
}

func (s *HandlerSuite) testDomainPlan() orchsessionpresentation.Plan {
	draft := s.testDomainDraft()
	return orchsessionpresentation.Plan{
		SchemaVersion:       draft.SchemaVersion,
		Session:             s.sessionID,
		PresentationID:      draft.PresentationID,
		AuthoritySeq:        draft.AuthoritySeq,
		Roller:              s.memberID,
		Attempt:             draft.Attempt,
		PhysicsSchema:       draft.PhysicsSchema,
		ColliderFingerprint: repeatedBytes(0x11),
		Bodies:              append([]orchsessionpresentation.BodyInitial(nil), draft.Bodies...),
		Contacts:            append([]orchsessionpresentation.ContactCheckpoint(nil), draft.Contacts...),
		Terminal:            append([]orchsessionpresentation.BodyTerminal(nil), draft.Terminal...),
	}
}

func (s *HandlerSuite) testProtoDraft() *presentationpb.DiceThrowDraft {
	return &presentationpb.DiceThrowDraft{
		SchemaVersion:       1,
		PresentationId:      "present-1",
		AuthoritySeq:        42,
		Attempt:             2,
		PhysicsSchema:       presentationpb.DicePhysicsSchema_DICE_PHYSICS_SCHEMA_RAPIER_DUNGEON_D20_V1,
		ColliderFingerprint: repeatedBytes(0x11),
		Bodies: []*presentationpb.DiceBodyInitial{
			{DieId: "attack-d20", Shape: presentationpb.DiceShape_DICE_SHAPE_D20, State: rigidBodyStateProto(1, 2, 3)},
			{DieId: "support-d20", Shape: presentationpb.DiceShape_DICE_SHAPE_D20, State: rigidBodyStateProto(-1, -2, -3)},
		},
		Contacts: []*presentationpb.ContactCheckpoint{
			{
				Step:         1,
				PrimaryDieId: "attack-d20",
				Target:       &presentationpb.ContactCheckpoint_OtherDieId{OtherDieId: "support-d20"},
				After: []*presentationpb.DiceBodyCheckpoint{
					{DieId: "attack-d20", State: rigidBodyStateProto(10, 20, 30)},
					{DieId: "support-d20", State: rigidBodyStateProto(11, 21, 31)},
				},
			},
			{
				Step:         2,
				PrimaryDieId: "attack-d20",
				Target: &presentationpb.ContactCheckpoint_StaticCollider{StaticCollider: &presentationpb.StaticColliderContact{
					Kind:       presentationpb.DiceStaticContactKind_DICE_STATIC_CONTACT_KIND_DOOR,
					ColliderId: "door:hall-1",
				}},
				After: []*presentationpb.DiceBodyCheckpoint{{DieId: "attack-d20", State: rigidBodyStateProto(12, 22, 32)}},
			},
		},
		Terminal: &presentationpb.ThrowTerminal{Dice: []*presentationpb.DiceBodyTerminal{
			{DieId: "attack-d20", Step: 3, Kind: presentationpb.DiceTerminalKind_DICE_TERMINAL_KIND_SETTLED, State: rigidBodyStateProto(13, 23, 33)},
			{DieId: "support-d20", Step: 4, Kind: presentationpb.DiceTerminalKind_DICE_TERMINAL_KIND_OFF_TABLE, State: rigidBodyStateProto(14, 24, 34)},
		}},
	}
}

func (s *HandlerSuite) testProtoPlan() *presentationpb.DiceThrowPlan {
	draft := s.testProtoDraft()
	return &presentationpb.DiceThrowPlan{
		SchemaVersion:       draft.GetSchemaVersion(),
		Session:             s.sessionID,
		PresentationId:      draft.GetPresentationId(),
		AuthoritySeq:        draft.GetAuthoritySeq(),
		Roller:              s.memberID,
		Attempt:             draft.GetAttempt(),
		PhysicsSchema:       draft.GetPhysicsSchema(),
		ColliderFingerprint: repeatedBytes(0x11),
		Bodies:              draft.GetBodies(),
		Contacts:            draft.GetContacts(),
		Terminal:            draft.GetTerminal(),
	}
}

func rigidBodyState(x, y, z float64) *orchsessionpresentation.RigidBodyState {
	return &orchsessionpresentation.RigidBodyState{
		Position:        &orchsessionpresentation.Vector3{X: x, Y: y, Z: z},
		Rotation:        &orchsessionpresentation.Quaternion{X: 0, Y: 0, Z: 0, W: 1},
		LinearVelocity:  &orchsessionpresentation.Vector3{X: x + 1, Y: y + 1, Z: z + 1},
		AngularVelocity: &orchsessionpresentation.Vector3{X: x + 2, Y: y + 2, Z: z + 2},
	}
}

func rigidBodyStateProto(x, y, z float32) *presentationpb.RigidBodyState {
	return &presentationpb.RigidBodyState{
		Position:        &presentationpb.Vector3{X: x, Y: y, Z: z},
		Rotation:        &presentationpb.Quaternion{X: 0, Y: 0, Z: 0, W: 1},
		LinearVelocity:  &presentationpb.Vector3{X: x + 1, Y: y + 1, Z: z + 1},
		AngularVelocity: &presentationpb.Vector3{X: x + 2, Y: y + 2, Z: z + 2},
	}
}

func repeatedBytes(fill byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = fill
	}
	return out
}

type callOrderRecorder struct {
	mu    sync.Mutex
	steps []string
}

func (r *callOrderRecorder) Add(step string) {
	r.mu.Lock()
	r.steps = append(r.steps, step)
	r.mu.Unlock()
}

func (r *callOrderRecorder) Steps() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.steps...)
}

type fakeRosterRepo struct {
	getFn func(context.Context, string) (*rosterrepo.Data, error)
}

func (f *fakeRosterRepo) Get(ctx context.Context, encounterID string) (*rosterrepo.Data, error) {
	if f.getFn != nil {
		return f.getFn(ctx, encounterID)
	}
	return nil, nil
}

func (f *fakeRosterRepo) Save(context.Context, *rosterrepo.Data) error { return nil }

type fakePlanStream struct {
	grpc.ServerStream
	ctx     context.Context
	sendErr error
	sent    chan *presentationpb.DiceThrowPlan
}

func newFakePlanStream(ctx context.Context) *fakePlanStream {
	return &fakePlanStream{ctx: ctx, sent: make(chan *presentationpb.DiceThrowPlan, 8)}
}

func (s *fakePlanStream) Context() context.Context { return s.ctx }

func (s *fakePlanStream) Send(plan *presentationpb.DiceThrowPlan) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	select {
	case s.sent <- plan:
		return nil
	default:
		return context.DeadlineExceeded
	}
}

func (s *fakePlanStream) WaitForSend(t *testing.T) *presentationpb.DiceThrowPlan {
	t.Helper()
	select {
	case plan := <-s.sent:
		return plan
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for streamed plan")
		return nil
	}
}

type fakeSubscription struct {
	plans      chan orchsessionpresentation.Plan
	closeErr   error
	closeCalls atomic.Int32
}

func (f *fakeSubscription) Plans() <-chan orchsessionpresentation.Plan { return f.plans }

func (f *fakeSubscription) Close() error {
	f.closeCalls.Add(1)
	return f.closeErr
}
