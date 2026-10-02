// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package characterintegration

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// These tests run the REAL produced seams end to end: the harness composes the
// production auth + role-access interceptors, the real Redis character/draft/
// dice repositories, and the real toolkit session SDK for advancement. The
// same Dev player selects world A or world B through the measured
// x-rpg-guild-id selector, proving isolation on ONE server and ONE Redis
// (rpg-project#522 S2 / #518 local simulation contract).

// finalizeDwarfFighter creates and finalizes a level-1 dwarf fighter in the
// world the context selects, returning the new character id.
func (s *CharacterCreationSuite) finalizeDwarfFighter(ctx context.Context) string {
	draftID := s.completeDwarfFighterDraft(ctx)
	resp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(resp.GetCharacter().GetId())
	return resp.GetCharacter().GetId()
}

func (s *CharacterCreationSuite) requireNotFound(err error) {
	s.T().Helper()
	s.Require().Error(err)
	s.Equal(codes.NotFound, status.Code(err), "foreign/missing private resources must read as NotFound")
}

// requireRepoNotFound asserts the repository's own not-found, which is an
// apierr rather than a gRPC status until a handler converts it.
func (s *CharacterCreationSuite) requireRepoNotFound(err error) {
	s.T().Helper()
	s.Require().Error(err)
	s.True(apierr.IsNotFound(err), "foreign/missing records must read as NotFound: %v", err)
}

func (s *CharacterCreationSuite) requireCode(err error, want codes.Code) {
	s.T().Helper()
	s.Require().Error(err)
	s.Equal(want, status.Code(err))
}

// TestWorldIsolation_SamePlayerSeparateCharactersAndDrafts is done-when 1: one
// account has independent characters and drafts in A and B, and finalizing in
// one world never touches the other.
func (s *CharacterCreationSuite) TestWorldIsolation_SamePlayerSeparateCharactersAndDrafts() {
	const player = "world-matrix-player"
	ctxA := s.authCtxInWorld(player, harness.DevWorldA)
	ctxB := s.authCtxInWorld(player, harness.DevWorldB)

	charA := s.finalizeDwarfFighter(ctxA)
	charB := s.finalizeDwarfFighter(ctxB)
	s.Require().NotEqual(charA, charB)

	listA, err := s.server.CharacterClient.ListCharacters(ctxA, &dnd5ev1alpha1.ListCharactersRequest{})
	s.Require().NoError(err)
	s.Require().Len(listA.GetCharacters(), 1)
	s.Equal(charA, listA.GetCharacters()[0].GetId())

	listB, err := s.server.CharacterClient.ListCharacters(ctxB, &dnd5ev1alpha1.ListCharactersRequest{})
	s.Require().NoError(err)
	s.Require().Len(listB.GetCharacters(), 1)
	s.Equal(charB, listB.GetCharacters()[0].GetId())

	// The stored envelopes carry their own world and are not visible across it.
	storedA, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charA})
	s.Require().NoError(err)
	s.Equal(harness.DevWorldA, storedA.Character.WorldID)
	s.Equal(player, storedA.Character.Data.PlayerID)

	storedB, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldB, ID: charB})
	s.Require().NoError(err)
	s.Equal(harness.DevWorldB, storedB.Character.WorldID)

	_, err = s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldB, ID: charA})
	s.requireRepoNotFound(err)
}

// TestWorldIsolation_FinalizePreservesWorld: a finalized character is owned by
// the world its draft was finalized in.
func (s *CharacterCreationSuite) TestWorldIsolation_FinalizePreservesWorld() {
	const player = "finalize-world-player"
	ctxB := s.authCtxInWorld(player, harness.DevWorldB)

	charID := s.finalizeDwarfFighter(ctxB)

	stored, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldB, ID: charID})
	s.Require().NoError(err)
	s.Equal(harness.DevWorldB, stored.Character.WorldID)

	_, err = s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.requireRepoNotFound(err)
}

