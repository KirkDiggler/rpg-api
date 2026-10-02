// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package characterintegration

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
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

// privatePath is ONE implemented private, direct-ID RPC plus the single request
// envelope sent in every context. The same envelope is sent by the owner in the
// owning world, by the same player in another world, and by a different player
// in the owning world -- so a refusal can only come from the owned-record gate,
// never from envelope validation.
type privatePath struct {
	name   string
	invoke func(ctx context.Context) error
}

// dwarfRaceChoices and fighterClassChoices mirror the envelopes
// completeDwarfFighterDraft uses, kept local so a positive re-application is a
// valid submission rather than a partial one.
func dwarfRaceChoices() []*dnd5ev1alpha1.ChoiceData {
	return []*dnd5ev1alpha1.ChoiceData{{
		Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
		Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
		ChoiceId: "dwarf-tools",
		Selection: &dnd5ev1alpha1.ChoiceData_Tools{Tools: &dnd5ev1alpha1.ToolSelection{
			Tools: []dnd5ev1alpha1.Tool{dnd5ev1alpha1.Tool_TOOL_SMITH_TOOLS},
		}},
	}}
}

func fighterClassChoices() []*dnd5ev1alpha1.ChoiceData {
	return []*dnd5ev1alpha1.ChoiceData{
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			Selection: &dnd5ev1alpha1.ChoiceData_Skills{Skills: &dnd5ev1alpha1.SkillSelection{
				Skills: []dnd5ev1alpha1.Skill{dnd5ev1alpha1.Skill_SKILL_ATHLETICS, dnd5ev1alpha1.Skill_SKILL_PERCEPTION},
			}},
		},
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{
				Style: dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE,
			}},
		},
		{
			Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId:  "fighter-armor",
			OptionId:  "fighter-armor-a",
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}},
		},
		{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId: "fighter-weapons-primary",
			OptionId: "fighter-weapon-a",
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{
				Items: []*dnd5ev1alpha1.EquipmentSelectionItem{{
					Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{Weapon: dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD},
				}},
			}},
		},
		{
			Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId:  "fighter-weapons-secondary",
			OptionId:  "fighter-ranged-a",
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}},
		},
		{
			Category:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
			Source:    dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			ChoiceId:  "fighter-pack",
			OptionId:  "fighter-pack-a",
			Selection: &dnd5ev1alpha1.ChoiceData_Equipment{Equipment: &dnd5ev1alpha1.EquipmentSelection{}},
		},
	}
}

