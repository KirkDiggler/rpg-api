package sessionv1alpha1

import (
	"context"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
)

type rosterCharacter struct {
	owner string
	name  string
	class string
	race  string
}

func charactersOf(ctrl *gomock.Controller, rows map[string]rosterCharacter) characterrepo.Repository {
	repo := charactermock.NewMockRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			row, ok := rows[in.ID]
			if !ok {
				return nil, apierr.NotFound("character not found")
			}
			return &characterrepo.GetOutput{Character: &entities.Character{
				Data: &tkcharacter.Data{
					ID: in.ID, PlayerID: row.owner, Name: row.name,
					ClassID: row.class, RaceID: races.Race(row.race),
				},
			}}, nil
		},
	).AnyTimes()
	return repo
}
