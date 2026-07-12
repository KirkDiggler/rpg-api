package encounter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	tkevents "github.com/KirkDiggler/rpg-toolkit/encounter/events"
)

// HandlerConfig configures a v2 encounter Handler.
type HandlerConfig struct {
	Broker                 *encounter.Broker
	Repo                   encountersv2.Repository
	Resolver               CharacterResolver            // optional; defaults to StubCharacterResolver
	CombatResolver         CombatResolver               // optional; overrides CombatResolverConfig when set
	CombatResolverConfig   *Dnd5eCombatResolverConfig   // optional; used to build Dnd5eCombatResolver per-request
	MovementResolverConfig *Dnd5eMovementResolverConfig // optional; used to build Dnd5eMovementResolver per-request (Wave 2.11e #539)
	Now                    func() time.Time             // optional; defaults to time.Now
}

// Handler implements dnd5e.api.v1alpha2.encounter.EncounterServiceServer.
//
// Only MoveEntity and StreamEncounter ship in slice 1. Every other RPC
// returns codes.Unimplemented via the embedded server.
type Handler struct {
	encounterv2pb.UnimplementedEncounterServiceServer
	broker   *encounter.Broker
	encRepo  encountersv2.Repository
	resolver CharacterResolver
	// combatResolver is non-nil when a fixed resolver is wired (e.g. tests that
	// use StandInCombatResolver for deterministic outcomes). When nil,
	// buildCombatResolver creates a per-request Dnd5eCombatResolver.
	combatResolver         CombatResolver
	combatResolverConfig   Dnd5eCombatResolverConfig
	movementResolverConfig Dnd5eMovementResolverConfig
	now                    func() time.Time
	// orch is the v2 encounter orchestrator (rpg-api#582 carve-out): the single
	// load → toolkit-verb → persist core for every action verb. The handler
	// method is a thin proto↔input map + sentinel→gRPC status mapping. As of
	// #582 step 4 the handler-package Runner is retired — ActivateFeature was its
	// last caller and now dispatches through orch.ActivateFeature.
	orch *encounterorch.Orchestrator
}

// New constructs a Handler. Returns error on missing required deps.
func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, errors.New("HandlerConfig is required")
	}
	if cfg.Broker == nil {
		return nil, errors.New("HandlerConfig.Broker is required")
	}
	if cfg.Repo == nil {
		return nil, errors.New("HandlerConfig.Repo is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	resolver := cfg.Resolver
	if resolver == nil {
		// Wave 2.9 default: zero-modifier stub. SubmitCheck can resolve rolls
		// (total = roll + 0 + 0) without an explicit character lookup. Replace
		// with a real bridge to the character store once player→character
		// resolution lands on the encounter (follow-up to #514).
		resolver = StubCharacterResolver{}
	}

	// combatResolver selection:
	//  - Explicit CombatResolver wins — used by tests that need a fixed-output
	//    resolver (e.g. StandInCombatResolver) without a character repo.
	//  - CombatResolverConfig present → build Dnd5eCombatResolver per-request
	//    (the production path; see buildCombatResolver).
	//  - Neither set → build Dnd5eCombatResolver per-request with no char repo
	//    (falls back to stand-in math for player attacks when char repo absent).
	//    This keeps existing handler tests passing without requiring a char repo.
	combatResolverConfig := Dnd5eCombatResolverConfig{}
	if cfg.CombatResolverConfig != nil {
		combatResolverConfig = *cfg.CombatResolverConfig
	}

	// Wave 2.11e #539: MovementResolver wired alongside CombatResolver. Optional
	// — when nil, the encounter falls back to single-jump movement (no OAs)
	// preserving non-combat-encounter behavior. Production handlers always
	// supply the config so player and NPC movement both trigger OAs.
	movementResolverConfig := Dnd5eMovementResolverConfig{}
	if cfg.MovementResolverConfig != nil {
		movementResolverConfig = *cfg.MovementResolverConfig
	}

	h := &Handler{
		broker:                 cfg.Broker,
		encRepo:                cfg.Repo,
		resolver:               resolver,
		combatResolver:         cfg.CombatResolver, // nil unless caller provides a fixed resolver
		combatResolverConfig:   combatResolverConfig,
		movementResolverConfig: movementResolverConfig,
		now:                    now,
	}

	// v2 encounter orchestrator (#582). The handler supplies its existing
	// per-request resolver builders so the orchestrator stays free of rulebook
	// imports — the rulebook-importing Dnd5eCombatResolver / Dnd5eMovementResolver
	// adapters are built here and handed in behind interface-typed builders.
	// The #689 hydration cascade and the reaction-resume rulebook decode/
	// modifier-build are supplied as funcs so the orchestrator stays free of the
	// rulebooks/dnd5e/{character,combat} imports — the marshaling + rule
	// magnitudes (Shield +5 AC) live in this handler package's adapter seam
	// (hydrate_players.go, reaction_resume.go).
	charRepo := combatResolverConfig.CharacterRepo
	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:              cfg.Broker,
		EncounterRepo:       cfg.Repo,
		Resolver:            resolver,
		BuildCombatResolver: h.buildCombatResolver,
		BuildMovementResolver: func(data *encounter.Data) encounter.MovementResolver {
			return h.buildMovementResolver(data)
		},
		CharacterData: encounterorch.CharacterDataCascade{
			Attach: func(ctx context.Context, data *encounter.Data) error {
				return attachPlayerCharacterData(ctx, data, charRepo)
			},
			Persist: func(ctx context.Context, data *encounter.Data) error {
				return persistPlayerCharacterData(ctx, data, charRepo)
			},
			SeedActorTurn: func(ctx context.Context, data *encounter.Data) error {
				return SeedTurnEconomyForData(ctx, data, charRepo)
			},
		},
		ReactionResume: encounterorch.ReactionResume{
			MarshalAttackContext:   marshalReactionAttackContext,
			DecodeAttackContext:    decodeReactionAttackContext,
			BuildReactionModifiers: buildReactionModifiers,
			IsOneShotReaction:      isOneShotReaction,
		},
		Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("build encounter orchestrator: %w", err)
	}
	h.orch = orch

	return h, nil
}

