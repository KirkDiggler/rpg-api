package authoringv1alpha1_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	dungeonsmock "github.com/KirkDiggler/rpg-api/internal/dungeons/mock"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// HandlerSuite drives the AuthoringService handler over a mocked registry:
// the handler's job is transport (which failures are a status, which are a
// body) and the registry's contract is tested in internal/dungeons.
type HandlerSuite struct {
	suite.Suite

	ctx      context.Context
	ctrl     *gomock.Controller
	registry *dungeonsmock.MockRegistry
	handler  *authoringhandler.Handler
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}

func (s *HandlerSuite) SetupTest() {
	s.ctx = auth.WithPlayerID(context.Background(), "alice")
	s.ctrl = gomock.NewController(s.T())
	s.registry = dungeonsmock.NewMockRegistry(s.ctrl)

	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: s.registry})
	s.Require().NoError(err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
	s.handler = h
}

func (s *HandlerSuite) requireCode(err error, want codes.Code) {
	s.T().Helper()
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok, "not a status error: %v", err)
	s.Require().Equal(want, st.Code(), st.Message())
}

func (s *HandlerSuite) TestNew_RequiresOrchestrator() {
	_, err := authoringhandler.New(&authoringhandler.HandlerConfig{})
	s.Require().Error(err)
	_, err = authoringhandler.New(nil)
	s.Require().Error(err)
}

func (s *HandlerSuite) TestPutDungeon_NoAuth_Unauthenticated() {
	_, err := s.handler.PutDungeon(context.Background(), &authoringpb.PutDungeonRequest{Key: "crypt"})
	s.requireCode(err, codes.Unauthenticated)
}

func (s *HandlerSuite) TestPutDungeon_EmptyKey_InvalidArgument() {
	_, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Yaml: "key: crypt"})
	s.requireCode(err, codes.InvalidArgument)
}

// TestPutDungeon_MalformedRequestIsAStatus pins the proto's first transport
// rule: a request that cannot name its target is a gRPC status, no body.
func (s *HandlerSuite) TestPutDungeon_MalformedRequestIsAStatus() {
	s.registry.EXPECT().Put(gomock.Any(), gomock.Any()).Return(nil, dungeons.ErrInvalidKey)
	_, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Key: "Bad Key", Yaml: "x"})
	s.requireCode(err, codes.InvalidArgument)

	s.registry.EXPECT().Put(gomock.Any(), gomock.Any()).Return(nil, dungeons.ErrKeyMismatch)
	_, err = s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Key: "crypt", Yaml: "key: other"})
	s.requireCode(err, codes.InvalidArgument)
}

// TestPutDungeon_AFileThatDoesNotCompileIsABody pins the second rule: a
// well-formed request whose file does not compile answers OK with errors
// and no atlas, so the builder's inline-error path has the list.
func (s *HandlerSuite) TestPutDungeon_AFileThatDoesNotCompileIsABody() {
	s.registry.EXPECT().
		Put(gomock.Any(), &dungeons.PutInput{Key: "crypt", YAML: []byte("version: 2\n"), ValidateOnly: true}).
		Return(&dungeons.PutResult{Errors: []dungeons.FieldError{{Path: "start", Message: "start is required"}}}, nil)

	resp, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key: "crypt", Yaml: "version: 2\n", ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Require().Len(resp.GetErrors(), 1)
	s.Equal("start", resp.GetErrors()[0].GetPath())
	s.Equal("start is required", resp.GetErrors()[0].GetMessage())
	s.Nil(resp.GetAtlas(), "no atlas for a file that did not compile")
}

// TestPutDungeon_TheRequestReachesTheRegistryVerbatim: the bytes and the
// validate_only flag cross the handler untouched -- the registry stores
// exactly the text the author sent.
func (s *HandlerSuite) TestPutDungeon_TheRequestReachesTheRegistryVerbatim() {
	yaml := "version: 2\nkey: crypt\n# a comment\n"
	s.registry.EXPECT().
		Put(gomock.Any(), &dungeons.PutInput{Key: "crypt", YAML: []byte(yaml), ValidateOnly: false}).
		Return(&dungeons.PutResult{Entry: &dungeons.Entry{Key: "crypt", YAML: []byte(yaml)}}, nil)

	resp, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Key: "crypt", Yaml: yaml})
	s.Require().NoError(err)
	s.Empty(resp.GetErrors(), "an empty error list IS success")
	// TODO(256): assert resp.Atlas once dungeons.Entry.Atlas is populated
	// (session.AtlasOf, plan T3).
}

func (s *HandlerSuite) TestPutDungeon_RegistryFailureIsInternal() {
	s.registry.EXPECT().Put(gomock.Any(), gomock.Any()).Return(nil, errors.New("disk full"))
	_, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Key: "crypt", Yaml: "x"})
	s.requireCode(err, codes.Internal)
}

func (s *HandlerSuite) TestGetDungeon_NoAuth_Unauthenticated() {
	_, err := s.handler.GetDungeon(context.Background(), &authoringpb.GetDungeonRequest{Key: "crypt"})
	s.requireCode(err, codes.Unauthenticated)
}

func (s *HandlerSuite) TestGetDungeon_EmptyKey_InvalidArgument() {
	_, err := s.handler.GetDungeon(s.ctx, &authoringpb.GetDungeonRequest{})
	s.requireCode(err, codes.InvalidArgument)
}

func (s *HandlerSuite) TestGetDungeon_Unknown_NotFound() {
	s.registry.EXPECT().Get(gomock.Any(), "nope").Return(nil, dungeons.ErrNotFound)
	_, err := s.handler.GetDungeon(s.ctx, &authoringpb.GetDungeonRequest{Key: "nope"})
	s.requireCode(err, codes.NotFound)
}

// TestGetDungeon_ReturnsTheStoredBytesVerbatim: comments and spacing come
// back exactly, because the handler hands the registry's bytes through.
func (s *HandlerSuite) TestGetDungeon_ReturnsTheStoredBytesVerbatim() {
	yaml := "version: 2\nkey: crypt   # trailing spaces and a comment\n"
	s.registry.EXPECT().Get(gomock.Any(), "crypt").Return(&dungeons.Entry{Key: "crypt", YAML: []byte(yaml)}, nil)

	resp, err := s.handler.GetDungeon(s.ctx, &authoringpb.GetDungeonRequest{Key: "crypt"})
	s.Require().NoError(err)
	s.Equal(yaml, resp.GetYaml())
}
