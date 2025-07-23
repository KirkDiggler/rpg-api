// Package mocks provides mock expectation helpers for common testing patterns
package mocks

import (
	"context"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	externalmock "github.com/KirkDiggler/rpg-api/internal/clients/external/mock"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
)

// ExpectDraftHydration sets up mock expectations for hydrating a draft with external data
func ExpectDraftHydration(ctx context.Context, mockClient *externalmock.MockClient, draft *dnd5e.CharacterDraft) {
	if draft.RaceID != "" {
		mockClient.EXPECT().
			GetRaceData(ctx, draft.RaceID).
			Return(&external.RaceData{
				ID:   draft.RaceID,
				Name: "Test Race",
				Subraces: []external.SubraceData{
					{
						ID:   draft.SubraceID,
						Name: "Test Subrace",
					},
				},
			}, nil).
			AnyTimes()
	}

	if draft.ClassID != "" {
		mockClient.EXPECT().
			GetClassData(ctx, draft.ClassID).
			Return(&external.ClassData{
				ID:      draft.ClassID,
				Name:    "Test Class",
				HitDice: "1d10",
			}, nil).
			AnyTimes()
	}

	if draft.BackgroundID != "" {
		mockClient.EXPECT().
			GetBackgroundData(ctx, draft.BackgroundID).
			Return(&external.BackgroundData{
				ID:   draft.BackgroundID,
				Name: "Test Background",
			}, nil).
			AnyTimes()
	}
}

// ExpectDraftGet sets up a mock expectation for getting a draft from repository
func ExpectDraftGet(
	ctx context.Context, mockRepo *draftrepomock.MockRepository,
	draftID string, draft *dnd5e.CharacterDraftData, err error,
) {
	mockRepo.EXPECT().
		Get(ctx, draftrepo.GetInput{ID: draftID}).
		Return(&draftrepo.GetOutput{Draft: draft}, err)
}

// ExpectDraftGetByPlayerID sets up a mock expectation for getting a draft by player ID
func ExpectDraftGetByPlayerID(
	ctx context.Context, mockRepo *draftrepomock.MockRepository,
	playerID string, draft *dnd5e.CharacterDraftData, err error,
) {
	mockRepo.EXPECT().
		GetByPlayerID(ctx, draftrepo.GetByPlayerIDInput{PlayerID: playerID}).
		Return(&draftrepo.GetByPlayerIDOutput{Draft: draft}, err)
}

// ExpectDraftCreate sets up a mock expectation for creating a draft
func ExpectDraftCreate(ctx context.Context, mockRepo *draftrepomock.MockRepository) *gomock.Call {
	return mockRepo.EXPECT().
		Create(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.CreateInput) (*draftrepo.CreateOutput, error) {
			// Simulate repository behavior - it would set ID and timestamps
			if input.Draft.ID == "" {
				input.Draft.ID = "generated-draft-id"
			}
			now := clock.Now().Unix()
			if input.Draft.CreatedAt == 0 {
				input.Draft.CreatedAt = now
			}
			if input.Draft.UpdatedAt == 0 {
				input.Draft.UpdatedAt = now
			}
			return &draftrepo.CreateOutput{Draft: input.Draft}, nil
		})
}

// ExpectDraftUpdate sets up a mock expectation for updating a draft
func ExpectDraftUpdate(ctx context.Context, mockRepo *draftrepomock.MockRepository) *gomock.Call {
	return mockRepo.EXPECT().
		Update(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
			// Simulate repository behavior - it would update timestamp
			input.Draft.UpdatedAt = clock.Now().Unix()
			return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
		})
}

// ExpectDraftDelete sets up a mock expectation for deleting a draft
func ExpectDraftDelete(ctx context.Context, mockRepo *draftrepomock.MockRepository, draftID string, err error) {
	mockRepo.EXPECT().
		Delete(ctx, draftrepo.DeleteInput{ID: draftID}).
		Return(&draftrepo.DeleteOutput{}, err)
}

var clock = &testClock{}

type testClock struct{}

func (c *testClock) Now() time.Time {
	return time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
}
