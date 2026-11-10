package session_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// TestAcceptance_TheSiteIsSharedByHashAndStoredOnce is rpg-project#879 S5 on
// the server path: two games on one content compile one site; the
// checkpoint names its site by hash and carries no field; the site piece is
// stored once under its hash; a cold process (a fresh provider over the same
// Redis) loads the checkpoint through that piece; and a site neither the
// provider nor the store holds refuses the verb by name, FAILED_PRECONDITION.
func TestAcceptance_TheSiteIsSharedByHashAndStoredOnce(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := context.Background()
	for _, who := range []struct{ id, player string }{{"alice", "player-alice"}, {"bob", "player-bob"}} {
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(who.id, who.player)},
		})
		require.NoError(t, err)
	}
	dungeon, err := sessionworld.Compile([]byte(dungeonstest.HeirloomVaultYAML))
	require.NoError(t, err)

	sites, ok := h.manager.Sites.(*sessionorch.Sites)
	require.True(t, ok, "an orchestrator built with no provider builds the api's own")
	h.launch(t, "site-game-a", dungeon.Spec, seatAt("alice", aliceSeatCol, aliceSeatRow))
	h.launch(t, "site-game-b", dungeon.Spec, seatAt("bob", aliceSeatCol, aliceSeatRow))
	require.Equal(t, 1, sites.Compiles(), "two games on one content compile one site")

	// The checkpoint names the site and carries no field; the piece is
	// stored once, under that hash, with no expiry.
	hash := storedSiteHash(t, h, "site-game-a")
	require.Equal(t, hash, storedSiteHash(t, h, "site-game-b"), "both games name one site")
	pieceKey := "session-site:v1alpha1:" + hash
	n, err := h.redis.Exists(ctx, pieceKey).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "the launch stored the site piece under its hash")
	ttl, err := h.redis.TTL(ctx, pieceKey).Result()
	require.NoError(t, err)
	require.Equal(t, time.Duration(-1), ttl, "a site piece is content: no expiry")

	alice := auth.WithPlayerID(ctx, "player-alice")
	bob := auth.WithPlayerID(ctx, "player-bob")
	before, err := h.handler.GetWhere(alice, &sessionpb.GetWhereRequest{Session: "site-game-a", Member: "alice"})
	require.NoError(t, err)

	// A COLD PROCESS: a fresh provider over the same store. Its first load
	// finds no site in memory, reads the piece and compiles it once; the
	// second game's load shares that compile.
	coldSites, err := sessionorch.NewSites(nil)
	require.NoError(t, err)
	cold := freshSessionHandler(t, h, coldSites)
	where, err := cold.GetWhere(alice, &sessionpb.GetWhereRequest{Session: "site-game-a", Member: "alice"})
	require.NoError(t, err, "a cold process loads the checkpoint through the stored piece")
	require.Equal(t, before.GetPosition().String(), where.GetPosition().String())
	_, err = cold.GetWhere(bob, &sessionpb.GetWhereRequest{Session: "site-game-b", Member: "bob"})
	require.NoError(t, err)
	require.Equal(t, 1, coldSites.Compiles(), "the cold process compiles the stored piece once for both games")

	// UNKNOWN ON BOTH: a fresh provider and no piece. The verb fails closed
	// by name, and the code says the stored state refuses it.
	require.NoError(t, h.redis.Del(ctx, pieceKey).Err())
	lost, err := sessionorch.NewSites(nil)
	require.NoError(t, err)
	_, err = freshSessionHandler(t, h, lost).GetWhere(alice,
		&sessionpb.GetWhereRequest{Session: "site-game-a", Member: "alice"})
	require.Error(t, err)
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "%v", err)
	require.Contains(t, err.Error(), "unknown site")
	require.Zero(t, lost.Compiles())
}

// storedSiteHash reads the site hash a stored checkpoint names, and pins
// that the checkpoint carries no field.
func storedSiteHash(t *testing.T, h *acceptanceHarness, run string) string {
	t.Helper()
	raw, err := h.redis.Get(context.Background(), "session:v1alpha1:"+run).Bytes()
	require.NoError(t, err)
	var record struct {
		Encounter string `json:"encounter"`
	}
	require.NoError(t, json.Unmarshal(raw, &record))
	raw, err = h.redis.Get(context.Background(), "session-enc:v1alpha2:"+record.Encounter).Bytes()
	require.NoError(t, err)
	var blob map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &blob))
	require.NotContains(t, blob, "field", "the checkpoint references its site, never carries it")
	var hash string
	require.NoError(t, json.Unmarshal(blob["site_hash"], &hash))
	require.NotEmpty(t, hash)
	return hash
}

// freshSessionHandler is a second process over h's store: its own
// orchestrator on the given site provider, and a handler on it.
func freshSessionHandler(t *testing.T, h *acceptanceHarness, sites *sessionorch.Sites) *sessionhandler.Handler {
	t.Helper()
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: h.redis, Characters: h.charRepo, TTL: 24 * time.Hour, Dice: testDice{},
		PresentationIDs: idgen.NewSequential("presentation"), AnswerWhole: true, Sites: sites,
	})
	require.NoError(t, err)
	handler, err := sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager: orch.Manager, Characters: h.charRepo, Preferences: orch.Preferences,
	})
	require.NoError(t, err)
	return handler
}
