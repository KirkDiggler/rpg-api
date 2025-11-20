package encounter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	encountermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/mock"
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
					OriginalDiceRolls: []int{6},
					FinalDiceRolls:    []int{6},
					Rerolls:           []encounter.RerollEvent{},
					FlatBonus:         0,
					DamageType:        "slashing",
					IsCritical:        false,
				},
				{
					Source:            "ability",
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
	s.Assert().Equal("weapon", weaponComp.Source)
	s.Assert().Equal([]int32{6}, weaponComp.OriginalDiceRolls)
	s.Assert().Equal([]int32{6}, weaponComp.FinalDiceRolls)
	s.Assert().Empty(weaponComp.Rerolls)
	s.Assert().Equal(int32(0), weaponComp.FlatBonus)
	s.Assert().Equal("slashing", weaponComp.DamageType)
	s.Assert().False(weaponComp.IsCritical)

	// Verify ability component
	abilityComp := resp.Result.DamageBreakdown.Components[1]
	s.Assert().Equal("ability", abilityComp.Source)
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
	s.Assert().Equal("weapon", weaponComp.Source)
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
	// Arrange - Mock service to return encounter ID
	s.mockService.EXPECT().
		CreateDungeon(gomock.Any(), &encounter.CreateDungeonInput{
			PlayerID: "", // Phase 2: No player tracking yet
		}).
		Return(&encounter.CreateDungeonOutput{
			EncounterID: "enc-123",
		}, nil)

	// Act
	resp, err := s.handler.DungeonStart(context.Background(), &dnd5ev1alpha1.DungeonStartRequest{
		CharacterIds: []string{"char-1", "char-2"}, // Proto has character_ids
	})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().Equal("enc-123", resp.EncounterId)
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
	s.mockService.EXPECT().
		CreateDungeon(gomock.Any(), &encounter.CreateDungeonInput{
			PlayerID: "", // Phase 2: No player tracking
		}).
		Return(&encounter.CreateDungeonOutput{
			EncounterID: "enc-456",
		}, nil)

	// Act
	resp, err := s.handler.DungeonStart(context.Background(), &dnd5ev1alpha1.DungeonStartRequest{})

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().Equal("enc-456", resp.EncounterId)
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

// TestMoveCharacter_Unimplemented tests that MoveCharacter returns Unimplemented
func (s *HandlerTestSuite) TestMoveCharacter_Unimplemented() {
	req := &dnd5ev1alpha1.MoveCharacterRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
		Path: []*apiv1alpha1.Position{
			{X: 5, Y: 5},
		},
	}

	resp, err := s.handler.MoveCharacter(context.Background(), req)

	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Unimplemented, st.Code())
}

// TestEndTurn_Unimplemented tests that EndTurn returns Unimplemented
func (s *HandlerTestSuite) TestEndTurn_Unimplemented() {
	req := &dnd5ev1alpha1.EndTurnRequest{
		EncounterId: "enc-1",
		EntityId:    "char-1",
	}

	resp, err := s.handler.EndTurn(context.Background(), req)

	s.Require().Error(err)
	s.Assert().Nil(resp)

	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Assert().Equal(codes.Unimplemented, st.Code())
}
