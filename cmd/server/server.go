package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionrecordhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	sessionaccess "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	sessionpresentationhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionpresentation/v1alpha1"
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

	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	authoringv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	lobbyv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	sessionpresentationv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	sessionv1alpha1pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sessionv1alpha2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	apiv1alpha1handler "github.com/KirkDiggler/rpg-api/internal/handlers/api/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
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
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// lobbyTTL is long enough for any single playtest session, short enough
// that abandoned WAITING lobbies don't accumulate forever (lobby-surface.md
// "Abandonment").
const lobbyTTL = 24 * time.Hour

// Content registry environment (rpg-api#806, rpg-project#256).
const (
	// envContentDir is the directory of authored dungeon files the registry
	// loads at boot and the AuthoringService writes to. Defaults to
	// dungeons.ShippedContentDir relative to the working directory, which is
	// where the repo (and the image) keep the shipped dungeons; when set,
	// every shipped file it lacks is seeded into it.
	envContentDir = "RPG_CONTENT_DIR"
	// envAuthoringEnabled set to "1" registers the AuthoringService
	// (PutDungeon writes into envContentDir). Off by default: the registry
	// still loads and serves content, and ListDungeons/GetDungeon remain
	// ungated reads.
	envAuthoringEnabled = "RPG_AUTHORING_ENABLED"
	// envShippedContentDir overrides where the shipped dungeons are seeded FROM
	// when envContentDir is set and empty. Defaults to
	// dungeons.ShippedContentDir; the image sets it to an immutable copy
	// outside the working directory so a volume mounted over ./content
	// cannot hide it.
	envShippedContentDir = "RPG_SHIPPED_CONTENT_DIR"
	// envStaleTargetPolicy selects the host's known-creature cast policy.
	// Unset defaults to refuse in the session orchestrator; attempt opts in
	// to paid misses. Invalid values fail manager construction.
	envStaleTargetPolicy = "RPG_STALE_TARGET_POLICY"
	// envSiteCacheEntries bounds how many compiled sites the process holds
	// (rpg-project#879 S5). Unset is sessionorch.DefaultSiteEntries; it must
	// be a positive integer when set.
	envSiteCacheEntries = "RPG_SITE_CACHE_ENTRIES"
	// envResidentEntries bounds how many sessions' resident encounters the
	// process keeps between verbs (rpg-project#879 S6). Unset is
	// sessionorch.DefaultResidentEntries; when set it must be a positive
	// integer, or -1 to keep none (every verb loads, as before S6).
	envResidentEntries = "RPG_RESIDENT_ENTRIES"
	// envResidentMaxMB bounds the resident encounters' estimated memory
	// together, in MiB. Unset is sessionorch.DefaultResidentMaxMB; it must be
	// a positive integer when set.
	envResidentMaxMB = "RPG_RESIDENT_MAX_MB"
)

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

	worldAccessEnforced, err := configuredWorldAccessEnforcement()
	if err != nil {
		return err
	}
	log.Printf("World-role admission enforcement enabled: %t (%s)", worldAccessEnforced, envWorldAccessEnforcement)

	// Check if dev mode is enabled (allows "Dev <player_id>" auth scheme).
	// Keep stream credentials only when the role gate will consume them.
	authConfig := &auth.InterceptorConfig{
		DevMode:            os.Getenv(envAuthDevMode) == envEnabledValue,
		WorldScopedStreams: worldAccessEnforced,
	}
	authoringEnabled := os.Getenv(envAuthoringEnabled) == "1"
	development, err := configuredDevelopmentAccess(authConfig.DevMode)
	if err != nil {
		return fmt.Errorf("development access: %w", err)
	}
	membershipCache, err := auth.NewMembershipCache(&auth.MembershipCacheConfig{
		TTL:        30 * time.Second,
		MaxEntries: 1024,
		Now:        time.Now,
	})
	if err != nil {
		return fmt.Errorf("create membership cache: %w", err)
	}
	worldResolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
		MembershipVerifier: discordClient,
		IdentityCache:      tokenCache,
		MembershipCache:    membershipCache,
		DevWorldID:         configuredDevWorldID(authConfig.DevMode),
		DevelopmentRoles:   development.Roles,
	})
	if err != nil {
		return fmt.Errorf("create world resolver: %w", err)
	}
	if authConfig.DevMode {
		// Per-call debug logging (the act stream's page and send traces among
		// it) is cheap but silent under slog's default Info level -- raise it
		// here, gated on the same
		// signal that already means "this is a local-dev box, never
		// production" (AUTH_DEV_MODE), so a missing beat is a one-look
		// diagnosis by default without a second flag to remember.
		slog.SetLogLoggerLevel(slog.LevelDebug)
		log.Println("⚠️  AUTH_DEV_MODE enabled - Dev authentication scheme allowed")
	}

	redisClient := mustRedisClient()
	worldRepository, err := worldrepo.NewRedis(&worldrepo.RedisConfig{Client: redisClient})
	if err != nil {
		return fmt.Errorf("world repository: %w", err)
	}
	gameplayAccess, err := newGameplayAccess(&gameplayAccessInput{
		Enforce: worldAccessEnforced,
		Roles: &auth.RoleAccessConfig{
			Resolver: worldResolver, Worlds: worldRepository,
			DevelopmentPermissions: development.Default, DevelopmentPlayers: development.Players,
		},
	})
	if err != nil {
		return err
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			auth.UnaryAuthInterceptor(discordClient, tokenCache, authConfig),
			auth.UnaryWorldManagementInterceptor(&auth.WorldManagementConfig{
				Resolver: worldResolver, Ownership: discordClient,
				IdentityCache: tokenCache, MembershipCache: membershipCache,
				DevelopmentOwner:         configuredDevWorldOwner(authConfig.DevMode),
				DevelopmentOwnerPlayerID: os.Getenv(envDevWorldOwnerPlayer),
			}),
			gameplayAccess.Unary,
			grpc_logging.UnaryServerInterceptor(grpc_logging.LoggerFunc(logFunc)),
			grpc_recovery.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			auth.StreamAuthInterceptor(discordClient, tokenCache, authConfig),
			gameplayAccess.Stream,
			grpc_logging.StreamServerInterceptor(grpc_logging.LoggerFunc(logFunc)),
			grpc_recovery.StreamServerInterceptor(),
		),
	)

	if registrationErr := registerWorldService(srv, worldRepository); registrationErr != nil {
		return registrationErr
	}

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

	// BUILT AHEAD OF THE CHARACTER SERVICE, which is why this sits here
	// rather than beside the rest of its own wiring further down: the
	// character service equips through the session Manager's Equip/Unequip
	// verbs (rpg-project#542). The session orchestrator takes the character
	// REPOSITORY, not the service, so there is no cycle, only an order.
	// Capture uses a dedicated JSONL stdout stream in every server mode.
	// It is independent of the browser-only development diagnostic gate.
	recordCapture, err := sessionorch.NewRecordCapture(&sessionorch.RecordCaptureConfig{Writer: os.Stdout})
	if err != nil {
		return fmt.Errorf("session record capture: %w", err)
	}
	defer func() {
		// On normal shutdown the continuation drain below runs first. This
		// also closes the worker on a construction/listener error before serve.
		drain, cancel := context.WithTimeout(context.Background(), recordCaptureDrainBudget)
		defer cancel()
		if captureErr := recordCapture.Drain(drain); captureErr != nil {
			log.Printf("session record capture not drained cleanly: %v", captureErr)
		}
	}()
	// ONE SITE PROVIDER FOR THE PROCESS (rpg-project#879 S5): every game on
	// one content shares one compiled site. The content registry below
	// projects every shipped dungeon's atlas through this same provider, so
	// the shelf's sites compile at boot (one "site compiled" line per hash)
	// and a launch on shipped content compiles nothing.
	//
	// ONE RESIDENCY FOR THE PROCESS (rpg-project#879 S6): each session's
	// write-verb encounter stays in memory between verbs, checked against
	// the store's checkpoint version on every verb. Nothing stored changes;
	// a restart starts cold.
	sites, residency, err := newProcessCaches()
	if err != nil {
		return err
	}
	sessionOrch, err := sessionorch.New(sessionorch.Config{
		Sites:             sites,
		Residency:         residency,
		RecordCapture:     recordCapture,
		StaleTargetPolicy: sdk.StaleTargetPolicy(os.Getenv(envStaleTargetPolicy)),
		Redis:             redisClient,
		Characters:        charRepo,
		TTL:               24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("session orchestrator: %w", err)
	}
	lobbyRepo := lobbyrepo.NewRedis(redisClient, lobbyTTL)

	// Initialize services
	characterService, err := character.New(&character.Config{
		DraftRepo:        draftRepo,
		CharacterRepo:    charRepo,
		DiceService:      diceService,
		IDGenerator:      idgen.NewUUID("char"),
		DraftIDGenerator: idgen.NewUUID("draft"),
		// Equipping is the SDK's verb: the character's seat decides whether
		// it touches a run, and the verb tells watchers itself.
		Equipment: sessionOrch.Manager,
	})
	if err != nil {
		return fmt.Errorf("failed to create character service: %w", err)
	}

	// Initialize handlers
	characterHandler, err := character2.NewHandler(&character2.HandlerConfig{
		CharacterService: characterService,
		// Advancement is the SDK's, not this service's (design R6.1). The
		// character handler calls the session Manager for it and projects
		// the answer; the same Manager already flows into the character
		// service's Equipment above, so it is built by here.
		Sessions: sessionOrch.Manager,
		// Character configuration (rpg-project#753 R2): SetPreference writes
		// it under the character guard of the Manager's own locker.
		Preferences: sessionOrch.Preferences,
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
	access, err := sessionaccess.New(charRepo, sessionOrch.Manager)
	if err != nil {
		return fmt.Errorf("session access: %w", err)
	}
	sessionHandlerImpl, err := sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager:    sessionOrch.Manager,
		Characters: charRepo,
		Access:     access,
		// The deprecated SetDiscoverySharing writes character configuration
		// (rpg-project#753 R2); GetKnowledge reports it.
		Preferences: sessionOrch.Preferences,
	})
	if err != nil {
		return fmt.Errorf("session handler: %w", err)
	}
	sessionv1alpha1pb.RegisterSessionServiceServer(srv, sessionHandlerImpl)

	// RecordService, session v1alpha2 (rpg-project#628): the record stream
	// for the debug panel. Same seat gate as the act stream; its own broker,
	// which closes a lagging subscriber as lagged instead of skipping it.
	if recordErr := registerRecordService(&recordServiceInput{
		Registrar: srv, Development: authConfig.DevMode,
		Manager: sessionOrch.Manager, Broker: sessionOrch.RecordBroker, Gate: access,
	}); recordErr != nil {
		return fmt.Errorf("session record handler: %w", recordErr)
	}

	// StoryService, session v1alpha2 (rpg-project#630): the act stream, per
	// recipient. Same seat gate; its own broker, keyed by session and member,
	// which closes a lagging subscriber only between two ids.
	storyHandlerImpl, err := sessionrecordhandler.NewStory(&sessionrecordhandler.StoryHandlerConfig{
		Manager: sessionOrch.Manager,
		Broker:  sessionOrch.ActBroker,
		Gate:    access,
	})
	if err != nil {
		return fmt.Errorf("session story handler: %w", err)
	}
	sessionv1alpha2pb.RegisterStoryServiceServer(srv, storyHandlerImpl)

	presentationService := sessionpresentationorch.New(sessionpresentationrepo.NewRedis(redisClient))
	presentationHandlerImpl, err := sessionpresentationhandler.New(&sessionpresentationhandler.HandlerConfig{
		Service: presentationService,
		Access:  access,
	})
	if err != nil {
		return fmt.Errorf("session presentation handler: %w", err)
	}
	sessionpresentationv1alpha1pb.RegisterSessionPresentationServiceServer(srv, presentationHandlerImpl)

	// Content registry (rpg-api#806): constructed UNCONDITIONALLY -- the
	// default dungeon must load whether or not authoring is on, because a
	// StartEncounter with no dungeon_key plays it. A file that does not
	// compile fails the boot here, naming itself.
	contentDir := os.Getenv(envContentDir)
	if contentDir == "" {
		if authoringEnabled {
			return fmt.Errorf("%s=1 requires %s to be set", envAuthoringEnabled, envContentDir)
		}
		contentDir = dungeons.ShippedContentDir
	} else {
		// A mounted content directory starts EMPTY on a fresh box; the
		// registry refuses a directory without the default, so seed it from the
		// copy the image ships. Never overwrites an existing file.
		shippedDir := os.Getenv(envShippedContentDir)
		if shippedDir == "" {
			shippedDir = dungeons.ShippedContentDir
		}
		seeded, seedErr := dungeons.SeedShipped(contentDir, shippedDir)
		if seedErr != nil {
			return fmt.Errorf("content registry: %w", seedErr)
		}
		if len(seeded) > 0 {
			//nolint:gosec // operator-supplied env var, logged once at boot
			log.Printf("content registry: seeded %s into %s from the shipped copy",
				strings.Join(seeded, ", "), contentDir)
		}
	}
	// sessionOrch.Manager is the AtlasProjector: Manager.AtlasOf projects the
	// compiled dungeon the way GetAtlas does, so PutDungeon's atlas and the game's are one producer.
	// Two stores (rpg-project#585): the content directory is the SHIPPED
	// store, read once and never written by a request; saves land in the
	// author's world store in Redis, owned by the author.
	projector := registryProjector{sessionOrch.Manager}
	shippedStore, err := dungeons.NewFileRegistry(contentDir, projector)
	if err != nil {
		return fmt.Errorf("content registry: %w", err)
	}
	// The benchmark keep's server row "registry compile ms" (rpg-project#740):
	// one line, read by the harness, so its shape is fixed.
	load := shippedStore.LoadReport()
	//nolint:gosec // LoadReport.String prints only a file count and a duration
	log.Printf("content registry: %s", load)
	if warmErr := warmShippedSites(context.Background(), shippedStore, sites); warmErr != nil {
		return fmt.Errorf("content registry: %w", warmErr)
	}
	worldDungeons, err := worlddungeons.NewRedis(&worlddungeons.RedisConfig{Client: redisClient})
	if err != nil {
		return fmt.Errorf("world dungeon store: %w", err)
	}
	registry, err := dungeons.NewRegistry(&dungeons.RegistryConfig{
		Shipped:   shippedStore,
		World:     worldDungeons,
		Authoring: authoringEnabled,
		Projector: projector,
	})
	if err != nil {
		return fmt.Errorf("content registry: %w", err)
	}
	//nolint:gosec // operator-supplied env var, logged once at boot
	log.Printf("content registry: shipped %q, world store redis dungeon:world:<world> (authoring enabled: %t)",
		contentDir, authoringEnabled)

	// LobbyService v1alpha1 (rpg-api#629): party assembly + the sole encounter
	// construction path. StartEncounter builds directly onto sessionOrch's
	// Manager -- the session stack is the only stack now (rpg-project#227).
	lobbyBroker := lobbyorch.NewBroker()
	lobbyCfg := &lobbyorch.Config{
		LobbyRepo:            lobbyRepo,
		LobbyBroker:          lobbyBroker,
		CharacterRepo:        charRepo,
		LobbyIDGenerator:     idgen.NewUUID("lobby"),
		JoinRefGenerator:     idgen.NewUUID("join"),
		EncounterIDGenerator: idgen.NewUUID(""),
		SessionManager:       sessionOrch.Manager,
		Dungeons:             registry,
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
	healthServer.SetServingStatus(sessionpresentationv1alpha1pb.SessionPresentationService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("dnd5e.api.lobby.v1alpha1.LobbyService", grpc_health_v1.HealthCheckResponse_SERVING)

	compositionRegistered, err := registerCompositionService(srv, &compositionRegistrationConfig{
		AuthoringEnabled: authoringEnabled,
		Redis:            redisClient,
	})
	if err != nil {
		return fmt.Errorf("composition service: %w", err)
	}
	if compositionRegistered {
		healthServer.SetServingStatus(compositionv1alpha1ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	}

	// AuthoringService v1alpha1 (rpg-api#806, rpg-project#256): the dungeon
	// builder's seam. Registered only when RPG_AUTHORING_ENABLED=1, so a
	// client with the gate off sees Unimplemented -- its way of telling
	// "authoring is off" from "server unreachable" (the proto's own words).
	// GetDungeon rides the same service, so with the gate off the builder's
	// probe fails and the Home button stays hidden.
	if authoringEnabled {
		authoringOrch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
		if err != nil {
			return fmt.Errorf("authoring orchestrator: %w", err)
		}
		authoringHandlerImpl, err := authoringhandler.New(&authoringhandler.HandlerConfig{
			Orchestrator: authoringOrch,
		})
		if err != nil {
			return fmt.Errorf("authoring handler: %w", err)
		}
		authoringv1alpha1pb.RegisterAuthoringServiceServer(srv, authoringHandlerImpl)
		healthServer.SetServingStatus("dnd5e.api.authoring.v1alpha1.AuthoringService", grpc_health_v1.HealthCheckResponse_SERVING)
		log.Println("AuthoringService registered (RPG_AUTHORING_ENABLED=1)")
	}

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

		// A Move may have answered before its save (rpg-project#879 R2):
		// wait for the continuations still running to save and deliver. The
		// drain has its own budget, because the forced path reaches here
		// with shutdownCtx already spent (an open stream keeps GracefulStop
		// from returning). Stop does not wait for handlers, so a Move still
		// mid-verb may reach the runner after the drain began; the runner
		// refuses it and that verb fails with nothing saved.
		drainCtx, drainCancel := context.WithTimeout(context.Background(), continuationDrainBudget)
		defer drainCancel()
		if err := sessionOrch.Drain(drainCtx); err != nil {
			log.Printf("session continuations not drained before shutdown: %v", err)
		} else {
			log.Println("Session continuations drained")
		}

		if err := lobbyBroker.Close(); err != nil {
			log.Printf("lobby broker close: %v", err)
		}

		return nil
	case err := <-errChan:
		return err
	}
}

// continuationDrainBudget is how long shutdown waits for session
// continuations (verbs that answered before their save) after the gRPC server
// stopped, whichever way it stopped.
const continuationDrainBudget = 10 * time.Second

// recordCaptureDrainBudget bounds accepted diagnostic export after the
// continuation drain; capture errors never roll back a committed game verb.
const recordCaptureDrainBudget = 10 * time.Second

// fieldMillis reads the middleware's `grpc.time_ms` field. go-grpc-middleware
// v2 sends it as a STRING (DurationToTimeMillisFields formats a float32 with
// %v); a float form is accepted too. Anything else (absent, unparseable, NaN,
// infinite, another type) reports false, and logFunc prints `?ms` with the
// raw value rather than a measurement: the harness reads the duration off the
// "completed in" line (rpg-project#740), and a silent 0.00ms would be recorded
// as a real row. Fail closed.
func fieldMillis(v any) (float64, bool) {
	var ms float64
	switch t := v.(type) {
	case string:
		parsed, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0, false
		}
		ms = parsed
	case float64:
		ms = t
	case float32:
		ms = float64(t)
	default:
		return 0, false
	}
	if math.IsNaN(ms) || math.IsInf(ms, 0) {
		return 0, false
	}
	return ms, true
}

// callMillis is the duration as the call lines print it: two decimals, or
// `?` with the raw field when fieldMillis could not read it.
func callMillis(raw any) string {
	if ms, ok := fieldMillis(raw); ok {
		return strconv.FormatFloat(ms, 'f', 2, 64) + "ms"
	}
	return fmt.Sprintf("?ms (grpc.time_ms=%v)", raw)
}

func logFunc(_ context.Context, level grpc_logging.Level, msg string, fields ...any) {
	// Extract useful information from fields
	var method, code, errorMsg string
	var timeMs any

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
			timeMs = fields[i+1]
		}
	}

	// Format based on message type
	switch msg {
	case "started call":
		log.Printf("→ %s started", method)
	case "finished call":
		if code == "OK" || code == "0" {
			log.Printf("✓ %s completed in %s", method, callMillis(timeMs))
		} else {
			log.Printf("✗ %s failed (%s) in %s: %s", method, code, callMillis(timeMs), errorMsg)
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

// newSites is the process's site provider, bounded by envSiteCacheEntries,
// logging each compile.
func newSites() (*sessionorch.Sites, error) {
	entries := 0
	if v := os.Getenv(envSiteCacheEntries); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("%s=%q: want a positive integer", envSiteCacheEntries, v)
		}
		entries = n
	}
	sites, err := sessionorch.NewSites(&sessionorch.SitesConfig{Entries: entries, Logf: log.Printf})
	if err != nil {
		return nil, fmt.Errorf("site provider: %w", err)
	}
	return sites, nil
}

