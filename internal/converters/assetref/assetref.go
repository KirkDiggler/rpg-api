// Package assetref mints the identity a client keys a model off.
//
// This namespace is NOT a rules identity and the toolkit does not own it. The
// rulebook keeps two namespaces apart because the rules care about the
// difference — a shield is `dnd5e:armor:shield` to armour class, a longsword is
// `dnd5e:weapons:longsword` to an attack — and the asset pipeline flattens both
// into one `item`, because the thing that puts a model in a hand does not care
// which rule the object feeds.
//
// That flattening is a real translation with a real job, and this package is
// where it lives. It is matched by the rpg-game-assets provider manifest that
// ships with the web client, which is the authority for what ids exist; adding
// an id here that the manifest does not carry renders nothing.
//
// The toolkit hands this layer BARE ITEM IDS ("longsword") and never learns
// what happens to them. rpg-api is the side that knows both the rulebook id and
// the provider manifest, so rpg-api is the side that mints.
package assetref

// Module and TypeItem are the first two segments of every asset identity this
// package mints. They are constants in one place on purpose: a second copy is
// how two seams start disagreeing about what a shield is called.
const (
	// Module is the rulebook these assets belong to.
	Module = "dnd5e"

	// TypeItem is the asset pipeline's single equipment type, flattening the
	// rulebook's weapons and armor namespaces. See the package doc.
	TypeItem = "item"
)

// Item formats a bare item id as the asset identity a client keys a model off,
// for example "longsword" to "dnd5e:item:longsword".
//
// AN EMPTY ID STAYS EMPTY. A hand observed holding nothing is a positive fact,
// and "dnd5e:item:" is not a thing anybody can render — it would turn "I saw
// empty hands" into a lookup miss, which reads to a client as "we don't know".
// Those are the two claims this whole seam exists to keep apart.
func Item(id string) string {
	if id == "" {
		return ""
	}
	return Module + ":" + TypeItem + ":" + id
}
