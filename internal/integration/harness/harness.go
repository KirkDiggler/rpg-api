// Package harness provides a test server for integration testing.
// It spins up a real Redis container via testcontainers and runs the gRPC server
// in-process with bufconn for fast, wire-accurate testing.
package harness

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	lobbyv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	apiv1alpha1handler "github.com/KirkDiggler/rpg-api/internal/handlers/api/v1alpha1"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	character2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	characterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	encounterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	diceorc "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	dicesessionrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

const bufSize = 1024 * 1024

// TestServer encapsulates a fully wired gRPC server with real Redis for integration testing.
type TestServer struct {
	grpcServer *grpc.Server
	listener   *bufconn.Listener
	conn       *grpc.ClientConn

	// Container management
	redisContainer testcontainers.Container
	redisClient    redis.Client

	// Proto-generated clients for tests to use
	CharacterClient   dnd5ev1alpha1.CharacterServiceClient
	DiceClient        apiv1alpha1.DiceServiceClient
	EncounterClientV2 encounterv2pb.EncounterServiceClient
	LobbyClient       lobbyv1alpha1pb.LobbyServiceClient

	// Exposed for test setup (seeding data, etc.)
	CharacterRepo characterrepo.Repository
	BrokerV2      *tkenc.Broker
	EncRepoV2     encountersv2.Repository
	LobbyBroker   *lobbyorch.Broker
	LobbyRepo     lobbyrepo.Repository
}

// Config allows customization of the test server.
type Config struct {
	// DevMode enables "Dev <player_id>" auth scheme for easier testing
	DevMode bool
}

// DefaultConfig returns a config suitable for most integration tests.
func DefaultConfig() *Config {
	return &Config{
		DevMode: true, // Tests typically want easy auth
	}
}

// New creates a new TestServer with real Redis and in-process gRPC.
// Call Close() when done to clean up containers.
func New(ctx context.Context, cfg *Config) (*TestServer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ts := &TestServer{}

	// Start Redis container
	redisC, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		return nil, fmt.Errorf("failed to start redis container: %w", err)
	}
	ts.redisContainer = redisC

	// Get Redis connection string
	redisAddr, err := redisC.ConnectionString(ctx)
	if err != nil {
		ts.Close()
		return nil, fmt.Errorf("failed to get redis connection string: %w", err)
	}

	// Strip redis:// prefix if present (go-redis expects host:port)
	redisAddr = strings.TrimPrefix(redisAddr, "redis://")

	log.Printf("Integration test Redis started at: %s", redisAddr)

	// Create Redis client
	redisClient, err := redis.NewClient(redisAddr, nil)
	if err != nil {
		ts.Close()
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}
	ts.redisClient = redisClient

	// Verify Redis connection
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		ts.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	// Wire up all the dependencies (mirrors cmd/server/server.go)
	if err := ts.wireServices(cfg); err != nil {
		ts.Close()
		return nil, fmt.Errorf("failed to wire services: %w", err)
	}

	// Start serving
	go func() {
		if err := ts.grpcServer.Serve(ts.listener); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// Create client connection via bufconn
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(ts.bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		ts.Close()
		return nil, fmt.Errorf("failed to dial bufconn: %w", err)
	}
	ts.conn = conn

	// Create typed clients
	ts.CharacterClient = dnd5ev1alpha1.NewCharacterServiceClient(conn)
	ts.DiceClient = apiv1alpha1.NewDiceServiceClient(conn)
	ts.EncounterClientV2 = encounterv2pb.NewEncounterServiceClient(conn)
	ts.LobbyClient = lobbyv1alpha1pb.NewLobbyServiceClient(conn)

	return ts, nil
}

