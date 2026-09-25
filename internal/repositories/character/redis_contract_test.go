package character_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// These fixtures are records, not playable characters. No constructor, rule,
// equipment projection or dice engine is needed to specify storage behavior.
type CharacterRedisContractSuite struct {
	suite.Suite
	ctx    context.Context
	server *miniredis.Miniredis
	client *redis.Client
	repo   characterrepo.Repository
}

func TestCharacterRedisContractSuite(t *testing.T) { suite.Run(t, new(CharacterRedisContractSuite)) }

func (s *CharacterRedisContractSuite) SetupTest() {
	s.ctx = context.Background()
	s.server = miniredis.RunT(s.T())
	s.client = redis.NewClient(&redis.Options{Addr: s.server.Addr(), MaxRetries: -1})
	client := s.client
	s.T().Cleanup(func() { _ = client.Close() })
	var err error
	s.repo, err = characterrepo.NewRedis(&characterrepo.RedisConfig{Client: s.client})
	s.Require().NoError(err)
}

func populatedCharacter() *entities.Character {
	zero := uint32(0)
	return &entities.Character{Data: &tkcharacter.Data{
		ID: "char-a", PlayerID: "owner-a", Name: "Stored name", Level: 3, ClassID: "fighter", RaceID: "human",
		HitPoints: 7, MaxHitPoints: 23, ArmorClass: 17,
		EquipmentSlots: tkcharacter.EquipmentSlots{tkcharacter.SlotMainHand: "item-a"},
		Resources:      map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{"opaque-pool": {Current: 2, Maximum: 5}},
		Conditions:     []json.RawMessage{json.RawMessage(`{"opaque":"payload","count":2}`)},
		ActionEconomy:  &tkcharacter.ActionEconomyData{TurnNumber: 4, ActionsRemaining: 0, ReactionsRemaining: 1, MovementRemaining: 15},
		Appearance:     &customization.Appearance{Hair: &customization.HairCustomization{ColorSRGB: &zero}},
	}}
}

func (s *CharacterRedisContractSuite) create(in *entities.Character) {
	out, err := s.repo.Create(s.ctx, characterrepo.CreateInput{Character: in})
	s.Require().NoError(err)
	s.Require().NotNil(out)
	s.Equal(in, out.Character)
}

func (s *CharacterRedisContractSuite) get(id string) *characterrepo.GetOutput {
	out, err := s.repo.Get(s.ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	s.Require().NotNil(out)
	return out
}

func (s *CharacterRedisContractSuite) TestPopulatedRoundTrip_DetachedReadsAndNoExpiry() {
	in := populatedCharacter()
	s.create(in)
	got := s.get("char-a")
	s.Equal(in, got.Character)
	s.NotEmpty(got.Version)
	s.Equal(got.Version, s.get("char-a").Version)
	s.NotSame(in.Data, got.Character.Data)
	s.Require().NotNil(got.Character.Data.Appearance.Hair.ColorSRGB)
	s.Zero(*got.Character.Data.Appearance.Hair.ColorSRGB)
	s.Nil(got.Character.Data.Appearance.Hair.Roughness)
	// Neither mutating the caller's input nor a loaded nested value is a write.
	in.Data.Resources["opaque-pool"] = tkcharacter.RecoverableResourceData{Current: 99}
	got.Character.Data.EquipmentSlots[tkcharacter.SlotMainHand] = "not-saved"
	*got.Character.Data.Appearance.Hair.ColorSRGB = 123
	got.Character.Data.Conditions[0][2] = 'x'
	s.server.FastForward(365 * 24 * time.Hour)
	s.Equal(populatedCharacter(), s.get("char-a").Character)
	s.Zero(s.server.TTL("character:char-a"))
}

func (s *CharacterRedisContractSuite) TestCreateDuplicate_DoesNotOverwriteOrReindex() {
	s.create(populatedCharacter())
	duplicate := populatedCharacter()
	duplicate.Data.PlayerID = "other-owner"
	out, err := s.repo.Create(s.ctx, characterrepo.CreateInput{Character: duplicate})
	s.Require().True(apierr.IsAlreadyExists(err), "%v", err)
	s.Nil(out)
	s.Equal(populatedCharacter(), s.get("char-a").Character)
	other, err := s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "other-owner"})
	s.Require().NoError(err)
	s.Empty(other.Characters)
}

