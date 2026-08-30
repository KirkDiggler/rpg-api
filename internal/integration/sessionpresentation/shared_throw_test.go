package sessionpresentation_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"

	lobbypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

const (
	playerAlice = "player-alice"
	playerBob   = "player-bob"
	memberAlice = "char-alice"
	memberBob   = "char-bob"

	campaignID                = "campaign-shared-throw"
	presentationChannelPrefix = "sessionpresentation:channel:"
)

func TestSharedThrowCrossesServerInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	sharedRedis, err := harness.StartRedis(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer termCancel()
		require.NoError(t, sharedRedis.Terminate(termCtx))
	})

	serverA, err := harness.NewWithRedis(ctx, harness.DefaultConfig(), sharedRedis.Addr)
	require.NoError(t, err)
	t.Cleanup(serverA.Close)
	require.NoError(t, serverA.FlushRedis(ctx))

	serverB, err := harness.NewWithRedis(ctx, harness.DefaultConfig(), sharedRedis.Addr)
	require.NoError(t, err)
	t.Cleanup(serverB.Close)

	seedCharacter(ctx, t, serverA.CharacterRepo, memberAlice, playerAlice, "Alice")
	seedCharacter(ctx, t, serverA.CharacterRepo, memberBob, playerBob, "Bob")

	sessionID := launchSessionThroughServerA(ctx, t, serverA)
	require.NotEmpty(t, sessionID)

	_, err = serverA.HealthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: presentationpb.SessionPresentationService_ServiceDesc.ServiceName,
	})
	require.NoError(t, err, "TestServer must report the exact presentation health service as SERVING")

	aliceCtx := authContext(ctx, playerAlice)
	bobCtx := authContext(ctx, playerBob)

	beforeSeqs := storySeqs(aliceCtx, t, serverA.SessionClient, sessionID, memberAlice)
	require.NotEmpty(t, beforeSeqs, "lobby launch must have written a toolkit Story sequence before presentation publishes")
	authoritySeq := beforeSeqs[len(beforeSeqs)-1]

	streamCtxA, cancelStreamA := context.WithCancel(aliceCtx)
	defer cancelStreamA()
	streamA, err := serverA.SessionPresentationClient.StreamDiceThrows(streamCtxA, &presentationpb.StreamDiceThrowsRequest{
		Session: sessionID,
		Member:  memberAlice,
	})
	require.NoError(t, err, "server A stream must pass the real seated-member gate")

	streamCtxB, cancelStreamB := context.WithCancel(bobCtx)
	defer cancelStreamB()
	streamB, err := serverB.SessionPresentationClient.StreamDiceThrows(streamCtxB, &presentationpb.StreamDiceThrowsRequest{
		Session: sessionID,
		Member:  memberBob,
	})
	require.NoError(t, err, "server B stream must pass the same Redis-backed seated-member gate")

	waitForPresentationSubscribers(ctx, t, serverA, sessionID, 2)

	draft := validOneBodyDraft(authoritySeq)
	published, err := serverA.SessionPresentationClient.PublishDiceThrow(aliceCtx, &presentationpb.PublishDiceThrowRequest{
		Session: sessionID,
		Member:  memberAlice,
		Draft:   draft,
	})
	require.NoError(t, err, "publish goes through server A")
	require.NotNil(t, published.GetPlan())

	planA := recvPlan(t, streamA, "server A stream")
	planB := recvPlan(t, streamB, "server B stream")

	publishedBytes := mustMarshalDeterministic(t, published.GetPlan())
	require.True(t, bytes.Equal(publishedBytes, mustMarshalDeterministic(t, planA)), "server A stream plan must be byte-equal to publish response")
	require.True(t, bytes.Equal(publishedBytes, mustMarshalDeterministic(t, planB)), "server B stream plan must be byte-equal to publish response")

	afterSeqs := storySeqs(aliceCtx, t, serverA.SessionClient, sessionID, memberAlice)
	require.Equal(t, beforeSeqs, afterSeqs, "presentation-only publish must not append to or renumber toolkit Story")
}

func seedCharacter(ctx context.Context, t *testing.T, repo characterrepo.Repository, id, playerID, name string) {
	t.Helper()
	_, err := repo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: &character.Data{
		ID:           id,
		PlayerID:     playerID,
		Name:         name,
		Level:        1,
		HitPoints:    10,
		MaxHitPoints: 10,
		ArmorClass:   10,
	}}})
	require.NoError(t, err)
}

