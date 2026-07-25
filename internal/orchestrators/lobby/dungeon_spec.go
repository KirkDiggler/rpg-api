package lobby

import (
	"errors"
	"fmt"
	"log/slog"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/content"
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

// contentSpecResult stores either a successfully compiled content-backed
// dungeon spec, or the load failure that key's file produced at startup.
// A failed spec is not dropped from the registry: resolveContentDungeonSpec
// still reports its key as FOUND (so it never silently falls through to
// the legacy map), just disabled.
type contentSpecResult struct {
	compiled dungeonspec.CompiledDungeon
	err      error
}

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

// loadContentSpecs builds the startup registry of every content-hosted
// dungeon spec by calling content.AllSpecs() exactly ONCE (New calls this
// once at construction and stores the result on *Orchestrator —
// resolveContentDungeonSpec below is a plain map lookup against that
// stored result, never a second content.AllSpecs/SpecByKey call: those
// re-read RPG_CONTENT_DIR and re-parse every file on every call, which is
// fine for a one-time startup pass and wrong for a per-request path).
//
// A spec whose header decodes but whose body fails dungeonspec.Load (a
// schema/business-rule violation) is stored as a contentSpecResult
// carrying that error, not dropped — see contentSpecResult's doc. Returns
// an error only when content.AllSpecs() itself fails: RPG_CONTENT_DIR set
// but unreadable as a directory, an operator/config mistake that must
// fail lobbyorch.New() loudly (Task E2b's same validate-at-construction
// posture for RPG_DUNGEON_KEY) rather than silently falling back to
// embedded-only content.
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
func loadContentSpecs() (map[DungeonKey]contentSpecResult, error) {
	specs, problems, err := content.AllSpecs()
	if err != nil {
		return nil, fmt.Errorf("lobby orchestrator: load content specs: %w", err)
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

	results := make(map[DungeonKey]contentSpecResult, len(specs))
	for key, raw := range specs {
		compiled, loadErr := dungeonspec.Load(raw)
		if loadErr != nil {
			slog.Warn("lobby orchestrator: content spec failed to load, key disabled", "key", key, "error", loadErr)
			results[DungeonKey(key)] = contentSpecResult{err: loadErr}
			continue
		}
		results[DungeonKey(key)] = contentSpecResult{compiled: compiled}
	}
	return results, nil
}

// resolveContentDungeonSpec looks up key in the pre-built content registry
// (o.contentSpecs, built ONCE by loadContentSpecs at construction). The
// three-value return is deliberate, not a bool: found reports whether key
// is a content-backed key AT ALL (callers fall through to the legacy
// dungeonSpecs map when false); when found is true, err distinguishes a
// spec that compiled cleanly at startup (nil, compiled is ready to use)
// from one that failed to load (a *DisabledDungeonKeyError wrapping the
// stored cause) — the caller must fail immediately in that case, never
// silently fall through to the legacy path for a key that DOES exist in
// content but is broken. err is the LAST return (not the plan's own
// example order) to satisfy this repo's error-return lint convention —
// still three distinct signals, just Go-idiomatic positionally.
func (o *Orchestrator) resolveContentDungeonSpec(key DungeonKey) (compiled dungeonspec.CompiledDungeon, found bool, err error) {
	result, ok := o.contentSpecs[key]
	if !ok {
		return dungeonspec.CompiledDungeon{}, false, nil
	}
	if result.err != nil {
		return dungeonspec.CompiledDungeon{}, true, &DisabledDungeonKeyError{Key: key, Cause: result.err}
	}
	return result.compiled, true, nil
}

// validateDungeonKeyOverride checks that override (Config.DungeonKeyOverride,
// Task E2b's RPG_DUNGEON_KEY mechanism) resolves to a real, ENABLED
// dungeon spec before construction succeeds. "" (unset) is always valid —
// a no-op. A non-empty override must resolve via EITHER contentSpecs (the
// registry loadContentSpecs just built) OR the legacy dungeonSpecs map; a
// content key that resolves but is DISABLED (its file failed
// dungeonspec.Load) is rejected here too — an operator pointing
// RPG_DUNGEON_KEY at a broken key must see a construction failure
// immediately, not a deferred failure on the first real StartEncounter
// call.
func validateDungeonKeyOverride(override string, contentSpecs map[DungeonKey]contentSpecResult) error {
	if override == "" {
		return nil
	}
	key := DungeonKey(override)
	if result, ok := contentSpecs[key]; ok {
		if result.err != nil {
			return fmt.Errorf(
				"lobby orchestrator: Config.DungeonKeyOverride %q resolves to a disabled content key: %w",
				override, result.err)
		}
		return nil
	}
	if _, ok := dungeonSpecs[key]; ok {
		return nil
	}
	return fmt.Errorf(
		"lobby orchestrator: Config.DungeonKeyOverride %q does not resolve via content or the legacy dungeonSpecs map",
		override)
}
