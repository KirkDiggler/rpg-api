// Package levelfixture provides shared character level records for hand-built
// tkcharacter.Data test fixtures across rpg-api's test suites.
//
// rpg-toolkit#1766 gave a character an append-only record of the levels it has
// taken, and made that record the source of truth for its level. A stored
// sheet that claims a level above 1 with no record behind it is refused at
// load, because the history of a higher level cannot be guessed (design
// R2.7) — only a level-1 character predates the record and gets one
// synthesized (R2.6). Every fixture here builds a level-3 or level-5 sheet by
// hand, so every one of them needs a record to keep loading, the same way
// [monsterfixture] exists because a monster fixture needs real DataJSON.
//
// Real characters never call this: they get their record from
// Draft.ToCharacter at creation and from Character.Advance on the way up.
package levelfixture

import (
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
)

// Synthetic returns a record of n levels taken in one class.
//
// The hit point gain is left at zero. These are fixtures for tests that care
// about the level, not about how the character's hit points got there, and
// every one of them states its own hit points; a number invented here would
// be a second, quieter claim about the same sheet.
func Synthetic(class classes.Class, n int) []tkcharacter.LevelEntry {
	if n <= 0 {
		return nil
	}

	entries := make([]tkcharacter.LevelEntry, 0, n)
	for i := 1; i <= n; i++ {
		entries = append(entries, tkcharacter.LevelEntry{
			Level:          i,
			ClassID:        class,
			HitPointMethod: tkcharacter.HitPointMethodAverage,
		})
	}
	return entries
}

// Reclass re-dresses a fixture as another class, record and all.
//
// Every entry in the record is taken in the character's own class, and an
// entry naming a different one is refused at load — that refusal is the seam
// multiclassing will open (design R2.4). A fixture that reuses one class's
// stored sheet as another must re-take its levels in the new class, or it
// stops loading.
func Reclass(data *tkcharacter.Data, class classes.Class) {
	data.ClassID = class
	data.Levels = Synthetic(class, data.Level)
}
