package sessionv1alpha1

import (
	"context"

	"go.uber.org/mock/gomock"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
)

// ownedCharacterRepo is a character repository under which exactly member is
// owned by playerID.
func ownedCharacterRepo(ctrl *gomock.Controller, member, playerID string) characterrepo.Repository {
	repo := charactermock.NewMockRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: member}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: &tkcharacter.Data{ID: member, PlayerID: playerID}}}, nil,
	).AnyTimes()
	return repo
}

// anyMemberOwnedBy is a character repository under which every member id is
// owned by playerID, for handler tests that exercise a verb past the
// entitlement check. (Both lived beside the deleted v1 StreamEvents tests.)
func anyMemberOwnedBy(ctrl *gomock.Controller, playerID string) characterrepo.Repository {
	repo := charactermock.NewMockRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			return &characterrepo.GetOutput{
				Character: &entities.Character{Data: &tkcharacter.Data{ID: in.ID, PlayerID: playerID}},
			}, nil
		},
	).AnyTimes()
	return repo
}
