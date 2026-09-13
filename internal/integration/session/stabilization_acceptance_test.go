package session_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type stabilizationDice struct{ calls atomic.Int32 }

func (d *stabilizationDice) Roll(ctx context.Context, size int) (int, error) {
	d.calls.Add(1)
	return failedSaveDice{}.Roll(ctx, size)
}

func TestAcceptance_Stabilization(t *testing.T) {
	for _, stable := range []bool{false, true} {
		t.Run(fmt.Sprintf("already_stable=%t", stable), func(t *testing.T) {
			h, ctx := clericCastScene(t)
			patientCtx := auth.WithPlayerID(context.Background(), "player-alice")
			// Fixture setup: retain the patient's existing initiative slot, then injure them.
			caster, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: "bella"})
			require.NoError(t, err)
			caster.Character.Data.KnownCantrips = []string{refs.Spells.SpareTheDying().String()}
			_, err = h.charRepo.Update(ctx, characterrepo.UpdateInput{Character: caster.Character})
			require.NoError(t, err)
			patient, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: "alice"})
			require.NoError(t, err)
			patient.Character.Data.HitPoints = 0
			patient.Character.Data.DeathSaveState = &saves.DeathSaveState{Successes: 1, Failures: 1, Stabilized: stable}
			_, err = h.charRepo.Update(ctx, characterrepo.UpdateInput{Character: patient.Character})
			require.NoError(t, err)

			roller := &stabilizationDice{}
			reopen := func() {
				orch, openErr := sessionorch.New(sessionorch.Config{Redis: h.redis, Characters: h.charRepo, Dice: roller, TurnDriver: sdk.Pass{}})
				require.NoError(t, openErr)
				h.manager = orch
				h.handler, openErr = sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Broker: orch.Broker, Characters: h.charRepo})
				require.NoError(t, openErr)
			}
			reopen()
			row := castRowFor(ctx, t, h, "bella", spells.SpareTheDying)
			require.True(t, row.GetAvailable(), "%s", row.GetWhy())
			var live []*sessionpb.Event
			patientLive := watchCast(patientCtx, t, h, "alice", func() {
				live = watchCast(ctx, t, h, "bella", func() {
					_, castErr := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: "bella", DeclarationId: row.GetId(), Targets: []string{"alice"}})
					require.NoError(t, castErr)
				})
			})
			var result *sessionpb.Stabilized
			var kinds []sessionpb.EventKind
			for _, event := range live {
				if event.GetKind() == sessionpb.EventKind_EVENT_KIND_CAST || event.GetKind() == sessionpb.EventKind_EVENT_KIND_ACTIVATION_RESULT {
					kinds = append(kinds, event.GetKind())
				}
				if activation := event.GetActivationResult(); activation != nil {
					require.Equal(t, "bella", activation.GetActor())
					require.Nil(t, activation.GetHealingApplied())
					result = activation.GetStabilized()
				}
			}
			require.Equal(t, []sessionpb.EventKind{sessionpb.EventKind_EVENT_KIND_CAST, sessionpb.EventKind_EVENT_KIND_ACTIVATION_RESULT}, kinds)
			require.NotNil(t, result)
			before := sessionpb.LifeState_LIFE_STATE_DYING
			if stable {
				before = sessionpb.LifeState_LIFE_STATE_STABILIZED
			}
			want := &sessionpb.Stabilized{Target: "alice", SourceRef: refs.Spells.SpareTheDying().String(), SourceName: "Spare the Dying", Before: before, After: sessionpb.LifeState_LIFE_STATE_STABILIZED, Progress: &sessionpb.DeathSaveProgress{SuccessesNeeded: 3, FailuresRemaining: 3, Stabilized: true}}
			require.True(t, proto.Equal(want, result), "%v", result)
			caster, err = h.charRepo.Get(ctx, characterrepo.GetInput{ID: "bella"})
			require.NoError(t, err)
			require.Zero(t, caster.Character.Data.ActionEconomy.ActionsRemaining)
			require.Equal(t, 2, caster.Character.Data.Resources[resources.SpellSlotLevel1].Current)
			require.False(t, castRowFor(ctx, t, h, "bella", spells.SpareTheDying).GetAvailable())

			reopen()
			require.NotEmpty(t, patientLive)
			patientStory, err := h.handler.GetStory(patientCtx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: "alice", FromSeq: patientLive[0].GetSeq()})
			require.NoError(t, err)
			require.Len(t, patientStory.GetEntries(), len(patientLive))
			patientResults := 0
			for i, event := range patientLive {
				require.True(t, proto.Equal(event, patientStory.Entries[i]), "compare each recipient's own sequence")
				if stabilization := event.GetActivationResult().GetStabilized(); stabilization != nil {
					patientResults++
					require.True(t, proto.Equal(want, stabilization))
				}
			}
			require.Equal(t, 1, patientResults)
			story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: castSessionID, Member: "bella", FromSeq: live[0].GetSeq()})
			require.NoError(t, err)
			require.Len(t, story.GetEntries(), len(live))
			for i, event := range live {
				require.True(t, proto.Equal(event, story.Entries[i]))
				if i > 0 {
					require.Greater(t, event.GetSeq(), live[i-1].GetSeq())
				}
			}
			owner, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
			require.NoError(t, err)
			private, err := owner.GetCharacterData(patientCtx, &characterpb.GetCharacterDataRequest{CharacterId: "alice"})
			require.NoError(t, err)
			require.Zero(t, private.GetCharacter().GetHitPoints().GetCurrent())
			require.Equal(t, want.After, private.GetCharacter().GetLifeState())
			require.True(t, proto.Equal(want.Progress, private.GetCharacter().GetDeathSaves()))
			_, err = owner.GetCharacterData(ctx, &characterpb.GetCharacterDataRequest{CharacterId: "alice"})
			require.Equal(t, codes.NotFound, status.Code(err), "non-owners must not learn that the private character exists")
			_, err = owner.GetCharacterData(context.Background(), &characterpb.GetCharacterDataRequest{CharacterId: "alice"})
			require.Equal(t, codes.Unauthenticated, status.Code(err))
			_, err = h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: castSessionID, Member: "bella", DeclarationId: currentDeclarationID(ctx, t, h.handler, castSessionID, "bella", sessionpb.Verb_VERB_END_TURN)})
			require.NoError(t, err)
			turn, err := h.handler.Turn(patientCtx, &sessionpb.TurnRequest{Session: castSessionID, Member: "alice"})
			require.NoError(t, err)
			require.NotEqual(t, "alice", turn.GetActive())
			for _, participant := range turn.GetParticipants() {
				if participant.GetMember() == "alice" {
					require.Equal(t, want.After, participant.GetLifeState())
					require.True(t, proto.Equal(want.Progress, participant.GetDeathSaves()))
				}
			}
			require.Zero(t, roller.calls.Load(), "cast, replay, private refresh and turn advancement must roll no dice")
		})
	}
}
