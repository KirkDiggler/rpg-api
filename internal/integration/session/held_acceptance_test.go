package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// The hold (the-mad-king.yaml's `held`, rpg-project#990), given by a
// shoulder-past line on the kitchen's cook as the castle's hall guard gives
// it.
const (
	pushPastLine = "push-past"
	heldName     = "Held"
	heldSentence = "You are Held; you cannot move."
)

// TestAcceptance_AHeldCharacterCannotMoveAndAFightFreesThem is rpg-project#990
// through the api's own doors, on the small kitchen site: alice fails to
// shoulder past the cook (every d20 lands 1) and is "Held" on her own sheet;
// a step is refused FAILED_PRECONDITION with that sentence alone, and she has not
// moved. She attacks the cook, a fight forms, and "Held" is gone.
func TestAcceptance_AHeldCharacterCannotMoveAndAFightFreesThem(t *testing.T) {
	const run = "held-run"
	ctx := auth.WithPlayerID(context.Background(), statusPlayer)
	dice := &gateDice{}
	dice.face.Store(1)
	h := newAcceptanceHarnessWithDice(t, dice)
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", statusPlayer)},
	})
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(kitchenWithCookLines(t, `          - id: push-past
            say: (shoulder past him)
            check:
              - ability: athletics
                dc: 12
            success:
              - say: Oi! Mind where you’re going.
            failure:
              - say: Not so fast. You stay right there.
                apply_status: held
`))
	require.NoError(t, err)
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 0, 1))
	require.Empty(t, out.Formed, "precondition: nobody fights")
	sheet, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
	require.NoError(t, err)
	require.Zero(t, rowsNamed(aliceSheet(t, sheet), heldName), "precondition: alice is not held")

	_, err = sayTo(t, h, run, "alice", statusPlayer, kitchenCook, pushPastLine)
	require.NoError(t, err, "alice tries to shoulder past")
	require.Equal(t, 1, rowsNamed(aliceSheet(t, sheet), heldName), "she failed: Held, on her own sheet")

	before := whereIs(t, h, run, "alice")
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: []*sessionpb.Position{pbAt(-1, 1)}})
	st, ok := status.FromError(err)
	require.True(t, ok, "a refused step is a gRPC status: %v", err)
	require.Equal(t, codes.FailedPrecondition, st.Code())
	require.Equal(t, heldSentence, st.Message(), "the hold's sentence alone")
	require.Equal(t, before, whereIs(t, h, run, "alice"), "she has not moved")

	_, err = h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: run, Attacker: "alice", Target: kitchenCook,
		DeclarationId: currentDeclarationID(ctx, t, h.handler, run, "alice", sessionpb.Verb_VERB_ATTACK),
	})
	require.NoError(t, err, "alice attacks the cook")
	turn, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: run, Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_TURN, turn.GetClock(), "a fight formed: alice is on a turn clock")
	require.Zero(t, rowsNamed(aliceSheet(t, sheet), heldName), "the fight starting freed her")
}
