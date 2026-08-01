package lobby

import (
	"errors"
	"fmt"
	"log/slog"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/content"
	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

// DungeonKey selects a named dungeon specification passed to the
// toolkit's generic tkenc.Encounter.InitDungeon generator (rpg-api#688).
// rpg-api never computes dungeon geometry itself — a key maps to a
// tkenc.DungeonParams builder, and only the toolkit turns that spec into
// an actual Space. rpg-api#694 goes one step further for the crypt key:
// the builder doesn't even hold its OWN region widths/heights/connectors/
// obstacle specs — it calls the toolkit's tkenc.CryptDungeonParams
// constructor, which is the single source of the crypt's shape, theme,
// archetypes, and physical set-piece specs. rpg-api's only remaining job
// is key/config orchestration: which key maps to which toolkit
// constructor, and which caller-assigned door IDs that constructor's
// connectors get.
type DungeonKey string

// DungeonKeyCrypt is the first authored dungeon spec (rpg-toolkit#814's
// Approved Slice 3 corrections; rpg-toolkit#826 for its physical set
// pieces): one continuous 3-region linear chain — entrance -> corridor ->
// boss chamber — themed "crypt".
const DungeonKeyCrypt DungeonKey = "crypt"

// defaultDungeonKey is used when StartEncounterInput.DungeonKey is the
// zero value — "a named default for now" per rpg-api#688's Scope section
// ("Where the key comes from: lobby settings or a named default for now
// — the point is that rpg-api passes a key and receives geometry back").
// Lobby-settings-driven key selection is future work; no proto field
// carries a key today, so every real caller gets this default.
const defaultDungeonKey = DungeonKeyCrypt

// Crypt spec connector door identifiers — the two caller-assigned entity
// IDs threaded into tkenc.CryptDungeonParams' entranceDoorID/bossDoorID
// parameters below. This is the one piece of "shape" rpg-api still owns
// for the crypt key: naming its own connector doors, not sizing or
// placing anything inside the regions they join.
const (
	cryptDoorEntranceToCorridor core.EntityID = "crypt-door-entrance-corridor"
	cryptDoorCorridorToBoss     core.EntityID = "crypt-door-corridor-boss"
)

// dungeonSpecBuilder produces a toolkit tkenc.DungeonParams for a given
// seed. rpg-api#694: this replaces the retired literal dungeonSpec struct
// (theme/height/regions/connectors hand-authored per key) — a builder
// closure lets each key call whatever toolkit constructor owns its
// content (CryptDungeonParams for the crypt key today) instead of rpg-api
// duplicating that constructor's output as its own literal.
type dungeonSpecBuilder func(seed int64) tkenc.DungeonParams

// dungeonSpecs is the registry of named dungeon builders StartEncounter
// can select by DungeonKey. Exactly one entry today (crypt), forwarding
// verbatim to tkenc.CryptDungeonParams — rpg-api supplies only the seed
// (threaded through from StartEncounterInput.RandomSeed) and its own two
// connector door IDs; every region width/height/archetype/theme/obstacle
// spec is the toolkit's, not rpg-api's. Adding a second named template is
// additive here — never a change to StartEncounter's call site or to
// seedGoblins' endpoint-seeding rule.
var dungeonSpecs = map[DungeonKey]dungeonSpecBuilder{
	DungeonKeyCrypt: func(seed int64) tkenc.DungeonParams {
		return tkenc.CryptDungeonParams(seed, cryptDoorEntranceToCorridor, cryptDoorCorridorToBoss)
	},
}

// ErrUnknownDungeonKey means a DungeonKey has no registered dungeonSpec.
// StartEncounter never receives a caller-supplied key via the current
// proto surface (no proto changes in this issue; lobby-settings-driven
// key selection is future work per the issue's Scope section), so this is
// reachable only via direct orchestrator calls (tests) until that wiring
// lands — unclassified sentinel errors fall through lobbyStatusError's
// default codes.Internal case, an honest mapping for a case no real
// client can trigger yet.
var ErrUnknownDungeonKey = errors.New("lobby orchestrator: unknown dungeon key")

// resolveDungeonSpec maps a DungeonKey to its toolkit-constructed
// tkenc.DungeonParams for the given seed, falling back to
// defaultDungeonKey for the zero value ("a named default for now" —
// rpg-api#688's Scope section). Returns ErrUnknownDungeonKey for any
// other unregistered key — rpg-api surfaces the lookup failure; it never
// invents geometry for a key it doesn't recognize. Returns the EFFECTIVE
// key alongside the params so callers building error messages after a
// successful resolution don't need to re-run the zero-value fallback
// themselves.
func resolveDungeonSpec(key DungeonKey, seed int64) (DungeonKey, tkenc.DungeonParams, error) {
	if key == "" {
		key = defaultDungeonKey
	}
	build, ok := dungeonSpecs[key]
	if !ok {
		return key, tkenc.DungeonParams{}, fmt.Errorf("%w: %q", ErrUnknownDungeonKey, key)
	}
	return key, build(seed), nil
}

// --- Task E2: content-backed dungeon spec resolution (rpg-project#709) ---
//
// A content-hosted key (internal/content's embedded dungeons/*.yaml, plus
// any RPG_CONTENT_DIR override) resolves through this parallel path
// instead of the legacy dungeonSpecs map above — StartEncounter checks
// resolveContentDungeonSpec FIRST and only falls through to
// resolveDungeonSpec when the key isn't content-backed at all. The legacy
// crypt path (dungeonSpecs, resolveDungeonSpec, seedRegionMonsters) is
// untouched by this addition.

// DisabledDungeonKeyError means a content-backed dungeon key resolved to a
// FILE that exists but failed rpg-toolkit's dungeonspec.Load at startup —
// a broken content commit that slipped past CI, or a broken RPG_CONTENT_DIR
// dev-loop override file. Cause is the exact validation error content
// authoring needs to fix the file; lobbyStatusError (Task E3) maps this to
// codes.InvalidArgument, carrying that message to the caller.
type DisabledDungeonKeyError struct {
	Key   DungeonKey
	Cause error
}

func (e *DisabledDungeonKeyError) Error() string {
	return fmt.Sprintf("lobby orchestrator: dungeon key %q is disabled: %v", e.Key, e.Cause)
}

func (e *DisabledDungeonKeyError) Unwrap() error {
	return e.Cause
}

// LoadContentRegistry builds the startup *dungeonregistry.Registry of
// every content-hosted dungeon spec by calling content.AllSpecs() exactly
// ONCE. Exported so cmd/server/server.go can call it directly at startup
// (the Architecture decision in plan.md: the registry is built ONCE,
// outside either orchestrator, then passed by pointer into BOTH
// lobbyorch.Config.Registry and the new authoringorch.Config.Registry) —
// lobbyorch.New() itself no longer builds a registry; it requires one
// already built. Callers must invoke this BEFORE lobbyorch.New(): an
// unreadable RPG_CONTENT_DIR fails HERE, loudly, rather than silently
// degrading to embedded-only content — the same "operator/config mistake
// must fail construction" posture Task E2b already applies to
// RPG_DUNGEON_KEY, just moved to this new call site now that New() no
// longer touches RPG_CONTENT_DIR at all.
//
// A spec whose header decodes but whose body fails dungeonspec.Load (a
// schema/business-rule violation) is stored as a dungeonregistry.Entry
// carrying that Err, not dropped — see Entry's doc. Also calls
// dungeonspec.Decode (separately from Load, which discards the decoded
// Spec after compiling — CompiledDungeon carries no Name field) purely to
// capture each spec's declared display Name for Entry.Name, which
// ListDungeons needs (plan.md S1's Name-capture note: zero toolkit
// change, one extra already-public function call).
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
		// No "lobby orchestrator:" prefix here -- this function is no
		// longer lobby-exclusive infrastructure, and its caller
		// (cmd/server/server.go) wraps any error from it with its own
		// context, the same class of double-prefix bug Task E3's rider
		// fixed for validateDungeonKeyOverride.
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
		compiled, loadErr := dungeonspec.Load(raw)
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
		// package's belt-and-suspenders style elsewhere (compile.go's
		// nil-boss.At guard).
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

// resolveContentDungeonSpec looks up key in o.registry (Config.Registry,
// the shared live dungeon-spec registry). The three-value return is
// deliberate, not a bool: found reports whether key is a content-backed
// key AT ALL (callers fall through to the legacy dungeonSpecs map when
// false); when found is true, err distinguishes a spec that compiled
// cleanly (nil, compiled is ready to use) from one that failed to load (a
// *DisabledDungeonKeyError wrapping the stored cause) — the caller must
// fail immediately in that case, never silently fall through to the
// legacy path for a key that DOES exist in content but is broken. err is
// the LAST return (not the plan's own example order) to satisfy this
// repo's error-return lint convention — still three distinct signals,
// just Go-idiomatic positionally.
func (o *Orchestrator) resolveContentDungeonSpec(key DungeonKey) (compiled dungeonspec.CompiledDungeon, found bool, err error) {
	entry, ok := o.registry.Get(string(key))
	if !ok {
		return dungeonspec.CompiledDungeon{}, false, nil
	}
	if entry.Err != nil {
		return dungeonspec.CompiledDungeon{}, true, &DisabledDungeonKeyError{Key: key, Cause: entry.Err}
	}
	return entry.Compiled, true, nil
}

// validateDungeonKeyOverride checks that override (Config.DungeonKeyOverride,
// populated from the RPG_DUNGEON_KEY env var — Task E2b's dev-loop
// mechanism) resolves to a real, ENABLED dungeon spec before construction
// succeeds. "" (unset) is always valid — a no-op. A non-empty override
// must resolve via EITHER registry (the shared dungeonregistry.Registry
// Config.Registry supplies) OR the legacy dungeonSpecs map; a content key
// that resolves but is DISABLED (its file failed dungeonspec.Load) is
// rejected here too — an operator pointing RPG_DUNGEON_KEY at a broken key
// must see a construction failure immediately, not a deferred failure on
// the first real StartEncounter call.
//
// Error messages name RPG_DUNGEON_KEY (the env var an operator actually
// set), not Config.DungeonKeyOverride (the Go field name) — this is the
// error an operator hits during the M1 manual walkthrough. They also omit
// the "lobby orchestrator:" prefix other Config validation errors in this
// package carry: cmd/server/server.go's lobbyorch.New call site already
// wraps ANY error from New with fmt.Errorf("lobby orchestrator: %w", err),
// so including it here would double it up ("lobby orchestrator: lobby
// orchestrator: ...") for the one caller that actually sees this message.
func validateDungeonKeyOverride(override string, registry *dungeonregistry.Registry) error {
	if override == "" {
		return nil
	}
	key := DungeonKey(override)
	if entry, ok := registry.Get(string(key)); ok {
		if entry.Err != nil {
			return fmt.Errorf(
				"RPG_DUNGEON_KEY %q resolves to a disabled content key: %w",
				override, entry.Err)
		}
		return nil
	}
	if _, ok := dungeonSpecs[key]; ok {
		return nil
	}
	return fmt.Errorf(
		"RPG_DUNGEON_KEY %q does not resolve via content or the legacy dungeonSpecs map",
		override)
}