func (ts *TestServer) wireServices(cfg *Config) error {
	// Create repositories
	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{
		Client: ts.redisClient,
	})
	if err != nil {
		return fmt.Errorf("character repo: %w", err)
	}
	ts.CharacterRepo = charRepo

	draftRepo, err := characterdraftrepo.NewRedis(&characterdraftrepo.Config{
		Clock:       clock.New(),
		IDGenerator: idgen.NewPrefixed("draft-"),
		Client:      ts.redisClient,
	})
	if err != nil {
		return fmt.Errorf("draft repo: %w", err)
	}

	diceSessionRepo, err := dicesessionrepo.NewRedisRepository(&dicesessionrepo.Config{
		Client: ts.redisClient,
		Clock:  clock.New(),
	})
	if err != nil {
		return fmt.Errorf("dice session repo: %w", err)
	}

	// Create orchestrators
	diceService, err := diceorc.NewOrchestrator(&diceorc.Config{
		DiceSessionRepo: diceSessionRepo,
		IDGenerator:     idgen.NewPrefixed("roll-"),
	})
	if err != nil {
		return fmt.Errorf("dice service: %w", err)
	}

	characterService, err := character.New(&character.Config{
		DraftRepo:        draftRepo,
		CharacterRepo:    charRepo,
		DiceService:      diceService,
		IDGenerator:      idgen.NewUUID("char"),
		DraftIDGenerator: idgen.NewUUID("draft"),
	})
	if err != nil {
		return fmt.Errorf("character service: %w", err)
	}

	// Create handlers
	characterHandler, err := character2.NewHandler(&character2.HandlerConfig{
		CharacterService: characterService,
	})
	if err != nil {
		return fmt.Errorf("character handler: %w", err)
	}

	diceHandler, err := apiv1alpha1handler.NewDiceHandler(&apiv1alpha1handler.DiceHandlerConfig{
		DiceService: diceService,
	})
	if err != nil {
		return fmt.Errorf("dice handler: %w", err)
	}

	// Create gRPC server with auth interceptor
	discordClient := auth.NewDiscordClient()
	tokenCache := auth.NewTokenCache(5 * time.Minute)
	authConfig := &auth.InterceptorConfig{DevMode: cfg.DevMode}

	ts.grpcServer = grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			auth.UnaryAuthInterceptor(discordClient, tokenCache, authConfig),
		),
		grpc.ChainStreamInterceptor(
			auth.StreamAuthInterceptor(discordClient, tokenCache, authConfig),
		),
	)

	// v1alpha2 character wiring (rpg-api#680): out-of-encounter sheet Equip/
	// UnequipItem, sharing the same characterService as the v1alpha1 handler
	// above — one rules-correct equip path for both surfaces.
	characterHandlerV2, err := characterhandlerv2.New(&characterhandlerv2.HandlerConfig{
		CharacterService: characterService,
	})
	if err != nil {
		return fmt.Errorf("character v2 handler: %w", err)
	}

	// Register services
	dnd5ev1alpha1.RegisterCharacterServiceServer(ts.grpcServer, characterHandler)
	characterv2pb.RegisterCharacterServiceServer(ts.grpcServer, characterHandlerV2)
	apiv1alpha1.RegisterDiceServiceServer(ts.grpcServer, diceHandler)

	// v1alpha2 encounter wiring — broker and repo are stored on ts so tests can
	// seed data via ts.EncRepoV2.Save(...) and the registered handler sees the
	// same state (single shared instance).
	//
	// CombatResolverConfig/MovementResolverConfig carry charRepo (mirrors
	// cmd/server/server.go's production wiring) so the #689 characterData
	// cascade actually hydrates players from DataJSON on combat-capable RPCs.
	// Without this, EVERY player attack in this harness fell back to the
	// stat-snapshot stand-in path regardless of what a test seeds — harmless
	// for tests that seed explicit AttackBonus/DamageDice (the pre-#634
	// pattern), but it silently defeated any test of a HYDRATED player (a
	// lobby-created character, which by design carries no flat combat
	// snapshot — rpg-api#634 Part 1). Roller is left nil so the rulebook uses
	// its crypto-random default, matching production.
	ts.BrokerV2 = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	ts.EncRepoV2 = encountersv2.NewInMemory()
	encV2Handler, err := encounterhandlerv2.New(&encounterhandlerv2.HandlerConfig{
		Broker: ts.BrokerV2,
		Repo:   ts.EncRepoV2,
		CombatResolverConfig: &encounterhandlerv2.Dnd5eCombatResolverConfig{
			CharacterRepo: charRepo,
		},
		MovementResolverConfig: &encounterhandlerv2.Dnd5eMovementResolverConfig{
			CharacterRepo: charRepo,
		},
	})
	if err != nil {
		return fmt.Errorf("v2 encounter handler: %w", err)
	}
	encounterv2pb.RegisterEncounterServiceServer(ts.grpcServer, encV2Handler)

	// LobbyService v1alpha1 — broker and repo are stored on ts so tests can
	// seed/inspect lobby state directly, mirroring BrokerV2/EncRepoV2 above.
	// StartEncounter builds onto the SAME BrokerV2/EncRepoV2 the v1alpha2
	// encounter service reads from.
	ts.LobbyBroker = lobbyorch.NewBroker()
	ts.LobbyRepo = lobbyrepo.NewInMemory()
	lobbyOrch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:             ts.LobbyRepo,
		LobbyBroker:           ts.LobbyBroker,
		CharacterRepo:         charRepo,
		EncounterRepo:         ts.EncRepoV2,
		EncounterBroker:       ts.BrokerV2,
		CharacterResolver:     encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver:   lobbyhandler.BuildCombatResolver(encounterhandlerv2.Dnd5eCombatResolverConfig{CharacterRepo: charRepo}),
		BuildMovementResolver: lobbyhandler.BuildMovementResolver(encounterhandlerv2.Dnd5eMovementResolverConfig{CharacterRepo: charRepo}),
		LobbyIDGenerator:      idgen.NewUUID("lobby"),
		JoinRefGenerator:      idgen.NewUUID("join"),
		EncounterIDGenerator:  idgen.NewUUID(""),
	})
	if err != nil {
		return fmt.Errorf("lobby orchestrator: %w", err)
	}
	lobbyHandlerImpl, err := lobbyhandler.New(&lobbyhandler.HandlerConfig{
		Orchestrator: lobbyOrch,
		Broker:       ts.LobbyBroker,
	})
	if err != nil {
		return fmt.Errorf("lobby handler: %w", err)
	}
	lobbyv1alpha1pb.RegisterLobbyServiceServer(ts.grpcServer, lobbyHandlerImpl)

	// Create bufconn listener
	ts.listener = bufconn.Listen(bufSize)

	return nil
}

