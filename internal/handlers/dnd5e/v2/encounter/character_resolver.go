package encounter

import (
	"strings"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// CharacterResolver bridges the toolkit's encounter.CharacterResolver to
// rpg-api's character store so SubmitCheck can total a roll with the player's
// ability modifier (and tool-proficiency bonus, if any).
//
// Wave 2.9 ships a stub implementation (StubCharacterResolver) that reports
// every modifier as zero. This unblocks the SubmitCheck wire flow without
// coupling the encounter handler to the character orchestrator yet — sourcing
// real ability modifiers requires a player→character lookup that does not
// exist on the encounter today (PlayerData.EntityID currently mirrors PlayerID
// rather than referencing a character).
//
// Tracking issue for real lookups: see rpg-api follow-up filed alongside #514.
//
// The interface alias here exists so the rest of the handler package can
// reference a single name regardless of which implementation is wired.
type CharacterResolver = tkenc.CharacterResolver

// StubCharacterResolver returns (0, true) for every modifier and proficiency
// query. It satisfies tkenc.CharacterResolver so SubmitCheck can resolve
// rolls (total = roll + 0 + 0) until the real character store bridge ships.
//
// "ok=true" is intentional: the toolkit treats ok=false as "unknown player,
// modifier is zero anyway" — but the orchestrator wants the reported total
// to be deterministic during wire-shape verification, so we say "yes I know
// this player, the modifier is zero" until real data flows.
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
