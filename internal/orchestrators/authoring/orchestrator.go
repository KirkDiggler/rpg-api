// Package authoring is the AuthoringService orchestrator: the dungeon
// builder's seam onto the content registry (rpg-api#806, rpg-project#256).
//
// It is thin on purpose. Compilation, validation and the write discipline
// live in internal/dungeons (and through it the toolkit); this package only
// shapes the registry's answers into the verbs the wire speaks. No rule
// lives here and no geometry is computed here.
package authoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/scenarios"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

// Config holds the dependencies for an Orchestrator.
type Config struct {
	// Dungeons is the content registry PutDungeon writes to and GetDungeon
	// reads from. Required.
	Dungeons dungeons.Registry
}

// Orchestrator is the AuthoringService's business logic.
type Orchestrator struct {
	dungeons dungeons.Registry
}

// New constructs an Orchestrator. Returns an error (never a nil
// Orchestrator) when a required dependency is missing.
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, errors.New("authoring orchestrator: Config is required")
	}
	if cfg.Dungeons == nil {
		return nil, errors.New("authoring orchestrator: Config.Dungeons is required")
	}

	return &Orchestrator{dungeons: cfg.Dungeons}, nil
}

// PutDungeonInput is one PutDungeon call.
type PutDungeonInput struct {
	Key          string
	YAML         []byte
	ValidateOnly bool
}

// PutDungeonOutput is PutDungeon's answer on a well-formed request: Errors
// (did not compile, nothing written) or Atlas (compiled; stored unless
// ValidateOnly). Exactly one of the two is meaningful; an empty Errors IS
// success.
type PutDungeonOutput struct {
	Errors []dungeons.FieldError

	// Atlas is the compiled map, the same shape GetAtlas serves.
	Atlas *sdk.Atlas
}

