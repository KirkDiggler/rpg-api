package encounter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	encountermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// HandlerTestSuite tests the encounter handler
type HandlerTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *encountermock.MockService
	handler     *Handler
}

// SetupTest runs before each test
func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = encountermock.NewMockService(s.ctrl)

	handler, err := New(&HandlerConfig{
		EncounterService: s.mockService,
	})
	s.Require().NoError(err)
	s.handler = handler
}

// TearDownTest runs after each test
func (s *HandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestHandlerSuite runs the test suite
func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}

// TestNew_Success tests successful handler creation
func (s *HandlerTestSuite) TestNew_Success() {
	handler, err := New(&HandlerConfig{
		EncounterService: s.mockService,
	})

	s.Require().NoError(err)
	s.Assert().NotNil(handler)
	s.Assert().Equal(s.mockService, handler.encounterService)
}

// TestNew_MissingService tests handler creation with missing service
func (s *HandlerTestSuite) TestNew_MissingService() {
	handler, err := New(&HandlerConfig{
		EncounterService: nil,
	})

	s.Require().Error(err)
	s.Assert().Nil(handler)
	s.Assert().Contains(err.Error(), "encounter service is required")
}

// TestAttack_Success tests successful attack with hit
func (s *HandlerTestSuite) TestAttack_Success() {
	// Arrange - Mock service to return successful attack
	expectedResult := &encounter.AttackResult{
		Hit:         true,
		AttackRoll:  15,
		AttackBonus: 5,
		TotalAttack: 20,
		TotalDamage: 12,
		DamageType:  "slashing",
		Critical:    false,
		Breakdown: &encounter.DamageBreakdown{
			Components: []encounter.DamageComponent{
				{
					Source:            "weapon",
					SourceRef:         refs.Weapons.Longsword(),
					OriginalDiceRolls: []int{6},
					FinalDiceRolls:    []int{6},
					Rerolls:           []encounter.RerollEvent{},
					FlatBonus:         0,
					DamageType:        "slashing",
					IsCritical:        false,
				},
				{
					Source:            "ability",
					SourceRef:         refs.Abilities.Strength(),
					OriginalDiceRolls: []int{},
					FinalDiceRolls:    []int{},
					Rerolls:           []encounter.RerollEvent{},
					FlatBonus:         6,
					DamageType:        "slashing",
					IsCritical:        false,
				},
			},
			AbilityUsed: "str",
			TotalDamage: 12,
		},
	}

	s.mockService.EXPECT().
		ResolveAttack(gomock.Any(), &encounter.ResolveAttackInput{
			EncounterID: "enc-1",
			AttackerID:  "char-1",
			TargetID:    "goblin-1",
			WeaponID:    "",
			AttackHand:  combat.AttackHandMain, // Default for unspecified
		}).
		Return(&encounter.ResolveAttackOutput{
			Result:      expectedResult,
			MonsterHP:   3,
			MonsterDead: false,
		}, nil)

	// Act
	resp, err := s.handler.Attack(context.Background(), &dnd5ev1alpha1.AttackRequest{
		EncounterId: "enc-1",
		AttackerId:  "char-1",
		TargetId:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
	s.Require().NotNil(resp.Result)
	s.Assert().True(resp.Result.Hit)
	s.Assert().Equal(int32(15), resp.Result.AttackRoll)
	s.Assert().Equal(int32(20), resp.Result.AttackTotal)
	s.Assert().Equal(int32(12), resp.Result.Damage)
	s.Assert().Equal("slashing", resp.Result.DamageType)
	s.Assert().False(resp.Result.Critical)

	// Verify breakdown is properly converted
	s.Require().NotNil(resp.Result.DamageBreakdown, "DamageBreakdown should be present")
	s.Assert().Equal("str", resp.Result.DamageBreakdown.AbilityUsed)
	s.Assert().Equal(int32(12), resp.Result.DamageBreakdown.TotalDamage)
	s.Require().Len(resp.Result.DamageBreakdown.Components, 2, "Should have 2 damage components")

	// Verify weapon component
	weaponComp := resp.Result.DamageBreakdown.Components[0]
	s.Require().NotNil(weaponComp.SourceRef, "SourceRef should be present for weapon")
	s.Assert().Equal(dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD, weaponComp.SourceRef.GetWeapon())
	s.Assert().Equal([]int32{6}, weaponComp.OriginalDiceRolls)
	s.Assert().Equal([]int32{6}, weaponComp.FinalDiceRolls)
	s.Assert().Empty(weaponComp.Rerolls)
	s.Assert().Equal(int32(0), weaponComp.FlatBonus)
	s.Assert().Equal("slashing", weaponComp.DamageType)
	s.Assert().False(weaponComp.IsCritical)

	// Verify ability component
	abilityComp := resp.Result.DamageBreakdown.Components[1]
	s.Require().NotNil(abilityComp.SourceRef, "SourceRef should be present for ability")
	s.Assert().Equal(dnd5ev1alpha1.Ability_ABILITY_STRENGTH, abilityComp.SourceRef.GetAbility())
	s.Assert().Empty(abilityComp.OriginalDiceRolls)
	s.Assert().Empty(abilityComp.FinalDiceRolls)
	s.Assert().Empty(abilityComp.Rerolls)
	s.Assert().Equal(int32(6), abilityComp.FlatBonus)
	s.Assert().Equal("slashing", abilityComp.DamageType)
	s.Assert().False(abilityComp.IsCritical)
}

// TestAttack_CriticalHit tests critical hit scenario
func (s *HandlerTestSuite) TestAttack_CriticalHit() {
	// Arrange - Test critical hit scenario with rerolls (e.g., Great Weapon Fighting)
	expectedResult := &encounter.AttackResult{
		Hit:             true,
		AttackRoll:      20,
		AttackBonus:     3,
		TotalAttack:     23,
		TotalDamage:     24, // Doubled damage
		DamageType:      "piercing",
		Critical:        true,
		IsNaturalTwenty: true,
		Breakdown: &encounter.DamageBreakdown{
			Components: []encounter.DamageComponent{
				{
					Source:            "weapon",
					SourceRef:         refs.Weapons.Rapier(),
					OriginalDiceRolls: []int{1, 8}, // Rolled 1 and 8 on 2d12 (crit)
					FinalDiceRolls:    []int{7, 8}, // Rerolled the 1 to 7
					Rerolls: []encounter.RerollEvent{
						{
							DieIndex: 0,
							Before:   1,
							After:    7,
							Reason:   "great_weapon_fighting",
						},
					},
					FlatBonus:  0,
					DamageType: "piercing",
					IsCritical: true,
				},
				{
					Source:            "ability",
					SourceRef:         refs.Abilities.Strength(),
					OriginalDiceRolls: []int{},
					FinalDiceRolls:    []int{},
					Rerolls:           []encounter.RerollEvent{},
					FlatBonus:         9,
					DamageType:        "piercing",
					IsCritical:        false,
				},
			},
			AbilityUsed: "str",
			TotalDamage: 24,
		},
	}

	s.mockService.EXPECT().
		ResolveAttack(gomock.Any(), gomock.Any()).
		Return(&encounter.ResolveAttackOutput{
			Result:      expectedResult,
			MonsterHP:   0,
			MonsterDead: true,
		}, nil)

	// Act
	resp, err := s.handler.Attack(context.Background(), &dnd5ev1alpha1.AttackRequest{
		EncounterId: "enc-1",
		AttackerId:  "char-1",
		TargetId:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
	s.Require().NotNil(resp.Result)
	s.Assert().True(resp.Result.Critical)
	s.Assert().Equal(int32(20), resp.Result.AttackRoll)
	s.Assert().Equal(int32(24), resp.Result.Damage)

	// Verify breakdown with rerolls
	s.Require().NotNil(resp.Result.DamageBreakdown)
	s.Require().Len(resp.Result.DamageBreakdown.Components, 2)

	// Verify weapon component with reroll
	weaponComp := resp.Result.DamageBreakdown.Components[0]
	s.Require().NotNil(weaponComp.SourceRef, "SourceRef should be present for weapon")
	s.Assert().Equal(dnd5ev1alpha1.Weapon_WEAPON_RAPIER, weaponComp.SourceRef.GetWeapon())
	s.Assert().Equal([]int32{1, 8}, weaponComp.OriginalDiceRolls)
	s.Assert().Equal([]int32{7, 8}, weaponComp.FinalDiceRolls)
	s.Assert().True(weaponComp.IsCritical)
	s.Require().Len(weaponComp.Rerolls, 1)

	// Verify reroll event
	reroll := weaponComp.Rerolls[0]
	s.Assert().Equal(int32(0), reroll.DieIndex)
	s.Assert().Equal(int32(1), reroll.Before)
	s.Assert().Equal(int32(7), reroll.After)
	s.Assert().Equal("great_weapon_fighting", reroll.Reason)
}

// TestAttack_Miss tests miss scenario
func (s *HandlerTestSuite) TestAttack_Miss() {
	// Arrange - Test miss scenario
	expectedResult := &encounter.AttackResult{
		Hit:         false,
		AttackRoll:  5,
		AttackBonus: 3,
		TotalAttack: 8,
		TotalDamage: 0,
		DamageType:  "bludgeoning",
		Critical:    false,
	}

	s.mockService.EXPECT().
		ResolveAttack(gomock.Any(), gomock.Any()).
		Return(&encounter.ResolveAttackOutput{
			Result:      expectedResult,
			MonsterHP:   15,
			MonsterDead: false,
		}, nil)

	// Act
	resp, err := s.handler.Attack(context.Background(), &dnd5ev1alpha1.AttackRequest{
		EncounterId: "enc-1",
		AttackerId:  "char-1",
		TargetId:    "goblin-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
	s.Require().NotNil(resp.Result)
	s.Assert().False(resp.Result.Hit)
	s.Assert().Equal(int32(0), resp.Result.Damage)
	s.Assert().False(resp.Result.Critical)
}

// TestAttack_WithWeapon tests attack with specific weapon
func (s *HandlerTestSuite) TestAttack_WithWeapon() {
	// Arrange
	expectedResult := &encounter.AttackResult{
		Hit:         true,
		AttackRoll:  14,
		AttackBonus: 5,
		TotalAttack: 19,
		TotalDamage: 10,
		DamageType:  "slashing",
		Critical:    false,
	}

	s.mockService.EXPECT().
		ResolveAttack(gomock.Any(), &encounter.ResolveAttackInput{
			EncounterID: "enc-1",
			AttackerID:  "char-1",
			TargetID:    "goblin-1",
			WeaponID:    "longsword",
			AttackHand:  combat.AttackHandMain, // Default for unspecified
		}).
		Return(&encounter.ResolveAttackOutput{
			Result:      expectedResult,
			MonsterHP:   5,
			MonsterDead: false,
		}, nil)

	// Act
	resp, err := s.handler.Attack(context.Background(), &dnd5ev1alpha1.AttackRequest{
		EncounterId: "enc-1",
		AttackerId:  "char-1",
		TargetId:    "goblin-1",
		WeaponId:    "longsword",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
	s.Assert().True(resp.Result.Hit)
	s.Assert().Equal(int32(10), resp.Result.Damage)
}

// TestAttack_MissingEncounterId tests validation error for missing encounter_id
func (s *HandlerTestSuite) TestAttack_MissingEncounterId() {
	// Act
	resp, err := s.handler.Attack(context.Background(), &dnd5ev1alpha1.AttackRequest{
		AttackerId: "char-1",
		TargetId:   "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	// Check for InvalidArgument error code
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

// TestAttack_MissingAttackerId tests validation error for missing attacker_id
func (s *HandlerTestSuite) TestAttack_MissingAttackerId() {
	// Act
	resp, err := s.handler.Attack(context.Background(), &dnd5ev1alpha1.AttackRequest{
		EncounterId: "enc-1",
		TargetId:    "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	// Check for InvalidArgument error code
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "attacker_id is required")
}

// TestAttack_MissingTargetId tests validation error for missing target_id
func (s *HandlerTestSuite) TestAttack_MissingTargetId() {
	// Act
	resp, err := s.handler.Attack(context.Background(), &dnd5ev1alpha1.AttackRequest{
		EncounterId: "enc-1",
		AttackerId:  "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	// Check for InvalidArgument error code
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "target_id is required")
}

// TestAttack_ServiceError tests when service returns error
func (s *HandlerTestSuite) TestAttack_ServiceError() {
	// Arrange - Test when service returns error
	s.mockService.EXPECT().
		ResolveAttack(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "character not found"))

	// Act
	resp, err := s.handler.Attack(context.Background(), &dnd5ev1alpha1.AttackRequest{
		EncounterId: "enc-1",
		AttackerId:  "char-1",
		TargetId:    "goblin-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	// Check for Internal error code (handler wraps service errors)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Internal, st.Code())
}

// TestDungeonStart_Success tests successful dungeon creation
func (s *HandlerTestSuite) TestDungeonStart_Success() {
	// Arrange - Mock service to return encounter ID and room data
	expectedRoom := &spatial.RoomData{
		ID:       "enc-123-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeSquare,
		Entities: make(map[string]spatial.EntityPlacement),
	}

	s.mockService.EXPECT().
		CreateDungeon(gomock.Any(), &encounter.CreateDungeonInput{
			CharacterIDs: []string{"char-1", "char-2"},
		}).
		Return(&encounter.CreateDungeonOutput{
			EncounterID: "enc-123",
			Room:        expectedRoom,
		}, nil)

	// Act
	resp, err := s.handler.DungeonStart(context.Background(), &dnd5ev1alpha1.DungeonStartRequest{
		CharacterIds: []string{"char-1", "char-2"},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().Equal("enc-123", resp.EncounterId)

	// Verify room data is present in response
	s.Require().NotNil(resp.Room)
	s.Assert().Equal("enc-123-room", resp.Room.Id)
	s.Assert().Equal("dungeon", resp.Room.Type)
	s.Assert().Equal(int32(20), resp.Room.Width)
	s.Assert().Equal(int32(20), resp.Room.Height)
	s.Assert().Equal(apiv1alpha1.GridType_GRID_TYPE_SQUARE, resp.Room.GridType)
	s.Assert().NotNil(resp.Room.Entities)
}

// TestDungeonStart_ServiceError tests when service returns error
func (s *HandlerTestSuite) TestDungeonStart_ServiceError() {
	// Arrange - Test when service returns error
	s.mockService.EXPECT().
		CreateDungeon(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.Internal, "failed to create encounter"))

	// Act
	resp, err := s.handler.DungeonStart(context.Background(), &dnd5ev1alpha1.DungeonStartRequest{
		CharacterIds: []string{"char-1"},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	// Check for Internal error code
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Internal, st.Code())
}

// TestDungeonStart_EmptyRequest tests with no character_ids
func (s *HandlerTestSuite) TestDungeonStart_EmptyRequest() {
	// Arrange - Test with empty request
	expectedRoom := &spatial.RoomData{
		ID:       "enc-456-room",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeSquare,
		Entities: make(map[string]spatial.EntityPlacement),
	}

	s.mockService.EXPECT().
		CreateDungeon(gomock.Any(), gomock.Any()). // Accept any input for empty request test
		Return(&encounter.CreateDungeonOutput{
			EncounterID: "enc-456",
			Room:        expectedRoom,
		}, nil)

	// Act
	resp, err := s.handler.DungeonStart(context.Background(), &dnd5ev1alpha1.DungeonStartRequest{})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().Equal("enc-456", resp.EncounterId)
	s.Assert().NotNil(resp.Room)
	s.Assert().Equal("enc-456-room", resp.Room.Id)
}

// TestGetCombatState_Unimplemented tests that GetCombatState returns Unimplemented
func (s *HandlerTestSuite) TestGetCombatState_Unimplemented() {
	req := &dnd5ev1alpha1.GetCombatStateRequest{
		EncounterId: "enc-1",
	}

	resp, err := s.handler.GetCombatState(context.Background(), req)

	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Unimplemented, st.Code())
}

// TestMoveCharacter_Success tests successful movement
// Note: The handler converts cube coordinates (from client) to offset coordinates (for service)
// The test uses hex grid, so input coords (5, -8, 3) cube -> (5, 5) offset after conversion
func (s *HandlerTestSuite) TestMoveCharacter_Success() {
	// Arrange - use cube coordinates (x + y + z = 0)
	cubeX := 5.0
	cubeY := -10.0 // y = -x - z
	cubeZ := 5.0

	expectedRoom := &spatial.RoomData{
		ID:         "enc-1-room",
		Type:       "dungeon",
		Width:      20,
		Height:     20,
		GridType:   spatial.GridTypeHex,
		HexFlatTop: false,
		CubeEntities: map[string]spatial.EntityCubePlacement{
			"char-1": {
				EntityID:       "char-1",
				EntityType:     "character",
				CubePosition:   spatial.CubeCoordinate{X: 5, Y: -10, Z: 5},
				Size:           1,
				BlocksMovement: true,
			},
		},
	}

	// Handler passes cube coordinates directly to service
	s.mockService.EXPECT().
		MoveCharacter(gomock.Any(), &encounter.MoveCharacterInput{
			EncounterID: "enc-1",
			EntityID:    "char-1",
			TargetPosition: &encounter.Position{
				X: cubeX,
				Y: cubeY,
				Z: cubeZ,
			},
		}).
		Return(&encounter.MoveCharacterOutput{
			Success: true,
			FinalPosition: &encounter.Position{
				X: cubeX,
				Y: cubeY,
				Z: cubeZ,
			},
			MovementRemaining: 30,
			StopReason:        "completed",
			UpdatedRoom:       expectedRoom,
		}, nil)

	// Act - send cube coordinates (what client would send)
	resp, err := s.handler.MoveCharacter(context.Background(), &dnd5ev1alpha1.MoveCharacterRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		Path: []*apiv1alpha1.Position{
			{X: cubeX, Y: cubeY, Z: cubeZ},
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
	s.Assert().Equal(int32(30), resp.MovementRemaining)
	s.Require().NotNil(resp.UpdatedRoom)
	s.Assert().Equal("enc-1-room", resp.UpdatedRoom.Id)
	s.Assert().Equal(int32(20), resp.UpdatedRoom.Width)
	s.Assert().Equal(int32(20), resp.UpdatedRoom.Height)
	s.Assert().Len(resp.UpdatedRoom.Entities, 1)
}

// TestMoveCharacter_MissingEncounterId tests validation for missing encounter_id
func (s *HandlerTestSuite) TestMoveCharacter_MissingEncounterId() {
	// Act
	resp, err := s.handler.MoveCharacter(context.Background(), &dnd5ev1alpha1.MoveCharacterRequest{
		EntityId: "char-1",
		Path: []*apiv1alpha1.Position{
			{X: 5, Y: 5},
		},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

// TestMoveCharacter_MissingEntityId tests validation for missing entity_id
func (s *HandlerTestSuite) TestMoveCharacter_MissingEntityId() {
	// Act
	resp, err := s.handler.MoveCharacter(context.Background(), &dnd5ev1alpha1.MoveCharacterRequest{
		EncounterId: "enc-1",
		Path: []*apiv1alpha1.Position{
			{X: 5, Y: 5},
		},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "entity_id is required")
}

// TestMoveCharacter_MissingPath tests validation for missing path
func (s *HandlerTestSuite) TestMoveCharacter_MissingPath() {
	// Act
	resp, err := s.handler.MoveCharacter(context.Background(), &dnd5ev1alpha1.MoveCharacterRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		Path:        []*apiv1alpha1.Position{},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "path is required")
}

// TestMoveCharacter_ServiceError tests handling of service errors
func (s *HandlerTestSuite) TestMoveCharacter_ServiceError() {
	// Arrange
	s.mockService.EXPECT().
		MoveCharacter(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Act
	resp, err := s.handler.MoveCharacter(context.Background(), &dnd5ev1alpha1.MoveCharacterRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		Path: []*apiv1alpha1.Position{
			{X: 5, Y: 5},
		},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Internal, st.Code())
	s.Assert().Contains(st.Message(), "database error")
}

// TestMoveCharacter_OutOfBounds tests movement to invalid position
// The handler passes cube coordinates directly to the service
func (s *HandlerTestSuite) TestMoveCharacter_OutOfBounds() {
	// Use out-of-bounds cube coordinates (x + y + z = 0)
	cubeX := 100.0
	cubeY := -200.0 // y = -x - z
	cubeZ := 100.0

	// Arrange
	s.mockService.EXPECT().
		MoveCharacter(gomock.Any(), &encounter.MoveCharacterInput{
			EncounterID: "enc-1",
			EntityID:    "char-1",
			TargetPosition: &encounter.Position{
				X: cubeX,
				Y: cubeY,
				Z: cubeZ,
			},
		}).
		Return(&encounter.MoveCharacterOutput{
			Success: false,
			FinalPosition: &encounter.Position{
				X: cubeX,
				Y: cubeY,
				Z: cubeZ,
			},
			MovementRemaining: 0,
			StopReason:        "out_of_bounds",
			UpdatedRoom:       nil,
		}, nil)

	// Act - send cube coordinates (what client would send)
	resp, err := s.handler.MoveCharacter(context.Background(), &dnd5ev1alpha1.MoveCharacterRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		Path: []*apiv1alpha1.Position{
			{X: cubeX, Y: cubeY, Z: cubeZ},
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().False(resp.Success)
	s.Assert().Equal(int32(0), resp.MovementRemaining)
}

// TestEndTurn_Success tests successful turn ending
func (s *HandlerTestSuite) TestEndTurn_Success() {
	// Arrange - Note: entity_id is no longer required, server determines active entity
	s.mockService.EXPECT().
		EndTurn(gomock.Any(), &encounter.EndTurnInput{
			EncounterID: "enc-1",
			PlayerID:    "test-player",
		}).
		Return(&encounter.EndTurnOutput{
			CombatState: &encounter.CombatState{
				EncounterID: "enc-1",
				Round:       1,
				TurnOrder: []encounter.InitiativeEntry{
					{EntityID: "char-1", EntityType: "character", InitiativeTotal: 20},
					{EntityID: "char-2", EntityType: "character", InitiativeTotal: 15},
				},
				ActiveIndex:       1,
				MovementRemaining: 30,
				CombatStarted:     true,
				CombatEnded:       false,
			},
			TurnChange: &encounter.TurnChangeEvent{
				PreviousEntityID: "char-1",
				NextEntityID:     "char-2",
				Round:            1,
				NewRound:         false,
			},
		}, nil)

	// Act - Client only sends encounter_id, player ID comes from auth context
	ctx := auth.WithPlayerID(context.Background(), "test-player")
	resp, err := s.handler.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)

	// Verify combat state
	s.Require().NotNil(resp.CombatState)
	s.Assert().Equal("enc-1", resp.CombatState.EncounterId)
	s.Assert().Equal(int32(1), resp.CombatState.Round)
	s.Assert().Equal(int32(1), resp.CombatState.ActiveIndex)
	s.Assert().True(resp.CombatState.CombatStarted)
	s.Assert().False(resp.CombatState.CombatEnded)

	// Verify turn change event
	s.Require().NotNil(resp.TurnChange)
	s.Assert().Equal("char-1", resp.TurnChange.PreviousEntityId)
	s.Assert().Equal("char-2", resp.TurnChange.NextEntityId)
	s.Assert().Equal(int32(1), resp.TurnChange.Round)
	s.Assert().False(resp.TurnChange.NewRound)
}

// TestEndTurn_NewRound tests turn ending that advances to a new round
func (s *HandlerTestSuite) TestEndTurn_NewRound() {
	// Arrange
	s.mockService.EXPECT().
		EndTurn(gomock.Any(), &encounter.EndTurnInput{
			EncounterID: "enc-1",
			PlayerID:    "test-player",
		}).
		Return(&encounter.EndTurnOutput{
			CombatState: &encounter.CombatState{
				EncounterID: "enc-1",
				Round:       2,
				TurnOrder: []encounter.InitiativeEntry{
					{EntityID: "char-1", EntityType: "character", InitiativeTotal: 20},
					{EntityID: "char-2", EntityType: "character", InitiativeTotal: 15},
				},
				ActiveIndex:       0,
				MovementRemaining: 30,
				CombatStarted:     true,
				CombatEnded:       false,
			},
			TurnChange: &encounter.TurnChangeEvent{
				PreviousEntityID: "char-2",
				NextEntityID:     "char-1",
				Round:            2,
				NewRound:         true,
			},
		}, nil)

	// Act - Client only sends encounter_id, player ID comes from auth context
	ctx := auth.WithPlayerID(context.Background(), "test-player")
	resp, err := s.handler.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
	s.Assert().Equal(int32(2), resp.CombatState.Round)
	s.Assert().True(resp.TurnChange.NewRound)
}

// TestEndTurn_MissingEncounterId tests validation for missing encounter_id
func (s *HandlerTestSuite) TestEndTurn_MissingEncounterId() {
	// Act - need auth context even though encounter_id is missing
	ctx := auth.WithPlayerID(context.Background(), "test-player")
	resp, err := s.handler.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EntityId: "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

// TestEndTurn_Unauthenticated tests that EndTurn requires authentication
func (s *HandlerTestSuite) TestEndTurn_Unauthenticated() {
	// Act - no auth context
	resp, err := s.handler.EndTurn(context.Background(), &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Unauthenticated, st.Code())
	s.Assert().Contains(st.Message(), "player not authenticated")
}

// TestEndTurn_ServiceError tests handling of service errors
func (s *HandlerTestSuite) TestEndTurn_ServiceError() {
	// Arrange
	s.mockService.EXPECT().
		EndTurn(gomock.Any(), &encounter.EndTurnInput{
			EncounterID: "enc-1",
			PlayerID:    "test-player",
		}).
		Return(nil, fmt.Errorf("database error"))

	// Act - Client only sends encounter_id, player ID comes from auth context
	ctx := auth.WithPlayerID(context.Background(), "test-player")
	resp, err := s.handler.EndTurn(ctx, &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: "enc-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Internal, st.Code())
	s.Assert().Contains(st.Message(), "database error")
}

// ============================================================================
// ActivateFeature Tests
// ============================================================================

// TestActivateFeature_Success tests successful feature activation
func (s *HandlerTestSuite) TestActivateFeature_Success() {
	// Arrange - All features route through ActivateFeature orchestrator
	s.mockService.EXPECT().
		ActivateFeature(gomock.Any(), &encounter.ActivateFeatureInput{
			EncounterID: "enc-1",
			CharacterID: "char-1",
			FeatureID:   refs.Features.Rage().ID,
		}).
		Return(&encounter.ActivateFeatureOutput{
			Success: true,
			Message: "Rage activated successfully",
		}, nil)

	// Act
	resp, err := s.handler.ActivateFeature(context.Background(), &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: "enc-1",
		CharacterId: "char-1",
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
	s.Assert().Contains(resp.Message, "activated")
}

// TestActivateFeature_FeatureNotFound tests when ability activation fails
func (s *HandlerTestSuite) TestActivateFeature_FeatureNotFound() {
	// Arrange
	s.mockService.EXPECT().
		ActivateFeature(gomock.Any(), &encounter.ActivateFeatureInput{
			EncounterID: "enc-1",
			CharacterID: "char-1",
			FeatureID:   refs.Features.Rage().ID,
		}).
		Return(&encounter.ActivateFeatureOutput{
			Success: false,
			Message: "feature 'rage' not found on character",
		}, nil)

	// Act
	resp, err := s.handler.ActivateFeature(context.Background(), &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: "enc-1",
		CharacterId: "char-1",
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})

	// Assert
	s.Require().NoError(err) // Not an error, just unsuccessful
	s.Require().NotNil(resp)
	s.Assert().False(resp.Success)
	s.Assert().Contains(resp.Message, "not found")
}

// TestActivateFeature_CannotActivate tests when feature cannot be activated
func (s *HandlerTestSuite) TestActivateFeature_CannotActivate() {
	// Arrange - e.g., already raging
	s.mockService.EXPECT().
		ActivateFeature(gomock.Any(), &encounter.ActivateFeatureInput{
			EncounterID: "enc-1",
			CharacterID: "char-1",
			FeatureID:   refs.Features.Rage().ID,
		}).
		Return(&encounter.ActivateFeatureOutput{
			Success: false,
			Message: "cannot activate rage: already raging",
		}, nil)

	// Act
	resp, err := s.handler.ActivateFeature(context.Background(), &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: "enc-1",
		CharacterId: "char-1",
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().False(resp.Success)
	s.Assert().Contains(resp.Message, "cannot activate")
}

// TestActivateFeature_MissingEncounterId tests validation for missing encounter_id
func (s *HandlerTestSuite) TestActivateFeature_MissingEncounterId() {
	// Act
	resp, err := s.handler.ActivateFeature(context.Background(), &dnd5ev1alpha1.ActivateFeatureRequest{
		CharacterId: "char-1",
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

// TestActivateFeature_MissingCharacterId tests validation for missing character_id
func (s *HandlerTestSuite) TestActivateFeature_MissingCharacterId() {
	// Act
	resp, err := s.handler.ActivateFeature(context.Background(), &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: "enc-1",
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "character_id is required")
}

// TestActivateFeature_MissingFeatureId tests validation for missing feature_id
func (s *HandlerTestSuite) TestActivateFeature_MissingFeatureId() {
	// Act
	resp, err := s.handler.ActivateFeature(context.Background(), &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: "enc-1",
		CharacterId: "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "feature_id is required")
}

// TestActivateFeature_ServiceError tests handling of service errors
func (s *HandlerTestSuite) TestActivateFeature_ServiceError() {
	// Arrange
	s.mockService.EXPECT().
		ActivateFeature(gomock.Any(), &encounter.ActivateFeatureInput{
			EncounterID: "enc-1",
			CharacterID: "char-1",
			FeatureID:   refs.Features.Rage().ID,
		}).
		Return(nil, fmt.Errorf("database error"))

	// Act
	resp, err := s.handler.ActivateFeature(context.Background(), &dnd5ev1alpha1.ActivateFeatureRequest{
		EncounterId: "enc-1",
		CharacterId: "char-1",
		FeatureId:   dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Internal, st.Code())
	s.Assert().Contains(st.Message(), "database error")
}

// ============================================================================
// Lobby Methods Tests
// ============================================================================

// TestCreateEncounter_Success tests successful encounter creation
func (s *HandlerTestSuite) TestCreateEncounter_Success() {
	// Arrange
	ctx := auth.WithPlayerID(context.Background(), "test-player-1")
	s.mockService.EXPECT().
		CreateEncounter(gomock.Any(), &encounter.CreateEncounterInput{
			PlayerID:     "test-player-1",
			CharacterIDs: []string{"char-1"},
		}).
		Return(&encounter.CreateEncounterOutput{
			EncounterID: "enc-123",
			JoinCode:    "ABC123",
			Room:        nil,
		}, nil)

	// Act
	resp, err := s.handler.CreateEncounter(ctx, &dnd5ev1alpha1.CreateEncounterRequest{
		CharacterIds: []string{"char-1"},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().Equal("enc-123", resp.EncounterId)
	s.Assert().Equal("ABC123", resp.JoinCode)
}

// TestCreateEncounter_MissingCharacterIds tests validation
func (s *HandlerTestSuite) TestCreateEncounter_MissingCharacterIds() {
	// Act
	resp, err := s.handler.CreateEncounter(context.Background(), &dnd5ev1alpha1.CreateEncounterRequest{
		CharacterIds: []string{},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "character_ids is required")
}

// TestJoinEncounter_Success tests successful encounter joining
func (s *HandlerTestSuite) TestJoinEncounter_Success() {
	// Arrange
	ctx := auth.WithPlayerID(context.Background(), "test-player-2")
	s.mockService.EXPECT().
		JoinEncounter(gomock.Any(), &encounter.JoinEncounterInput{
			JoinCode:     "ABC123",
			PlayerID:     "test-player-2",
			CharacterIDs: []string{"char-2"},
		}).
		Return(&encounter.JoinEncounterOutput{
			EncounterID: "enc-123",
			Room:        nil,
			Party: []*encounter.PartyMember{
				{PlayerID: "player-1", CharacterID: "char-1", IsHost: true, IsReady: true, IsConnected: true},
				{PlayerID: "test-player-2", CharacterID: "char-2", IsHost: false, IsReady: false, IsConnected: true},
			},
			State: "waiting",
		}, nil)

	// Act
	resp, err := s.handler.JoinEncounter(ctx, &dnd5ev1alpha1.JoinEncounterRequest{
		JoinCode:     "ABC123",
		CharacterIds: []string{"char-2"},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().Equal("enc-123", resp.EncounterId)
	s.Assert().Len(resp.Party, 2)
	s.Assert().Equal(dnd5ev1alpha1.EncounterState_ENCOUNTER_STATE_WAITING, resp.State)
}

// TestJoinEncounter_MissingJoinCode tests validation
func (s *HandlerTestSuite) TestJoinEncounter_MissingJoinCode() {
	// Act
	resp, err := s.handler.JoinEncounter(context.Background(), &dnd5ev1alpha1.JoinEncounterRequest{
		JoinCode:     "",
		CharacterIds: []string{"char-2"},
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "join_code is required")
}

// TestSetReady_Success tests successful ready status change
func (s *HandlerTestSuite) TestSetReady_Success() {
	// Arrange
	s.mockService.EXPECT().
		SetReady(gomock.Any(), &encounter.SetReadyInput{
			EncounterID: "enc-1",
			PlayerID:    "player-1",
			IsReady:     true,
		}).
		Return(&encounter.SetReadyOutput{Success: true}, nil)

	// Act
	resp, err := s.handler.SetReady(context.Background(), &dnd5ev1alpha1.SetReadyRequest{
		EncounterId: "enc-1",
		PlayerId:    "player-1",
		IsReady:     true,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
}

// TestSetReady_MissingEncounterId tests validation
func (s *HandlerTestSuite) TestSetReady_MissingEncounterId() {
	// Act
	resp, err := s.handler.SetReady(context.Background(), &dnd5ev1alpha1.SetReadyRequest{
		EncounterId: "",
		PlayerId:    "player-1",
		IsReady:     true,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

// TestStartCombat_Success tests successful combat start
func (s *HandlerTestSuite) TestStartCombat_Success() {
	// Arrange
	ctx := auth.WithPlayerID(context.Background(), "test-host-player")
	s.mockService.EXPECT().
		StartCombat(gomock.Any(), &encounter.StartCombatInput{
			EncounterID: "enc-1",
			PlayerID:    "test-host-player",
		}).
		Return(&encounter.StartCombatOutput{
			CombatState: &encounter.CombatState{
				EncounterID:       "enc-1",
				Round:             1,
				TurnOrder:         []encounter.InitiativeEntry{},
				ActiveIndex:       0,
				MovementRemaining: 30,
				CombatStarted:     true,
				CombatEnded:       false,
			},
			MonsterTurns: nil,
		}, nil)

	// Act
	resp, err := s.handler.StartCombat(ctx, &dnd5ev1alpha1.StartCombatRequest{
		EncounterId: "enc-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.CombatState)
	s.Assert().Equal(int32(1), resp.CombatState.Round)
}

// TestStartCombat_MissingEncounterId tests validation
func (s *HandlerTestSuite) TestStartCombat_MissingEncounterId() {
	// Act
	resp, err := s.handler.StartCombat(context.Background(), &dnd5ev1alpha1.StartCombatRequest{
		EncounterId: "",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

// TestLeaveEncounter_Success tests successful player leaving
func (s *HandlerTestSuite) TestLeaveEncounter_Success() {
	// Arrange
	s.mockService.EXPECT().
		LeaveEncounter(gomock.Any(), &encounter.LeaveEncounterInput{
			EncounterID: "enc-1",
			PlayerID:    "player-1",
		}).
		Return(&encounter.LeaveEncounterOutput{
			Success:          true,
			EncounterDeleted: false,
		}, nil)

	// Act
	resp, err := s.handler.LeaveEncounter(context.Background(), &dnd5ev1alpha1.LeaveEncounterRequest{
		EncounterId: "enc-1",
		PlayerId:    "player-1",
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
}

// TestLeaveEncounter_MissingPlayerId tests validation
func (s *HandlerTestSuite) TestLeaveEncounter_MissingPlayerId() {
	// Act
	resp, err := s.handler.LeaveEncounter(context.Background(), &dnd5ev1alpha1.LeaveEncounterRequest{
		EncounterId: "enc-1",
		PlayerId:    "",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "player_id is required")
}

// ============================================================================
// GetEncounterState Tests
// ============================================================================

func (s *HandlerTestSuite) TestGetEncounterState_Success() {
	encounterID := "enc-123"
	playerID := "player-1"

	s.mockService.EXPECT().
		GetEncounterState(gomock.Any(), &encounter.GetEncounterStateInput{
			EncounterID: encounterID,
			PlayerID:    playerID,
		}).
		Return(&encounter.GetEncounterStateOutput{
			EncounterID: encounterID,
			State:       "waiting",
			JoinCode:    "ABC123",
			HostID:      playerID,
			LastEventID: "01JFABC123",
			Party: []*encounter.PartyMember{
				{
					PlayerID:    playerID,
					CharacterID: "char-1",
					IsHost:      true,
				},
			},
		}, nil)

	resp, err := s.handler.GetEncounterState(context.Background(), &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})

	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().Equal(encounterID, resp.EncounterId)
	s.Assert().Equal(dnd5ev1alpha1.EncounterState_ENCOUNTER_STATE_WAITING, resp.State)
	s.Assert().Equal("ABC123", resp.JoinCode)
	s.Assert().Equal(playerID, resp.HostId)
	s.Assert().Equal("01JFABC123", resp.LastEventId)
	s.Assert().Len(resp.Party, 1)
}

func (s *HandlerTestSuite) TestGetEncounterState_MissingEncounterId() {
	resp, err := s.handler.GetEncounterState(context.Background(), &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: "",
		PlayerId:    "player-1",
	})

	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

func (s *HandlerTestSuite) TestGetEncounterState_MissingPlayerId() {
	resp, err := s.handler.GetEncounterState(context.Background(), &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: "enc-123",
		PlayerId:    "",
	})

	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "player_id is required")
}

func (s *HandlerTestSuite) TestGetEncounterState_EncounterNotFound() {
	encounterID := "nonexistent"
	playerID := "player-1"

	s.mockService.EXPECT().
		GetEncounterState(gomock.Any(), &encounter.GetEncounterStateInput{
			EncounterID: encounterID,
			PlayerID:    playerID,
		}).
		Return(nil, encounter.ErrEncounterNotFound)

	resp, err := s.handler.GetEncounterState(context.Background(), &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})

	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.NotFound, st.Code())
}

func (s *HandlerTestSuite) TestGetEncounterState_PlayerNotInEncounter() {
	encounterID := "enc-123"
	playerID := "other-player"

	s.mockService.EXPECT().
		GetEncounterState(gomock.Any(), &encounter.GetEncounterStateInput{
			EncounterID: encounterID,
			PlayerID:    playerID,
		}).
		Return(nil, encounter.ErrPlayerNotInEncounter)

	resp, err := s.handler.GetEncounterState(context.Background(), &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})

	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.PermissionDenied, st.Code())
}

// =============================================================================
// convertToProtoEvent Tests - MovementCompleted with Walls
// =============================================================================

func (s *HandlerTestSuite) TestConvertToProtoEvent_MovementCompleted_IncludesWalls() {
	// Arrange: Create a MovementCompletedEvent with walls
	roomData := &spatial.RoomData{
		ID:       "room-1",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
	}

	walls := []dungeon.WallSegment{
		{
			ID:             "wall-1",
			Start:          dungeon.Position{X: 5, Y: -10, Z: 5},
			End:            dungeon.Position{X: 6, Y: -12, Z: 6},
			Type:           dungeon.WallTypeIndestructible,
			BlocksMovement: true,
		},
		{
			ID:             "wall-2",
			Start:          dungeon.Position{X: 10, Y: -20, Z: 10},
			End:            dungeon.Position{X: 12, Y: -24, Z: 12},
			Type:           dungeon.WallTypeDestructible,
			BlocksMovement: true,
		},
	}

	event := &entities.EncounterEvent{
		ID:        "evt-123",
		Type:      entities.EventTypeMovementCompleted,
		Timestamp: time.Now(),
		MovementCompleted: &entities.MovementCompletedEvent{
			EntityID:   "char-1",
			EntityType: "character",
			FinalPosition: &entities.Position{
				X: 5,
				Y: -10,
				Z: 5,
			},
			MovementRemaining: 15,
			StopReason:        "completed",
			UpdatedRoom:       roomData,
			Walls:             walls,
		},
	}

	// Act: Convert to proto
	protoEvent, err := s.handler.convertToProtoEvent(event)

	// Assert: No error and walls are present
	s.Require().NoError(err)
	s.Require().NotNil(protoEvent)

	// Get the MovementCompleted event
	movementCompleted := protoEvent.GetMovementCompleted()
	s.Require().NotNil(movementCompleted, "MovementCompleted should be set")

	// Verify entity ID is passed through
	s.Equal("char-1", movementCompleted.EntityId)

	// Verify path contains the final position
	s.Require().Len(movementCompleted.Path, 1)
	s.Equal(float64(5), movementCompleted.Path[0].X)
	s.Equal(float64(-10), movementCompleted.Path[0].Y)
	s.Equal(float64(5), movementCompleted.Path[0].Z)

	// UpdatedEntity and CombatState are nil until orchestrator populates them
	s.Nil(movementCompleted.UpdatedEntity, "UpdatedEntity should be nil until orchestrator wires it")
	s.Nil(movementCompleted.CombatState, "CombatState should be nil until orchestrator wires it")
}

func (s *HandlerTestSuite) TestConvertToProtoEvent_TurnEnded_IncludesWalls() {
	// Arrange: Create a TurnEndedEvent with walls
	roomData := &spatial.RoomData{
		ID:       "room-1",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
	}

	walls := []dungeon.WallSegment{
		{
			ID:             "wall-1",
			Start:          dungeon.Position{X: 5, Y: -10, Z: 5},
			End:            dungeon.Position{X: 6, Y: -12, Z: 6},
			Type:           dungeon.WallTypeIndestructible,
			BlocksMovement: true,
		},
	}

	event := &entities.EncounterEvent{
		ID:        "evt-123",
		Type:      entities.EventTypeTurnEnded,
		Timestamp: time.Now(),
		TurnEnded: &entities.TurnEndedEvent{
			PreviousEntityID: "char-1",
			NextEntityID:     "char-2",
			Round:            1,
			NewRound:         false,
			Room:             roomData,
			Walls:            walls,
		},
	}

	// Act: Convert to proto
	protoEvent, err := s.handler.convertToProtoEvent(event)

	// Assert: No error and walls are present
	s.Require().NoError(err)
	s.Require().NotNil(protoEvent)

	// Get the TurnEnded event
	turnEnded := protoEvent.GetTurnEnded()
	s.Require().NotNil(turnEnded, "TurnEnded should be set")

	// CombatState is nil because the internal event's CombatState field was nil (no legacy data set)
	// and the new CombatStateProto field is also nil. The handler falls back to legacy conversion
	// which returns nil for nil input.
	s.Nil(turnEnded.CombatState, "CombatState should be nil when no combat state data is set")

	// UpdatedEntities is nil until orchestrator populates it
	s.Nil(turnEnded.UpdatedEntities, "UpdatedEntities should be nil until orchestrator wires it")
}

func (s *HandlerTestSuite) TestConvertToProtoEvent_MonsterTurnCompleted_IncludesWalls() {
	// Arrange: Create a MonsterTurnCompletedEvent with walls
	roomData := &spatial.RoomData{
		ID:       "room-1",
		Type:     "dungeon",
		Width:    20,
		Height:   20,
		GridType: spatial.GridTypeHex,
	}

	walls := []dungeon.WallSegment{
		{
			ID:             "wall-1",
			Start:          dungeon.Position{X: 5, Y: -10, Z: 5},
			End:            dungeon.Position{X: 6, Y: -12, Z: 6},
			Type:           dungeon.WallTypeIndestructible,
			BlocksMovement: true,
		},
		{
			ID:             "wall-2",
			Start:          dungeon.Position{X: 10, Y: -20, Z: 10},
			End:            dungeon.Position{X: 12, Y: -24, Z: 12},
			Type:           dungeon.WallTypeDestructible,
			BlocksMovement: true,
		},
	}

	event := &entities.EncounterEvent{
		ID:        "evt-456",
		Type:      entities.EventTypeMonsterTurnCompleted,
		Timestamp: time.Now(),
		MonsterTurnCompleted: &entities.MonsterTurnCompletedEvent{
			MonsterID:   "monster-1",
			MonsterName: "Skeleton",
			Actions:     []entities.MonsterExecutedAction{},
			Movement:    []entities.Position{},
			Room:        roomData,
			Walls:       walls,
		},
	}

	// Act: Convert to proto
	protoEvent, err := s.handler.convertToProtoEvent(event)

	// Assert: No error and walls are present
	s.Require().NoError(err)
	s.Require().NotNil(protoEvent)

	// Get the MonsterTurnCompleted event
	monsterTurn := protoEvent.GetMonsterTurnCompleted()
	s.Require().NotNil(monsterTurn, "MonsterTurnCompleted should be set")

	// Verify monster turn result fields are passed through
	s.Require().NotNil(monsterTurn.MonsterTurn)
	s.Equal("monster-1", monsterTurn.MonsterTurn.MonsterId)
	s.Equal("Skeleton", monsterTurn.MonsterTurn.MonsterName)
	s.Empty(monsterTurn.MonsterTurn.Actions)
	s.Empty(monsterTurn.MonsterTurn.MovementPath)

	// UpdatedEntities and CombatState are nil until orchestrator populates them
	s.Nil(monsterTurn.UpdatedEntities, "UpdatedEntities should be nil until orchestrator wires it")
	s.Nil(monsterTurn.CombatState, "CombatState should be nil until orchestrator wires it")
}

// ============================================================================
// ActivateCombatAbility Tests
// ============================================================================

// TestActivateCombatAbility_Success tests successful ability activation (ATTACK)
func (s *HandlerTestSuite) TestActivateCombatAbility_Success() {
	// Arrange
	s.mockService.EXPECT().
		ActivateCombatAbility(gomock.Any(), &encounter.ActivateCombatAbilityInput{
			EncounterID: "enc-1",
			EntityID:    "char-1",
			AbilityID:   dnd5ev1alpha1.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
		}).
		Return(&encounter.ActivateCombatAbilityOutput{
			Success:         true,
			GrantedCapacity: "Granted 1 attack",
			ActionEconomy: &entities.ActionEconomyState{
				ActionsRemaining:      0, // Used by ATTACK
				BonusActionsRemaining: 1,
				ReactionsRemaining:    1,
				AttacksRemaining:      1, // Granted by ATTACK
			},
			CombatState: &entities.CombatState{
				EncounterID:       "enc-1",
				Round:             1,
				MovementRemaining: 30,
				CombatStarted:     true,
			},
			AvailableAbilities: []*entities.AvailableAbility{
				{AbilityID: "attack", Name: "Attack", CanUse: false, Reason: "no actions remaining"},
				{AbilityID: "dash", Name: "Dash", CanUse: false, Reason: "no actions remaining"},
			},
			AvailableActions: []*entities.AvailableAction{
				{ActionID: "strike", Name: "Strike", CanUse: true, Reason: ""},
				{ActionID: "move", Name: "Move", CanUse: true, Reason: ""},
			},
		}, nil)

	// Act
	resp, err := s.handler.ActivateCombatAbility(context.Background(), &dnd5ev1alpha1.ActivateCombatAbilityRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		AbilityId:   dnd5ev1alpha1.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)
	s.Assert().Equal("Granted 1 attack", resp.GrantedCapacity)

	// Verify ActionEconomy
	s.Require().NotNil(resp.ActionEconomy)
	s.Assert().False(resp.ActionEconomy.StandardActionAvailable) // Used
	s.Assert().True(resp.ActionEconomy.BonusActionAvailable)
	s.Assert().True(resp.ActionEconomy.ReactionAvailable)
	s.Assert().Equal(int32(1), resp.ActionEconomy.AttacksRemaining)
	s.Assert().Equal(int32(30), resp.ActionEconomy.MovementRemaining)

	// Verify AvailableAbilities (issue #404)
	s.Require().NotNil(resp.AvailableAbilities)
	s.Assert().Len(resp.AvailableAbilities, 2)
	s.Assert().Equal(dnd5ev1alpha1.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK, resp.AvailableAbilities[0].AbilityId)
	s.Assert().False(resp.AvailableAbilities[0].CanUse)

	// Verify AvailableActions (issue #404)
	s.Require().NotNil(resp.AvailableActions)
	s.Assert().Len(resp.AvailableActions, 2)
	s.Assert().Equal(dnd5ev1alpha1.ActionId_ACTION_ID_STRIKE, resp.AvailableActions[0].ActionId)
	s.Assert().True(resp.AvailableActions[0].CanUse)
}

// TestActivateCombatAbility_NoActionAvailable tests when no action is available
func (s *HandlerTestSuite) TestActivateCombatAbility_NoActionAvailable() {
	// Arrange
	s.mockService.EXPECT().
		ActivateCombatAbility(gomock.Any(), &encounter.ActivateCombatAbilityInput{
			EncounterID: "enc-1",
			EntityID:    "char-1",
			AbilityID:   dnd5ev1alpha1.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
		}).
		Return(&encounter.ActivateCombatAbilityOutput{
			Success: false,
			Error:   "no action available",
			ActionEconomy: &entities.ActionEconomyState{
				ActionsRemaining: 0,
			},
		}, nil)

	// Act
	resp, err := s.handler.ActivateCombatAbility(context.Background(), &dnd5ev1alpha1.ActivateCombatAbilityRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		AbilityId:   dnd5ev1alpha1.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert
	s.Require().NoError(err) // Business logic failure, not an error
	s.Require().NotNil(resp)
	s.Assert().False(resp.Success)
	s.Assert().Equal("no action available", resp.Error)
}

// TestActivateCombatAbility_MissingEncounterId tests validation
func (s *HandlerTestSuite) TestActivateCombatAbility_MissingEncounterId() {
	// Act
	resp, err := s.handler.ActivateCombatAbility(context.Background(), &dnd5ev1alpha1.ActivateCombatAbilityRequest{
		EntityId:  "char-1",
		AbilityId: dnd5ev1alpha1.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

// TestActivateCombatAbility_MissingEntityId tests validation
func (s *HandlerTestSuite) TestActivateCombatAbility_MissingEntityId() {
	// Act
	resp, err := s.handler.ActivateCombatAbility(context.Background(), &dnd5ev1alpha1.ActivateCombatAbilityRequest{
		EncounterId: "enc-1",
		AbilityId:   dnd5ev1alpha1.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "entity_id is required")
}

// TestActivateCombatAbility_MissingAbilityId tests validation
func (s *HandlerTestSuite) TestActivateCombatAbility_MissingAbilityId() {
	// Act
	resp, err := s.handler.ActivateCombatAbility(context.Background(), &dnd5ev1alpha1.ActivateCombatAbilityRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "ability_id is required")
}

// TestActivateCombatAbility_ServiceError tests handling of service errors
func (s *HandlerTestSuite) TestActivateCombatAbility_ServiceError() {
	// Arrange
	s.mockService.EXPECT().
		ActivateCombatAbility(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Act
	resp, err := s.handler.ActivateCombatAbility(context.Background(), &dnd5ev1alpha1.ActivateCombatAbilityRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		AbilityId:   dnd5ev1alpha1.CombatAbilityId_COMBAT_ABILITY_ID_ATTACK,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Internal, st.Code())
}

// ============================================================================
// ExecuteAction Tests
// ============================================================================

// TestExecuteAction_Strike_Success tests successful strike action
func (s *HandlerTestSuite) TestExecuteAction_Strike_Success() {
	// Arrange
	s.mockService.EXPECT().
		ExecuteAction(gomock.Any(), &encounter.ExecuteActionInput{
			EncounterID: "enc-1",
			EntityID:    "char-1",
			ActionID:    dnd5ev1alpha1.ActionId_ACTION_ID_STRIKE,
			TargetID:    "goblin-1",
			WeaponID:    "longsword",
		}).
		Return(&encounter.ExecuteActionOutput{
			Success: true,
			ActionEconomy: &entities.ActionEconomyState{
				ActionsRemaining: 0,
				AttacksRemaining: 0, // Consumed
			},
			AttackResult: &encounter.AttackResult{
				Hit:         true,
				AttackRoll:  15,
				TotalAttack: 20,
				TotalDamage: 10,
				DamageType:  "slashing",
			},
			CombatState: &entities.CombatState{
				EncounterID:       "enc-1",
				MovementRemaining: 30,
			},
			AvailableAbilities: []*entities.AvailableAbility{
				{AbilityID: "attack", Name: "Attack", CanUse: false, Reason: "no actions remaining"},
			},
			AvailableActions: []*entities.AvailableAction{
				{ActionID: "strike", Name: "Strike", CanUse: false, Reason: "no attacks remaining (activate Attack ability first)"},
				{ActionID: "move", Name: "Move", CanUse: true, Reason: ""},
			},
		}, nil)

	// Act
	resp, err := s.handler.ExecuteAction(context.Background(), &dnd5ev1alpha1.ExecuteActionRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		ActionId:    dnd5ev1alpha1.ActionId_ACTION_ID_STRIKE,
		Input: &dnd5ev1alpha1.ExecuteActionRequest_Strike{
			Strike: &dnd5ev1alpha1.StrikeInput{
				TargetId: "goblin-1",
				WeaponId: "longsword",
			},
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)

	// Verify strike result
	strikeResult := resp.GetStrikeResult()
	s.Require().NotNil(strikeResult)
	s.Assert().True(strikeResult.Hit)
	s.Assert().Equal(int32(10), strikeResult.Damage)

	// Verify action economy
	s.Require().NotNil(resp.ActionEconomy)
	s.Assert().Equal(int32(0), resp.ActionEconomy.AttacksRemaining)

	// Verify AvailableAbilities (issue #404)
	s.Require().NotNil(resp.AvailableAbilities)
	s.Assert().NotEmpty(resp.AvailableAbilities)

	// Verify AvailableActions (issue #404)
	s.Require().NotNil(resp.AvailableActions)
	s.Assert().NotEmpty(resp.AvailableActions)
}

// TestExecuteAction_Move_Success tests successful move action
func (s *HandlerTestSuite) TestExecuteAction_Move_Success() {
	// Arrange - use cube coordinates (x + y + z = 0)
	path := []encounter.Position{
		{X: 1, Y: -2, Z: 1},
		{X: 2, Y: -4, Z: 2},
	}

	s.mockService.EXPECT().
		ExecuteAction(gomock.Any(), &encounter.ExecuteActionInput{
			EncounterID: "enc-1",
			EntityID:    "char-1",
			ActionID:    dnd5ev1alpha1.ActionId_ACTION_ID_MOVE,
			Path:        path,
		}).
		Return(&encounter.ExecuteActionOutput{
			Success: true,
			ActionEconomy: &entities.ActionEconomyState{
				ActionsRemaining: 1,
			},
			MoveResult: &encounter.MoveResult{
				FinalPosition: &encounter.Position{X: 2, Y: -4, Z: 2},
				MovementUsed:  10,
				StopReason:    "completed",
			},
			CombatState: &entities.CombatState{
				EncounterID:       "enc-1",
				MovementRemaining: 20, // 30 - 10
			},
		}, nil)

	// Act
	resp, err := s.handler.ExecuteAction(context.Background(), &dnd5ev1alpha1.ExecuteActionRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		ActionId:    dnd5ev1alpha1.ActionId_ACTION_ID_MOVE,
		Input: &dnd5ev1alpha1.ExecuteActionRequest_Move{
			Move: &dnd5ev1alpha1.MoveInput{
				Path: []*apiv1alpha1.Position{
					{X: 1, Y: -2, Z: 1},
					{X: 2, Y: -4, Z: 2},
				},
			},
		},
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().True(resp.Success)

	// Verify move result
	moveResult := resp.GetMoveResult()
	s.Require().NotNil(moveResult)
	s.Assert().Equal(int32(10), moveResult.MovementUsed)
	s.Assert().Equal("completed", moveResult.StopReason)
	s.Require().NotNil(moveResult.FinalPosition)
	s.Assert().Equal(float64(2), moveResult.FinalPosition.X)

	// Verify action economy shows updated movement
	s.Require().NotNil(resp.ActionEconomy)
	s.Assert().Equal(int32(20), resp.ActionEconomy.MovementRemaining)
}

// TestExecuteAction_NoAttacksRemaining tests strike without attacks
func (s *HandlerTestSuite) TestExecuteAction_NoAttacksRemaining() {
	// Arrange
	s.mockService.EXPECT().
		ExecuteAction(gomock.Any(), gomock.Any()).
		Return(&encounter.ExecuteActionOutput{
			Success: false,
			Error:   "no attacks remaining",
			ActionEconomy: &entities.ActionEconomyState{
				AttacksRemaining: 0,
			},
		}, nil)

	// Act
	resp, err := s.handler.ExecuteAction(context.Background(), &dnd5ev1alpha1.ExecuteActionRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		ActionId:    dnd5ev1alpha1.ActionId_ACTION_ID_STRIKE,
		Input: &dnd5ev1alpha1.ExecuteActionRequest_Strike{
			Strike: &dnd5ev1alpha1.StrikeInput{
				TargetId: "goblin-1",
			},
		},
	})

	// Assert
	s.Require().NoError(err) // Business logic failure, not an error
	s.Require().NotNil(resp)
	s.Assert().False(resp.Success)
	s.Assert().Equal("no attacks remaining", resp.Error)
}

// TestExecuteAction_MissingEncounterId tests validation
func (s *HandlerTestSuite) TestExecuteAction_MissingEncounterId() {
	// Act
	resp, err := s.handler.ExecuteAction(context.Background(), &dnd5ev1alpha1.ExecuteActionRequest{
		EntityId: "char-1",
		ActionId: dnd5ev1alpha1.ActionId_ACTION_ID_STRIKE,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "encounter_id is required")
}

// TestExecuteAction_MissingEntityId tests validation
func (s *HandlerTestSuite) TestExecuteAction_MissingEntityId() {
	// Act
	resp, err := s.handler.ExecuteAction(context.Background(), &dnd5ev1alpha1.ExecuteActionRequest{
		EncounterId: "enc-1",
		ActionId:    dnd5ev1alpha1.ActionId_ACTION_ID_STRIKE,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "entity_id is required")
}

// TestExecuteAction_MissingActionId tests validation
func (s *HandlerTestSuite) TestExecuteAction_MissingActionId() {
	// Act
	resp, err := s.handler.ExecuteAction(context.Background(), &dnd5ev1alpha1.ExecuteActionRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.InvalidArgument, st.Code())
	s.Assert().Contains(st.Message(), "action_id is required")
}

// TestExecuteAction_ServiceError tests handling of service errors
func (s *HandlerTestSuite) TestExecuteAction_ServiceError() {
	// Arrange
	s.mockService.EXPECT().
		ExecuteAction(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	// Act
	resp, err := s.handler.ExecuteAction(context.Background(), &dnd5ev1alpha1.ExecuteActionRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		ActionId:    dnd5ev1alpha1.ActionId_ACTION_ID_STRIKE,
	})

	// Assert
	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Internal, st.Code())
}
