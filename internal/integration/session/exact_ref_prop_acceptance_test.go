package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// TestAcceptance_AnExactRefPropReachesTheWireWhole is rpg-api#922: a ref is
// module:type:id, the id is everything after the second colon, and a prop
// whose id carries a part of its own survives the whole trip.
//
// The trip is the point. The ref is written in a file, compiled by the
// toolkit's dungeonspec, carried through sessionworld into a started run, and
// read back off GetAtlas by a joined player -- and it has to come out the far
// end byte-for-byte what the author wrote. Two things could break that and
// only one of them was fixed: the compiler used to REFUSE the four-part ref
// (rpg-toolkit#1536), and anything between here and the wire could still be
// splitting a ref and keeping a piece of it.
//
// So the assertion is equality against the authored literal rather than
// "contains plushie" or a count of props. A trimmed ref -- "skeleton-dog", or
// "dnd5e:props:plushie" -- is a prop no client can resolve to a model, and it
// would pass every weaker check written here.
func TestAcceptance_AnExactRefPropReachesTheWireWhole(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	// Compiled from authored YAML, because the compiler is half of what is
	// under test. A world built by hand here would place the prop by
	// assignment and prove nothing about the ref grammar.
	dungeon, err := sessionworld.Compile([]byte(dungeonstest.ExactRefPropsYAML))
	require.NoError(t, err, "a dungeon placing a four-part props ref must compile")

	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "exact-ref-run", Encounter: "toy-room", World: dungeon.World,
	})
	require.NoError(t, err, "and the run must start on it")

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "exact-ref-run", Member: "alice", Position: pbAt(0, 1),
	})
	require.NoError(t, err)

	atlas, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{
		Session: "exact-ref-run", Member: "alice",
	})
	require.NoError(t, err)

	refs := make([]string, 0, len(atlas.GetProps()))
	for _, p := range atlas.GetProps() {
		refs = append(refs, p.GetRef())
	}

	require.ElementsMatch(t,
		[]string{dungeonstest.ExactRefPropsRef, dungeonstest.ExactRefPropsPlainRef}, refs,
		"both props reach the wire, each carrying the ref the file wrote")
}