// PutDungeon compiles and, unless ValidateOnly, stores a dungeon. Registry
// sentinels (dungeons.ErrInvalidKey, ErrKeyMismatch, ErrAuthoringDisabled)
// pass through for the handler to map.
//
// ErrAuthoringDisabled belongs to the SAVE alone (rpg-project#481): a
// validate-only call is answered by a read-only registry, so the builder's
// per-edit preview works on a server that will not store a byte.
func (o *Orchestrator) PutDungeon(ctx context.Context, in *PutDungeonInput) (*PutDungeonOutput, error) {
	if in == nil {
		return nil, errors.New("authoring orchestrator: PutDungeonInput is required")
	}

	res, err := o.dungeons.Put(ctx, &dungeons.PutInput{
		Key: in.Key, YAML: in.YAML, ValidateOnly: in.ValidateOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("put dungeon: %w", err)
	}
	if len(res.Errors) > 0 {
		return &PutDungeonOutput{Errors: res.Errors}, nil
	}

	return &PutDungeonOutput{Atlas: res.Entry.Atlas}, nil
}

// GetDungeonInput names a stored dungeon.
type GetDungeonInput struct {
	Key string
}

// GetDungeonOutput is the stored file, verbatim.
type GetDungeonOutput struct {
	YAML []byte
}

// GetDungeon returns the stored file for a key. dungeons.ErrNotFound passes
// through for the handler to map.
func (o *Orchestrator) GetDungeon(ctx context.Context, in *GetDungeonInput) (*GetDungeonOutput, error) {
	if in == nil {
		return nil, errors.New("authoring orchestrator: GetDungeonInput is required")
	}

	entry, err := o.dungeons.Get(ctx, in.Key)
	if err != nil {
		return nil, fmt.Errorf("get dungeon: %w", err)
	}

	return &GetDungeonOutput{YAML: entry.YAML}, nil
}

// ListScenariosInput asks for the whole registry. No fields, and none are
// coming: the set of scenarios is a property of the SERVER'S rulebook build,
// not of any one dungeon, so there is nothing here to scope it by. Kept as a
// type anyway, because every verb at every layer of this repo takes one.
type ListScenariosInput struct{}

// ListScenariosOutput is every scenario this build's rulebook offers, in the
// order the toolkit sorts them.
//
// THE TOOLKIT'S OWN TYPE, not a mirror of it. A scenario's descriptor is
// CONTENT -- the field keys its constructor validates and the refusal
// sentences that constructor uses -- and an rpg-api struct in the middle
// would be a second copy of words nobody here wrote, free to drift from the
// ones the builder actually has to satisfy. So the one conversion happens at
// the handler/proto boundary, where every other toolkit type's does.
type ListScenariosOutput struct {
	Scenarios []scenarios.Scenario
}

// ListScenarios reports every scenario a dungeon may be bound to.
//
// UNGATED, on GetDungeon's precedent and for GetDungeon's reason: it reads
// and mutates nothing. It does not even reach the registry -- the answer is a
// property of the binary, so a build with no content directory at all still
// has one. PutDungeon's SAVE keeps its own gate; its validate-only grade does
// not (rpg-project#481).
//
// Empty is legal and means this build offers none; the builder shows no
// scenario panel rather than an error, because a dungeon with no scenario
// bound is a perfectly good dungeon -- which is every dungeon shipped before
// this one.
func (o *Orchestrator) ListScenarios(_ context.Context, in *ListScenariosInput) (*ListScenariosOutput, error) {
	if in == nil {
		return nil, errors.New("authoring orchestrator: ListScenariosInput is required")
	}

	return &ListScenariosOutput{Scenarios: scenarios.All()}, nil
}

// ListWeaponsInput asks for the whole catalog. No fields, and none are
// coming, for ListScenariosInput's reason: the set of weapons is a property
// of the SERVER'S rulebook build, not of any one dungeon and not of any one
// monster -- a builder that has placed nothing yet still needs the list to
// show what a monster could be armed with. Kept as a type anyway, because
// every verb at every layer of this repo takes one.
type ListWeaponsInput struct{}

// WeaponDescriptor is one weapon in the form the builder's action palette
// needs: what the author writes, what the chip shows, and enough to group by
// without reading the rulebook.
//
// AN rpg-api TYPE, where ListScenariosOutput deliberately carries the
// toolkit's own -- and the one difference is the ref. A scenario descriptor
// is one rulebook object's words handed through; a weapon descriptor is a
// JOIN of two rulebook facts, the catalog entry and the refs namespace that
// names it, and a join has a failure the objects do not (see ListWeapons).
// Making the join here keeps the handler exactly what ListScenarios' handler
// is -- a field-for-field copy with no lookup and no failure of its own.
// Every word on this struct is still the rulebook's; none was written here.
//
// It is deliberately NOT the weapon: no dice, no range, no properties. The
// palette does not compute an attack -- the toolkit assembles one at spawn
// from the wielding monster's own scores (design rpg-project#448, decision
// 1), which is why a skeleton shortbow and a goblin shortbow are the same
// weapon. Numbers here would be a second copy of the rulebook's.
type WeaponDescriptor struct {
	// Ref is the FULL ref, e.g. "dnd5e:weapons:shortbow" -- the exact string
	// a placement's `actions:` list carries, so the palette emits it verbatim
	// and no client ever rebuilds an id from its parts.
	Ref string

	// Name is the rulebook's author-facing name, for the chip's label.
	Name string

	// Ranged is the rulebook's own predicate (weapons.Weapon.IsRanged), not a
	// reading of Category. The day the rulebook spells a ranged category some
	// other way, this stays true and a substring hunt would not.
	Ranged bool

	// Category is the rulebook's category word, verbatim and opaque, for
	// grouping and for showing. Nothing branches on it.
	Category string
}

// ListWeaponsOutput is every weapon this build's rulebook can arm a placed
// monster with, in the rulebook's own presentation order.
type ListWeaponsOutput struct {
	Weapons []WeaponDescriptor
}

// ListWeapons reports the arming catalog for the builder's action palette
// (rpg-project#448).
//
// UNGATED, on GetDungeon's precedent and for ListScenarios' reason: it reads
// and mutates nothing, and never reaches the registry -- the answer is a
// property of the binary.
//
// THE RULEBOOK'S OWN ACCESSORS DECIDE WHAT IS IN THE LIST. Simple then
// martial, each in weapons.GetByCategory's registry order, which is why the
// answer is grouped by category and stable across calls -- the catalog's own
// map is never ranged over here. That also settles the special weapons: the
// unarmed strike is excluded because the rulebook excludes it from its
// category accessors as "not equippable", and arming a placement with it is
// not a thing an author reaches the palette for. No filter is written here;
// the exclusion is inherited, so a rulebook that decides otherwise changes
// this list without changing this file.
//
// A weapon the catalog offers but the refs namespace cannot name FAILS THE
// WHOLE CALL rather than traveling with a blank ref. It is a producer defect
// by construction -- both halves are the rulebook's -- and a palette chip
// that emitted an empty string would write a file the compiler refuses,
// which is a worse thing to learn later.
//
// Empty is legal and means this build offers none; the builder shows no
// weapon palette rather than an error, because a monster with no authored
// actions falls back to its definition's own -- which is every monster
// shipped before this one.
func (o *Orchestrator) ListWeapons(_ context.Context, in *ListWeaponsInput) (*ListWeaponsOutput, error) {
	if in == nil {
		return nil, errors.New("authoring orchestrator: ListWeaponsInput is required")
	}

	simple := weapons.GetSimpleWeapons()
	martial := weapons.GetMartialWeapons()

	out := make([]WeaponDescriptor, 0, len(simple)+len(martial))
	for _, w := range append(simple, martial...) {
		ref := refs.Weapons.ByID(w.ID)
		if ref == nil {
			return nil, fmt.Errorf(
				"list weapons: the rulebook's catalog offers %q but its refs namespace does not name it", w.ID)
		}

		out = append(out, WeaponDescriptor{
			Ref:      ref.String(),
			Name:     w.Name,
			Ranged:   w.IsRanged(),
			Category: string(w.Category),
		})
	}

	return &ListWeaponsOutput{Weapons: out}, nil
}
