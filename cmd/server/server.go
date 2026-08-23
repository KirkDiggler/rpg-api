package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	character2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	characterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	lobbyv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	sessionv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	apiv1alpha1handler "github.com/KirkDiggler/rpg-api/internal/handlers/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	diceorc "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	dicesessionrepo "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// lobbyTTL is long enough for any single playtest session, short enough
// that abandoned WAITING lobbies don't accumulate forever (lobby-surface.md
// "Abandonment").
const lobbyTTL = 24 * time.Hour

var (
	grpcPort int
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the gRPC server",
	Long:  `Start the RPG API gRPC server with all configured services.`,
	RunE:  runServer,
}

func init() {
	serverCmd.Flags().IntVar(&grpcPort, "port", 50051, "gRPC server port")
}

func runServer(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, gracefully stopping...")
		cancel()
	}()

	lc := net.ListenConfig{}
	lis, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// Initialize Discord authentication
	discordClient := auth.NewDiscordClient()
	tokenCache := auth.NewTokenCache(5 * time.Minute)

	// Check if dev mode is enabled (allows "Dev <player_id>" auth scheme)
	authConfig := &auth.InterceptorConfig{
		DevMode: os.Getenv("AUTH_DEV_MODE") == "true",
	}
	if authConfig.DevMode {
		// The StreamEvents send trace (rpg-api#819, session/v1alpha1/stream_events.go)
		// and other per-call debug logging are cheap but silent under
		// slog's default Info level -- raise it here, gated on the same
		// signal that already means "this is a local-dev box, never
		// production" (AUTH_DEV_MODE), so a missing beat is a one-look
		// diagnosis by default without a second flag to remember.
		slog.SetLogLoggerLevel(slog.LevelDebug)
		log.Println("⚠️  AUTH_DEV_MODE enabled - Dev authentication scheme allowed")
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			auth.UnaryAuthInterceptor(discordClient, tokenCache, authConfig),
			grpc_logging.UnaryServerInterceptor(grpc_logging.LoggerFunc(logFunc)),
			grpc_recovery.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			auth.StreamAuthInterceptor(discordClient, tokenCache, authConfig),
			grpc_logging.StreamServerInterceptor(grpc_logging.LoggerFunc(logFunc)),
			grpc_recovery.StreamServerInterceptor(),
		),
	)

	redisClient := mustRedisClient()

	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{
		Client: redisClient,
	})
	if err != nil {
		return fmt.Errorf("failed to create character repository: %w", err)
	}

	draftRepo, err := characterdraftrepo.NewRedis(&characterdraftrepo.Config{
		Clock:       clock.New(),
		IDGenerator: idgen.NewPrefixed("draft-"),
		Client:      redisClient,
	})
	if err != nil {
		return fmt.Errorf("failed to create character draft repository: %w", err)
	}

	// Create dice session repository
	diceSessionRepo, err := dicesessionrepo.NewRedisRepository(&dicesessionrepo.Config{
		Client: redisClient,
		Clock:  clock.New(),
	})
	if err != nil {
		return fmt.Errorf("failed to create dice session repository: %w", err)
	}

	// Create dice service
	diceService, err := diceorc.NewOrchestrator(&diceorc.Config{
		DiceSessionRepo: diceSessionRepo,
		IDGenerator:     idgen.NewPrefixed("roll-"),
	})
	if err != nil {
		return fmt.Errorf("failed to create dice service: %w", err)
	}

	// Initialize services
	characterService, err := character.New(&character.Config{
		DraftRepo:        draftRepo,
		CharacterRepo:    charRepo,
		DiceService:      diceService,
		IDGenerator:      idgen.NewUUID("char"),
		DraftIDGenerator: idgen.NewUUID("draft"),
	})
	if err != nil {
		return fmt.Errorf("failed to create character service: %w", err)
	}

	// Initialize handlers
	characterHandler, err := character2.NewHandler(&character2.HandlerConfig{
		CharacterService: characterService,
	})
	if err != nil {
		return fmt.Errorf("failed to create character handler: %w", err)
	}

	diceHandler, err := apiv1alpha1handler.NewDiceHandler(&apiv1alpha1handler.DiceHandlerConfig{
		DiceService: diceService,
	})
	if err != nil {
		return fmt.Errorf("failed to create dice handler: %w", err)
	}

	// v1alpha2 character wiring (rpg-api#680): out-of-encounter sheet Equip/
	// UnequipItem. Shares the SAME characterService the v1alpha1 handler
	// uses — one rules-correct equip path (internal/orchestrators/character's
	// EquipItem/UnequipItem call the toolkit, not a bare Set) for both API
	// surfaces.
	characterHandlerV2, err := characterhandlerv2.New(&characterhandlerv2.HandlerConfig{
		CharacterService: characterService,
	})
	if err != nil {
		return fmt.Errorf("failed to create character v2 handler: %w", err)
	}

	// Register services
	dnd5ev1alpha1.RegisterCharacterServiceServer(srv, characterHandler)
	characterv2pb.RegisterCharacterServiceServer(srv, characterHandlerV2)
	apiv1alpha1.RegisterDiceServiceServer(srv, diceHandler)

	// SessionService v1alpha1 (rpg-api#796 / rpg-project#227 W2 / #227's
	// rip-out): the thin integration of the toolkit's rulebooks/dnd5e/session
	// SDK -- the sole encounter-construction stack now that the old
	// v1alpha2 encounter path (github.com/KirkDiggler/rpg-toolkit/encounter)
	// has been removed. charRepo is shared with the rest of the stack.
	sessionOrch, err := sessionorch.New(sessionorch.Config{
		Redis:      redisClient,
		Characters: charRepo,
		TTL:        24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("session orchestrator: %w", err)
	}
	sessionHandlerImpl, err := sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager:    sessionOrch.Manager,
		Broker:     sessionOrch.Broker,
		Characters: charRepo,
	})
	if err != nil {
		return fmt.Errorf("session handler: %w", err)
	}
	sessionv1alpha1pb.RegisterSessionServiceServer(srv, sessionHandlerImpl)

	// LobbyService v1alpha1 (rpg-api#629): party assembly + the sole encounter
	// construction path. StartEncounter builds directly onto sessionOrch's
	// Manager -- the session stack is the only stack now (rpg-project#227).
	lobbyBroker := lobbyorch.NewBroker()
	lobbyRepo := lobbyrepo.NewRedis(redisClient, lobbyTTL)
	lobbyCfg := &lobbyorch.Config{
		LobbyRepo:            lobbyRepo,
		LobbyBroker:          lobbyBroker,
		CharacterRepo:        charRepo,
		LobbyIDGenerator:     idgen.NewUUID("lobby"),
		JoinRefGenerator:     idgen.NewUUID("join"),
		EncounterIDGenerator: idgen.NewUUID(""),
		SessionManager:       sessionOrch.Manager,
	}
	lobbyOrch, err := lobbyorch.New(lobbyCfg)
	if err != nil {
		return fmt.Errorf("lobby orchestrator: %w", err)
	}
	lobbyHandlerImpl, err := lobbyhandler.New(&lobbyhandler.HandlerConfig{
		Orchestrator: lobbyOrch,
		Broker:       lobbyBroker,
	})
	if err != nil {
		return fmt.Errorf("lobby handler: %w", err)
	}
	lobbyv1alpha1pb.RegisterLobbyServiceServer(srv, lobbyHandlerImpl)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthServer)

	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("dnd5e.api.v1alpha1.CharacterService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("api.v1alpha1.DiceService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("dnd5e.api.session.v1alpha1.SessionService", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("dnd5e.api.lobby.v1alpha1.LobbyService", grpc_health_v1.HealthCheckResponse_SERVING)

	// AuthoringService (PutDungeon) was old-dialect-only -- it wrote through
	// internal/dungeonregistry against the OLD rpg-toolkit/encounter/
	// dungeonspec compiler, with no path for the session stack (which plays
	// a fixed embedded dungeon, see start_encounter_session_stack.go) to
	// ever read what it wrote. Deleted alongside dungeonregistry and the
	// rest of the old encounter stack (rpg-project#227) rather than kept
	// registered behind RPG_AUTHORING_ENABLED for a compiler dialect
	// nothing else uses any more -- it was never registered in any real
	// deployment anyway (the gate defaulted off).

	reflection.Register(srv)

	errChan := make(chan error, 1)
	go func() {
		log.Printf("gRPC server starting on port %d...", grpcPort)
		if err := srv.Serve(lis); err != nil {
			errChan <- fmt.Errorf("failed to serve: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutting down gRPC server...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		stopped := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(stopped)
		}()

		select {
		case <-shutdownCtx.Done():
			log.Println("Graceful shutdown timeout exceeded, forcing stop")
			srv.Stop()
		case <-stopped:
			log.Println("Server stopped gracefully")
		}

		if err := lobbyBroker.Close(); err != nil {
			log.Printf("lobby broker close: %v", err)
		}

		return nil
	case err := <-errChan:
		return err
	}
}

func logFunc(_ context.Context, level grpc_logging.Level, msg string, fields ...any) {
	// Extract useful information from fields
	var method, code, errorMsg string
	var timeMs float64

	// Parse fields (they come in pairs: key, value)
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}

		switch key {
		case "grpc.method":
			method, _ = fields[i+1].(string)
		case "grpc.code":
			if codeVal, ok := fields[i+1].(codes.Code); ok {
				code = codeVal.String()
			} else if codeStr, ok := fields[i+1].(string); ok {
				code = codeStr
			}
		case "grpc.error":
			if err, ok := fields[i+1].(error); ok {
				errorMsg = err.Error()
			} else if errStr, ok := fields[i+1].(string); ok {
				errorMsg = errStr
			}
		case "grpc.time_ms":
			timeMs, _ = fields[i+1].(float64)
		}
	}

	// Format based on message type
	switch msg {
	case "started call":
		log.Printf("→ %s started", method)
	case "finished call":
		if code == "OK" || code == "0" {
			log.Printf("✓ %s completed in %.2fms", method, timeMs)
		} else {
			log.Printf("✗ %s failed (%s) in %.2fms: %s", method, code, timeMs, errorMsg)
		}
	default:
		// Fallback to original format for other messages
		log.Printf("[%v] %s %v", level, msg, fields)
	}
}

func mustRedisClient() redis.Client {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	slog.Info("connecting to Redis", "address", redisAddr)

	client, err := redis.NewClient(redisAddr, nil)
	if err != nil {
		slog.Error("failed to create Redis client", "error", err.Error())
		panic(err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		slog.Error("failed to connect to Redis", "address", redisAddr, "error", err.Error())
		panic(fmt.Errorf("failed to connect to Redis at %s: %w", redisAddr, err))
	}

	slog.Info("successfully connected to Redis", "address", redisAddr)
	return client
}
