// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package character_integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	"github.com/KirkDiggler/rpg-api/internal/sandboxseed"
)

const (
	sandboxFighterID   = "toolkit-sandbox-fighter"
	sandboxBarbarianID = "toolkit-sandbox-barbarian"
)

// SandboxSeedSuite exercises the fixed production-RPC seed path over the
// real CharacterService and verifies the two seeded characters bind only to
// their authenticated lobby identities.
type SandboxSeedSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	server  *harness.TestServer
	release func()
}

func TestSandboxSeedSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(SandboxSeedSuite))
}

func (s *SandboxSeedSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 3*time.Minute)
	s.release = sharedRedis.Lease()

	var err error
	s.server, err = harness.NewWithRedis(s.ctx, nil, sharedRedis.Addr)
	s.Require().NoError(err, "failed to create test server")
	s.Require().NoError(s.server.FlushRedis(s.ctx), "failed to flush redis")
}

func (s *SandboxSeedSuite) TearDownTest() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.release != nil {
		s.release()
	}
}

func (s *SandboxSeedSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

func (s *SandboxSeedSuite) listExactlyOne(identity string) *dnd5ev1alpha1.Character {
	s.T().Helper()
	response, err := s.server.CharacterClient.ListCharacters(s.authCtx(identity), &dnd5ev1alpha1.ListCharactersRequest{
		PageSize: 100,
	})
	require.NoError(s.T(), err)
	require.Empty(s.T(), response.GetNextPageToken())
	require.Len(s.T(), response.GetCharacters(), 1)
	return response.GetCharacters()[0]
}

func (s *SandboxSeedSuite) TestSandboxSeed_ResetsTwoIdentitiesThroughRPCs() {
	require.NoError(s.T(), sandboxseed.Seed(s.ctx, s.server.CharacterClient))
	require.NoError(s.T(), sandboxseed.Seed(s.ctx, s.server.CharacterClient))

	recordingClient := &recordingCharacterClient{CharacterRPC: s.server.CharacterClient}
	require.NoError(s.T(), sandboxseed.Seed(s.ctx, recordingClient))
	require.Equal(s.T(), []string{
		"toolkit-sandbox-fighter:ListCharacters",
		"toolkit-sandbox-fighter:DeleteCharacter",
		"toolkit-sandbox-fighter:CreateDraft",
		"toolkit-sandbox-fighter:UpdateName",
		"toolkit-sandbox-fighter:UpdateRace",
		"toolkit-sandbox-fighter:UpdateClass",
		"toolkit-sandbox-fighter:UpdateBackground",
		"toolkit-sandbox-fighter:UpdateAbilityScores",
		"toolkit-sandbox-fighter:GetDraft",
		"toolkit-sandbox-fighter:FinalizeDraft",
		"toolkit-sandbox-fighter:ListCharacters",
		"toolkit-sandbox-fighter:GetCharacter",
		"toolkit-sandbox-fighter:EquipItem",
		"toolkit-sandbox-fighter:ListCharacters",
		"toolkit-sandbox-barbarian:ListCharacters",
		"toolkit-sandbox-barbarian:DeleteCharacter",
		"toolkit-sandbox-barbarian:CreateDraft",
		"toolkit-sandbox-barbarian:UpdateName",
		"toolkit-sandbox-barbarian:UpdateRace",
		"toolkit-sandbox-barbarian:UpdateClass",
		"toolkit-sandbox-barbarian:UpdateBackground",
		"toolkit-sandbox-barbarian:UpdateAbilityScores",
		"toolkit-sandbox-barbarian:GetDraft",
		"toolkit-sandbox-barbarian:FinalizeDraft",
		"toolkit-sandbox-barbarian:ListCharacters",
		"toolkit-sandbox-barbarian:GetCharacter",
		"toolkit-sandbox-barbarian:ListCharacters",
	}, recordingClient.calls)

	fighter := s.listExactlyOne("toolkit-sandbox-fighter")
	require.Equal(s.T(), "Toolkit Sandbox Fighter", fighter.GetName())
	require.Contains(s.T(), fighter.GetFightingStyles(), dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_PROTECTION)
	require.Equal(s.T(), "shield", fighter.GetEquipmentSlots().GetOffHand().GetItemId())
	require.EqualValues(s.T(), 16, fighter.GetAbilityScores().GetStrength())
	require.Equal(s.T(), dnd5ev1alpha1.Race_RACE_HUMAN, fighter.GetRace())
	require.Equal(s.T(), dnd5ev1alpha1.Class_CLASS_FIGHTER, fighter.GetClass())
	require.Equal(s.T(), int32(14), fighter.GetAbilityScores().GetDexterity())
	require.Equal(s.T(), int32(15), fighter.GetAbilityScores().GetConstitution())
	require.Equal(s.T(), int32(11), fighter.GetAbilityScores().GetIntelligence())
	require.Equal(s.T(), int32(13), fighter.GetAbilityScores().GetWisdom())
	require.Equal(s.T(), int32(9), fighter.GetAbilityScores().GetCharisma())

	barbarian := s.listExactlyOne("toolkit-sandbox-barbarian")
	require.Equal(s.T(), "Toolkit Sandbox Barbarian", barbarian.GetName())
	require.Equal(s.T(), dnd5ev1alpha1.Race_RACE_HUMAN, barbarian.GetRace())
	require.Equal(s.T(), dnd5ev1alpha1.Class_CLASS_BARBARIAN, barbarian.GetClass())
	require.EqualValues(s.T(), 16, barbarian.GetAbilityScores().GetStrength())
	require.Equal(s.T(), int32(14), barbarian.GetAbilityScores().GetDexterity())
	require.Equal(s.T(), int32(15), barbarian.GetAbilityScores().GetConstitution())
	require.Equal(s.T(), int32(9), barbarian.GetAbilityScores().GetIntelligence())
	require.Equal(s.T(), int32(13), barbarian.GetAbilityScores().GetWisdom())
	require.Equal(s.T(), int32(11), barbarian.GetAbilityScores().GetCharisma())

	_, err := s.server.LobbyClient.CreateLobby(s.authCtx(sandboxBarbarianID), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId:  "toolkit-sandbox-wrong-owner-create",
		CharacterId: fighter.GetId(),
	})
	require.Equal(s.T(), codes.PermissionDenied, status.Code(err))

	wrongOwnerHost, err := s.server.LobbyClient.CreateLobby(s.authCtx(sandboxFighterID), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId:  "toolkit-sandbox-wrong-owner-join",
		CharacterId: fighter.GetId(),
	})
	require.NoError(s.T(), err)
	_, err = s.server.LobbyClient.JoinLobby(s.authCtx(sandboxBarbarianID), &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef:     wrongOwnerHost.GetJoinRef(),
		CharacterId: fighter.GetId(),
	})
	require.Equal(s.T(), codes.PermissionDenied, status.Code(err))

	s.assertPartyOrder("toolkit-sandbox-fighter-then-barbarian", sandboxFighterID, fighter.GetId(), sandboxBarbarianID, barbarian.GetId())
	s.assertPartyOrder("toolkit-sandbox-barbarian-then-fighter", sandboxBarbarianID, barbarian.GetId(), sandboxFighterID, fighter.GetId())
}

