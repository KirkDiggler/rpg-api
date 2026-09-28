package characterdraft_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
)

type DraftRedisContractSuite struct {
	suite.Suite
	ctx    context.Context
	server *miniredis.Miniredis
	client *redis.Client
	repo   draftrepo.Repository
}

func TestDraftRedisContractSuite(t *testing.T) { suite.Run(t, new(DraftRedisContractSuite)) }
func (s *DraftRedisContractSuite) SetupTest() {
	s.ctx = context.Background()
	s.server = miniredis.RunT(s.T())
	s.client = redis.NewClient(&redis.Options{Addr: s.server.Addr(), MaxRetries: -1})
	client := s.client
	s.T().Cleanup(func() { _ = client.Close() })
	var err error
	s.repo, err = draftrepo.NewRedis(&draftrepo.Config{Client: s.client, Clock: clock.New(), IDGenerator: idgen.NewPrefixed("generated-")})
	s.Require().NoError(err)
}

func populatedDraft() *entities.CharacterDraft {
	name := "Saved choice"
	return &entities.CharacterDraft{Data: &tkcharacter.DraftData{
		ID: "draft-a", PlayerID: "owner-a", Name: "Draft name", Race: "human", Class: "fighter", Background: "soldier",
		BaseAbilityScores: shared.AbilityScores{"str": 13, "dex": 9},
		Choices:           []choices.ChoiceData{{Category: "name", Source: "player", NameSelection: &name}},
	}}
}
func (s *DraftRedisContractSuite) create(in *entities.CharacterDraft) *draftrepo.CreateOutput {
	out, err := s.repo.Create(s.ctx, draftrepo.CreateInput{Draft: in})
	s.Require().NoError(err)
	s.Require().NotNil(out)
	return out
}
func (s *DraftRedisContractSuite) get(id string) *entities.CharacterDraft {
	out, err := s.repo.Get(s.ctx, draftrepo.GetInput{ID: id})
	s.Require().NoError(err)
	s.Require().NotNil(out)
	return out.Draft
}

func (s *DraftRedisContractSuite) TestPopulatedRoundTrip_DetachedReads() {
	in := populatedDraft()
	out := s.create(in)
	s.Equal(in, out.Draft)
	got := s.get("draft-a")
	s.Equal(in, got)
	in.Data.BaseAbilityScores["str"] = 99
	*got.Data.Choices[0].NameSelection = "mutated read"
	s.Equal(populatedDraft(), s.get("draft-a"))
	byPlayer, err := s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{PlayerID: "owner-a"})
	s.Require().NoError(err)
	s.Equal(populatedDraft(), byPlayer.Draft)
	s.NotSame(got.Data, byPlayer.Draft.Data)
}

func (s *DraftRedisContractSuite) TestCreateGeneratesMissingID_AndPreservesSuppliedID() {
	in := populatedDraft()
	in.Data.ID = ""
	out := s.create(in)
	s.NotEmpty(out.Draft.Data.ID)
	s.Equal(out.Draft, s.get(out.Draft.Data.ID))
	supplied := populatedDraft()
	supplied.Data.PlayerID = "owner-b"
	s.Equal("draft-a", s.create(supplied).Draft.Data.ID)
}

func (s *DraftRedisContractSuite) TestReplacementRemovesOldDraft_WithoutTouchingOtherPlayer() {
	s.create(populatedDraft())
	other := populatedDraft()
	other.Data.ID = "draft-b"
	other.Data.PlayerID = "owner-b"
	s.create(other)
	replacement := populatedDraft()
	replacement.Data.ID = "replacement"
	replacement.Data.Name = "Replacement"
	s.create(replacement)
	missing, err := s.repo.Get(s.ctx, draftrepo.GetInput{ID: "draft-a"})
	s.True(apierr.IsNotFound(err))
	s.Nil(missing)
	mapped, err := s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{PlayerID: "owner-a"})
	s.Require().NoError(err)
	s.Equal(replacement, mapped.Draft)
	otherMapped, err := s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{PlayerID: "owner-b"})
	s.Require().NoError(err)
	s.Equal(other, otherMapped.Draft)
}

func (s *DraftRedisContractSuite) TestUpdateRefreshesTTL_ButReadsDoNot() {
	s.create(populatedDraft())
	s.Equal(24*time.Hour, s.server.TTL("draft:draft-a"))
	s.server.FastForward(23 * time.Hour)
	updated := s.get("draft-a")
	s.Equal(time.Hour, s.server.TTL("draft:draft-a"))
	updated.Data.Name = "Updated"
	out, err := s.repo.Update(s.ctx, draftrepo.UpdateInput{Draft: updated})
	s.Require().NoError(err)
	s.Equal(updated, out.Draft)
	s.Equal(updated, s.get("draft-a"))
	s.Equal(24*time.Hour, s.server.TTL("draft:draft-a"))
	s.server.FastForward(24 * time.Hour)
	missing, err := s.repo.Get(s.ctx, draftrepo.GetInput{ID: "draft-a"})
	s.True(apierr.IsNotFound(err))
	s.Nil(missing)
	// The mapping has no TTL; lookup lazily removes it after expiry.
	s.True(s.server.Exists("draft:player:owner-a"))
	_, err = s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{PlayerID: "owner-a"})
	s.True(apierr.IsNotFound(err))
	s.False(s.server.Exists("draft:player:owner-a"))
}

