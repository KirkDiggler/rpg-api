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
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

// characterRepository adapts rpg-api's existing character store to the
// session SDK's CharacterRepository contract.
//
// The adaptation is small because entities.Character carries the SDK's own
// *tkcharacter.Data as its only field next to the API-owned WorldID
// (internal/entities/character.go).
//
// # World comes from ctx, never from the SDK
//
// The SDK signatures are unchanged and carry no world. Every call is scoped to
// the trusted world the auth/role boundary installed on the request context,
// so a party save during an encounter in world A can only ever touch world A's
// records. A missing trusted world fails closed instead of defaulting.
//
// # Ownership belongs to the API, not to the SDK
//
// The SDK may load and save ANY character in the world, because a party action
// legitimately saves another seated member's sheet. So this adapter does NOT
// gate on the invoking player. The repository enforces the one ownership rule
// that must hold for every caller: a stored record's player and world are
// immutable. SaveCharacter loads the existing wrapper and reuses its world,
// which is what keeps a foreign-world id from creating or rehoming a record.
type characterRepository struct {
	repo characterrepo.Repository
}

// NewCharacterRepository adapts repo to the session SDK's CharacterRepository.
func NewCharacterRepository(repo characterrepo.Repository) sdk.CharacterRepository {
	return &characterRepository{repo: repo}
}

// trustedWorld returns the world the request was admitted into.
func trustedWorld(ctx context.Context) (string, error) {
	world, ok := worldcontext.Get(ctx)
	if !ok || world.WorldID == "" {
		return "", errors.New("session character repository: trusted world context is required")
	}
	return world.WorldID, nil
}

// GetCharacter implements sdk.CharacterRepository.
func (r *characterRepository) GetCharacter(ctx context.Context, id string) (*tkcharacter.Data, error) {
	worldID, err := trustedWorld(ctx)
	if err != nil {
		return nil, err
	}
	out, err := r.repo.Get(ctx, characterrepo.GetInput{WorldID: worldID, ID: id})
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
// The wrapper is loaded first, so a missing or foreign-world record is refused
// (ErrNotFound) rather than created. Only the toolkit data is replaced: the
// stored WorldID is reused and the repository independently refuses any change
// to the stored player, so this can never rehome or transfer ownership.
func (r *characterRepository) SaveCharacter(ctx context.Context, data *tkcharacter.Data) error {
	if data == nil {
		return errors.New("session: SaveCharacter data is required")
	}
	if data.ID == "" {
		return errors.New("session: SaveCharacter data.ID is required")
	}
	worldID, err := trustedWorld(ctx)
	if err != nil {
		return err
	}

	existing, err := r.repo.Get(ctx, characterrepo.GetInput{WorldID: worldID, ID: data.ID})
	if err != nil {
		if apierr.IsNotFound(err) {
			return fmt.Errorf("character %q: %w", data.ID, sdk.ErrNotFound)
		}
		return fmt.Errorf("load character %q: %w", data.ID, err)
	}
	if existing == nil || existing.Character == nil {
		return fmt.Errorf("character %q: %w", data.ID, sdk.ErrBadRepository)
	}

	if _, err := r.repo.Update(ctx, characterrepo.UpdateInput{
		Character: &entities.Character{WorldID: existing.Character.WorldID, Data: data},
	}); err != nil {
		if apierr.IsNotFound(err) {
			return fmt.Errorf("character %q: %w", data.ID, sdk.ErrNotFound)
		}
		return fmt.Errorf("save character %q: %w", data.ID, err)
	}
	return nil
}
