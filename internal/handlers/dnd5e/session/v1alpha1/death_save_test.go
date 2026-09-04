package sessionv1alpha1

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestDeathSave_UnauthenticatedRefusesBeforeManagerCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{
		manager:    sessionv1alpha1mock.NewMockManager(ctrl),
		characters: anyMemberOwnedBy(ctrl, "alice"),
	}

	_, err := h.DeathSave(context.Background(), &sessionpb.DeathSaveRequest{
		Session: "sess-1", Member: "char-1", DeclarationId: "decl-save-1",
	})
	requireCode(t, err, codes.Unauthenticated)
}

func TestDeathSave_MapsRequestAndResponseFieldForField(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().DeathSave(gomock.Any(), &sdk.DeathSaveInput{
		Session: "sess-1", Member: "char-1", DeclarationID: "decl-save-1",
	}).Return(&sdk.DeathSaveOutput{
		Roll: 14, Outcome: sdk.DeathSaveOutcomeSuccess,
		SuccessesAdded: 1, FailuresAdded: 0,
		Successes: 2, Failures: 1, SuccessesNeeded: 1, FailuresRemaining: 2,
		Stabilized: false, Dead: false, Recovered: false, HPRestored: 0,
		Continuation:   sdk.DeathSaveContinuationEndTurn,
		PresentationID: "presentation_17", Seq: 41,
		Saved:    sdk.SaveReport{Written: []string{"character", "encounter", "session"}},
		Delivery: sdk.DeliveryReport{Events: 2, Failed: true},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.DeathSave(ctx, &sessionpb.DeathSaveRequest{
		Session: "sess-1", Member: "char-1", DeclarationId: "decl-save-1",
	})
	require.NoError(t, err)
	require.Equal(t, int32(14), resp.GetRoll())
	require.Equal(t, sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_SUCCESS, resp.GetOutcome())
	require.Equal(t, int32(1), resp.GetSuccessesAdded())
	require.Zero(t, resp.GetFailuresAdded())
	require.Equal(t, int32(2), resp.GetSuccesses())
	require.Equal(t, int32(1), resp.GetFailures())
	require.Equal(t, int32(1), resp.GetSuccessesNeeded())
	require.Equal(t, int32(2), resp.GetFailuresRemaining())
	require.False(t, resp.GetStabilized())
	require.False(t, resp.GetDead())
	require.False(t, resp.GetRecovered())
	require.Zero(t, resp.GetHpRestored())
	require.Equal(t, sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_END_TURN, resp.GetContinuation())
	require.Equal(t, "presentation_17", resp.GetPresentationId())
	require.Equal(t, uint64(41), resp.GetSeq(), "recipient-local sequence stays separate from the opaque token")
	require.Equal(t, []string{"character", "encounter", "session"}, resp.GetSaved().GetWritten())
	require.Equal(t, int32(2), resp.GetDelivery().GetEvents())
	require.True(t, resp.GetDelivery().GetFailed())
}

func TestDeathSave_EmptyInputsAndStateSentinelsMapConsistently(t *testing.T) {
	t.Run("empty member", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		h := &Handler{
			manager:    sessionv1alpha1mock.NewMockManager(ctrl),
			characters: anyMemberOwnedBy(ctrl, "alice"),
		}
		ctx := auth.WithPlayerID(context.Background(), "alice")
		_, err := h.DeathSave(ctx, &sessionpb.DeathSaveRequest{Session: "sess-1", DeclarationId: "decl"})
		requireCode(t, err, codes.InvalidArgument)
	})

	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "missing declaration", err: sdk.ErrNoDeclarationID, code: codes.InvalidArgument},
		{name: "missing session", err: sdk.ErrNoSession, code: codes.NotFound},
		{name: "stale declaration", err: sdk.ErrStaleDeclaration, code: codes.FailedPrecondition},
		{name: "not turn", err: sdk.ErrNotYourTurn, code: codes.FailedPrecondition},
		{name: "not dying", err: sdk.ErrStaleDeclaration, code: codes.FailedPrecondition},
		{name: "capacity spent", err: sdk.ErrStaleDeclaration, code: codes.FailedPrecondition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mgr := sessionv1alpha1mock.NewMockManager(ctrl)
			mgr.EXPECT().DeathSave(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("death save: %w", tt.err))
			h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			_, err := h.DeathSave(ctx, &sessionpb.DeathSaveRequest{
				Session: "sess-1", Member: "char-1", DeclarationId: "decl-save-1",
			})
			requireCode(t, err, tt.code)
		})
	}
}

func TestDeathSaveEnumConverters(t *testing.T) {
	outcomes := map[sdk.DeathSaveOutcome]sessionpb.DeathSaveOutcome{
		sdk.DeathSaveOutcomeSuccess:      sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_SUCCESS,
		sdk.DeathSaveOutcomeFailure:      sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_FAILURE,
		sdk.DeathSaveOutcomeCriticalFail: sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_CRITICAL_FAILURE,
		sdk.DeathSaveOutcomeStabilized:   sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_STABILIZED,
		sdk.DeathSaveOutcomeDead:         sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_DEAD,
		sdk.DeathSaveOutcomeRecovered:    sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_RECOVERED,
	}
	for input, want := range outcomes {
		require.Equal(t, want, deathSaveOutcomeToProto(input))
	}
	require.Equal(t, sessionpb.DeathSaveOutcome_DEATH_SAVE_OUTCOME_UNSPECIFIED,
		deathSaveOutcomeToProto(sdk.DeathSaveOutcome("future")))

	continuations := map[sdk.DeathSaveContinuation]sessionpb.DeathSaveContinuation{
		sdk.DeathSaveContinuationEndTurn:         sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_END_TURN,
		sdk.DeathSaveContinuationKeepTurn:        sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_KEEP_TURN,
		sdk.DeathSaveContinuationAlreadyAdvanced: sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_ALREADY_ADVANCED,
	}
	for input, want := range continuations {
		require.Equal(t, want, deathSaveContinuationToProto(input))
	}
	require.Equal(t, sessionpb.DeathSaveContinuation_DEATH_SAVE_CONTINUATION_UNSPECIFIED,
		deathSaveContinuationToProto(sdk.DeathSaveContinuation("future")))
}