func (s *DraftRedisContractSuite) TestDeleteRemovesRecordAndMapping_WithoutTouchingNeighbor() {
	s.create(populatedDraft())
	other := populatedDraft()
	other.Data.ID = "draft-b"
	other.Data.PlayerID = "owner-b"
	s.create(other)
	out, err := s.repo.Delete(s.ctx, draftrepo.DeleteInput{ID: "draft-a"})
	s.Require().NoError(err)
	s.NotNil(out)
	s.False(s.server.Exists("draft:draft-a"))
	s.False(s.server.Exists("draft:player:owner-a"))
	s.Equal(other, s.get("draft-b"))
}

func (s *DraftRedisContractSuite) TestMissingRecordsReturnNotFound() {
	_, err := s.repo.Get(s.ctx, draftrepo.GetInput{ID: "missing"})
	s.True(apierr.IsNotFound(err))
	_, err = s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{PlayerID: "missing"})
	s.True(apierr.IsNotFound(err))
	_, err = s.repo.Update(s.ctx, draftrepo.UpdateInput{Draft: populatedDraft()})
	s.True(apierr.IsNotFound(err))
	_, err = s.repo.Delete(s.ctx, draftrepo.DeleteInput{ID: "missing"})
	s.True(apierr.IsNotFound(err))
	s.Empty(s.server.Keys())
}

func (s *DraftRedisContractSuite) TestMalformedPayloadPropagates_WithoutRemovingMapping() {
	s.Require().NoError(s.server.Set("draft:draft-a", "{"))
	s.Require().NoError(s.server.Set("draft:player:owner-a", "draft-a"))
	_, err := s.repo.Get(s.ctx, draftrepo.GetInput{ID: "draft-a"})
	var syntax *json.SyntaxError
	s.ErrorAs(err, &syntax)
	s.False(apierr.IsNotFound(err))
	_, err = s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{PlayerID: "owner-a"})
	s.ErrorAs(err, &syntax)
	_, err = s.repo.Delete(s.ctx, draftrepo.DeleteInput{ID: "draft-a"})
	s.ErrorAs(err, &syntax)
	s.True(s.server.Exists("draft:player:owner-a"))
	s.True(s.server.Exists("draft:draft-a"))
}

func (s *DraftRedisContractSuite) TestInvalidInputsDoNotWrite() {
	for _, in := range []*entities.CharacterDraft{nil, {}, {Data: &tkcharacter.DraftData{}}} {
		_, err := s.repo.Create(s.ctx, draftrepo.CreateInput{Draft: in})
		s.True(apierr.IsInvalidArgument(err))
		_, err = s.repo.Update(s.ctx, draftrepo.UpdateInput{Draft: in})
		s.True(apierr.IsInvalidArgument(err))
	}
	_, err := s.repo.Get(s.ctx, draftrepo.GetInput{})
	s.True(apierr.IsInvalidArgument(err))
	_, err = s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{})
	s.True(apierr.IsInvalidArgument(err))
	_, err = s.repo.Delete(s.ctx, draftrepo.DeleteInput{})
	s.True(apierr.IsInvalidArgument(err))
	s.Empty(s.server.Keys())
}

// Fail writes after existence/mapping reads succeeded; no partial execution is
// simulated here, and these checks do not claim Redis transaction rollback.
type draftWriteFailure struct{ cause error }

func (h draftWriteFailure) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h draftWriteFailure) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "set" {
			return h.cause
		}
		return next(ctx, cmd)
	}
}
func (h draftWriteFailure) ProcessPipelineHook(_ redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error { return h.cause }
}
func (s *DraftRedisContractSuite) TestWriteFailuresPreservePreviousRecordAndMapping() {
	original := populatedDraft()
	s.create(original)
	cause := errors.New("injected write failure")
	s.client.AddHook(draftWriteFailure{cause: cause})
	replacement := populatedDraft()
	replacement.Data.ID = "replacement"
	_, err := s.repo.Create(s.ctx, draftrepo.CreateInput{Draft: replacement})
	s.ErrorIs(err, cause)
	changed := populatedDraft()
	changed.Data.Name = "Changed"
	_, err = s.repo.Update(s.ctx, draftrepo.UpdateInput{Draft: changed})
	s.ErrorIs(err, cause)
	_, err = s.repo.Delete(s.ctx, draftrepo.DeleteInput{ID: "draft-a"})
	s.ErrorIs(err, cause)
	s.Equal(original, s.get("draft-a"))
	s.False(s.server.Exists("draft:replacement"))
	mapped, err := s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{PlayerID: "owner-a"})
	s.Require().NoError(err)
	s.Equal(original, mapped.Draft)
}
func (s *DraftRedisContractSuite) TestStorageReadFailuresKeepCause() {
	s.Require().NoError(s.client.Close())
	calls := []func() error{
		func() error { _, e := s.repo.Create(s.ctx, draftrepo.CreateInput{Draft: populatedDraft()}); return e },
		func() error { _, e := s.repo.Get(s.ctx, draftrepo.GetInput{ID: "draft-a"}); return e },
		func() error {
			_, e := s.repo.GetByPlayerID(s.ctx, draftrepo.GetByPlayerIDInput{PlayerID: "owner-a"})
			return e
		},
		func() error { _, e := s.repo.Update(s.ctx, draftrepo.UpdateInput{Draft: populatedDraft()}); return e },
		func() error { _, e := s.repo.Delete(s.ctx, draftrepo.DeleteInput{ID: "draft-a"}); return e },
	}
	for _, call := range calls {
		err := call()
		s.ErrorIs(err, redis.ErrClosed)
		s.False(apierr.IsNotFound(err))
	}
}
