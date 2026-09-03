package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
)

// TestEveryMemberTakingVerbRefusesAForeignMember is the pin for the entitlement
// gate, and it is written as one table over ALL of them on purpose.
//
// The bug it exists to prevent was not a wrong check; it was a MISSING one, in
// eleven of the twelve handlers that take a member. A per-verb test would have
// been added next to the verb that got fixed and would have said nothing about
// the ten that did not. Anyone adding a verb to this service will find this
// table failing until they route it through callerActingAs, which is the only
// way a gate like this stays whole.
//
// Every case gives the manager a mock with NO expectations. That is the real
// assertion: gomock fails on any unexpected call, so a verb that reached the
// SDK before checking ownership fails here even if it later returned an error
// for some other reason. Refusing afterwards is not refusing -- Move would have
// already moved.
func TestEveryMemberTakingVerbRefusesAForeignMember(t *testing.T) {
	const (
		caller  = "alice"
		foreign = "goblin-1" // learnable from a story beat; owned by nobody alice controls
		owner   = "someone-else"
	)

	for name, call := range memberTakingVerbs(foreign) {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			h := &Handler{
				manager:    sessionv1alpha1mock.NewMockManager(ctrl),
				characters: ownedCharacterRepo(ctrl, foreign, owner),
				roster:     testRoster(),
			}
			err := call(auth.WithPlayerID(context.Background(), caller), h)
			requireCode(t, err, codes.PermissionDenied)
		})
	}
}

func TestEveryMemberTakingVerbRefusesAMissingMember(t *testing.T) {
	const (
		caller = "alice"
		member = "missing-char"
	)

	for name, call := range memberTakingVerbs(member) {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			manager := sessionv1alpha1mock.NewMockManager(ctrl)
			characters := charactermock.NewMockRepository(ctrl)
			characters.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: member}).Return(nil, apierr.NotFound("missing")).AnyTimes()

			h, err := New(&HandlerConfig{
				Manager:    manager,
				Broker:     sessionorch.NewBroker(),
				Characters: characters,
				Roster:     rosterrepo.NewInMemory(),
			})
			require.NoError(t, err)

			err = call(auth.WithPlayerID(context.Background(), caller), h)
			requireCode(t, err, codes.NotFound)
		})
	}
}

func memberTakingVerbs(member string) map[string]func(ctx context.Context, h *Handler) error {
	return map[string]func(ctx context.Context, h *Handler) error{
		"Join": func(ctx context.Context, h *Handler) error {
			_, err := h.Join(ctx, &sessionpb.JoinRequest{Session: "sess-1", Member: member})
			return err
		},
		"Exit": func(ctx context.Context, h *Handler) error {
			_, err := h.Exit(ctx, &sessionpb.ExitRequest{Session: "sess-1", Member: member})
			return err
		},
		"Move": func(ctx context.Context, h *Handler) error {
			_, err := h.Move(ctx, &sessionpb.MoveRequest{
				Session: "sess-1", Member: member, DeclarationId: "decl-move-1",
				Path: []*sessionpb.Position{{X: 1, Y: 1}},
			})
			return err
		},
		"Attack": func(ctx context.Context, h *Handler) error {
			_, err := h.Attack(ctx, &sessionpb.AttackRequest{
				Session: "sess-1", Attacker: member, Target: "char-1", DeclarationId: "decl-attack-1",
			})
			return err
		},
		"DeathSave": func(ctx context.Context, h *Handler) error {
			_, err := h.DeathSave(ctx, &sessionpb.DeathSaveRequest{
				Session: "sess-1", Member: member, DeclarationId: "decl-save-1",
			})
			return err
		},
		"Afford": func(ctx context.Context, h *Handler) error {
			_, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: member})
			return err
		},
		"Turn": func(ctx context.Context, h *Handler) error {
			_, err := h.Turn(ctx, &sessionpb.TurnRequest{Session: "sess-1", Member: member})
			return err
		},
		"Activate": func(ctx context.Context, h *Handler) error {
			_, err := h.Activate(ctx, &sessionpb.ActivateRequest{
				Session: "sess-1", Member: member, DeclarationId: "decl-activate-1",
			})
			return err
		},
		"EndTurn": func(ctx context.Context, h *Handler) error {
			_, err := h.EndTurn(ctx, &sessionpb.EndTurnRequest{
				Session: "sess-1", Member: member, DeclarationId: "decl-end-1",
			})
			return err
		},
		"Dissolve": func(ctx context.Context, h *Handler) error {
			_, err := h.Dissolve(ctx, &sessionpb.DissolveRequest{
				Session: "sess-1", Member: member,
				Cause: sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION,
			})
			return err
		},
		"GetStory": func(ctx context.Context, h *Handler) error {
			_, err := h.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: member})
			return err
		},
		"GetView": func(ctx context.Context, h *Handler) error {
			_, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess-1", Member: member})
			return err
		},
		"GetWhere": func(ctx context.Context, h *Handler) error {
			_, err := h.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: "sess-1", Member: member})
			return err
		},
		"StreamEvents": func(ctx context.Context, h *Handler) error {
			return h.StreamEvents(
				&sessionpb.StreamEventsRequest{Session: "sess-1", Member: member},
				newCapturingStream(ctx),
			)
		},
	}
}

