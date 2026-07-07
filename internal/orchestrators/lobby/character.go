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

// seedMemberHP looks up characterID in the character store and returns its
// current/max HP, generalizing the single-caller seedPlayerHP that used to
// live in the deleted v1alpha2 CreateEncounter handler (rpg-api#612) to
// StartEncounter's N-member seeding. Without this seed, PlayerData.HP/MaxHP
// stays 0/0 forever — the toolkit's combat verbs only clamp HP downward, so
// an unseeded seat is "undying" and its HP bar always reads 0/0.
//
// A NotFound character (or a nil character repo, defensive for tests) leaves
// HP/MaxHP at 0 rather than failing StartEncounter — a character genuinely
// missing at start time is a data-consistency issue surfaced elsewhere, not
// a reason to block the whole party. Any OTHER character-store error
// surfaces: a real store failure should not be silently swallowed into an
// incorrect 0/0 seed.
func (o *Orchestrator) seedMemberHP(ctx context.Context, characterID string) (hp, maxHP int, err error) {
	if o.characterRepo == nil {
		return 0, 0, nil
	}
	out, getErr := o.characterRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if getErr != nil {
		if apierr.IsNotFound(getErr) {
			return 0, 0, nil
		}
		return 0, 0, getErr
	}
	if out == nil || out.Character == nil || out.Character.Data == nil {
		return 0, 0, nil
	}
	return out.Character.Data.HitPoints, out.Character.Data.MaxHitPoints, nil
}
