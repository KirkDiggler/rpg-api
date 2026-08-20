package session

import (
	"context"
	"errors"
	"fmt"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// characterRepository adapts rpg-api's existing character store to the
// session SDK's CharacterRepository contract.
//
// The adaptation is small because entities.Character already carries the
// SDK's own *tkcharacter.Data as a field (internal/entities/character.go) --
// the existing store persists the SDK's shape directly, it just wraps it in
// an API-owned envelope (Appearance) alongside it. GetCharacter unwraps that
// envelope; SaveCharacter replaces only the Data field on the existing
// entity, never the whole record, so API-owned fields the SDK knows nothing
// about survive every write (mirrors persistPlayerCharacterData's write-back
// in the old encounter path, hydrate_players.go).
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
// It loads the existing record first so a write touches only Data --
// overwriting the whole entity with a fresh {Data: ...} would silently drop
// API-owned fields (e.g. Appearance) the SDK never sees and so cannot round-trip.
func (r *characterRepository) SaveCharacter(ctx context.Context, data *tkcharacter.Data) error {
	if data == nil {
		return errors.New("session: SaveCharacter data is required")
	}
	if data.ID == "" {
		return errors.New("session: SaveCharacter data.ID is required")
	}

	// Load failures translate exactly as GetCharacter's do. They did not,
	// which made a save the one place the repository contract stopped being
	// spoken: a character that has gone missing came back as a wrapped store
	// error, so the Manager read a normal not-found as an internal failure and
	// could not tell "this record is gone" from "the store is broken". The two
	// call for different responses from a host, which is the whole reason the
	// SDK gives not-found its own sentinel.
	out, err := r.repo.Get(ctx, characterrepo.GetInput{ID: data.ID})
	if err != nil {
		if apierr.IsNotFound(err) {
			return fmt.Errorf("character %q: %w", data.ID, sdk.ErrNotFound)
		}
		return fmt.Errorf("load character %q for save: %w", data.ID, err)
	}
	// A successful Get that hands back nothing is a defect in the store, not a
	// missing character -- the same judgement GetCharacter makes on the same
	// shape. The previous code dereferenced out without checking it and would
	// PANIC here; the nil-Character branch below it quietly invented a blank
	// entity instead, which is worse than failing: it would write a record with
	// every API-owned field (Appearance) silently emptied, and this method
	// exists precisely to stop that from happening.
	if out == nil || out.Character == nil {
		return fmt.Errorf("character %q: %w", data.ID, sdk.ErrBadRepository)
	}
	existing := out.Character
	existing.Data = data

	if _, err := r.repo.Update(ctx, characterrepo.UpdateInput{Character: existing}); err != nil {
		return fmt.Errorf("save character %q: %w", data.ID, err)
	}
	return nil
}