// TestEveryMemberTakingVerbRefusesAnEmptyMember pins the other half of the
// gate. An unnamed member cannot be one this caller owns, so the ownership
// question has no meaning until the field is filled in -- and StreamEvents in
// particular would otherwise subscribe under an empty key and hang.
func TestEveryMemberTakingVerbRefusesAnEmptyMember(t *testing.T) {
	verbs := map[string]func(ctx context.Context, h *Handler) error{
		"Join": func(ctx context.Context, h *Handler) error {
			_, err := h.Join(ctx, &sessionpb.JoinRequest{Session: "sess-1"})
			return err
		},
		"Move": func(ctx context.Context, h *Handler) error {
			_, err := h.Move(ctx, &sessionpb.MoveRequest{Session: "sess-1"})
			return err
		},
		"Attack": func(ctx context.Context, h *Handler) error {
			_, err := h.Attack(ctx, &sessionpb.AttackRequest{Session: "sess-1", Target: "char-1"})
			return err
		},
		"DeathSave": func(ctx context.Context, h *Handler) error {
			_, err := h.DeathSave(ctx, &sessionpb.DeathSaveRequest{Session: "sess-1", DeclarationId: "decl-save-1"})
			return err
		},
		"Activate": func(ctx context.Context, h *Handler) error {
			_, err := h.Activate(ctx, &sessionpb.ActivateRequest{
				Session: "sess-1", DeclarationId: "decl-activate-1",
			})
			return err
		},
		"Afford": func(ctx context.Context, h *Handler) error {
			_, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1"})
			return err
		},
		"GetWhere": func(ctx context.Context, h *Handler) error {
			_, err := h.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: "sess-1"})
			return err
		},
		"StreamEvents": func(ctx context.Context, h *Handler) error {
			return h.StreamEvents(
				&sessionpb.StreamEventsRequest{Session: "sess-1"},
				newCapturingStream(ctx),
			)
		},
	}

	for name, call := range verbs {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			h := &Handler{
				manager:    sessionv1alpha1mock.NewMockManager(ctrl),
				characters: anyMemberOwnedBy(ctrl, "alice"),
				roster:     testRoster(),
			}
			err := call(auth.WithPlayerID(context.Background(), "alice"), h)
			requireCode(t, err, codes.InvalidArgument)
		})
	}
}

// TestStreamEventsRefusesAnEmptySession pins the one verb that never reaches
// the Manager. Unvalidated, an empty session does not error -- it subscribes
// under the empty key and the call HANGS, delivering nothing, which is the
// hardest kind of wrong to notice from a client.
func TestStreamEventsRefusesAnEmptySession(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{
		manager:    sessionv1alpha1mock.NewMockManager(ctrl),
		characters: anyMemberOwnedBy(ctrl, "alice"),
		roster:     testRoster(),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	err := h.StreamEvents(
		&sessionpb.StreamEventsRequest{Member: "char-1"},
		newCapturingStream(ctx),
	)
	requireCode(t, err, codes.InvalidArgument)
}