// buildCombatResolver returns the combat resolver for a given request.
// If a fixed combatResolver was configured (e.g. for tests), it is returned
// directly. Otherwise, a fresh Dnd5eCombatResolver is constructed with the
// encounter data so the resolver can access the monster map for rehydration.
//
// data comes from the orchestrator's load() (internal/orchestrators/encounter/v2)
// and is always the existing encounter's snapshot — never nil in production.
// (The lobby orchestrator's StartEncounter builds its own resolver the same
// way for a fresh, monster-less encounter; that is a separate Handler
// instance's construction, not this one.)
func (h *Handler) buildCombatResolver(data *encounter.Data) CombatResolver {
	if h.combatResolver != nil {
		return h.combatResolver
	}
	return NewDnd5eCombatResolverForData(h.combatResolverConfig, data)
}

// buildMovementResolver returns the movement resolver for a given request.
// Constructed fresh per LoadFromData / New site so the resolver has access
// to the current encounter data for spatial Room construction. Wave 2.11e
// #539 — wired alongside buildCombatResolver via WithMovementResolver.
func (h *Handler) buildMovementResolver(data *encounter.Data) *Dnd5eMovementResolver {
	return NewDnd5eMovementResolverForData(h.movementResolverConfig, data)
}

// MoveEntity moves the auth player's controlled entity along a proposed path.
// The handler validates the envelope (auth, encounter_id), converts the proto
// path positions to core.Hex, delegates the load → Move verb → persist cycle to
// the v2 orchestrator (#582 step 6, retiring the inline body), and maps the
// orchestrator's sentinel errors onto gRPC status codes. The empty
// MoveEntityResponse is by proto design — world changes (the mover landing,
// opportunity attacks resolving) flow as events on StreamEncounter.
//
// NO rule logic lives here. The toolkit's Encounter.Move owns all movement rules
// (per-step MovementChain, opportunity-attack triggers, path truncation); the
// movement resolver (the rulebook-importing tkenc↔dnd5e seam) is injected into
// the orchestrator via BuildMovementResolver, so the orchestrator stays
// rulebook-free.
func (h *Handler) MoveEntity(ctx context.Context, req *encounterv2pb.MoveEntityRequest) (*encounterv2pb.MoveEntityResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	// Proto → entity conversion: the path positions become toolkit hexes. Empty
	// path is classified by the orchestrator (ErrEmptyPath) AFTER load so the auth
	// sentinels keep precedence over the argument-shaped error.
	path := make([]core.Hex, 0, len(req.GetProposedPath()))
	for _, p := range req.GetProposedPath() {
		path = append(path, core.Hex{Q: int(p.X), R: int(p.Y), S: int(p.Z)})
	}

	_, err := h.orch.MoveEntity(ctx, &encounterorch.MoveEntityInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    core.PlayerID(playerID),
		EntityID:    core.EntityID(req.GetEntityId()),
		Path:        path,
	})
	if err != nil {
		return nil, moveEntityStatusError(err)
	}

	return &encounterv2pb.MoveEntityResponse{}, nil
}

