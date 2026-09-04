// Package harness provides a test server for integration testing.
// It spins up a real Redis container via testcontainers and runs the gRPC server
// in-process with bufconn for fast, wire-accurate testing.
package harness

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	lobbyv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	sessionpresentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	sessionv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	apiv1alpha1handler "github.com/KirkDiggler/rpg-api/internal/handlers/api/v1alpha1"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionaccess "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	sessionpresentationhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionpresentation/v1alpha1"
	character2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	characterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	diceorc "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	sessionpresentationorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	dicesessionrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	sessionpresentationrepo "github.com/KirkDiggler/rpg-api/internal/repositories/sessionpresentation"
)

const bufSize = 1024 * 1024

// TestServer encapsulates a fully wired gRPC server with real Redis for integration testing.
type TestServer struct {
	grpcServer *grpc.Server
	listener   *bufconn.Listener
	conn       *grpc.ClientConn

	// Container management. redisContainer is non-nil only when this
	// TestServer owns its Redis container (constructed via New) — it is
	// nil when constructed via NewWithRedis against a shared fixture
	// another owner (e.g. a package TestMain) is responsible for
	// terminating. Close() must only ever terminate a non-nil
	// redisContainer; see TestServer.Close.
	redisContainer testcontainers.Container
	redisClient    redis.Client
	closeOnce      sync.Once

	// Proto-generated clients for tests to use
	CharacterClient           dnd5ev1alpha1.CharacterServiceClient
	CharacterClientV2         characterv2pb.CharacterServiceClient
	DiceClient                apiv1alpha1.DiceServiceClient
	LobbyClient               lobbyv1alpha1pb.LobbyServiceClient
	SessionClient             sessionv1alpha1pb.SessionServiceClient
	SessionPresentationClient sessionpresentationpb.SessionPresentationServiceClient
	HealthClient              grpc_health_v1.HealthClient

	// Exposed for test setup (seeding data, etc.)
	CharacterRepo characterrepo.Repository
	LobbyBroker   *lobbyorch.Broker
	LobbyRepo     lobbyrepo.Repository
	SessionOrch   *sessionorch.Orchestrator
}

// Config allows customization of the test server.
type Config struct {
	// DevMode enables "Dev <player_id>" auth scheme for easier testing
	DevMode bool

	// ContentDir is the dungeon content directory the registry loads.
	// Optional — defaults to the repo's content/ found by walking up from
	// the working directory (read-only; tests never write the shipped tree).
	ContentDir string
}

// DefaultConfig returns a config suitable for most integration tests.
func DefaultConfig() *Config {
	return &Config{
		DevMode: true, // Tests typically want easy auth
	}
}

// New creates a new TestServer with its own dedicated Redis container and
// in-process gRPC. Call Close() when done to clean up containers.
//
// This is the standalone, container-owning path: each call to New starts a
// fresh Redis testcontainer. Callers running many tests against the same
// process (e.g. an `internal/integration` test package) should prefer
// starting one shared *RedisContainer via StartRedis (typically from
// TestMain) and calling NewWithRedis per test instead — New here remains
// for standalone callers (smoke tests, other packages, ad-hoc scripts)
// that want a fully self-contained, disposable server.
func New(ctx context.Context, cfg *Config) (*TestServer, error) {
	rc, err := StartRedis(ctx)
	if err != nil {
		return nil, err
	}

	ts, err := newWithClientSource(ctx, cfg, rc.Addr)
	if err != nil {
		// Nothing wired successfully owns rc yet, so terminate it
		// directly. Use a fresh background context with its own timeout
		// rather than ctx: ctx is a common reason newWithClientSource
		// just failed (e.g. already canceled/deadline-exceeded), and
		// reusing it here for best-effort cleanup could fail immediately
		// and leak the container.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = rc.Terminate(cleanupCtx)
		cancel()
		return nil, err
	}

	// From this point Close() owns rc's lifetime.
	ts.redisContainer = rc.container

	return ts, nil
}