// TestWorldIsolation_ForeignWorldAndPlayerRefusedOnEveryImplementedPath is the
// core S2 matrix: same account A/B plus a second player; every implemented
// private direct-ID path refuses a foreign world and a wrong player with
// indistinguishable NotFound, and writes nothing, while the owner still
// succeeds.
func (s *CharacterCreationSuite) TestWorldIsolation_ForeignWorldAndPlayerRefusedOnEveryImplementedPath() {
	const owner = "matrix-owner"
	const secondPlayer = "matrix-second"
	ctxOwnerA := s.authCtxInWorld(owner, harness.DevWorldA)
	ctxSamePlayerB := s.authCtxInWorld(owner, harness.DevWorldB)
	ctxSecondPlayerA := s.authCtxInWorld(secondPlayer, harness.DevWorldA)

	charID := s.finalizeDwarfFighter(ctxOwnerA)

	before, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.Require().NoError(err)

	foreignWorld := []struct {
		name string
		call func() error
	}{
		{"v1 GetCharacter", func() error {
			_, callErr := s.server.CharacterClient.GetCharacter(ctxSamePlayerB, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: charID})
			return callErr
		}},
		{"v1 EquipItem", func() error {
			_, callErr := s.server.CharacterClient.EquipItem(ctxSamePlayerB, &dnd5ev1alpha1.EquipItemRequest{
				CharacterId: charID, ItemId: "longsword", Slot: dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
			})
			return callErr
		}},
		{"v1 DeleteCharacter", func() error {
			_, callErr := s.server.CharacterClient.DeleteCharacter(ctxSamePlayerB, &dnd5ev1alpha1.DeleteCharacterRequest{CharacterId: charID})
			return callErr
		}},
		{"v1 GetCharacterInventory", func() error {
			_, callErr := s.server.CharacterClient.GetCharacterInventory(ctxSamePlayerB, &dnd5ev1alpha1.GetCharacterInventoryRequest{CharacterId: charID})
			return callErr
		}},
		{"v2 GetCharacterData", func() error {
			_, callErr := s.server.CharacterClientV2.GetCharacterData(ctxSamePlayerB, &characterpb.GetCharacterDataRequest{CharacterId: charID})
			return callErr
		}},
		{"v2 EquipItem", func() error {
			_, callErr := s.server.CharacterClientV2.EquipItem(ctxSamePlayerB, &characterpb.EquipItemRequest{
				CharacterId: charID, Item: &encounterv2pb.Ref{Id: "longsword"}, SlotKey: "main_hand",
			})
			return callErr
		}},
	}
	for _, tc := range foreignWorld {
		s.Run("foreign world "+tc.name, func() {
			s.requireNotFound(tc.call())
		})
	}

	wrongPlayer := []struct {
		name string
		call func() error
	}{
		{"v1 GetCharacter", func() error {
			_, callErr := s.server.CharacterClient.GetCharacter(ctxSecondPlayerA, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: charID})
			return callErr
		}},
		{"v1 EquipItem", func() error {
			_, callErr := s.server.CharacterClient.EquipItem(ctxSecondPlayerA, &dnd5ev1alpha1.EquipItemRequest{
				CharacterId: charID, ItemId: "longsword", Slot: dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
			})
			return callErr
		}},
		{"v1 DeleteCharacter", func() error {
			_, callErr := s.server.CharacterClient.DeleteCharacter(ctxSecondPlayerA, &dnd5ev1alpha1.DeleteCharacterRequest{CharacterId: charID})
			return callErr
		}},
		{"v2 GetCharacterData", func() error {
			_, callErr := s.server.CharacterClientV2.GetCharacterData(ctxSecondPlayerA, &characterpb.GetCharacterDataRequest{CharacterId: charID})
			return callErr
		}},
		{"v2 UnequipItem", func() error {
			_, callErr := s.server.CharacterClientV2.UnequipItem(ctxSecondPlayerA, &characterpb.UnequipItemRequest{
				CharacterId: charID, SlotKey: "main_hand",
			})
			return callErr
		}},
	}
	for _, tc := range wrongPlayer {
		s.Run("wrong player "+tc.name, func() {
			s.requireNotFound(tc.call())
		})
	}

	// A foreign/wrong-owner refusal is not a write: the stored record's bytes
	// and version are unchanged.
	after, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.Require().NoError(err)
	s.Equal(before.Character, after.Character)

	// The owner still succeeds in their own world.
	read, err := s.server.CharacterClient.GetCharacter(ctxOwnerA, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: charID})
	s.Require().NoError(err)
	s.Equal(charID, read.GetCharacter().GetId())

	equipped, err := s.server.CharacterClient.EquipItem(ctxOwnerA, &dnd5ev1alpha1.EquipItemRequest{
		CharacterId: charID, ItemId: "longsword", Slot: dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
	})
	s.Require().NoError(err)
	s.Equal("longsword", equipped.GetCharacter().GetEquipmentSlots().GetMainHand().GetItemId())

	// And the symmetric positive: v2 equip in the owner's own world.
	ownerItem := firstInventoryItemID(s.T(), read.GetCharacter())
	s.Require().NotEmpty(ownerItem)
	_, err = s.server.CharacterClientV2.EquipItem(ctxOwnerA, &characterpb.EquipItemRequest{
		CharacterId: charID, Item: &encounterv2pb.Ref{Id: ownerItem}, SlotKey: "off_hand",
	})
	// The off-hand item may not be equippable (e.g. a two-handed weapon or a
	// non-shield); a rule refusal is fine here -- what is asserted is that it
	// is NOT a NotFound, i.e. the ownership gate let the owner through.
	if err != nil {
		s.NotEqual(codes.NotFound, status.Code(err))
	}
}

