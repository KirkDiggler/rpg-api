package encounter

// hydrate_players.go — attach player character blobs onto the encounter Data
// so the toolkit's LoadFromData hydration cascade (rpg-toolkit#689) can build
// the held *character.Character for each player seat.
//
// #689 made Encounter.LoadFromData own combatant hydration: it rehydrates each
// player from PlayerData.DataJSON and each monster from MonsterData.DataJSON,
// holds the runtime entities, and subscribes their conditions to the encounter
// bus exactly once. Monsters already carry DataJSON on the persisted snapshot;
// players do NOT — the character store is the authoritative source for a
// player's character data.
//
// PlayerData.DataJSON is TRANSIENT (toolkit#689 Q1): the host re-attaches the
// blob after each repo.Get and before LoadFromData; it is not persisted on the
// encounter snapshot. This keeps the character store authoritative (no stale
// duplicate of character state living on the encounter) and mirrors how
// MonsterData.DataJSON / ActivateFeatureInput.CharDataJSON already work.
//
// This is the single place rpg-api fetches player character blobs for the load
// path. It replaces the per-attack character re-load the combat resolver used
// to do (loadCharacterWithBus) — which was the #684 double-subscribe source.

import (
	"context"
	"encoding/json"
	"fmt"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	tkenccore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkevents "github.com/KirkDiggler/rpg-toolkit/events"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// attachPlayerCharacterData fetches each player's character blob from the
// character store and sets it as transient PlayerData.DataJSON on the encounter
// Data, so the subsequent tkenc.LoadFromData cascade hydrates the held
// *character.Character. The player's EntityID is the character ID in the v2
// handler stack (confirmed by CreateEncounter seeding EntityID = the character
// ID and the fixtures using EntityID as the character ID directly).
//
// Called immediately before every tkenc.LoadFromData on a combat-capable path.
// When charRepo is nil (handler tests that don't wire a character store), this
// is a no-op: the player seats keep DataJSON empty and the SDK hydration cascade
// skips them, falling back to the resolver's stat-snapshot stand-in path.
//
// A per-player lookup failure is NOT fatal: that seat is left without DataJSON
// (stand-in fallback) rather than failing the whole RPC. A character that
// genuinely doesn't exist is a data-consistency issue the verb surfaces
// elsewhere; here we degrade to the snapshot rather than block the encounter.
// A marshal failure IS surfaced — it indicates a corrupt blob and silently
// dropping it would hide a real bug.
func attachPlayerCharacterData(
	ctx context.Context,
	data *tkenc.Data,
	charRepo characterrepo.Repository,
) error {
	if data == nil || charRepo == nil {
		return nil
	}
	for _, pd := range data.Players {
		if pd == nil || pd.EntityID == "" {
			continue
		}
		out, err := charRepo.Get(ctx, characterrepo.GetInput{ID: string(pd.EntityID)})
		if err != nil || out == nil || out.Character == nil || out.Character.Data == nil {
			// Character not found / not loadable — leave DataJSON empty so the
			// SDK skips hydration for this seat and the resolver uses the
			// stat-snapshot stand-in. Not fatal to the RPC.
			continue
		}
		blob, err := json.Marshal(out.Character.Data)
		if err != nil {
			return fmt.Errorf("marshal character data for %q: %w", pd.EntityID, err)
		}
		pd.DataJSON = blob
	}
	return nil
}

// SeedTurnEconomyForData seeds the active actor's turn-start action economy in
// the authoritative character store for a turn-based encounter, closing the
// "first actor is never in combat" gap (rpg-api#598).
//
// Why this is needed: the toolkit (encounter v0.20.0, rpg-toolkit#697) now
// enforces the action economy server-side and seeds it via character.StartTurn,
// triggered by the encounter's SetMode(TurnBased) (first actor) and EndTurn
// (next actor) — but ONLY on a HELD *character.Character. The v2 turn-based-entry
// sites build the encounter via tkenc.New + AddPlayer + SetMode; New never
// hydrates combatants (only LoadFromData does), so at SetMode time no character
// is held and the first-actor seeding no-ops. The character store therefore
// keeps ActionEconomy == nil, the held character rehydrates with InCombat() ==
// false, and the first actor's TakeAction is rejected "not in combat". EndTurn
// correctly seeds every SUBSEQUENT actor (it loads with character data), so only
// the first actor of a freshly-entered turn-based encounter needs this.
//
// What it does: load the active actor's character from the store, run the
// toolkit's OWN StartTurn verb (the same one SetMode/EndTurn call — rpg-api
// invokes a turn-boundary verb by reference, exactly as it invokes EndTurn; it
// authors no economy values, the toolkit owns them — Invariant 2/3), and write
// the seeded character.Data back to the store. The next combat-verb load then
// rehydrates an in-combat character.
//
// No-op when charRepo is nil (tests without a store), the encounter is not
// turn-based, the active index is out of range, the active actor is not a player
// seat, or the active character is already in combat (idempotent guard so a
// re-entry does not reset a turn already underway). NPCs / flat stat-snapshot
// seats carry no stored character and are skipped by the not-found path; they
// take the attack resolver's stand-in cost.
func SeedTurnEconomyForData(
	ctx context.Context,
	data *tkenc.Data,
	charRepo characterrepo.Repository,
) error {
	if data == nil || charRepo == nil {
		return nil
	}
	if data.Mode != tkenccore.ModeTurnBased || len(data.Initiative) == 0 {
		return nil
	}
	if data.ActiveIdx < 0 || data.ActiveIdx >= len(data.Initiative) {
		return nil
	}
	return seedActorTurnEconomy(ctx, data, data.Initiative[data.ActiveIdx], charRepo)
}

// SeedActorTurnEconomyForData seeds a NAMED actor's turn-start economy (vs. the
// active actor). ActivateFeature uses this: the toolkit's ActivateFeature verb
// requires the supplied CharDataJSON to already be InCombat (carry an
// ActionEconomy) and does NOT itself gate on whose turn it is, so the host seeds
// the activating actor's economy here regardless of active-index. This replaces
// the former rpg-api-authored ActionEconomy{1,1,1,30} injection (Invariant 2):
// the values now come from the toolkit's StartTurn, not from rpg-api.
//
// No-op when the encounter is not turn-based, the actor is not a player seat /
// has no stored character, or the actor is already in combat.
func SeedActorTurnEconomyForData(
	ctx context.Context,
	data *tkenc.Data,
	actorID tkenccore.EntityID,
	charRepo characterrepo.Repository,
) error {
	if data == nil || charRepo == nil || actorID == "" {
		return nil
	}
	if data.Mode != tkenccore.ModeTurnBased {
		return nil
	}
	return seedActorTurnEconomy(ctx, data, actorID, charRepo)
}

// seedActorTurnEconomy is the shared core: load the actor's stored character,
// run the toolkit's StartTurn verb (the same one SetMode/EndTurn call — rpg-api
// invokes a turn-boundary verb by reference; it authors no economy values, the
// toolkit owns them — Invariant 2/3), and write the seeded character.Data back
// to the store so the next load rehydrates an in-combat character.
func seedActorTurnEconomy(
	ctx context.Context,
	data *tkenc.Data,
	actorID tkenccore.EntityID,
	charRepo characterrepo.Repository,
) error {
	// The actor must be a player seat to carry a character economy.
	isPlayer := false
	for _, pd := range data.Players {
		if pd != nil && pd.EntityID == actorID {
			isPlayer = true
			break
		}
	}
	if !isPlayer {
		return nil // NPC / monster — no character economy to seed.
	}

	out, err := charRepo.Get(ctx, characterrepo.GetInput{ID: string(actorID)})
	if err != nil || out == nil || out.Character == nil || out.Character.Data == nil {
		// No stored character (stand-in seat) — nothing to seed. Not fatal:
		// the attack resolver falls back to the flat-stat one-action cost.
		return nil
	}
	charData := out.Character.Data

	// Already seeded (in combat) — do not reset a turn already underway.
	if charData.ActionEconomy != nil {
		return nil
	}

	// Run the toolkit's own turn-start seeding verb on the loaded character.
	char, err := tkcharacter.LoadFromData(ctx, charData, tkevents.NewEventBus())
	if err != nil {
		return fmt.Errorf("load character %q for turn-economy seeding: %w", actorID, err)
	}
	if _, err := char.StartTurn(ctx, &tkcharacter.StartTurnInput{
		Speed:      char.GetSpeed(),
		TurnNumber: data.Round,
	}); err != nil {
		return fmt.Errorf("seed turn economy for actor %q: %w", actorID, err)
	}

	// Persist the seeded character.Data back to the authoritative store, owning
	// ONLY Data (mirrors persistPlayerCharacterData: the rest of the entity stays
	// as the store last saw it).
	updated := out.Character
	updated.Data = char.ToData()
	if _, err := charRepo.Update(ctx, characterrepo.UpdateInput{Character: updated}); err != nil {
		return fmt.Errorf("persist seeded turn economy for %q: %w", actorID, err)
	}
	return nil
}

// persistPlayerCharacterData writes each player's (post-verb) DataJSON back to
// the character store, so the next RPC's attachPlayerCharacterData re-fetches
// the updated state. This is the cross-RPC persistence half of the #689 round
// trip: the SDK's ToData cascade re-serializes held-entity state (e.g.
// RagingCondition.DidAttackThisTurn, SneakAttack.UsedThisTurn) into the
// transient PlayerData.DataJSON; since the character store (NOT the encounter
// snapshot) is authoritative for player character state, the host must flush
// that DataJSON back to the store. This replaces the old per-attack
// saveAttackerConditionState write-back the combat resolver used to do.
//
// Called after enc.ToData() + SyncErr(), before encRepo.Save, on combat-capable
// verbs. No-op when charRepo is nil (handler tests without a store) or a player
// seat has no DataJSON (was not hydrated this RPC).
//
// A per-player unmarshal/update failure is surfaced: a dropped write-back means
// lost cross-RPC condition state, which is a correctness regression and must be
// observable rather than silently swallowed.
func persistPlayerCharacterData(
	ctx context.Context,
	data *tkenc.Data,
	charRepo characterrepo.Repository,
) error {
	if data == nil || charRepo == nil {
		return nil
	}
	for _, pd := range data.Players {
		if pd == nil || pd.EntityID == "" || len(pd.DataJSON) == 0 {
			continue
		}
		var charData tkcharacter.Data
		if err := json.Unmarshal(pd.DataJSON, &charData); err != nil {
			return fmt.Errorf("unmarshal cascaded character data for %q: %w", pd.EntityID, err)
		}
		// Fetch the existing record and replace ONLY its Data: the character
		// repo's Update marshals the whole entities.Character (including
		// API-owned fields like Appearance) over the record, so writing a
		// fresh {Data: ...} with everything else zero would silently drop those
		// fields on every combat-capable RPC. The cascade only owns Data; the
		// rest of the character entity stays as the store last saw it.
		existing, err := charRepo.Get(ctx, characterrepo.GetInput{ID: string(pd.EntityID)})
		if err != nil || existing == nil || existing.Character == nil {
			return fmt.Errorf("load character for write-back %q: %w", pd.EntityID, err)
		}
		updated := existing.Character
		updated.Data = &charData
		if _, err := charRepo.Update(ctx, characterrepo.UpdateInput{
			Character: updated,
		}); err != nil {
			return fmt.Errorf("persist character data for %q: %w", pd.EntityID, err)
		}
		// DataJSON is transient (#689 Q1): now that it's flushed to the
		// authoritative character store, clear it so it does NOT persist onto
		// the encounter snapshot (which encRepo.Save writes next). Leaving it
		// would bloat the snapshot with a duplicate, soon-stale char blob; the
		// next RPC re-attaches a fresh copy from the store regardless.
		pd.DataJSON = nil
	}
	return nil
}
