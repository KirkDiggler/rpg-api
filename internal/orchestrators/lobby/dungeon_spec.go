package lobby

// dungeon_spec.go held the old encounter stack's DungeonKey resolution:
// resolveDungeonSpec (a hand-authored builder table keyed off
// tkenc.DungeonParams, DungeonKeyCrypt's default), resolveContentDungeonSpec
// (per-request lookup into the content registry, returning the OLD
// rpg-toolkit/encounter/dungeonspec.CompiledDungeon shape), and
// DisabledDungeonKeyError/ErrUnknownDungeonKey (their caller-visible
// failure modes) plus validateDungeonKeyOverride (Config.DungeonKeyOverride's
// RPG_DUNGEON_KEY validation). All of it was reachable only from the old
// start_encounter.go's StartEncounter body, deleted with the rest of the old
// stack (rpg-project#227) — start_encounter_session_stack.go's StartEncounter
// never resolves a DungeonKey at all; it always plays the one embedded
// reference-tomb dungeon (see that file's doc comment for the
// content-authoring gap this leaves open).
//
// DungeonKey and LoadContentRegistry survive: DungeonKey is still the type
// StartEncounterInput carries (unused by the sole remaining implementation,
// but part of the proto-mirrored request shape), and LoadContentRegistry is
// still how ListDungeons and the authoring orchestrator get the shared live
// dungeonregistry.Registry — internal/dungeonregistry and internal/content
// still speak the OLD rpg-toolkit/encounter/dungeonspec dialect themselves
// (a separate, not-yet-decided migration — see this branch's PR description).

import (
	"fmt"
	"log/slog"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/content"
	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

// DungeonKey selects a named dungeon specification. Carried on
// StartEncounterInput for parity with the proto's dungeon_key field; the
// session-stack StartEncounter does not consult it today (see
// start_encounter_session_stack.go).
type DungeonKey string

// LoadContentRegistry builds the startup *dungeonregistry.Registry of
// every content-hosted dungeon spec by calling content.AllSpecs() exactly
// ONCE. It compiles every spec through LoadWithConfig with DefaultPartyCap,
// the normal product roster capacity used by the startup authoring and runtime
// paths. Exported so cmd/server/server.go can call it directly at startup
// (the Architecture decision in plan.md: the registry is built ONCE,
// outside either orchestrator, then passed by pointer into BOTH
// lobbyorch.Config.Registry and the new authoringorch.Config.Registry) —
// lobbyorch.New() itself no longer builds a registry; it requires one
// already built. Callers must invoke this BEFORE lobbyorch.New(): an
// unreadable RPG_CONTENT_DIR fails HERE, loudly, rather than silently
// degrading to embedded-only content.
//
// A spec whose header decodes but whose body fails dungeonspec.Load (a
// schema/business-rule violation) is stored as a dungeonregistry.Entry
// carrying that Err, not dropped — see Entry's doc. Also calls
// dungeonspec.Decode (separately from Load, which discards the decoded
// Spec after compiling — CompiledDungeon carries no Name field) purely to
// capture each spec's declared display Name for Entry.Name, which
// ListDungeons needs.
//
// Logs one line per problem found while building the registry, from BOTH
// failure sources this registry can hit: content.AllSpecs' own
// header-level problems (a malformed/missing key: field in an
// RPG_CONTENT_DIR file — logged here since AllSpecs only returns these as
// data, it never logs), and a dungeonspec.Load failure for a spec whose
// header decoded fine but whose body doesn't validate/compile. Also logs
// one DEBUG line per RPG_CONTENT_DIR key that shadows an embedded key of
// the same name — confirmation, for an author mid edit-restart loop, that
// their override actually took effect.
func LoadContentRegistry() (*dungeonregistry.Registry, error) {
	specs, problems, err := content.AllSpecs()
	if err != nil {
		return nil, fmt.Errorf("load content specs (RPG_CONTENT_DIR): %w", err)
	}
	for _, p := range problems {
		slog.Warn("lobby orchestrator: content spec problem, key excluded from registry", "detail", p)
	}

	if shadowed, oerr := content.OverriddenKeys(); oerr == nil {
		for _, key := range shadowed {
			slog.Debug("lobby orchestrator: content spec loaded from RPG_CONTENT_DIR override", "key", key)
		}
	}
	// oerr != nil here would mean content.AllSpecs() above raced its own
	// RPG_CONTENT_DIR read against this second one and got a different
	// answer -- astronomically unlikely for a startup-only path reading a
	// local directory, and this debug log is a nice-to-have, not a
	// correctness path, so it's worth skipping quietly rather than
	// failing construction over.

	entries := make(map[string]dungeonregistry.Entry, len(specs))
	for key, raw := range specs {
		compiled, loadErr := dungeonspec.LoadWithConfig(raw, dungeonspec.LoadConfig{
			PartyStartSeatCount: DefaultPartyCap,
		})
		if loadErr != nil {
			slog.Warn("lobby orchestrator: content spec failed to load, key disabled", "key", key, "error", loadErr)
			entries[key] = dungeonregistry.Entry{Err: loadErr}
			continue
		}
		// Load already Decoded raw successfully as part of compiling --
		// a second Decode call here failing would mean Load's own
		// internal Decode and this one disagree, which should be
		// impossible. Defensive fallback to the key itself as the name
		// rather than propagating a surprise error, matching this
		// package's belt-and-suspenders style elsewhere.
		name := key
		if spec, decodeErr := dungeonspec.Decode(raw); decodeErr == nil {
			name = spec.Name
		} else {
			slog.Warn("lobby orchestrator: content spec name capture failed unexpectedly after a successful Load",
				"key", key, "error", decodeErr)
		}
		entries[key] = dungeonregistry.Entry{Compiled: compiled, Name: name}
	}
	return dungeonregistry.New(entries), nil
}
