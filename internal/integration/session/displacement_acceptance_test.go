package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// pushDice keeps the skeleton alive after its failed save: the observable here
// is the push's walk act, not Thunderwave's damage.
type pushDice struct{}

func (pushDice) Roll(_ context.Context, _ int) (int, error) { return 1, nil }

// forcedWalk finds the one externally displaced walk in an observer's served
// story. The test's open room puts both seats in sight, so this asserts
// carriage without changing the audience boundary (rpg-toolkit#2227).
func forcedWalk(t *testing.T, acts []*storypb.Act, mover string) *storypb.Act {
	t.Helper()
	for _, act := range acts {
		walk := act.GetWalk()
		if walk != nil && walk.GetMover().GetId() == mover && walk.GetForced() {
			return act
		}
	}
	require.FailNowf(t, "forced walk missing", "no forced walk for %q", mover)
	return nil
}

// publishedActs takes only what the broker had already published when the
// verb returned. The non-blocking receive is deliberate: a later publication
// cannot make this assertion pass.
func publishedActs(subscription *sessionorch.ActSubscription) []sdk.Act {
	var acts []sdk.Act
	for {
		select {
		case brokered, ok := <-subscription.Acts():
			if !ok {
				return acts
			}
			acts = append(acts, brokered.Act)
		default:
			return acts
		}
	}
}

func publishedForcedWalk(t *testing.T, acts []sdk.Act, mover string) sdk.Act {
	t.Helper()
	for _, act := range acts {
		walk, ok := act.Body.(sdk.WalkAct)
		if ok && walk.Mover.ID == mover && walk.Forced {
			return act
		}
	}
	require.FailNowf(t, "published forced walk missing", "no forced walk for %q at Cast return", mover)
	return sdk.Act{}
}

// TestAcceptance_ThunderwaveForcedWalkReachesBothObservers drives the shipped
// Thunderwave spell through the real handler and reads each eligible
// observer's StoryService stream. Both receive the same forced walk act.
func TestAcceptance_ThunderwaveForcedWalkReachesBothObservers(t *testing.T) {
	h := newAcceptanceHarnessWithDice(t, pushDice{})
	const sessionID = "displacement-push"
	const fighterID = "fighter"
	const skeletonID = "skel-1"

	bardID := createFinalizedBaneBard(t, h, "player-bard")
	bardCtx := auth.WithPlayerID(context.Background(), "player-bard")
	fighterCtx := auth.WithPlayerID(context.Background(), "player-fighter")
	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter(fighterID, "player-fighter")},
	})
	require.NoError(t, err)

	h.launch(t, sessionID,
		withMonsters(buildOpenRoom(t, 12, 6), monsterAt(skeletonID, refs.Monsters.Skeleton().String(), 3, 0)),
		seatAt(bardID, 2, 0), seatAt(fighterID, 1, 1))
	inCombat(t, h.charRepo, bardID, 1)
	inCombat(t, h.charRepo, fighterID, 1)

	turn, err := h.handler.Turn(bardCtx, &sessionpb.TurnRequest{Session: sessionID, Member: bardID})
	require.NoError(t, err)
	if turn.GetActive() == fighterID {
		_, err = h.handler.EndTurn(fighterCtx, &sessionpb.EndTurnRequest{
			Session: sessionID, Member: fighterID,
			DeclarationId: currentDeclarationID(fighterCtx, t, h.handler, sessionID, fighterID, sessionpb.Verb_VERB_END_TURN),
		})
		require.NoError(t, err)
	}

	// Subscribe before the cast so these are the two live observer streams,
	// not merely independent catch-up reads.
	bardStreamCtx, cancelBardStream := context.WithCancel(bardCtx)
	defer cancelBardStream()
	bardStream := newActStream(bardStreamCtx)
	go func() {
		_ = h.story.StreamEvents(&storypb.StreamEventsRequest{Session: sessionID, Member: bardID}, bardStream)
	}()
	bardStream.settle(t, 1, 2*time.Second)

	fighterStreamCtx, cancelFighterStream := context.WithCancel(fighterCtx)
	defer cancelFighterStream()
	fighterStream := newActStream(fighterStreamCtx)
	go func() {
		_ = h.story.StreamEvents(&storypb.StreamEventsRequest{Session: sessionID, Member: fighterID}, fighterStream)
	}()
	fighterStream.settle(t, 1, 2*time.Second)

	// These subscriptions observe the server's publish boundary directly. They
	// must already contain the forced walk when Cast returns; unlike the live
	// stream flush below, no wait can admit a late publication.
	bardPublished, err := h.manager.ActBroker.Subscribe(sessionID, bardID)
	require.NoError(t, err)
	defer func() { _ = bardPublished.Close() }()
	fighterPublished, err := h.manager.ActBroker.Subscribe(sessionID, fighterID)
	require.NoError(t, err)
	defer func() { _ = fighterPublished.Close() }()

	row := castRowForSession(bardCtx, t, h, sessionID, bardID, thunderwaveSpellRef)
	_, err = h.handler.Cast(bardCtx, &sessionpb.CastRequest{
		Session: sessionID, Member: bardID, DeclarationId: row.GetId(), Cell: &sessionpb.Position{X: 3, Y: 0},
	})
	require.NoError(t, err)

	bardPublishedWalk := publishedForcedWalk(t, publishedActs(bardPublished), skeletonID)
	fighterPublishedWalk := publishedForcedWalk(t, publishedActs(fighterPublished), skeletonID)
	require.Equal(t, bardPublishedWalk.ID, fighterPublishedWalk.ID,
		"the same push must already be published to both eligible observers when Cast returns")

	// The waits below only flush acts that the publish-boundary assertions above
	// already proved were queued before Cast returned.
	bardStory := storyOf(bardCtx, t, h.story, sessionID, bardID)
	fighterStory := storyOf(fighterCtx, t, h.story, sessionID, fighterID)
	bardWalk := forcedWalk(t, bardStream.settle(t, len(bardStory), 2*time.Second), skeletonID)
	fighterWalk := forcedWalk(t, fighterStream.settle(t, len(fighterStory), 2*time.Second), skeletonID)
	require.Equal(t, bardPublishedWalk.ID, bardWalk.GetId())
	require.Equal(t, fighterPublishedWalk.ID, fighterWalk.GetId())
	require.Equal(t, bardWalk.GetId(), fighterWalk.GetId(), "the same push reaches both eligible observers")
	require.Equal(t, refs.Spells.Thunderwave().String(), bardWalk.GetWalk().GetCause())
	require.Equal(t, refs.Spells.Thunderwave().String(), fighterWalk.GetWalk().GetCause())
}
