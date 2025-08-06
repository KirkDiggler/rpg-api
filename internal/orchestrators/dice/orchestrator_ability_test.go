package dice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/errors"
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
			Return(nil, errors.NotFound("session not found"))

		// Mock creating a new session
		mockRepo.EXPECT().
			Create(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, input dicesession.CreateInput) (*dicesession.CreateOutput, error) {
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
				var keptSum int32
				for _, d := range roll.Dice {
					keptSum += d
				}
				assert.Equal(t, keptSum, roll.Total, "Total should be sum of kept dice")
				// Total should be between 3 and 18 (3d6 range)
				assert.GreaterOrEqual(t, roll.Total, int32(3))
				assert.LessOrEqual(t, roll.Total, int32(18))

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
		assert.GreaterOrEqual(t, output.Roll.Total, int32(3))
		assert.LessOrEqual(t, output.Roll.Total, int32(18))
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
			Return(nil, errors.NotFound("session not found"))

		// Mock creating a new session
		mockRepo.EXPECT().
			Create(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, input dicesession.CreateInput) (*dicesession.CreateOutput, error) {
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
				assert.GreaterOrEqual(t, roll.Total, int32(4))
				assert.LessOrEqual(t, roll.Total, int32(24))

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
		assert.GreaterOrEqual(t, output.Roll.Total, int32(4))
		assert.LessOrEqual(t, output.Roll.Total, int32(24))
	})
}