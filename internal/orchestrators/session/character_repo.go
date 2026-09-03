package session

import (
	"context"
	"errors"
	"fmt"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// characterRepository adapts rpg-api's existing character store to the
// session SDK's CharacterRepository contract.
//
// The adaptation is small because entities.Character carries the SDK's own
// *tkcharacter.Data as its only field (internal/entities/character.go).
type characterRepository struct {
	repo characterrepo.Repository
}

// NewCharacterRepository adapts repo to the session SDK's CharacterRepository.
func NewCharacterRepository(repo characterrepo.Repository) sdk.CharacterRepository {
	return &characterRepository{repo: repo}
}

// GetCharacter implements sdk.CharacterRepository.
func (r *characterRepository) GetCharacter(ctx context.Context, id string) (*tkcharacter.Data, error) {
	out, err := r.repo.Get(ctx, characterrepo.GetInput{ID: id})
	if err != nil {
		if apierr.IsNotFound(err) {
			return nil, fmt.Errorf("character %q: %w", id, sdk.ErrNotFound)
		}
		return nil, fmt.Errorf("get character %q: %w", id, err)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil {
		return nil, fmt.Errorf("character %q: %w", id, sdk.ErrBadRepository)
	}
	return out.Character.Data, nil
}

// SaveCharacter implements sdk.CharacterRepository.
func (r *characterRepository) SaveCharacter(ctx context.Context, data *tkcharacter.Data) error {
	if data == nil {
		return errors.New("session: SaveCharacter data is required")
	}
	if data.ID == "" {
		return errors.New("session: SaveCharacter data.ID is required")
	}

	if _, err := r.repo.Update(ctx, characterrepo.UpdateInput{
		Character: &entities.Character{Data: data},
	}); err != nil {
		if apierr.IsNotFound(err) {
			return fmt.Errorf("character %q: %w", data.ID, sdk.ErrNotFound)
		}
		return fmt.Errorf("save character %q: %w", data.ID, err)
	}
	return nil
}
