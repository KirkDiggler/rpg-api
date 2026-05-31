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
		},
		ReactionResume: encounterorch.ReactionResume{
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
// data may be nil for the CreateEncounter path (new encounter, no monsters).
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

// MoveEntity loads the encounter, validates that the request's entity_id matches
// the auth player's controlled entity, dispatches to the toolkit's Encounter.Move
// verb (which publishes per-viewer events to the broker), and saves the updated
// state. The empty MoveEntityResponse is by proto design — world changes flow as
// events on StreamEncounter.
func (h *Handler) MoveEntity(ctx context.Context, req *encounterv2pb.MoveEntityRequest) (*encounterv2pb.MoveEntityResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter %q: %v", req.GetEncounterId(), err)
	}

	// entity_id validation: the player's controlled entity must match the
	// request's entity_id. Read from data BEFORE LoadFromData consumes it.
	pd, ok := data.Players[core.PlayerID(playerID)]
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}
	if string(pd.EntityID) != req.GetEntityId() {
		return nil, status.Error(codes.PermissionDenied, "entity_id does not match player's controlled entity")
	}

	if len(req.GetProposedPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "proposed_path is required")
	}

	// #689: attach player character blobs (transient) so the hydration cascade
	// holds the mover; the movement resolver reads the held mover (no re-load).
	if attachErr := attachPlayerCharacterData(ctx, data, h.combatResolverConfig.CharacterRepo); attachErr != nil {
		return nil, status.Errorf(codes.Internal, "attach character data %q: %v", req.GetEncounterId(), attachErr)
	}

	enc, err := encounter.LoadFromData(ctx, data, h.broker,
		encounter.WithCharacterResolver(h.resolver),
		encounter.WithCombatResolver(h.buildCombatResolver(data)),
		encounter.WithMovementResolver(h.buildMovementResolver(data)))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data %q: %v", req.GetEncounterId(), err)
	}

	path := make([]core.Hex, 0, len(req.GetProposedPath()))
	for _, p := range req.GetProposedPath() {
		path = append(path, core.Hex{Q: int(p.X), R: int(p.Y), S: int(p.Z)})
	}

	if err := enc.Move(core.PlayerID(playerID), path); err != nil {
		// Toolkit Move errors are state-dependent (turn order, blocked path,
		// out-of-range, entity not in encounter, etc.) — these are
		// FailedPrecondition per gRPC convention. Empty-path is the only
		// genuinely argument-shaped error and is filtered before we reach Move.
		return nil, status.Errorf(codes.FailedPrecondition, "move: %v", err)
	}

	out := enc.ToData()
	if syncErr := enc.SyncErr(); syncErr != nil {
		return nil, status.Errorf(codes.Internal, "sync encounter state %q: %v", req.GetEncounterId(), syncErr)
	}
	if err := persistPlayerCharacterData(ctx, out, h.combatResolverConfig.CharacterRepo); err != nil {
		return nil, status.Errorf(codes.Internal, "persist character data %q: %v", req.GetEncounterId(), err)
	}
	if err := h.encRepo.Save(ctx, out); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter %q: %v", req.GetEncounterId(), err)
	}

	return &encounterv2pb.MoveEntityResponse{}, nil
}

// StreamEncounter opens a server-streaming session for the authenticated player.
// It emits an initial SnapshotDelivered event immediately, then forwards all
// subsequent broker events for the encounter until the client disconnects.
//
// Subscribe-before-snapshot ordering is intentional: subscribing first ensures
// no events are missed while the snapshot is being built. The broker's buffered
// channel holds any in-flight events until the forward loop starts.
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
	pbEncounter, err := ProjectFor(ctx, data, core.PlayerID(playerID), h.broker, now)
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