// moveEntityStatusError maps the orchestrator's MoveEntity errors onto gRPC
// status codes per pat-v2-status-code-mapping. The orchestrator's load sentinels
// carry the auth classification; ErrEmptyPath is the argument-shaped error; the
// toolkit Move refusals (joined with ErrMoveRefused) carry the state-dependent
// classification.
//
//   - ErrEncounterNotFound → NotFound
//   - ErrPlayerNotInEncounter / ErrEntityOwnershipMismatch → PermissionDenied
//   - ErrEmptyPath → InvalidArgument (the request shape is wrong, not the state)
//   - ErrMoveRefused → FailedPrecondition (turn order, blocked path, out-of-range)
//   - load / save / unclassified failures → Internal
func moveEntityStatusError(err error) error {
	switch {
	case errors.Is(err, encounterorch.ErrEncounterNotFound):
		return status.Error(codes.NotFound, "encounter not found")
	case errors.Is(err, encounterorch.ErrPlayerNotInEncounter):
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	case errors.Is(err, encounterorch.ErrEntityOwnershipMismatch):
		return status.Error(codes.PermissionDenied,
			"entity_id does not match player's controlled entity")
	case errors.Is(err, encounterorch.ErrEmptyPath):
		return status.Error(codes.InvalidArgument, "proposed_path is required")
	case errors.Is(err, encounterorch.ErrMoveRefused):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Errorf(codes.Internal, "move entity: %v", err)
}

// StreamEncounter opens a server-streaming session for the authenticated player.
// It emits an initial SnapshotDelivered event immediately, then forwards all
// subsequent broker events for the encounter until the client disconnects.
//
// Subscribe-before-snapshot ordering is intentional: subscribing first ensures
// no events are missed while the snapshot is being built. The broker's buffered
// channel holds any in-flight events until the forward loop starts.
//
// rpg-api#636 combat-entry kick: run BEFORE Subscribe, not after. A TURN_BASED
// encounter whose active actor is an NPC never gets driven by anything if
// nothing preceded this connect with an EndTurn (today: devseed
// --inject-combat writes Redis out-of-process, so the server never otherwise
// observes combat starting; the same gap applies to any future reconnect into
// an NPC-active encounter). DriveStalledNPCTurn is the same NPCAct+EndTurn
// dispatch loop EndTurn's own NPC chain uses (see
// orchestrators/encounter/v2/drive_npc.go); calling it here makes
// StreamEncounter/GetEncounter self-healing for that stall without special-
// casing the dev tool. Kicking BEFORE Subscribe (rather than after, per the
// brief's other candidate ordering) means the kick's own NPCAct/EndTurn broker
// events do NOT land in THIS connection's buffered channel — they fan out live
// to any OTHER already-subscribed viewers (who correctly see the goblin's turn
// resolve in real time), while this connecting client's snapshot (built after,
// from freshly re-read post-kick data) simply reflects the already-current
// state instead of replaying its own kick as a redundant post-snapshot event.
// Errors are logged and swallowed — a kick failure (or a caller who isn't a
// member, which DriveStalledNPCTurn's load enforces more strictly than this
// handler ever has) must never block the read; the subsequent membership-blind
// snapshot flow below is unchanged from pre-#636 behavior on any kick failure.
func (h *Handler) StreamEncounter(req *encounterv2pb.StreamEncounterRequest, stream encounterv2pb.EncounterService_StreamEncounterServer) error {
	ctx := stream.Context()
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return status.Error(codes.Unauthenticated, "no player id in context")
	}
	encID := core.EncounterID(req.GetEncounterId())
	if encID == "" {
		return status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	if _, kickErr := h.orch.DriveStalledNPCTurn(ctx, &encounterorch.DriveStalledNPCTurnInput{
		EncounterID: string(encID),
		PlayerID:    core.PlayerID(playerID),
	}); kickErr != nil {
		log.Printf("encounter/v2 StreamEncounter: combat-entry kick failed for %q: %v", string(encID), kickErr)
	}

	// Subscribe FIRST so the broker holds events in its buffered channel while
	// we build the snapshot. Any event firing between Subscribe and the forward
	// loop is captured and delivered after the snapshot send.
	sub, err := h.broker.Subscribe(encID, core.PlayerID(playerID))
	if err != nil {
		return status.Errorf(codes.Internal, "subscribe %q: %v", string(encID), err)
	}
	defer func() { _ = sub.Close() }()

	// Snapshot the encounter at-time-of-connect.
	data, err := h.encRepo.Get(ctx, string(encID))
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return status.Error(codes.NotFound, "encounter not found")
		}
		return status.Errorf(codes.Internal, "load encounter %q: %v", string(encID), err)
	}
	// Build the projected encounter once; reuse for both the snapshot envelope and
	// the replay events so ProjectFor's broadening logic runs exactly once.
	// ProjectFor internally rehydrates the encounter and computes the per-viewer
	// snapshot, so we don't need a separate LoadFromData/SnapshotFor here.
	now := h.now()
	pbEncounter, err := ProjectFor(ctx, data, core.PlayerID(playerID), h.broker, h.combatResolverConfig.CharacterRepo, now)
	if err != nil {
		return status.Errorf(codes.Internal, "project encounter %q: %v", string(encID), err)
	}

	snapEvent := TranslateSnapshot(pbEncounter, now)
	if err := stream.Send(snapEvent); err != nil {
		return err
	}

	// Send per-entity and geometry replay events before entering the live forward
	// loop. The broker's buffered channel holds any in-flight events that fired
	// between Subscribe and here; those will be drained after the replay completes.
	for _, replayEvt := range BuildReplayEvents(pbEncounter, now) {
		if err := stream.Send(replayEvt); err != nil {
			return err
		}
	}

	// Forward broker events until the client disconnects or the subscription closes.
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-sub.Events():
			if !ok {
				return nil
			}
			out, translateErr := h.translateForStream(ctx, encID, evt, core.PlayerID(playerID))
			switch {
			case errors.Is(translateErr, ErrViewerSawNothing):
				continue
			case errors.Is(translateErr, ErrEventSuppressed):
				// Translator deliberately drops this event — no wire shape,
				// expected behavior. Continue without log or send.
				continue
			case errors.Is(translateErr, ErrUnknownEventType):
				// Translator has no mapping for this event type — log so
				// the gap shows up in production rather than silently
				// dropping. TODO(metric): also increment a gap counter.
				log.Printf("encounter/v2 translator gap: encounter=%q event=%T", string(encID), evt)
				continue
			case translateErr != nil:
				return status.Errorf(codes.Internal, "translate %q: %v", string(encID), translateErr)
			}
			if err := stream.Send(out); err != nil {
				return err
			}
		}
	}
}