// TestWorldIsolation_EveryImplementedPrivatePath is the complete S2 direct-ID
// matrix. One inventory carries every implemented private character/draft RPC
// with a valid envelope, and each row is run in three contexts:
//
//   - owner in the owning world      -> success
//   - same player, foreign world B   -> indistinguishable NotFound
//   - different player, owning world -> indistinguishable NotFound
//
// Draft rows run first and finalize last; character rows run second and delete
// last, so the owner-positive pass is a real, ordered RPC sequence. Refusals
// must write nothing: the stored character's decoded state AND version are
// unchanged, the draft is unchanged, and the world-scoped player index still
// resolves only to the owned draft.
//
// Unimplemented private RPCs are deliberately NOT rows here; they are listed
// and asserted in TestWorldIsolation_UnimplementedRPCsStayUnimplemented.
func (s *CharacterCreationSuite) TestWorldIsolation_EveryImplementedPrivatePath() {
	const owner = "matrix-owner"
	const secondPlayer = "matrix-second"
	ctxOwnerA := s.authCtxInWorld(owner, harness.DevWorldA)
	ctxOwnerB := s.authCtxInWorld(owner, harness.DevWorldB)
	ctxSecondPlayerA := s.authCtxInWorld(secondPlayer, harness.DevWorldA)

	// The character is created first: creating another draft for the same
	// (world, player) replaces the player's single draft, so the draft-under-
	// test must be the most recent one.
	charID := s.finalizeDwarfFighter(ctxOwnerA)
	draftID := s.completeDwarfFighterDraft(ctxOwnerA)

	// Experience for the advancement rows; written through the repository, as
	// the sandbox seeder does (no RPC grants experience).
	seeded, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.Require().NoError(err)
	seeded.Character.Data.Experience = 300
	_, err = s.server.CharacterRepo.Update(context.Background(), characterrepo.UpdateInput{Character: seeded.Character})
	s.Require().NoError(err)

	// A creation-roll session in A so the roll-assignment envelope is valid.
	rollSession, err := s.server.DiceClient.GetRollSession(ctxOwnerA, &apiv1alpha1.GetRollSessionRequest{
		EntityId: owner, Context: "ability_scores",
	})
	s.Require().NoError(err)
	s.Require().Len(rollSession.GetRolls(), 6)
	rollAssignments := &dnd5ev1alpha1.RollAssignments{
		StrengthRollId:     rollSession.GetRolls()[0].GetRollId(),
		DexterityRollId:    rollSession.GetRolls()[1].GetRollId(),
		ConstitutionRollId: rollSession.GetRolls()[2].GetRollId(),
		IntelligenceRollId: rollSession.GetRolls()[3].GetRollId(),
		WisdomRollId:       rollSession.GetRolls()[4].GetRollId(),
		CharismaRollId:     rollSession.GetRolls()[5].GetRollId(),
	}

	draftRepo := s.newDraftRepository()
	beforeChar, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.Require().NoError(err)
	beforeDraft, err := draftRepo.Get(context.Background(), characterdraft.GetInput{WorldID: harness.DevWorldA, ID: draftID})
	s.Require().NoError(err)

	paths := []privatePath{
		// ---- draft direct-ID paths ----
		{"v1 GetDraft", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{DraftId: draftID})
			return callErr
		}},
		{"v1 UpdateName", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{DraftId: draftID, Name: "Matrix Hero"})
			return callErr
		}},
		{"v1 UpdateRace", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
				DraftId: draftID, Race: dnd5ev1alpha1.Race_RACE_DWARF, RaceChoices: dwarfRaceChoices(),
			})
			return callErr
		}},
		{"v1 UpdateClass", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
				DraftId: draftID, Class: dnd5ev1alpha1.Class_CLASS_FIGHTER, ClassChoices: fighterClassChoices(),
			})
			return callErr
		}},
		{"v1 UpdateBackground", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
				DraftId: draftID, Background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER, BackgroundChoices: soldierBackgroundChoices(),
			})
			return callErr
		}},
		{"v1 UpdateAbilityScores manual", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
				DraftId: draftID,
				ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{AbilityScores: &dnd5ev1alpha1.AbilityScores{
					Strength: 15, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 12, Charisma: 8,
				}},
			})
			return callErr
		}},
		{"v1 UpdateAbilityScores rolls", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
				DraftId:     draftID,
				ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{RollAssignments: rollAssignments},
			})
			return callErr
		}},
		{"v1 UpdateAppearance", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UpdateAppearance(ctx, &dnd5ev1alpha1.UpdateAppearanceRequest{
				DraftId: draftID, Appearance: hairTestAppearance(0.33),
			})
			return callErr
		}},
		{"v1 FinalizeDraft", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{DraftId: draftID})
			return callErr
		}},

		// ---- character direct-ID paths, v1 then v2 ----
		{"v1 GetCharacter", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.GetCharacter(ctx, &dnd5ev1alpha1.GetCharacterRequest{CharacterId: charID})
			return callErr
		}},
		{"v1 GetCharacterInventory", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.GetCharacterInventory(ctx, &dnd5ev1alpha1.GetCharacterInventoryRequest{CharacterId: charID})
			return callErr
		}},
		{"v1 EquipItem", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.EquipItem(ctx, &dnd5ev1alpha1.EquipItemRequest{
				CharacterId: charID, ItemId: "longsword", Slot: dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
			})
			return callErr
		}},
		{"v1 UnequipItem", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UnequipItem(ctx, &dnd5ev1alpha1.UnequipItemRequest{
				CharacterId: charID, Slot: dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
			})
			return callErr
		}},
		{"v2 GetCharacterData", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClientV2.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: charID})
			return callErr
		}},
		{"v2 EquipItem", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClientV2.EquipItem(ctx, &characterpb.EquipItemRequest{
				CharacterId: charID, Item: &encounterv2pb.Ref{Id: "longsword"}, SlotKey: "main_hand",
			})
			return callErr
		}},
		{"v2 UnequipItem", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClientV2.UnequipItem(ctx, &characterpb.UnequipItemRequest{
				CharacterId: charID, SlotKey: "main_hand",
			})
			return callErr
		}},
		{"v1 GetNextLevel", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.GetNextLevel(ctx, &dnd5ev1alpha1.GetNextLevelRequest{CharacterId: charID})
			return callErr
		}},
		{"v1 LevelUp", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.LevelUp(ctx, &dnd5ev1alpha1.LevelUpRequest{
				CharacterId: charID, HitPointMethod: dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE,
			})
			return callErr
		}},
		{"v1 DeleteCharacter", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.DeleteCharacter(ctx, &dnd5ev1alpha1.DeleteCharacterRequest{CharacterId: charID})
			return callErr
		}},
	}

	// Every implemented private direct-ID path refuses BOTH a foreign world
	// (same player, world B) and a wrong player (second player, world A) with
	// the SAME NotFound, using the same valid envelope the owner uses below.
	for _, path := range paths {
		s.Run("foreign world "+path.name, func() {
			s.requireNotFound(path.invoke(ctxOwnerB))
		})
		s.Run("wrong player "+path.name, func() {
			s.requireNotFound(path.invoke(ctxSecondPlayerA))
		})
	}

	// A refused foreign/wrong-player call must write nothing. The character's
	// decoded state AND its repository Version are unchanged, the draft is
	// unchanged, and the world-scoped player index still resolves only to the
	// owned draft.
	afterChar, err := s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.Require().NoError(err)
	s.Equal(beforeChar.Character, afterChar.Character)
	s.Equal(beforeChar.Version, afterChar.Version, "a refused foreign/wrong-player call must not bump the stored version")

	afterDraft, err := draftRepo.Get(context.Background(), characterdraft.GetInput{WorldID: harness.DevWorldA, ID: draftID})
	s.Require().NoError(err)
	s.Equal(beforeDraft.Draft, afterDraft.Draft)

	indexed, err := draftRepo.GetByPlayerID(context.Background(), characterdraft.GetByPlayerIDInput{WorldID: harness.DevWorldA, PlayerID: owner})
	s.Require().NoError(err)
	s.Equal(draftID, indexed.Draft.Data.ID)
	_, err = draftRepo.GetByPlayerID(context.Background(), characterdraft.GetByPlayerIDInput{WorldID: harness.DevWorldA, PlayerID: secondPlayer})
	s.requireRepoNotFound(err)
	_, err = draftRepo.GetByPlayerID(context.Background(), characterdraft.GetByPlayerIDInput{WorldID: harness.DevWorldB, PlayerID: owner})
	s.requireRepoNotFound(err)

	// The owner's own envelope succeeds for every row, in table order, proving
	// each refusal above was the ownership gate rather than a bad request.
	for _, path := range paths {
		s.Run("owner "+path.name, func() {
			s.Require().NoError(path.invoke(ctxOwnerA))
		})
	}

	// Owner finalization removed the owned draft; owner level-up left the
	// character advanced, and the final row deleted the owned character. The
	// character's own advancement round trip is asserted in
	// TestWorldIsolation_LevelUpPreservesOwnershipAndRoundTrip.
	_, err = draftRepo.Get(context.Background(), characterdraft.GetInput{WorldID: harness.DevWorldA, ID: draftID})
	s.requireRepoNotFound(err)
	_, err = s.server.CharacterRepo.Get(context.Background(), characterrepo.GetInput{WorldID: harness.DevWorldA, ID: charID})
	s.requireRepoNotFound(err)
}

