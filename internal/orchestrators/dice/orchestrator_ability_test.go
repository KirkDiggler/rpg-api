package dice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	dicemock "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session/mock"
)

func TestOrchestrator_RollDice_AbilityScores(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := dicemock.NewMockRepository(ctrl)
	idGen := idgen.NewUUID("roll")

	o, err := NewOrchestrator(&Config{
		DiceSessionRepo: mockRepo,
		IDGenerator:     idGen,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Test rolling 4d6 for ability scores - should drop lowest
	t.Run("4d6 ability scores drops lowest", func(t *testing.T) {
		input := &RollDiceInput{
			EntityID: "player-123",
			Context:  ContextAbilityScores,
			Notation: "4d6",
		}

		// Mock the repository to return NotFound (no existing session)
		mockRepo.EXPECT().
			Get(ctx, dicesession.GetInput{
				EntityID: "player-123",
				Context:  ContextAbilityScores,
			}).
			Return(nil, apierr.NotFound("session not found"))

		// Mock creating a new session
		mockRepo.EXPECT().
			Create(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input dicesession.CreateInput) (*dicesession.CreateOutput, error) {
				// Verify that we're storing the roll with dropped dice
				require.Equal(t, "player-123", input.EntityID)
				require.Equal(t, ContextAbilityScores, input.Context)
				require.Len(t, input.Rolls, 1)

				roll := input.Rolls[0]
				// Should have 3 kept dice (4d6 drop lowest)
				assert.Len(t, roll.Dice, 3, "Should have 3 kept dice")
				// Should have 1 dropped die
				assert.Len(t, roll.Dropped, 1, "Should have 1 dropped die")
				// Total should be sum of kept dice only
				var keptSum int
				for _, d := range roll.Dice {
					keptSum += d
				}
				assert.Equal(t, keptSum, roll.Total, "Total should be sum of kept dice")
				// Total should be between 3 and 18 (3d6 range)
				assert.GreaterOrEqual(t, roll.Total, 3)
				assert.LessOrEqual(t, roll.Total, 18)

				return &dicesession.CreateOutput{
					Session: &dicesession.DiceSession{
						EntityID: input.EntityID,
						Context:  input.Context,
						Rolls:    input.Rolls,
					},
				}, nil
			})

		output, err := o.RollDice(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, output)
		require.NotNil(t, output.Roll)

		// Verify the roll has the expected properties
		assert.Equal(t, "4d6", output.Roll.Notation)
		assert.Len(t, output.Roll.Dice, 3, "Should have 3 kept dice")
		assert.Len(t, output.Roll.Dropped, 1, "Should have 1 dropped die")
		assert.GreaterOrEqual(t, output.Roll.Total, 3)
		assert.LessOrEqual(t, output.Roll.Total, 18)
	})

	// Test rolling 4d6 for non-ability context - should NOT drop lowest
	t.Run("4d6 non-ability context keeps all dice", func(t *testing.T) {
		input := &RollDiceInput{
			EntityID: "player-123",
			Context:  "damage_rolls",
			Notation: "4d6",
		}

		// Mock the repository to return NotFound (no existing session)
		mockRepo.EXPECT().
			Get(ctx, dicesession.GetInput{
				EntityID: "player-123",
				Context:  "damage_rolls",
			}).
			Return(nil, apierr.NotFound("session not found"))

		// Mock creating a new session
		mockRepo.EXPECT().
			Create(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input dicesession.CreateInput) (*dicesession.CreateOutput, error) {
				// Verify that we're NOT dropping dice for non-ability context
				require.Equal(t, "player-123", input.EntityID)
				require.Equal(t, "damage_rolls", input.Context)
				require.Len(t, input.Rolls, 1)

				roll := input.Rolls[0]
				// Should have 4 dice (no dropping)
				assert.Len(t, roll.Dice, 4, "Should have all 4 dice")
				// Should have no dropped dice
				assert.Len(t, roll.Dropped, 0, "Should have no dropped dice")
				// Total should be between 4 and 24 (4d6 range)
				assert.GreaterOrEqual(t, roll.Total, 4)
				assert.LessOrEqual(t, roll.Total, 24)

				return &dicesession.CreateOutput{
					Session: &dicesession.DiceSession{
						EntityID: input.EntityID,
						Context:  input.Context,
						Rolls:    input.Rolls,
					},
				}, nil
			})

		output, err := o.RollDice(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, output)
		require.NotNil(t, output.Roll)

		// Verify the roll has the expected properties
		assert.Equal(t, "4d6", output.Roll.Notation)
		assert.Len(t, output.Roll.Dice, 4, "Should have all 4 dice")
		assert.Len(t, output.Roll.Dropped, 0, "Should have no dropped dice")
		assert.GreaterOrEqual(t, output.Roll.Total, 4)
		assert.LessOrEqual(t, output.Roll.Total, 24)
	})
}