// translateForStream wraps TranslateEvent with the data-aware path for
// InputRequiredDeliveredEvent (Wave 2.11d). Wave 2.11d's reaction prompts
// store their content on Encounter.Data.PendingReactionPrompts (rather than
// on the event payload), so the translator needs the encounter snapshot to
// look up the prompt content. Other event types continue to use
// TranslateEvent unchanged.
func (h *Handler) translateForStream(
	ctx context.Context,
	encID core.EncounterID,
	evt tkevents.EncounterEvent,
	viewer core.PlayerID,
) (*encounterv2pb.EncounterEvent, error) {
	if irEvt, ok := evt.(*tkevents.InputRequiredDeliveredEvent); ok {
		// Load the encounter to read the pending prompt content. The prompt
		// lives on Data.PendingReactionPrompts and is the canonical source
		// of truth for content (the event itself is metadata-only).
		data, err := h.encRepo.Get(ctx, string(encID))
		if err != nil {
			return nil, fmt.Errorf("load encounter for prompt translation: %w", err)
		}
		var prompt *encounter.PendingReactionPrompt
		if data != nil {
			prompt = data.PendingReactionPrompts[irEvt.ReactorID]
		}
		return TranslateInputRequiredDelivered(irEvt, viewer, h.now(), prompt)
	}
	return TranslateEvent(evt, viewer, h.now())
}
