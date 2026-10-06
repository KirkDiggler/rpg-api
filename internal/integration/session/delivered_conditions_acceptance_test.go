package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

// conditionsKey is the key a stored sight testimony carries its conditions
// under; a delivered payload must not carry it (rpg-project#520).
const conditionsKey = "conditions"

// TestAcceptance_DeliveredSightPayloadsCarryNoConditions pins what rpg-api
// relays, not what it does: the API forwards sight payloads byte for byte and
// never strips them. Encounter keeps the conditions a member was seen holding
// in STORED testimony and removes them from every DELIVERED payload. This
// walks each delivered route the API forwards -- live event payloads, the
// view's sightings, and a joiner's first contact -- with a member that holds
// Faerie Fire, and fails if any of them carries a conditions key.
func TestAcceptance_DeliveredSightPayloadsCarryNoConditions(t *testing.T) {
	h, ctx, id := nativeClericCombatSceneAt(t, 8, 2, spells.FaerieFire)
	useGuidingBoltDice(t, h, 1)
	row := castRowFor(ctx, t, h, id, spells.FaerieFire)
	require.True(t, row.GetAvailable(), row.GetWhy())

	live := watchCast(ctx, t, h, id, func() {
		_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{
			Session: castSessionID, Member: id, DeclarationId: row.GetId(), Cell: &sessionpb.Position{X: 8, Y: 2},
		})
		require.NoError(t, err)
	})
	applied := false
	for _, event := range live {
		if event.GetActivationResult().GetConditionApplied().GetTarget() == "skel-1" {
			applied = true
		}
		requireNoConditionsKey(t, event.GetPayload(), "event seq %d payload", event.GetSeq())
	}
	require.True(t, applied, "the scene needs skel-1 to hold Faerie Fire, or nothing below can fail")

	view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: castSessionID, Member: id})
	require.NoError(t, err)
	sawSkeleton := false
	for _, sighting := range view.GetSightings() {
		if sighting.GetSubject() == "skel-1" {
			sawSkeleton = true
		}
		requireNoConditionsKey(t, sighting.GetPayload(), "view sighting of %s", sighting.GetSubject())
	}
	require.True(t, sawSkeleton, "the caster's view must hold the conditioned skeleton")

	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	_, err = h.charRepo.Create(bobCtx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("bob", "player-bob")}})
	require.NoError(t, err)
	joined, err := h.handler.Join(bobCtx, &sessionpb.JoinRequest{Session: castSessionID, Member: "bob", Position: pbAt(4, 0)})
	require.NoError(t, err)
	reports := 0
	for observer, discovery := range joined.GetDiscovered() {
		for _, report := range discovery.GetFirstContact() {
			reports++
			requireNoConditionsKey(t, report.GetPayload(), "%s's first contact with %s", observer, report.GetSubject())
		}
	}
	require.NotZero(t, reports, "a joiner in sight of the room must make first contact")
}

// requireNoConditionsKey fails when payload is JSON holding a conditions key
// at any depth. An empty payload holds nothing.
func requireNoConditionsKey(t *testing.T, payload []byte, msg string, args ...any) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	where := fmt.Sprintf(msg, args...)
	var decoded any
	require.NoError(t, json.Unmarshal(payload, &decoded), where)
	require.False(t, holdsKey(decoded, conditionsKey), "%s carries %q: %s", where, conditionsKey, payload)
}

func holdsKey(v any, key string) bool {
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			if strings.EqualFold(k, key) || holdsKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if holdsKey(child, key) {
				return true
			}
		}
	}
	return false
}