func (ts *TestServer) bufDialer(ctx context.Context, _ string) (net.Conn, error) {
	return ts.listener.DialContext(ctx)
}

// Close cleans up all resources.
func (ts *TestServer) Close() {
	if ts.conn != nil {
		ts.conn.Close()
	}
	if ts.grpcServer != nil {
		ts.grpcServer.Stop()
	}
	if ts.BrokerV2 != nil {
		// Releases the broker's per-encounter listen goroutines and the
		// underlying transport. Without this, each integration test leaks
		// one listener per Subscribe call.
		_ = ts.BrokerV2.Close()
	}
	if ts.LobbyBroker != nil {
		_ = ts.LobbyBroker.Close()
	}
	if ts.redisClient != nil {
		ts.redisClient.Close()
	}
	if ts.redisContainer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := ts.redisContainer.Terminate(ctx); err != nil {
			log.Printf("Failed to terminate redis container: %v", err)
		}
	}
}

// FlushRedis clears all data from Redis. Useful between tests.
func (ts *TestServer) FlushRedis(ctx context.Context) error {
	return ts.redisClient.FlushAll(ctx).Err()
}

// RedisClient returns the underlying Redis client for direct data manipulation in tests.
func (ts *TestServer) RedisClient() redis.Client {
	return ts.redisClient
}
