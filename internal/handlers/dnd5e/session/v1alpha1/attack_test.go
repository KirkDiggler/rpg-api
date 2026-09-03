package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

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
