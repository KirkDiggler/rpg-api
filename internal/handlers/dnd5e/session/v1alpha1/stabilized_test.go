package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

func TestStabilizedResultPreservesProviderFields(t *testing.T) {
	for _, progress := range []sdk.DeathSaveProgress{{}, {Successes: 1, Failures: 2, SuccessesNeeded: 7, FailuresRemaining: 9, Stabilized: true, Dead: true}} {
		for _, before := range []sdk.LifeState{sdk.LifeStateDying, sdk.LifeStateStabilized} {
			body := &sdk.StabilizedBody{Target: "patient", SourceRef: "dnd5e:spells:spare-the-dying", SourceName: "Spare the Dying", Before: before, After: sdk.LifeStateStabilized, HitPoints: 17, Progress: progress}
			got, err := activationResultBodyToProto(sdk.ActivationResultBody{Actor: "cleric", Stabilized: body})
			require.NoError(t, err)
			require.NotNil(t, got)
			require.IsType(t, &sessionpb.ActivationResult_Stabilized{}, got.Result)
			require.Equal(t, "cleric", got.GetActor())
			want := &sessionpb.Stabilized{Target: body.Target, SourceRef: body.SourceRef, SourceName: body.SourceName, Before: lifeStateToProto(before), After: sessionpb.LifeState_LIFE_STATE_STABILIZED, HitPoints: 17, Progress: deathSaveProgressToProto(&progress)}
			encoded, err := proto.Marshal(got)
			require.NoError(t, err)
			decoded := &sessionpb.ActivationResult{}
			require.NoError(t, proto.Unmarshal(encoded, decoded))
			require.NotNil(t, decoded.GetStabilized().GetProgress(), "zero progress must retain presence")
			require.True(t, proto.Equal(want, decoded.GetStabilized()))
			require.Nil(t, decoded.GetHealingApplied())
			twoArms, err := activationResultBodyToProto(sdk.ActivationResultBody{Stabilized: body, HealingApplied: &sdk.HealingAppliedBody{}})
			require.NoError(t, err)
			require.Nil(t, twoArms, "new arm participates in the one-result invariant")
		}
	}
}
