package lobby

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// resolveCharacter loads characterID and validates it belongs to playerID
// (lobby-surface.md: "the server validates the character belongs to the
// authenticated player — the v1 lobby never validated this"). Returns the
// character's display name on success, for server-enrichment onto
// LobbyMember.CharacterName.
func (o *Orchestrator) resolveCharacter(ctx context.Context, playerID, characterID string) (string, error) {
	out, err := o.characterRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil {
		if apierr.IsNotFound(err) {
			return "", ErrCharacterNotFound
		}
		return "", fmt.Errorf("load character %q: %w", characterID, err)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil {
		return "", ErrCharacterNotFound
	}
	if out.Character.Data.PlayerID != playerID {
		return "", ErrCharacterOwnershipMismatch
	}
	return out.Character.Data.Name, nil
}
