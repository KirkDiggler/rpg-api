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

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
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
		if _, err := charRepo.Update(ctx, characterrepo.UpdateInput{
			Character: &entities.Character{Data: &charData},
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