func (s *CharacterRedisContractSuite) TestUpdateMovesPlayerIndex_AndDeleteRemovesRecordAndIndex() {
	s.create(populatedCharacter())
	neighbor := populatedCharacter()
	neighbor.Data.ID = "char-b"
	s.create(neighbor)
	before := s.get("char-a")
	changed := populatedCharacter()
	changed.Data.PlayerID = "owner-b"
	changed.Data.Name = "Changed"
	out, err := s.repo.Update(s.ctx, characterrepo.UpdateInput{Character: changed})
	s.Require().NoError(err)
	s.Equal(changed, out.Character)
	s.Equal(changed, s.get("char-a").Character)
	s.NotEqual(before.Version, s.get("char-a").Version)
	old, err := s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "owner-a"})
	s.Require().NoError(err)
	s.Equal([]*entities.Character{neighbor}, old.Characters)
	moved, err := s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "owner-b"})
	s.Require().NoError(err)
	s.Equal([]*entities.Character{changed}, moved.Characters)
	deleted, err := s.repo.Delete(s.ctx, characterrepo.DeleteInput{ID: "char-a"})
	s.Require().NoError(err)
	s.NotNil(deleted)
	// Assert the write's effect BEFORE ListByPlayerID can lazily repair a stale
	// member. A list-only assertion passes even if Delete forgets its SRem.
	remaining, indexErr := s.client.SMembers(s.ctx, "character:player:owner-b").Result()
	s.Require().NoError(indexErr)
	s.Empty(remaining)
	neighbors, indexErr := s.client.SMembers(s.ctx, "character:player:owner-a").Result()
	s.Require().NoError(indexErr)
	s.Equal([]string{"char-b"}, neighbors)
	missing, err := s.repo.Get(s.ctx, characterrepo.GetInput{ID: "char-a"})
	s.True(apierr.IsNotFound(err))
	s.Nil(missing)
	moved, err = s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "owner-b"})
	s.Require().NoError(err)
	s.Empty(moved.Characters)
	s.Equal(neighbor, s.get("char-b").Character)
}

func (s *CharacterRedisContractSuite) TestUpdateCanRemoveAndAssignPlayerIndex() {
	in := populatedCharacter()
	s.create(in)
	in.Data.PlayerID = ""
	_, err := s.repo.Update(s.ctx, characterrepo.UpdateInput{Character: in})
	s.Require().NoError(err)
	old, err := s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "owner-a"})
	s.Require().NoError(err)
	s.Empty(old.Characters)
	in.Data.PlayerID = "owner-b"
	_, err = s.repo.Update(s.ctx, characterrepo.UpdateInput{Character: in})
	s.Require().NoError(err)
	got, err := s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "owner-b"})
	s.Require().NoError(err)
	s.Equal([]*entities.Character{in}, got.Characters)
}

func (s *CharacterRedisContractSuite) TestListsResolveStoredRecordsAndCleanStaleIDs() {
	in := populatedCharacter()
	s.create(in)
	// Session membership is not written by character CRUD. Seed its existing
	// read-side contract explicitly, rather than inventing a membership writer.
	s.server.SAdd("character:session:session-a", "char-a", "missing")
	s.server.SAdd("character:player:owner-a", "missing")
	bySession, err := s.repo.ListBySessionID(s.ctx, characterrepo.ListBySessionIDInput{SessionID: "session-a"})
	s.Require().NoError(err)
	s.Equal([]*entities.Character{in}, bySession.Characters)
	byPlayer, err := s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "owner-a"})
	s.Require().NoError(err)
	s.Equal([]*entities.Character{in}, byPlayer.Characters)
	for _, key := range []string{"character:session:session-a", "character:player:owner-a"} {
		members, membersErr := s.server.Members(key)
		s.Require().NoError(membersErr)
		s.Equal([]string{"char-a"}, members)
	}
	other, err := s.repo.ListBySessionID(s.ctx, characterrepo.ListBySessionIDInput{SessionID: "session-b"})
	s.Require().NoError(err)
	s.Empty(other.Characters)
	byPlayer.Characters[0].Data.Name = "not-saved"
	s.Equal(in, s.get("char-a").Character)
}