// recordingCharacterClient proves Seed keeps each call under its fixed Dev
// identity and obtains the post-finalize ID from that identity's list before
// GetCharacter or EquipItem.
type recordingCharacterClient struct {
	sandboxseed.CharacterRPC
	calls []string
}

func (c *recordingCharacterClient) record(ctx context.Context, method string) {
	metadataValues, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		c.calls = append(c.calls, "missing-metadata:"+method)
		return
	}
	values := metadataValues.Get("authorization")
	if len(values) != 1 {
		c.calls = append(c.calls, "invalid-metadata:"+method)
		return
	}
	c.calls = append(c.calls, values[0][len("Dev "):]+":"+method)
}

func (c *recordingCharacterClient) ListCharacters(ctx context.Context, req *dnd5ev1alpha1.ListCharactersRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.ListCharactersResponse, error) {
	c.record(ctx, "ListCharacters")
	return c.CharacterRPC.ListCharacters(ctx, req, opts...)
}

func (c *recordingCharacterClient) DeleteCharacter(ctx context.Context, req *dnd5ev1alpha1.DeleteCharacterRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.DeleteCharacterResponse, error) {
	c.record(ctx, "DeleteCharacter")
	return c.CharacterRPC.DeleteCharacter(ctx, req, opts...)
}

