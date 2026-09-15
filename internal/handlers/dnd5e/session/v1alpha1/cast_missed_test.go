package sessionv1alpha1

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionmock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// The same mixed target sequence must survive live delivery and catch-up.
// Spell misses have only the provider's identities, never attack-roll data.
func TestCastMissed_LiveAndStoryPreserveMixedTargetOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	broker := sessionorch.NewBroker()
	mgr := sessionmock.NewMockManager(ctrl)
	h := &Handler{manager: mgr, broker: broker, characters: anyMemberOwnedBy(ctrl, "alice")}
	spell := sdk.SpellRef{Ref: "dnd5e:spells:bless", Name: "Bless"}
	events := []sdk.Event{
		{Session: "sess", Recipient: "cleric", Seq: 10, Kind: sdk.EventCastMissed,
			Body: sdk.CastMissedBody{Actor: "cleric", Target: "ally-first", Spell: spell}},
		{Session: "sess", Recipient: "cleric", Seq: 11, Kind: sdk.EventActivationResult,
			Body: sdk.ActivationResultBody{Actor: "cleric", ConditionApplied: &sdk.ConditionAppliedBody{
				Target: "ally-second", Ref: "dnd5e:conditions:blessed", Name: "Blessed", SourceID: "cleric",
			}}},
		{Session: "sess", Recipient: "cleric", Seq: 12, Kind: sdk.EventCastMissed,
			Body: sdk.CastMissedBody{Actor: "cleric", Target: "ally-third", Spell: spell}},
	}
	mgr.EXPECT().Story(gomock.Any(), &sdk.StoryInput{Session: "sess", Member: "cleric", FromSeq: 10}).Return(events, nil)
	ctx, cancel := context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
	defer cancel()
	stream := newCapturingStream(ctx)
	done := make(chan error, 1)
	go func() {
		done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess", Member: "cleric"}, stream)
	}()
	waitForPublishedEvent(t, broker, stream, sdk.Event{Session: "sess", Recipient: "cleric", Seq: 1, Kind: sdk.EventMoved})
	require.NoError(t, broker.Publish(ctx, events))
	story, err := h.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "sess", Member: "cleric", FromSeq: 10})
	require.NoError(t, err)
	require.Len(t, story.GetEntries(), 3)
	for i, target := range []string{"ally-first", "ally-second", "ally-third"} {
		live := stream.WaitForSend(t, time.Second)
		require.True(t, proto.Equal(live, story.Entries[i]))
		require.Equal(t, uint64(10+i), live.GetSeq())
		require.Equal(t, "cleric", live.GetRecipient())
		if i == 1 {
			condition := live.GetActivationResult().GetConditionApplied()
			require.Equal(t, target, condition.GetTarget())
			require.Equal(t, "cleric", condition.GetSourceId())
			continue
		}
		require.Equal(t, sessionpb.EventKind_EVENT_KIND_CAST_MISSED, live.GetKind())
		require.True(t, proto.Equal(&sessionpb.CastMissed{Actor: "cleric", Target: target,
			Spell: &sessionpb.SpellRef{Ref: spell.Ref, Name: spell.Name}}, live.GetCastMissed()))
		require.Nil(t, live.GetMissed())
	}
	cancel()
	require.NoError(t, <-done)
}
