package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestAttack_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Attack(context.Background(), &sessionpb.AttackRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestAttack_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Attack(gomock.Any(), &sdk.AttackInput{
		Session: "sess-1", Attacker: "char-1", Target: "goblin-1", DeclarationID: "decl-attack-1",
	}).Return(&sdk.AttackOutput{
		Roll: 18, Total: 21, Against: 13, Hit: true, Damage: 7, Seq: 9,
		Attack: sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
		Calculation: &sdk.RollCalculation{Total: 21, Components: []sdk.RollComponent{{
			Source:       sdk.RollSource{Ref: "dnd5e:spells:bane", Name: "Bane", SourceID: "bard-1"},
			Dice:         &sdk.DiceTrace{Notation: "1d4", DieSize: 4, OriginalRolls: []int{2}, FinalRolls: []int{2}, Subtotal: 2},
			SubtractDice: true,
		}}},
		PresentationID: "presentation_2f1c8b4a-0d6e-4a1b-9c3f-5e7a1b2c3d4e",
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Attack(ctx, &sessionpb.AttackRequest{
		Session: "sess-1", Attacker: "char-1", Target: "goblin-1", DeclarationId: "decl-attack-1",
	})
	require.NoError(t, err)
	require.True(t, resp.GetHit())
	require.Equal(t, int32(7), resp.GetDamage())

	// The beat line's "with a longsword ... 6 slashing" comes from here --
	// weapon identity the seam dropped since the first swing (rpg-toolkit#866).
	require.Equal(t, "dnd5e:weapons:longsword", resp.GetAttack().GetRef())
	require.Equal(t, "Longsword", resp.GetAttack().GetName())
	require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_SLASHING, resp.GetAttack().GetDamageType())
	require.Equal(t, int32(21), resp.GetCalculation().GetTotal())
	require.True(t, resp.GetCalculation().GetComponents()[0].GetSubtractDice())
	require.Equal(t, "bard-1", resp.GetCalculation().GetComponents()[0].GetSource().GetSourceId())

	// The attacker's half of the shared roll identity. Seq is per recipient, so
	// this token is the only thing the attacker and a witness can both name
	// this swing by -- the witness reads the same value off Struck/Missed.
	require.Equal(t, "presentation_2f1c8b4a-0d6e-4a1b-9c3f-5e7a1b2c3d4e", resp.GetPresentationId())
}

// TestAttack_Paused_ReturnsRollAndTotalOnly pins rpg-api#985: a swing that
// stopped to ask the attacker something (Bardic Inspiration) reaches the
// wire with paused true and only Roll/Total as answers -- mirroring the
// toolkit's own paused AttackOutput field-for-field, including which
// fields it leaves at their zero value, rather than this handler inventing
// a shape of its own.
func TestAttack_Paused_ReturnsRollAndTotalOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Attack(gomock.Any(), &sdk.AttackInput{
		Session: "sess-1", Attacker: "char-1", Target: "goblin-1", DeclarationID: "decl-attack-1",
	}).Return(&sdk.AttackOutput{
		Paused: true, Roll: 14, Total: 17, Seq: 4,
		Attack:         sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
		PresentationID: "presentation_paused-1",
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Attack(ctx, &sessionpb.AttackRequest{
		Session: "sess-1", Attacker: "char-1", Target: "goblin-1", DeclarationId: "decl-attack-1",
	})
	require.NoError(t, err)

	require.True(t, resp.GetPaused())
	require.Equal(t, int32(14), resp.GetRoll())
	require.Equal(t, int32(17), resp.GetTotal())

	// Nothing has landed and the AC has deliberately not been shown -- the
	// toolkit's own AttackOutput leaves these at zero when Paused, and this
	// handler must not fill them in.
	require.False(t, resp.GetHit())
	require.False(t, resp.GetCritical())
	require.Zero(t, resp.GetDamage())
	require.Zero(t, resp.GetAgainst())
}

func TestAttack_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Attack(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotACharacter)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Attack(ctx, &sessionpb.AttackRequest{Session: "sess-1", Attacker: "goblin-1", Target: "char-1"})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestAttack_NotATarget_IsAWorldRefusalNotAnInternalError pins the refusal
// rpg-project#493 R4 brought to this verb: the swing named a placed world
// NPC, and a merchant is not a thing you attack.
//
// FAILED_PRECONDITION, AND THE CODE IS THE POINT. The request is well-formed
// and names real things: a real session, a real attacker, a target the roster
// carries and the map draws. It is the KIND of that target that refuses the
// swing, exactly as ErrNotAVendor refuses a trade with a real, visible NPC.
// Internal would tell a client this server broke over an ordinary authoring
// fact, and NotFound would lie about a member the player can see standing
// there.
//
// SEPARATE FROM ErrStaleDeclaration ON PURPOSE, which is the sentence this
// used to answer. Stale tells a host to re-read the offers and try again;
// re-reading answers the same thing forever, because a world NPC is never in
// the SDK's candidate universe. The one thing that changes the answer is
// authoring the creature as a monster with a disposition, which is what the
// SDK's own message says. This test is what stops the two being folded back
// together at this seam.
func TestAttack_NotATarget_IsAWorldRefusalNotAnInternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Attack(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotATarget)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Attack(ctx, &sessionpb.AttackRequest{
		Session: "sess-1", Attacker: "char-1", Target: "merchant-1",
	})
	requireCode(t, err, codes.FailedPrecondition)

	// The SDK's sentence is the whole remedy -- "author it as a monster to
	// make it a target" -- so the message has to survive the translation
	// rather than be replaced by a generic one. A client that showed only
	// the code would send a builder looking for a rule that does not exist.
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Contains(t, st.Message(), sdk.ErrNotATarget.Error())
}
