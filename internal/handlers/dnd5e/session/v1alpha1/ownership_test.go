package sessionv1alpha1

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
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

	verbs := map[string]func(ctx context.Context, h *Handler) error{
		"Join": func(ctx context.Context, h *Handler) error {
			_, err := h.Join(ctx, &sessionpb.JoinRequest{Session: "sess-1", Member: foreign})
			return err
		},
		"Exit": func(ctx context.Context, h *Handler) error {
			_, err := h.Exit(ctx, &sessionpb.ExitRequest{Session: "sess-1", Member: foreign})
			return err
		},
		"Move": func(ctx context.Context, h *Handler) error {
			_, err := h.Move(ctx, &sessionpb.MoveRequest{
				Session: "sess-1", Member: foreign,
				Path: []*sessionpb.Position{{X: 1, Y: 1}},
			})
			return err
		},
		"Attack": func(ctx context.Context, h *Handler) error {
			_, err := h.Attack(ctx, &sessionpb.AttackRequest{
				Session: "sess-1", Attacker: foreign, Target: "char-1",
			})
			return err
		},
		"Turn": func(ctx context.Context, h *Handler) error {
			_, err := h.Turn(ctx, &sessionpb.TurnRequest{Session: "sess-1", Member: foreign})
			return err
		},
		"EndTurn": func(ctx context.Context, h *Handler) error {
			_, err := h.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: "sess-1", Member: foreign})
			return err
		},
		"Dissolve": func(ctx context.Context, h *Handler) error {
			_, err := h.Dissolve(ctx, &sessionpb.DissolveRequest{
				Session: "sess-1", Member: foreign,
				Cause: sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION,
			})
			return err
		},
		"GetStory": func(ctx context.Context, h *Handler) error {
			_, err := h.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "sess-1", Member: foreign})
			return err
		},
		"GetView": func(ctx context.Context, h *Handler) error {
			_, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess-1", Member: foreign})
			return err
		},
		"GetWhere": func(ctx context.Context, h *Handler) error {
			_, err := h.GetWhere(ctx, &sessionpb.GetWhereRequest{Session: "sess-1", Member: foreign})
			return err
		},
		"StreamEvents": func(ctx context.Context, h *Handler) error {
			return h.StreamEvents(
				&sessionpb.StreamEventsRequest{Session: "sess-1", Member: foreign},
				newCapturingStream(ctx),
			)
		},
	}

	for name, call := range verbs {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			h := &Handler{
				manager:    sessionv1alpha1mock.NewMockManager(ctrl),
				characters: ownedCharacterRepo(ctrl, foreign, owner),
			}
			err := call(auth.WithPlayerID(context.Background(), caller), h)
			requireCode(t, err, codes.PermissionDenied)
		})
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
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	err := h.StreamEvents(
		&sessionpb.StreamEventsRequest{Member: "char-1"},
		newCapturingStream(ctx),
	)
	requireCode(t, err, codes.InvalidArgument)
}
