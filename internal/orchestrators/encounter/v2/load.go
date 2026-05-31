package encounter

import (
	"context"
	"errors"
	"fmt"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// Orchestrator-level sentinel errors. These are entity-layer classifications;
// the thin handler maps them onto gRPC status codes (proto-layer concern). The
// orchestrator never imports proto, so it must surface these as typed sentinels
// rather than status.Error.
var (
	// ErrEncounterNotFound means the encounter id has no stored snapshot.
	// Handler maps to codes.NotFound.
	ErrEncounterNotFound = errors.New("encounter not found")

	// ErrPlayerNotInEncounter means the authenticated player is not a member of
	// the encounter. Handler maps to codes.PermissionDenied.
	ErrPlayerNotInEncounter = errors.New("player is not in this encounter")

	// ErrEntityOwnershipMismatch means the player asked to act with an entity
	// they do not control. Handler maps to codes.PermissionDenied.
	ErrEntityOwnershipMismatch = errors.New("entity does not match player's controlled entity")
)

// loadInput carries the per-request load context.
type loadInput struct {
	// EncounterID identifies the encounter snapshot to load.
	EncounterID string

	// PlayerID is the authenticated player making the request. Membership is
	// always verified against this id.
	PlayerID core.PlayerID

	// EntityID is the entity the player claims to control. When non-empty, load
	// verifies the encounter maps PlayerID → EntityID and returns
	// ErrEntityOwnershipMismatch otherwise. Empty skips the ownership check
	// (read-path / door verbs that act on a target rather than the actor).
	EntityID string

	// WithCharacterData requests the #689 hydration cascade: load attaches the
	// transient PlayerData.DataJSON (via the injected CharacterData.Attach)
	// BEFORE LoadFromData, so the held *character.Character is hydrated for each
	// seat. Combat-capable verbs (the reaction-resume path) set this; the
	// skill-check and door verbs leave it false (skill checks don't touch
	// combatant state). No-op when no Attach func is wired.
	WithCharacterData bool
}

// load is the single load core: Get the snapshot → verify membership (and
// optional entity ownership) from the snapshot BEFORE rehydration → LoadFromData
// onto the broker bus with the resolvers wired uniformly.
//
// It returns ONLY the encounter (2 returns, no side *Data): the encounter is
// the synced state — verbs read entities and orchestration state through it and
// persist via enc.ToData(). Returning a parallel *Data snapshot would invite
// drift between it and the held entities.
//
// Auth checks read data.Players directly and run before LoadFromData so the
// auth-fail path does not pay rehydration cost.
func (o *Orchestrator) load(ctx context.Context, in loadInput) (*tkenc.Encounter, error) {
	data, err := o.encRepo.Get(ctx, in.EncounterID)
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, ErrEncounterNotFound
		}
		return nil, fmt.Errorf("load encounter %q: %w", in.EncounterID, err)
	}

	pd, ok := data.Players[in.PlayerID]
	if !ok {
		return nil, ErrPlayerNotInEncounter
	}
	if in.EntityID != "" && string(pd.EntityID) != in.EntityID {
		return nil, ErrEntityOwnershipMismatch
	}

	// #689 hydration cascade: combat-capable verbs attach the transient
	// PlayerData.DataJSON from the character store before LoadFromData rehydrates
	// the held combatants. Runs after auth (no rehydration cost on the auth-fail
	// path) and before LoadFromData (which consumes the DataJSON). No-op when no
	// Attach func is wired (tests / non-combat verbs).
	if in.WithCharacterData && o.characterData.Attach != nil {
		if attachErr := o.characterData.Attach(ctx, data); attachErr != nil {
			return nil, fmt.Errorf("attach character data %q: %w", in.EncounterID, attachErr)
		}
	}

	// Resolvers are wired uniformly on every load, mirroring the handler today.
	// The dice roller is carried inside the resolver configs (the builders), not
	// passed as a separate WithRoller option — keeping a single roller source per
	// the "one representation" rule rather than a parallel encounter-level roller.
	enc, err := tkenc.LoadFromData(ctx, data, o.broker,
		tkenc.WithCharacterResolver(o.resolver),
		tkenc.WithCombatResolver(o.buildCombatResolver(data)),
		tkenc.WithMovementResolver(o.buildMovementResolver(data)))
	if err != nil {
		return nil, fmt.Errorf("load encounter from data %q: %w", in.EncounterID, err)
	}
	return enc, nil
}
