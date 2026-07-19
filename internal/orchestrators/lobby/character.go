package lobby

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkevents "github.com/KirkDiggler/rpg-toolkit/events"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
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

	if err := o.clearStaleActionEconomy(ctx, out.Character); err != nil {
		return memberCombatSnapshot{}, err
	}
	if err := o.restoreForNewEncounter(ctx, out.Character); err != nil {
		return memberCombatSnapshot{}, err
	}

	return memberCombatSnapshot{
		hp:    out.Character.Data.HitPoints,
		maxHP: out.Character.Data.MaxHitPoints,
		ac:    out.Character.Data.ArmorClass,
	}, nil
}

// restoreForNewEncounter applies rpg-toolkit's arcade recovery
// (tkcharacter.RestoreForNewEncounter, rpg-toolkit#785/#786) to char's
// persisted data before it is seated in a BRAND NEW encounter: death is an
// encounter-scoped outcome, not a persistent character state. A character
// carrying 0 HP or less is restored to full HP with death-save state cleared
// and the Unconscious condition stripped; a character already above 0 HP is
// untouched (see RestoreForNewEncounter's own doc for the full contract).
//
// rpg-api#670: StartEncounter's AddPlayer call deliberately does not carry
// DataJSON (hydration is the v2 orchestrator's characterData.Attach
// cascade's job — see TestStartEncounter_SeedsHonestCombatSnapshot), so the
// toolkit's own restoreForNewSeat (encounter.go, DataJSON-gated) never fires
// for a real StartEncounter-seeded seat. Calling RestoreForNewEncounter
// directly here — on the same character.Data this function already loads —
// gets the identical toolkit-owned restore rule applied without requiring
// StartEncounter to plumb a full character blob through AddPlayer's
// PlayerInput just to trigger it.
//
// This must run BEFORE the memberCombatSnapshot is built below, and its
// result (when it fires) must be persisted back to the character store:
// otherwise the very next StartEncounter for this character would read the
// same pre-restore hp=0 record straight out of the store again.
func (o *Orchestrator) restoreForNewEncounter(ctx context.Context, char *entities.Character) error {
	if char.Data == nil || !tkcharacter.RestoreForNewEncounter(char.Data) {
		return nil
	}
	if _, err := o.characterRepo.Update(ctx, characterrepo.UpdateInput{Character: char}); err != nil {
		return fmt.Errorf("persist arcade-recovery restore for character %q: %w", char.Data.ID, err)
	}
	return nil
}

// clearStaleActionEconomy resets char's toolkit action economy back to nil
// (out of combat) and persists that reset, if it's carrying one forward
// from a PRIOR encounter — a wave-close playtest blocker found live on the
// dev stack (rpg-api#644): alice's persisted action_economy carried
// movement_remaining=0 from an earlier session's combat into a brand-new
// StartEncounter call, and every MoveEntity on that fresh FREE_ROAM
// encounter failed "insufficient movement remaining: Nft, 0ft remaining" —
// even though nothing about the NEW encounter had happened yet.
//
// Root cause: character.Character.ActionEconomy has no encounter scoping —
// it is a flat field on the character record, not keyed by which encounter
// set it. rpg-toolkit's own Move() budget gate (encounter/combat.go's
// ErrInsufficientMovement) checks InCombat() as "does this character have a
// non-nil action economy right now", with no way to tell "mid-turn in the
// encounter I'm currently in" apart from "leftover from a different,
// already-ended encounter". ExitCombat() exists specifically to clear this
// ("Call this when the encounter ends, not between turns" — its own doc),
// but nothing in rpg-api or rpg-toolkit's own encounter package ever calls
// it (verified: zero call sites for either). SeedTurnEconomyForData
// (hydrate_players.go) can't fix this either — it deliberately treats ANY
// non-nil economy as "already legitimately seeded, don't touch it", because
// mid-encounter it can't tell live-and-depleted apart from stale-and-
// abandoned; that guard is correct for its own job and cannot double as
// this one.
//
// StartEncounter is the correct, narrowly-scoped place for this: it is the
// SOLE path a new encounter comes into existence (lobby-service.md), so at
// this exact moment every member's character is, by definition, not
// mid-turn in the encounter about to exist — clearing action economy here
// can never stomp a real in-progress turn. This is a defensive backstop,
// not the complete fix: the toolkit or rpg-api's encounter-end handling
// should still call ExitCombat() when an encounter properly ends, so a
// character never carries stale economy into the NEXT encounter in the
// first place; that gap remains open (see docs/status.md).
func (o *Orchestrator) clearStaleActionEconomy(ctx context.Context, char *entities.Character) error {
	if char.Data == nil || char.Data.ActionEconomy == nil {
		return nil
	}
	live, err := tkcharacter.LoadFromData(ctx, char.Data, tkevents.NewEventBus())
	if err != nil {
		return fmt.Errorf("load character %q to clear stale combat economy: %w", char.Data.ID, err)
	}
	if _, exitErr := live.ExitCombat(ctx, &tkcharacter.ExitCombatInput{}); exitErr != nil {
		return fmt.Errorf("clear stale combat economy for character %q: %w", char.Data.ID, exitErr)
	}
	char.Data = live.ToData()
	if _, updErr := o.characterRepo.Update(ctx, characterrepo.UpdateInput{Character: char}); updErr != nil {
		return fmt.Errorf("persist cleared combat economy for character %q: %w", char.Data.ID, updErr)
	}
	return nil
}