// NewWithRedis creates a new TestServer wired against an already-running
// Redis instance (e.g. a *RedisContainer started once by a package
// TestMain and shared across many tests) instead of starting its own
// container. Every other piece of per-test state — the gRPC server,
// bufconn listener, repositories, brokers, and proto clients — is fresh,
// matching New. The returned TestServer does not own the Redis container
// backing redisAddr: Close() closes this TestServer's own Redis client
// connection but never attempts to terminate the shared container.
//
// Callers are responsible for calling FlushRedis before (or after) each
// test to keep the shared instance's state isolated between tests.
func NewWithRedis(ctx context.Context, cfg *Config, redisAddr string) (*TestServer, error) {
	return newWithClientSource(ctx, cfg, redisAddr)
}

// newWithClientSource wires a TestServer against redisAddr. It never sets
// redisContainer — ownership of the container backing redisAddr is the
// caller's concern (New sets it afterward for the owning path).
func newWithClientSource(ctx context.Context, cfg *Config, redisAddr string) (*TestServer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ts := &TestServer{}

	// Each TestServer gets its own Redis client connection, even when
	// several TestServers share the same underlying container/address.
	// This keeps Close() simple and safe: closing this connection can
	// never affect another TestServer's connection to the same container.
	redisClient, err := redis.NewClient(redisAddr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}
	ts.redisClient = redisClient

	// Verify Redis connection
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if pingErr := redisClient.Ping(pingCtx).Err(); pingErr != nil {
		ts.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", pingErr)
	}

	// Wire up all the dependencies (mirrors cmd/server/server.go)
	if wireErr := ts.wireServices(cfg); wireErr != nil {
		ts.Close()
		return nil, fmt.Errorf("failed to wire services: %w", wireErr)
	}

	// Start serving
	go func() {
		if serveErr := ts.grpcServer.Serve(ts.listener); serveErr != nil {
			log.Printf("gRPC server error: %v", serveErr)
		}
	}()

	// Create client connection via bufconn. NewClient is the supported
	// replacement for DialContext; use the passthrough target plus the same
	// context dialer, then call Connect() to preserve DialContext's old
	// asynchronous connection kick without adding ready-wait behavior.
	conn, err := newBufconnClientConn(ts.bufDialer)
	if err != nil {
		ts.Close()
		return nil, fmt.Errorf("failed to create bufconn client: %w", err)
	}
	ts.conn = conn

	// Create typed clients
	ts.CharacterClient = dnd5ev1alpha1.NewCharacterServiceClient(conn)
	ts.CharacterClientV2 = characterv2pb.NewCharacterServiceClient(conn)
	ts.DiceClient = apiv1alpha1.NewDiceServiceClient(conn)
	ts.LobbyClient = lobbyv1alpha1pb.NewLobbyServiceClient(conn)
	ts.SessionClient = sessionv1alpha1pb.NewSessionServiceClient(conn)
	ts.SessionPresentationClient = sessionpresentationpb.NewSessionPresentationServiceClient(conn)
	ts.HealthClient = grpc_health_v1.NewHealthClient(conn)

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

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(ts.grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(dnd5ev1alpha1.CharacterService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(characterv2pb.CharacterService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(apiv1alpha1.DiceService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// SessionService's toolkit integration -- the sole encounter-construction
	// stack StartEncounter builds onto now that the old v1alpha2 encounter
	// path has been removed (rpg-project#227). Shares charRepo with the rest
	// of the harness's wiring, mirroring cmd/server/server.go's production
	// wiring.
	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: ts.redisClient, Characters: charRepo, TTL: 24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	if err != nil {
		return fmt.Errorf("session orchestrator: %w", err)
	}
	ts.SessionOrch = sessOrch

	access, err := sessionaccess.New(charRepo, sessOrch.Manager)
	if err != nil {
		return fmt.Errorf("session access: %w", err)
	}
	sessionHandlerImpl, err := sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager:    sessOrch.Manager,
		Broker:     sessOrch.Broker,
		Characters: charRepo,
		Access:     access,
	})
	if err != nil {
		return fmt.Errorf("session handler: %w", err)
	}
	sessionv1alpha1pb.RegisterSessionServiceServer(ts.grpcServer, sessionHandlerImpl)
	healthServer.SetServingStatus(sessionv1alpha1pb.SessionService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)

	presentationService := sessionpresentationorch.New(sessionpresentationrepo.NewRedis(ts.redisClient))
	presentationHandlerImpl, err := sessionpresentationhandler.New(&sessionpresentationhandler.HandlerConfig{
		Service: presentationService,
		Access:  access,
	})
	if err != nil {
		return fmt.Errorf("session presentation handler: %w", err)
	}
	sessionpresentationpb.RegisterSessionPresentationServiceServer(ts.grpcServer, presentationHandlerImpl)
	healthServer.SetServingStatus(sessionpresentationpb.SessionPresentationService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// LobbyService v1alpha1 -- broker and repo are stored on ts so tests can
	// seed/inspect lobby state directly. StartEncounter builds onto
	// ts.SessionOrch.Manager.
	contentDir := cfg.ContentDir
	if contentDir == "" {
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			return fmt.Errorf("content dir: getwd: %w", wdErr)
		}
		found, findErr := dungeons.FindContentDir(wd)
		if findErr != nil {
			return findErr
		}
		contentDir = found
	}
	registry, err := dungeons.NewFileRegistry(contentDir, false, dungeonstest.ProjectorFor(sessOrch.Manager))
	if err != nil {
		return fmt.Errorf("content registry: %w", err)
	}

	ts.LobbyBroker = lobbyorch.NewBroker()
	ts.LobbyRepo = lobbyrepo.NewInMemory()
	lobbyOrch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            ts.LobbyRepo,
		LobbyBroker:          ts.LobbyBroker,
		CharacterRepo:        charRepo,
		LobbyIDGenerator:     idgen.NewUUID("lobby"),
		JoinRefGenerator:     idgen.NewUUID("join"),
		EncounterIDGenerator: idgen.NewUUID(""),
		SessionManager:       sessOrch.Manager,
		Dungeons:             registry,
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
	healthServer.SetServingStatus(lobbyv1alpha1pb.LobbyService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)

	// Create bufconn listener
	ts.listener = bufconn.Listen(bufSize)

	return nil
}

func (ts *TestServer) bufDialer(ctx context.Context, _ string) (net.Conn, error) {
	return ts.listener.DialContext(ctx)
}

func newBufconnClientConn(dialer func(context.Context, string) (net.Conn, error)) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	conn.Connect()

	return conn, nil
}

// Close cleans up everything this TestServer owns: the gRPC server,
// bufconn listener, brokers, and its own Redis client connection. It only
// terminates a Redis container when ts.redisContainer is set — that is
// only true for TestServers constructed via New. TestServers constructed
// via NewWithRedis leave redisContainer nil, so Close() closes their
// per-instance Redis client but never terminates the shared container a
// package-level fixture (e.g. TestMain) owns. Close is idempotent — safe
// to call more than once (e.g. an explicit call plus a deferred one).
func (ts *TestServer) Close() {
	ts.closeOnce.Do(ts.closeAll)
}

func (ts *TestServer) closeAll() {
	if ts.conn != nil {
		_ = ts.conn.Close()
	}
	if ts.grpcServer != nil {
		ts.grpcServer.Stop()
	}
	if ts.LobbyBroker != nil {
		_ = ts.LobbyBroker.Close()
	}
	if ts.redisClient != nil {
		_ = ts.redisClient.Close()
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

// GRPCServer returns the underlying gRPC server. This package is
// test-support code only (never imported by production), so this is a
// minimal test-only seam — not a production API — exposed so tests can
// assert that two TestServers built against the same shared Redis
// container (see RedisContainer/NewWithRedis, rpg-api#699) are wired to
// distinct gRPC servers, not just distinct Go struct pointers.
func (ts *TestServer) GRPCServer() *grpc.Server {
	return ts.grpcServer
}