func (c *recordingCharacterClient) CreateDraft(ctx context.Context, req *dnd5ev1alpha1.CreateDraftRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.CreateDraftResponse, error) {
	c.record(ctx, "CreateDraft")
	return c.CharacterRPC.CreateDraft(ctx, req, opts...)
}

func (c *recordingCharacterClient) UpdateName(ctx context.Context, req *dnd5ev1alpha1.UpdateNameRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.UpdateNameResponse, error) {
	c.record(ctx, "UpdateName")
	return c.CharacterRPC.UpdateName(ctx, req, opts...)
}

func (c *recordingCharacterClient) UpdateRace(ctx context.Context, req *dnd5ev1alpha1.UpdateRaceRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.UpdateRaceResponse, error) {
	c.record(ctx, "UpdateRace")
	return c.CharacterRPC.UpdateRace(ctx, req, opts...)
}

func (c *recordingCharacterClient) UpdateClass(ctx context.Context, req *dnd5ev1alpha1.UpdateClassRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.UpdateClassResponse, error) {
	c.record(ctx, "UpdateClass")
	return c.CharacterRPC.UpdateClass(ctx, req, opts...)
}

func (c *recordingCharacterClient) UpdateBackground(ctx context.Context, req *dnd5ev1alpha1.UpdateBackgroundRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.UpdateBackgroundResponse, error) {
	c.record(ctx, "UpdateBackground")
	return c.CharacterRPC.UpdateBackground(ctx, req, opts...)
}

func (c *recordingCharacterClient) UpdateAbilityScores(ctx context.Context, req *dnd5ev1alpha1.UpdateAbilityScoresRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.UpdateAbilityScoresResponse, error) {
	c.record(ctx, "UpdateAbilityScores")
	return c.CharacterRPC.UpdateAbilityScores(ctx, req, opts...)
}

func (c *recordingCharacterClient) GetDraft(ctx context.Context, req *dnd5ev1alpha1.GetDraftRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.GetDraftResponse, error) {
	c.record(ctx, "GetDraft")
	return c.CharacterRPC.GetDraft(ctx, req, opts...)
}

func (c *recordingCharacterClient) FinalizeDraft(ctx context.Context, req *dnd5ev1alpha1.FinalizeDraftRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.FinalizeDraftResponse, error) {
	c.record(ctx, "FinalizeDraft")
	return c.CharacterRPC.FinalizeDraft(ctx, req, opts...)
}

func (c *recordingCharacterClient) GetCharacter(ctx context.Context, req *dnd5ev1alpha1.GetCharacterRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.GetCharacterResponse, error) {
	c.record(ctx, "GetCharacter")
	return c.CharacterRPC.GetCharacter(ctx, req, opts...)
}

func (c *recordingCharacterClient) EquipItem(ctx context.Context, req *dnd5ev1alpha1.EquipItemRequest, opts ...grpc.CallOption) (*dnd5ev1alpha1.EquipItemResponse, error) {
	c.record(ctx, "EquipItem")
	return c.CharacterRPC.EquipItem(ctx, req, opts...)
}

func (s *SandboxSeedSuite) assertPartyOrder(campaignID, hostIdentity, hostCharacterID, joinIdentity, joinCharacterID string) {
	s.T().Helper()

	created, err := s.server.LobbyClient.CreateLobby(s.authCtx(hostIdentity), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId:  campaignID,
		CharacterId: hostCharacterID,
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), hostIdentity, created.GetHostPlayerId())

	joined, err := s.server.LobbyClient.JoinLobby(s.authCtx(joinIdentity), &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef:     created.GetJoinRef(),
		CharacterId: joinCharacterID,
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), created.GetLobbyId(), joined.GetLobbyId())
	require.Len(s.T(), joined.GetMembers(), 2)

	characterByPlayer := make(map[string]string, len(joined.GetMembers()))
	for _, member := range joined.GetMembers() {
		characterByPlayer[member.GetPlayerId()] = member.GetCharacterId()
	}
	require.Equal(s.T(), hostCharacterID, characterByPlayer[hostIdentity])
	require.Equal(s.T(), joinCharacterID, characterByPlayer[joinIdentity])
}