// newProcessCaches is the process's site provider and residency, the
// residency's bounds logged.
func newProcessCaches() (*sessionorch.Sites, *sessionorch.Residency, error) {
	sites, err := newSites()
	if err != nil {
		return nil, nil, err
	}
	residency, err := newResidency()
	if err != nil {
		return nil, nil, err
	}
	log.Printf("session residency: %s", residency.Bounds())
	return sites, residency, nil
}

// newResidency is the process's residency, bounded by envResidentEntries and
// envResidentMaxMB.
func newResidency() (*sessionorch.Residency, error) {
	cfg := &sessionorch.ResidencyConfig{}
	if v := os.Getenv(envResidentEntries); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n <= 0 && n != -1) {
			return nil, fmt.Errorf("%s=%q: want a positive integer, or -1 to keep none", envResidentEntries, v)
		}
		cfg.Entries = n
	}
	if v := os.Getenv(envResidentMaxMB); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("%s=%q: want a positive integer", envResidentMaxMB, v)
		}
		cfg.MaxMB = n
	}
	residency, err := sessionorch.NewResidency(cfg)
	if err != nil {
		return nil, fmt.Errorf("residency: %w", err)
	}
	return residency, nil
}

// warmShippedSites resolves every shipped dungeon's site through the
// process's provider and logs which hash each key plays on (rpg-project#879
// S5). The registry's atlas projection has already compiled them, so this
// compiles nothing unless that path stopped going through the provider, and
// the "site compiled" lines above it would say so.
func warmShippedSites(ctx context.Context, store *dungeons.FileRegistry, sites *sessionorch.Sites) error {
	shipped, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("list shipped dungeons: %w", err)
	}
	for _, s := range shipped {
		entry, err := store.Get(ctx, s.Key)
		if err != nil {
			return fmt.Errorf("shipped dungeon %q: %w", s.Key, err)
		}
		hash, err := sites.Warm(ctx, &entry.Dungeon.Spec.Field)
		if err != nil {
			return fmt.Errorf("site of shipped dungeon %q: %w", s.Key, err)
		}
		//nolint:gosec // shipped dungeon keys match [a-z0-9-]+ and a hash is hex
		log.Printf("content registry: %s plays on site %s", s.Key, hash)
	}
	return nil
}

// registryProjector adapts *session.Manager's AtlasOf (which takes the SDK's
// own input struct) to dungeons.AtlasProjector.
type registryProjector struct{ m *sdk.Manager }

func (p registryProjector) AtlasOf(
	ctx context.Context, key string, dungeon *tkdungeonspec.Compiled,
) (*sdk.Atlas, error) {
	return p.m.AtlasOf(ctx, &sdk.AtlasOfInput{Dungeon: dungeon, DungeonKey: key})
}
