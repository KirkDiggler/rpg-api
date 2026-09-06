package compositionv1alpha1

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	compositionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/composition"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	compositionrepo "github.com/KirkDiggler/rpg-api/internal/repositories/composition"
)

func TestHandlerRedisCreateGetListImmutableSnapshots(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	repository, err := compositionrepo.NewRedis(&compositionrepo.RedisConfig{Client: client})
	require.NoError(t, err)
	service, err := compositionorch.New(&compositionorch.Config{
		Repository:  repository,
		IDGenerator: idgen.NewSequential("proof"),
	})
	require.NoError(t, err)
	handler, err := New(&HandlerConfig{Service: service, WorldID: "test-world", AuthoringEnabled: true})
	require.NoError(t, err)
	ctx := auth.WithPlayerID(context.Background(), "dev-player")

	firstRequest := &compositionpb.CreateCompositionRequest{
		WorldId: "test-world",
		Json:    `{"name":"API 8090 composition proof","items":[{"ref":"chair"}]}`,
	}
	firstCreated, err := handler.CreateComposition(ctx, firstRequest)
	require.NoError(t, err)
	require.Equal(t, "proof_1", firstCreated.GetComposition().GetId())
	require.Equal(t, "test-world", firstCreated.GetComposition().GetWorldId())
	require.JSONEq(t, firstRequest.GetJson(), firstCreated.GetComposition().GetJson())

	_, err = handler.CreateComposition(ctx, &compositionpb.CreateCompositionRequest{
		WorldId: "test-world",
		Json:    `{"name":"Second composition"}`,
	})
	require.NoError(t, err)

	// Mutating request/response protobufs cannot mutate the persisted snapshot.
	firstRequest.Json = `{"name":"mutated request"}`
	firstCreated.Composition.Id = "mutated-id"
	firstCreated.Composition.Json = `{"name":"mutated response"}`

	got, err := handler.GetComposition(ctx, &compositionpb.GetCompositionRequest{
		WorldId: "test-world",
		Id:      "proof_1",
	})
	require.NoError(t, err)
	require.Equal(t, "proof_1", got.GetComposition().GetId())
	require.JSONEq(t, `{"name":"API 8090 composition proof","items":[{"ref":"chair"}]}`, got.GetComposition().GetJson())

	listed, err := handler.ListCompositions(ctx, &compositionpb.ListCompositionsRequest{WorldId: "test-world"})
	require.NoError(t, err)
	require.Len(t, listed.GetCompositions(), 2)
	require.Equal(t, "proof_1", listed.GetCompositions()[0].GetId())
	require.Equal(t, "proof_2", listed.GetCompositions()[1].GetId())

	listed.Compositions[0].Json = `{"name":"mutated list result"}`
	gotAgain, err := handler.GetComposition(ctx, &compositionpb.GetCompositionRequest{
		WorldId: "test-world",
		Id:      "proof_1",
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"API 8090 composition proof","items":[{"ref":"chair"}]}`, gotAgain.GetComposition().GetJson())
}