func (s *CharacterRedisContractSuite) TestPatchChangesOnlyEquipmentAndVersion() {
	in := populatedCharacter()
	s.create(in)
	before := s.get("char-a")
	slots := tkcharacter.EquipmentSlots{tkcharacter.SlotOffHand: "item-b"}
	out, err := s.repo.PatchEquipment(s.ctx, characterrepo.PatchEquipmentInput{
		CharacterID: "char-a", ExpectedVersion: before.Version, ExpectedEquipmentSlots: in.Data.EquipmentSlots,
		EquipmentSlots: slots, ArmorClass: 31,
	})
	s.Require().NoError(err)
	s.Require().True(out.Applied)
	// 31 is supplied, never derived from item-b by this adapter.
	expected := populatedCharacter()
	expected.Data.EquipmentSlots = tkcharacter.EquipmentSlots{tkcharacter.SlotOffHand: "item-b"}
	expected.Data.ArmorClass = 31
	s.Equal(expected, out.Character)
	s.NotEqual(before.Version, out.Version)
	stored := s.get("char-a")
	s.Equal(expected, stored.Character)
	s.Equal(out.Version, stored.Version)
	slots[tkcharacter.SlotOffHand] = "mutated-input"
	s.Equal(expected, s.get("char-a").Character)
	s.Zero(s.server.TTL("character:char-a"))
}

func (s *CharacterRedisContractSuite) TestMissingRecordsKeepNotFoundIdentity() {
	_, err := s.repo.Get(s.ctx, characterrepo.GetInput{ID: "missing"})
	s.True(apierr.IsNotFound(err))
	_, err = s.repo.Delete(s.ctx, characterrepo.DeleteInput{ID: "missing"})
	s.True(apierr.IsNotFound(err))
	_, err = s.repo.Update(s.ctx, characterrepo.UpdateInput{Character: populatedCharacter()})
	s.True(apierr.IsNotFound(err))
	_, err = s.repo.PatchEquipment(s.ctx, characterrepo.PatchEquipmentInput{CharacterID: "missing", ExpectedVersion: "version"})
	s.True(apierr.IsNotFound(err))
}

func (s *CharacterRedisContractSuite) TestPatchRejectsNullDataWithoutWriting() {
	const corruptRecord = `{"data":null}`
	s.Require().NoError(s.server.Set("character:char-a", corruptRecord))
	out, err := s.repo.PatchEquipment(s.ctx, characterrepo.PatchEquipmentInput{
		CharacterID: "char-a", ExpectedVersion: "version", ArmorClass: 31,
	})
	s.Require().True(apierr.IsInternal(err), "%v", err)
	s.Nil(out)
	stored, readErr := s.server.Get("character:char-a")
	s.Require().NoError(readErr)
	s.Equal(corruptRecord, stored)
	// Update does not share this guard: its pre-existing panic is #1057,
	// not a behavior this test should endorse.
}

func (s *CharacterRedisContractSuite) TestMalformedPayloadIsNotMissing_AndListDoesNotDiscardIt() {
	s.Require().NoError(s.server.Set("character:broken", "{"))
	s.server.SAdd("character:player:owner-a", "broken")
	s.server.SAdd("character:session:session-a", "broken")
	calls := []func() error{
		func() error { _, e := s.repo.Get(s.ctx, characterrepo.GetInput{ID: "broken"}); return e },
		func() error {
			_, e := s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "owner-a"})
			return e
		},
		func() error {
			_, e := s.repo.ListBySessionID(s.ctx, characterrepo.ListBySessionIDInput{SessionID: "session-a"})
			return e
		},
		func() error {
			_, e := s.repo.PatchEquipment(s.ctx, characterrepo.PatchEquipmentInput{CharacterID: "broken", ExpectedVersion: "v"})
			return e
		},
	}
	for _, call := range calls {
		err := call()
		var syntax *json.SyntaxError
		s.ErrorAs(err, &syntax)
		s.False(apierr.IsNotFound(err))
	}
	s.True(s.server.Exists("character:broken"))
	members, err := s.server.Members("character:player:owner-a")
	s.Require().NoError(err)
	s.Equal([]string{"broken"}, members)
}

