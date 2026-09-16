// Package sessionv1alpha1 is the wire form of the toolkit's
// rulebooks/dnd5e/session SDK: pure proto <-> SDK translation, per
// rpg-project/ideas/session-api/design.md §3. No rule lives here (design
// rule 8, the Boundary Rule) -- every handler loads nothing, decides
// nothing, and calls exactly one Manager verb.
package sessionv1alpha1