// TestWorldIsolation_UnimplementedRPCsStayUnimplemented pins that the private
// RPCs this slice did not implement remain unimplemented. Each is called in the
// owner's own world with a non-empty envelope: an implemented resource would
// succeed or return the private NotFound for a foreign one, so requiring a
// non-OK, non-NotFound refusal is how "left unimplemented" is proved rather than
// assumed.
//
// Unimplemented at this head (deliberately not rows in the implemented matrix):
//
//	v1 DeleteDraft, UpdateSkills, ValidateDraft, GetDraftPreview,
//	GetRaceDetails, GetBackgroundDetails, GetFeature, RollAbilityScores,
//	AddToInventory, RemoveFromInventory.
func (s *CharacterCreationSuite) TestWorldIsolation_UnimplementedRPCsStayUnimplemented() {
	const owner = "matrix-unimplemented"
	ctxOwnerA := s.authCtxInWorld(owner, harness.DevWorldA)
	ctxOwnerB := s.authCtxInWorld(owner, harness.DevWorldB)

	unimplemented := []privatePath{
		{"v1 DeleteDraft", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.DeleteDraft(ctx, &dnd5ev1alpha1.DeleteDraftRequest{DraftId: "unused-draft"})
			return callErr
		}},
		{"v1 UpdateSkills", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.UpdateSkills(ctx, &dnd5ev1alpha1.UpdateSkillsRequest{DraftId: "unused-draft"})
			return callErr
		}},
		{"v1 ValidateDraft", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.ValidateDraft(ctx, &dnd5ev1alpha1.ValidateDraftRequest{DraftId: "unused-draft"})
			return callErr
		}},
		{"v1 GetDraftPreview", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.GetDraftPreview(ctx, &dnd5ev1alpha1.GetDraftPreviewRequest{DraftId: "unused-draft"})
			return callErr
		}},
		{"v1 GetRaceDetails", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.GetRaceDetails(ctx, &dnd5ev1alpha1.GetRaceDetailsRequest{RaceId: "human"})
			return callErr
		}},
		{"v1 GetBackgroundDetails", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.GetBackgroundDetails(ctx, &dnd5ev1alpha1.GetBackgroundDetailsRequest{BackgroundId: "soldier"})
			return callErr
		}},
		{"v1 GetFeature", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.GetFeature(ctx, &dnd5ev1alpha1.GetFeatureRequest{FeatureId: "unused-feature"})
			return callErr
		}},
		{"v1 RollAbilityScores", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.RollAbilityScores(ctx, &dnd5ev1alpha1.RollAbilityScoresRequest{DraftId: "unused-draft"})
			return callErr
		}},
		{"v1 AddToInventory", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.AddToInventory(ctx, &dnd5ev1alpha1.AddToInventoryRequest{CharacterId: "unused-char"})
			return callErr
		}},
		{"v1 RemoveFromInventory", func(ctx context.Context) error {
			_, callErr := s.server.CharacterClient.RemoveFromInventory(ctx, &dnd5ev1alpha1.RemoveFromInventoryRequest{
				CharacterId: "unused-char", ItemId: "longsword",
			})
			return callErr
		}},
	}

	for _, path := range unimplemented {
		s.Run(path.name, func() {
			for name, ctx := range map[string]context.Context{"owner world": ctxOwnerA, "foreign world": ctxOwnerB} {
				err := path.invoke(ctx)
				s.Require().Error(err, "%s must still refuse", name)
				s.NotEqual(codes.OK, status.Code(err), "%s must not silently succeed", name)
				s.NotEqual(codes.NotFound, status.Code(err),
					"%s must not be an ownership-gated resource (private NotFound)", name)
			}
		})
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