// TestWorldIsolation_ForeignRollAssignmentRejected: creation rolls are world
// and player scoped, and A's roll ids cannot be assigned in B.
func (s *CharacterCreationSuite) TestWorldIsolation_ForeignRollAssignmentRejected() {
	const player = "roll-matrix-player"
	ctxA := s.authCtxInWorld(player, harness.DevWorldA)
	ctxB := s.authCtxInWorld(player, harness.DevWorldB)

	// A creation-eligible draft in each world.
	draftA := s.beginDraft(ctxA)
	draftB := s.beginDraft(ctxB)

	// Roll ability scores in A under the authenticated player.
	rollSession, err := s.server.DiceClient.GetRollSession(ctxA, &apiv1alpha1.GetRollSessionRequest{
		EntityId: player, Context: "ability_scores",
	})
	s.Require().NoError(err)
	s.Require().Len(rollSession.GetRolls(), 6)
	rollIDs := make([]string, 0, len(rollSession.GetRolls()))
	for _, roll := range rollSession.GetRolls() {
		rollIDs = append(rollIDs, roll.GetRollId())
	}

	assignments := &dnd5ev1alpha1.RollAssignments{
		StrengthRollId:     rollIDs[0],
		DexterityRollId:    rollIDs[1],
		ConstitutionRollId: rollIDs[2],
		IntelligenceRollId: rollIDs[3],
		WisdomRollId:       rollIDs[4],
		CharismaRollId:     rollIDs[5],
	}

	// In the SAME world, the owner's roll ids assign.
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctxA, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId:     draftA,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{RollAssignments: assignments},
	})
	s.Require().NoError(err)

	// In world B the same player has its OWN roll session (the standard array
	// auto-create), so the world scoping is proven by the ids themselves: A's
	// roll ids are absent from B's session and refused, never silently
	// accepted. The draft in B is bound to that same world/player.
	_, err = s.server.DiceClient.GetRollSession(ctxB, &apiv1alpha1.GetRollSessionRequest{
		EntityId: player, Context: "ability_scores",
	})
	s.Require().NoError(err)

	_, err = s.server.CharacterClient.UpdateAbilityScores(ctxB, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId:     draftB,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{RollAssignments: assignments},
	})
	s.requireCode(err, codes.InvalidArgument)

	// The creation path binds the dice entity to the authenticated player: a
	// client naming another entity for ability_scores is refused.
	_, err = s.server.DiceClient.GetRollSession(ctxA, &apiv1alpha1.GetRollSessionRequest{
		EntityId: "somebody-else", Context: "ability_scores",
	})
	s.requireCode(err, codes.PermissionDenied)
}

// TestWorldIsolation_LevelUpPreservesOwnershipAndRoundTrip proves the real SDK
// advancement save keeps the stored world/player and round-trips the owned
// record, and that the same id is invisible in the other world.
func (s *CharacterCreationSuite) TestWorldIsolation_LevelUpPreservesOwnershipAndRoundTrip() {
	const player = "levelup-matrix-player"
	ctxA := s.authCtxInWorld(player, harness.DevWorldA)
	ctxB := s.authCtxInWorld(player, harness.DevWorldB)

	charID := s.finalizeDwarfFighter(ctxA)

	// Put the character one level-up from advancing, exactly as the sandbox
	// seeder does: experience has no RPC and is written through the repository.
	stored, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.Require().NoError(err)
	stored.Character.Data.Experience = 300
	_, err = s.server.CharacterRepo.Update(context.Background(), characterrepo.UpdateInput{Character: stored.Character})
	s.Require().NoError(err)

	next, err := s.server.CharacterClient.GetNextLevel(ctxA, &dnd5ev1alpha1.GetNextLevelRequest{CharacterId: charID})
	s.Require().NoError(err)
	s.Equal(int32(2), next.GetLevel())

	leveled, err := s.server.CharacterClient.LevelUp(ctxA, &dnd5ev1alpha1.LevelUpRequest{
		CharacterId:    charID,
		HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
	})
	s.Require().NoError(err)
	s.Require().NotNil(leveled.GetCharacter())
	s.Equal(int32(2), leveled.GetCharacter().GetLevel())

	// The SDK's save preserved ownership and the world, and the record still
	// carries its experience and inventory.
	after, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.Require().NoError(err)
	s.Equal(harness.DevWorldA, after.Character.WorldID)
	s.Equal(player, after.Character.Data.PlayerID)
	s.Equal(2, after.Character.Data.Level)
	s.Equal(300, after.Character.Data.Experience)
	s.NotEmpty(after.Character.Data.Inventory)

	// The same id is not visible from the other world.
	_, err = s.server.CharacterClient.GetCharacter(ctxB, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: charID})
	s.requireNotFound(err)
}

// beginDraft creates an empty draft in the world the context selects.
func (s *CharacterCreationSuite) beginDraft(ctx context.Context) string {
	created, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	s.Require().NotEmpty(created.GetDraft().GetId())
	return created.GetDraft().GetId()
}

func firstInventoryItemID(t *testing.T, char *dnd5ev1alpha1.Character) string {
	t.Helper()
	for _, item := range char.GetInventory() {
		if item.GetItemId() != "" {
			return item.GetItemId()
		}
	}
	return ""
}
