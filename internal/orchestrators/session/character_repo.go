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
//
// A WHOLE-RECORD WRITE, and safe as one because the SDK's sheet store is its
// only in-game caller and every SDK verb holds the guard the character's seat
// decides (rpg-project#542, "One sheet store per verb"). rpg-api has no other
// gameplay write of a character record: the equipment patch, its version check
// and its retry loop are gone. Creation (FinalizeDraft) and the sandbox seeder
// write outside any run.
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
