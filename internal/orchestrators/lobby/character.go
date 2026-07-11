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

// memberCombatSnapshot is the honest subset of a stored character's combat
// stats StartEncounter can seed onto a tkenc.PlayerInput without doing rules
// math: HP/MaxHP/AC are stored fields, copied verbatim. AttackBonus,
// DamageDice, and DamageType are deliberately absent — a character carries
// no precomputed field for them (they're derived at attack time from
// equipped weapon + ability scores + proficiency bonus, real rules math
// rpg-api must not duplicate). See start_encounter.go's isPlayerCombatant
// discussion (rpg-api#634).
type memberCombatSnapshot struct {
	hp, maxHP, ac int
}

// seedMemberCombatSnapshot looks up characterID in the character store and
// returns its current/max HP and AC, generalizing the single-caller
// seedPlayerHP that used to live in the deleted v1alpha2 CreateEncounter
// handler (rpg-api#612) to StartEncounter's N-member seeding. Without the HP
// seed, PlayerData.HP/MaxHP stays 0/0 forever — the toolkit's combat verbs
// only clamp HP downward, so an unseeded seat is "undying" and its HP bar
// always reads 0/0. Without the AC seed, a player who ends up as a
// stat-snapshot defender stub (e.g. a monster attacks before this player's
// own hydration cascade has run) would present AC 0 — trivially hit by
// every attack (rpg-toolkit encounter/npc.go's TargetAC: targetPlayer.AC).
//
// A NotFound character (or a nil character repo, defensive for tests) leaves
// the snapshot zeroed rather than failing StartEncounter — a character
// genuinely missing at start time is a data-consistency issue surfaced
// elsewhere, not a reason to block the whole party. Any OTHER character-store
// error surfaces: a real store failure should not be silently swallowed into
// an incorrect zeroed seed.
func (o *Orchestrator) seedMemberCombatSnapshot(ctx context.Context, characterID string) (memberCombatSnapshot, error) {
	if o.characterRepo == nil {
		return memberCombatSnapshot{}, nil
	}
	out, getErr := o.characterRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if getErr != nil {
		if apierr.IsNotFound(getErr) {
			return memberCombatSnapshot{}, nil
		}
		return memberCombatSnapshot{}, getErr
	}
	if out == nil || out.Character == nil || out.Character.Data == nil {
		return memberCombatSnapshot{}, nil
	}
	return memberCombatSnapshot{
		hp:    out.Character.Data.HitPoints,
		maxHP: out.Character.Data.MaxHitPoints,
		ac:    out.Character.Data.ArmorClass,
	}, nil
}
