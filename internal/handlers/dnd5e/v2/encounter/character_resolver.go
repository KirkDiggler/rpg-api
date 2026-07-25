package encounter

import (
	"context"
	"strings"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// CharacterResolver bridges the toolkit's encounter.CharacterResolver to
// rpg-api's character store so SubmitCheck can total a roll with the player's
// ability modifier (and tool-proficiency bonus, if any).
//
// Wave 2.9 shipped a stub implementation (StubCharacterResolver) that reports
// every modifier as zero, to unblock the SubmitCheck wire flow without
// coupling the encounter handler to the character store yet. rpg-api#516
// replaces it in production with Dnd5eCharacterResolver (below) — the stub
// remains the zero-modifier default for tests and the integration harness.
//
// The interface alias here exists so the rest of the handler package can
// reference a single name regardless of which implementation is wired.
type CharacterResolver = tkenc.CharacterResolver

// StubCharacterResolver returns (0, true) for every modifier and proficiency
// query. It satisfies tkenc.CharacterResolver so SubmitCheck can resolve
// rolls (total = roll + 0 + 0) deterministically — the default for tests and
// the integration harness (internal/integration/harness/harness.go), which
// want stable wire-shape totals rather than real character data.
//
// "ok=true" is intentional: the toolkit treats ok=false as "unknown player,
// modifier is zero anyway" — but callers of this stub want the reported total
// to be deterministic, so it says "yes I know this player, the modifier is
// zero" unconditionally.
type StubCharacterResolver struct{}

// AbilityModifier always returns 0, true. Ability is normalized to upper-case
// for forward compatibility with both door-construction conventions ("DEX"
// and "dex"); the stub doesn't act on the value but keeps the contract
// compatible with any future real implementation that does.
func (StubCharacterResolver) AbilityModifier(_ core.PlayerID, ability string) (int, bool) {
	_ = strings.ToUpper(ability)
	return 0, true
}

// ToolProficiencyBonus always returns 0, true. Tool refs are passed through
// untouched.
func (StubCharacterResolver) ToolProficiencyBonus(_ core.PlayerID, _ string) (int, bool) {
	return 0, true
}

// Dnd5eCharacterResolverConfig holds the dependencies needed to construct a
// Dnd5eCharacterResolver.
type Dnd5eCharacterResolverConfig struct {
	// CharacterRepo provides character lookup by ID. Required for real
	// ability/tool-proficiency resolution; a nil repo degrades every lookup
	// to ok=false (the toolkit treats that as a zero modifier — see
	// tkenc.CharacterResolver's doc), matching how callers without a wired
	// character store behave today.
	CharacterRepo characterrepo.Repository
}

// Dnd5eCharacterResolver implements tkenc.CharacterResolver against the
// dnd5e rulebook's stored character data.
//
// # Why this is built per-request (NewDnd5eCharacterResolverForData), not once
//
// The toolkit's CharacterResolver interface hands AbilityModifier /
// ToolProficiencyBonus only a core.PlayerID — no encounter ID, no EntityID
// (encounter/prompts.go's CharacterResolver doc). A player can own more than
// one character (character repo's ListByPlayerID), so PlayerID alone cannot
// unambiguously name "the" character; the only reliable mapping is the
// specific encounter's own data.Players[playerID].EntityID — exactly the
// field start_encounter.go already populates with the real character ID
// (EntityID: core.EntityID(m.CharacterID)), not a mirror of PlayerID as an
// earlier doc comment on this file assumed. Reading that mapping requires
// the loaded *tkenc.Data, so the resolver is constructed fresh per request
// from it, exactly like Dnd5eCombatResolver / Dnd5eMovementResolver
// (NewDnd5eCombatResolverForData / NewDnd5eMovementResolverForData) — do not
// share one instance across requests/encounters.
type Dnd5eCharacterResolver struct {
	cfg  Dnd5eCharacterResolverConfig
	data *tkenc.Data
}

// NewDnd5eCharacterResolverForData constructs a resolver bound to the given
// encounter data. data supplies the PlayerID -> EntityID (character ID)
// mapping for this encounter; nil data (e.g. the lobby orchestrator's
// StartEncounter, which builds resolvers before any player is seated) is
// handled by every lookup returning ok=false.
func NewDnd5eCharacterResolverForData(cfg Dnd5eCharacterResolverConfig, data *tkenc.Data) *Dnd5eCharacterResolver {
	return &Dnd5eCharacterResolver{cfg: cfg, data: data}
}

// AbilityModifier implements tkenc.CharacterResolver. It resolves playerID to
// its bound character (via data.Players[playerID].EntityID) and returns the
// toolkit's own shared.AbilityScores.Modifier for the named ability — this
// package never computes (score-10)/2 itself (rpg-api CLAUDE.md "Code Smell:
// Game Logic in the API"; the modifier formula is rpg-toolkit's rule, not
// rpg-api's).
//
// ability is matched case-insensitively: the toolkit's own DoorData.LockAbility
// doc documents 3-letter upper-case codes ("DEX") while rpg-api's crypt door
// construction stores lower-case ("dex") — both conventions appear in this
// codebase, so the ability string is lower-cased before the toolkit's
// abilities.GetByID lookup (whose canonical keys are lower-case: abilities.DEX
// == "dex").
//
// Returns ok=false — treated as a zero modifier by the toolkit's SubmitCheck
// (prompts.go) — for an unseated player, a player with no bound character, a
// character-store miss, or an unrecognized ability string. These are all
// "unknown" in the sense the interface documents, never a fabricated zero.
func (r *Dnd5eCharacterResolver) AbilityModifier(playerID core.PlayerID, ability string) (int, bool) {
	charData, ok := r.characterData(playerID)
	if !ok {
		return 0, false
	}

	abilityID, err := abilities.GetByID(strings.ToLower(ability))
	if err != nil {
		return 0, false
	}

	return charData.AbilityScores.Modifier(abilityID), true
}

// ToolProficiencyBonus implements tkenc.CharacterResolver. It resolves
// playerID to its bound character and reports the character's stored
// ProficiencyBonus when the character is proficient with tool, else a known
// zero (ok=true — a real character who simply lacks the proficiency, not an
// unknown answer). Proficiency bonus is a stored field on tkcharacter.Data,
// never derived from level here (that derivation, if ever needed, is a
// toolkit rule, not rpg-api's).
//
// tool arrives here as door.LockTool verbatim (rpg-toolkit's AttemptUnlock
// sets PendingPrompt.Tool = door.LockTool; SubmitCheck passes prompt.Tool
// straight through to this method) — a full toolkit ref such as
// "dnd5e:item:thieves-tools" (rpg-toolkit/encounter/data.go's LockTool doc),
// not the bare id tkcharacter.Data.ToolProficiencies stores
// (proficiencies.ToolThieves == "thieves-tools"). toolRefID normalizes tool
// to its bare id before comparing; a tool that isn't ref-shaped (splitRef
// returns nil) is compared as-is, since nothing in the interface guarantees
// the ref form.
//
// Returns ok=false for an unseated player, a player with no bound character,
// or a character-store miss — the same "unknown" cases AbilityModifier uses.
func (r *Dnd5eCharacterResolver) ToolProficiencyBonus(playerID core.PlayerID, tool string) (int, bool) {
	charData, ok := r.characterData(playerID)
	if !ok {
		return 0, false
	}

	toolID := toolRefID(tool)
	for _, prof := range charData.ToolProficiencies {
		if string(prof) == toolID {
			return charData.ProficiencyBonus, true
		}
	}
	return 0, true
}

// toolRefID extracts the bare id segment from a toolkit ref
// ("module:type:id" -> "id", e.g. "dnd5e:item:thieves-tools" ->
// "thieves-tools"), reusing this package's existing splitRef helper
// (translate.go) so ref-parsing semantics stay consistent with the rest of
// the handler package (projection, event translation, prompt translation
// all already use it). A ref that doesn't parse to exactly module:type:id
// (splitRef's documented nil return) is returned unmodified, so a bare id
// passed directly still compares correctly.
func toolRefID(ref string) string {
	if parts := splitRef(ref); parts != nil {
		return parts[2]
	}
	return ref
}

// characterData resolves playerID to the character store's Data for the
// encounter-bound character (data.Players[playerID].EntityID). Every failure
// mode — no repo wired, no data wired, player not seated, no bound
// character, or a character-store miss/error — returns ok=false, which both
// exported methods above translate into the toolkit's documented "unknown,
// treat as zero" contract rather than a panic or a fabricated value.
func (r *Dnd5eCharacterResolver) characterData(playerID core.PlayerID) (*tkcharacter.Data, bool) {
	if r.cfg.CharacterRepo == nil || r.data == nil {
		return nil, false
	}

	pd, ok := r.data.Players[playerID]
	if !ok || pd.EntityID == "" {
		return nil, false
	}

	out, err := r.cfg.CharacterRepo.Get(context.Background(), characterrepo.GetInput{ID: string(pd.EntityID)})
	if err != nil || out == nil || out.Character == nil || out.Character.Data == nil {
		return nil, false
	}
	return out.Character.Data, true
}