func TestOrchestrator_RollAbilityScores_StandardArray(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := dicemock.NewMockRepository(ctrl)
	idGen := idgen.NewUUID("roll")

	o, err := NewOrchestrator(&Config{
		DiceSessionRepo: mockRepo,
		IDGenerator:     idGen,
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("creates 6 rolls with standard array values", func(t *testing.T) {
		input := &RollAbilityScoresInput{
			EntityID: "draft-123",
			Method:   MethodStandardArray,
		}

		// Mock creating the session
		mockRepo.EXPECT().
			Create(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, createInput dicesession.CreateInput) (*dicesession.CreateOutput, error) {
				require.Equal(t, "draft-123", createInput.EntityID)
				require.Equal(t, ContextAbilityScores, createInput.Context)
				require.Len(t, createInput.Rolls, 6)

				// Verify standard array values: 15, 14, 13, 12, 10, 8
				expectedValues := []int{15, 14, 13, 12, 10, 8}
				for i, roll := range createInput.Rolls {
					assert.Equal(t, expectedValues[i], roll.Total, "Roll %d should have value %d", i, expectedValues[i])
					assert.Equal(t, MethodStandardArray, roll.Notation)
					assert.Nil(t, roll.Dropped)
				}

				return &dicesession.CreateOutput{
					Session: &dicesession.DiceSession{
						EntityID: createInput.EntityID,
						Context:  createInput.Context,
						Rolls:    createInput.Rolls,
					},
				}, nil
			})

		output, err := o.RollAbilityScores(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, output)
		require.Len(t, output.Rolls, 6)

		// Verify output rolls have standard array values
		expectedValues := []int{15, 14, 13, 12, 10, 8}
		for i, roll := range output.Rolls {
			assert.Equal(t, expectedValues[i], roll.Total)
		}
	})
}

func TestOrchestrator_GetRollSession_AutoCreatesStandardArray(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := dicemock.NewMockRepository(ctrl)
	idGen := idgen.NewUUID("roll")

	o, err := NewOrchestrator(&Config{
		DiceSessionRepo: mockRepo,
		IDGenerator:     idGen,
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("auto-creates standard array for ability_scores when not found", func(t *testing.T) {
		// First call returns not found
		mockRepo.EXPECT().
			Get(ctx, dicesession.GetInput{
				EntityID: "draft-456",
				Context:  ContextAbilityScores,
			}).
			Return(nil, apierr.NotFound("session not found"))

		// Then it should create a standard array session
		mockRepo.EXPECT().
			Create(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, createInput dicesession.CreateInput) (*dicesession.CreateOutput, error) {
				require.Equal(t, "draft-456", createInput.EntityID)
				require.Equal(t, ContextAbilityScores, createInput.Context)
				require.Len(t, createInput.Rolls, 6)

				// Verify it's creating standard array
				expectedValues := []int{15, 14, 13, 12, 10, 8}
				for i, roll := range createInput.Rolls {
					assert.Equal(t, expectedValues[i], roll.Total)
				}

				return &dicesession.CreateOutput{
					Session: &dicesession.DiceSession{
						EntityID: createInput.EntityID,
						Context:  createInput.Context,
						Rolls:    createInput.Rolls,
					},
				}, nil
			})

		output, err := o.GetRollSession(ctx, &GetRollSessionInput{
			EntityID: "draft-456",
			Context:  ContextAbilityScores,
		})
		require.NoError(t, err)
		require.NotNil(t, output)
		require.NotNil(t, output.Session)
		require.Len(t, output.Session.Rolls, 6)
	})

	t.Run("returns error for non-ability_scores context when not found", func(t *testing.T) {
		mockRepo.EXPECT().
			Get(ctx, dicesession.GetInput{
				EntityID: "player-789",
				Context:  "damage_rolls",
			}).
			Return(nil, apierr.NotFound("session not found"))

		output, err := o.GetRollSession(ctx, &GetRollSessionInput{
			EntityID: "player-789",
			Context:  "damage_rolls",
		})
		require.Error(t, err)
		require.Nil(t, output)
	})

	t.Run("returns existing session if found", func(t *testing.T) {
		existingSession := &dicesession.DiceSession{
			EntityID: "draft-existing",
			Context:  ContextAbilityScores,
			Rolls: []dicesession.DiceRoll{
				{RollID: "r1", Total: 15},
				{RollID: "r2", Total: 14},
				{RollID: "r3", Total: 13},
				{RollID: "r4", Total: 12},
				{RollID: "r5", Total: 10},
				{RollID: "r6", Total: 8},
			},
		}

		mockRepo.EXPECT().
			Get(ctx, dicesession.GetInput{
				EntityID: "draft-existing",
				Context:  ContextAbilityScores,
			}).
			Return(&dicesession.GetOutput{Session: existingSession}, nil)

		output, err := o.GetRollSession(ctx, &GetRollSessionInput{
			EntityID: "draft-existing",
			Context:  ContextAbilityScores,
		})
		require.NoError(t, err)
		require.NotNil(t, output)
		require.Equal(t, existingSession, output.Session)
	})
}