func (s *CharacterRedisContractSuite) TestInvalidInputsDoNotWrite() {
	for _, in := range []*entities.Character{nil, {}, {Data: &tkcharacter.Data{}}} {
		_, err := s.repo.Create(s.ctx, characterrepo.CreateInput{Character: in})
		s.True(apierr.IsInvalidArgument(err))
		_, err = s.repo.Update(s.ctx, characterrepo.UpdateInput{Character: in})
		s.True(apierr.IsInvalidArgument(err))
	}
	_, err := s.repo.Get(s.ctx, characterrepo.GetInput{})
	s.True(apierr.IsInvalidArgument(err))
	_, err = s.repo.Delete(s.ctx, characterrepo.DeleteInput{})
	s.True(apierr.IsInvalidArgument(err))
	_, err = s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{})
	s.True(apierr.IsInvalidArgument(err))
	_, err = s.repo.ListBySessionID(s.ctx, characterrepo.ListBySessionIDInput{})
	s.True(apierr.IsInvalidArgument(err))
	for _, in := range []characterrepo.PatchEquipmentInput{{}, {CharacterID: "char-a"}} {
		_, err = s.repo.PatchEquipment(s.ctx, in)
		s.True(apierr.IsInvalidArgument(err))
	}
	s.Empty(s.server.Keys())
}

// Fail only the transaction execution boundary; reads still hit miniredis.
// This catches swallowed write errors, which a disconnected-client test alone
// would miss because Update/Delete read before they write.
type characterWriteFailure struct{ cause error }

func (h characterWriteFailure) DialHook(next redis.DialHook) redis.DialHook {
	return next
}
func (h characterWriteFailure) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }
func (h characterWriteFailure) ProcessPipelineHook(_ redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error { return h.cause }
}

func (s *CharacterRedisContractSuite) TestTransactionFailuresPropagateWithoutClaimingSuccess() {
	in := populatedCharacter()
	s.create(in)
	version := s.get("char-a").Version
	cause := errors.New("injected transaction failure")
	s.client.AddHook(characterWriteFailure{cause: cause})
	fresh := populatedCharacter()
	fresh.Data.ID = "new"
	_, err := s.repo.Create(s.ctx, characterrepo.CreateInput{Character: fresh})
	s.ErrorIs(err, cause)
	changed := populatedCharacter()
	changed.Data.PlayerID = "owner-b"
	_, err = s.repo.Update(s.ctx, characterrepo.UpdateInput{Character: changed})
	s.ErrorIs(err, cause)
	_, err = s.repo.Delete(s.ctx, characterrepo.DeleteInput{ID: "char-a"})
	s.ErrorIs(err, cause)
	_, err = s.repo.PatchEquipment(s.ctx, characterrepo.PatchEquipmentInput{CharacterID: "char-a", ExpectedVersion: version, ExpectedEquipmentSlots: in.Data.EquipmentSlots, ArmorClass: 31})
	s.ErrorIs(err, cause)
	s.Equal(in, s.get("char-a").Character)
	s.False(s.server.Exists("character:new"))
	s.False(s.server.Exists("character:player:owner-b"))
}

func (s *CharacterRedisContractSuite) TestStorageReadFailuresAreNotNotFound() {
	s.Require().NoError(s.client.Close())
	calls := []func() error{
		func() error {
			_, e := s.repo.Create(s.ctx, characterrepo.CreateInput{Character: populatedCharacter()})
			return e
		},
		func() error { _, e := s.repo.Get(s.ctx, characterrepo.GetInput{ID: "char-a"}); return e },
		func() error {
			_, e := s.repo.Update(s.ctx, characterrepo.UpdateInput{Character: populatedCharacter()})
			return e
		},
		func() error { _, e := s.repo.Delete(s.ctx, characterrepo.DeleteInput{ID: "char-a"}); return e },
		func() error {
			_, e := s.repo.ListByPlayerID(s.ctx, characterrepo.ListByPlayerIDInput{PlayerID: "owner-a"})
			return e
		},
		func() error {
			_, e := s.repo.ListBySessionID(s.ctx, characterrepo.ListBySessionIDInput{SessionID: "session-a"})
			return e
		},
		func() error {
			_, e := s.repo.PatchEquipment(s.ctx, characterrepo.PatchEquipmentInput{CharacterID: "char-a", ExpectedVersion: "v"})
			return e
		},
	}
	for _, call := range calls {
		err := call()
		s.ErrorIs(err, redis.ErrClosed)
		s.False(apierr.IsNotFound(err))
	}
}