func launchSessionThroughServerA(ctx context.Context, t *testing.T, serverA *harness.TestServer) string {
	t.Helper()
	aliceCtx := authContext(ctx, playerAlice)
	bobCtx := authContext(ctx, playerBob)

	created, err := serverA.LobbyClient.CreateLobby(aliceCtx, &lobbypb.CreateLobbyRequest{
		CampaignId:  campaignID,
		CharacterId: memberAlice,
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.GetLobbyId())
	require.NotEmpty(t, created.GetJoinRef())

	_, err = serverA.LobbyClient.JoinLobby(bobCtx, &lobbypb.JoinLobbyRequest{
		JoinRef:     created.GetJoinRef(),
		CharacterId: memberBob,
	})
	require.NoError(t, err)

	_, err = serverA.LobbyClient.SetReady(aliceCtx, &lobbypb.SetReadyRequest{LobbyId: created.GetLobbyId(), Ready: true})
	require.NoError(t, err)
	_, err = serverA.LobbyClient.SetReady(bobCtx, &lobbypb.SetReadyRequest{LobbyId: created.GetLobbyId(), Ready: true})
	require.NoError(t, err)

	started, err := serverA.LobbyClient.StartEncounter(aliceCtx, &lobbypb.StartEncounterRequest{LobbyId: created.GetLobbyId()})
	require.NoError(t, err)
	return started.GetEncounterId()
}

func storySeqs(ctx context.Context, t *testing.T, client sessionpb.SessionServiceClient, sessionID, memberID string) []uint64 {
	t.Helper()
	story, err := client.GetStory(ctx, &sessionpb.GetStoryRequest{Session: sessionID, Member: memberID, FromSeq: 0})
	require.NoError(t, err)
	seqs := make([]uint64, len(story.GetEntries()))
	for i, event := range story.GetEntries() {
		seqs[i] = event.GetSeq()
	}
	return seqs
}

func validOneBodyDraft(authoritySeq uint64) *presentationpb.DiceThrowDraft {
	return &presentationpb.DiceThrowDraft{
		SchemaVersion:       1,
		PresentationId:      "present-attack-d20",
		AuthoritySeq:        authoritySeq,
		Attempt:             1,
		PhysicsSchema:       presentationpb.DicePhysicsSchema_DICE_PHYSICS_SCHEMA_RAPIER_DUNGEON_D20_V1,
		ColliderFingerprint: bytes.Repeat([]byte{0x42}, 32),
		Bodies: []*presentationpb.DiceBodyInitial{
			{DieId: "attack-d20", Shape: presentationpb.DiceShape_DICE_SHAPE_D20, State: rigidBodyState(1, 2, 3)},
		},
		Terminal: &presentationpb.ThrowTerminal{Dice: []*presentationpb.DiceBodyTerminal{
			{DieId: "attack-d20", Step: 12, Kind: presentationpb.DiceTerminalKind_DICE_TERMINAL_KIND_SETTLED, State: rigidBodyState(4, 5, 6)},
		}},
	}
}

func rigidBodyState(x, y, z float32) *presentationpb.RigidBodyState {
	return &presentationpb.RigidBodyState{
		Position:        &presentationpb.Vector3{X: x, Y: y, Z: z},
		Rotation:        &presentationpb.Quaternion{X: 0, Y: 0, Z: 0, W: 1},
		LinearVelocity:  &presentationpb.Vector3{X: x + 1, Y: y + 1, Z: z + 1},
		AngularVelocity: &presentationpb.Vector3{X: x + 2, Y: y + 2, Z: z + 2},
	}
}

func recvPlan(t *testing.T, stream presentationpb.SessionPresentationService_StreamDiceThrowsClient, name string) *presentationpb.DiceThrowPlan {
	t.Helper()
	planCh := make(chan *presentationpb.DiceThrowPlan, 1)
	errCh := make(chan error, 1)
	go func() {
		plan, err := stream.Recv()
		if err != nil {
			errCh <- err
			return
		}
		planCh <- plan
	}()

	select {
	case plan := <-planCh:
		return plan
	case err := <-errCh:
		if err == io.EOF {
			t.Fatalf("%s closed before delivering the plan", name)
		}
		require.NoError(t, err, "%s receive", name)
		return nil
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

func waitForPresentationSubscribers(ctx context.Context, t *testing.T, server *harness.TestServer, sessionID string, want int64) {
	t.Helper()
	channel := presentationChannel(sessionID)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		counts, err := server.RedisClient().PubSubNumSub(ctx, channel).Result()
		require.NoError(c, err)
		require.Equal(c, want, counts[channel], "subscription count for %s", channel)
	}, 5*time.Second, 25*time.Millisecond)
}

func presentationChannel(sessionID string) string {
	hash := sha256.Sum256([]byte(sessionID))
	return fmt.Sprintf("%s%s", presentationChannelPrefix, hex.EncodeToString(hash[:]))
}

func mustMarshalDeterministic(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	out, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	require.NoError(t, err)
	return out
}

func authContext(ctx context.Context, playerID string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Dev "+playerID)
}
