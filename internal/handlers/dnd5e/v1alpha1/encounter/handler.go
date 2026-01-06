// Package encounter handles the gRPC service interface for encounter management
package encounter

import (
	"context"
	"errors"
	"fmt"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	apierrors "github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	encounterpub "github.com/KirkDiggler/rpg-api/internal/publishers/encounter"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// HandlerConfig holds dependencies for the handler
type HandlerConfig struct {
	EncounterService encounter.Service
	Publisher        encounterpub.Publisher // For event streaming
}

// Validate ensures all required dependencies are present
func (c *HandlerConfig) Validate() error {
	if c.EncounterService == nil {
		return apierrors.InvalidArgument("encounter service is required")
	}
	// Publisher is optional - streaming just won't work without it
	return nil
}

// Handler implements the D&D 5e Encounter gRPC service
type Handler struct {
	dnd5ev1alpha1.UnimplementedEncounterServiceServer
	encounterService encounter.Service
	publisher        encounterpub.Publisher
}

// New creates a new handler with the given configuration
func New(cfg *HandlerConfig) (*Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Handler{
		encounterService: cfg.EncounterService,
		publisher:        cfg.Publisher,
	}, nil
}

// Attack handles attack requests in combat
func (h *Handler) Attack(ctx context.Context, req *dnd5ev1alpha1.AttackRequest) (*dnd5ev1alpha1.AttackResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetAttackerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "attacker_id is required")
	}
	if req.GetTargetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_id is required")
	}

	// 2. Create service input
	input := &encounter.ResolveAttackInput{
		EncounterID: req.GetEncounterId(),
		AttackerID:  req.GetAttackerId(),
		TargetID:    req.GetTargetId(),
		WeaponID:    req.GetWeaponId(),                             // Optional
		AttackHand:  protoAttackHandToToolkit(req.GetAttackHand()), // For two-weapon fighting
	}

	// 3. Call service
	output, err := h.encounterService.ResolveAttack(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert to proto response
	return &dnd5ev1alpha1.AttackResponse{
		Success: true,
		Result:  convertAttackResultToProto(output.Result),
		// TODO: Add combat_state, updated_room when available
	}, nil
}

// DungeonStart handles dungeon start requests
func (h *Handler) DungeonStart(
	ctx context.Context,
	req *dnd5ev1alpha1.DungeonStartRequest,
) (*dnd5ev1alpha1.DungeonStartResponse, error) {
	// 1. Create service input with character IDs
	input := &encounter.CreateDungeonInput{
		CharacterIDs: req.GetCharacterIds(),
	}

	// 2. Call service
	output, err := h.encounterService.CreateDungeon(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 3. Extract grid info for cube coordinate conversion
	gridType, hexOrientation := extractGridInfo(output.Room)

	// 4. Convert to proto response with cube coordinates for hex grids
	return &dnd5ev1alpha1.DungeonStartResponse{
		EncounterId:  output.EncounterID,
		DungeonId:    output.DungeonID,
		Room:         convertRoomDataToProto(output.Room),
		CombatState:  convertCombatStateToProto(output.CombatState, gridType, hexOrientation),
		MonsterTurns: convertMonsterTurnsToProto(output.MonsterTurns, gridType, hexOrientation),
		Doors:        convertDoorInfoSliceToProto(output.Doors),
	}, nil
}

// OpenDoor opens a door to reveal the next room and add monsters to initiative
func (h *Handler) OpenDoor(
	ctx context.Context,
	req *dnd5ev1alpha1.OpenDoorRequest,
) (*dnd5ev1alpha1.OpenDoorResponse, error) {
	// 1. Validate request
	if req.GetDungeonId() == "" {
		return nil, status.Error(codes.InvalidArgument, "dungeon_id is required")
	}
	if req.GetConnectionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "connection_id is required")
	}

	// 2. Create service input
	input := &encounter.OpenDoorInput{
		DungeonID:    req.GetDungeonId(),
		ConnectionID: req.GetConnectionId(),
	}

	// 3. Call service
	output, err := h.encounterService.OpenDoor(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert to proto response
	// Use default hex grid settings for coordinate conversion
	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	return &dnd5ev1alpha1.OpenDoorResponse{
		Success:      true,
		EncounterId:  output.EncounterID,
		Room:         convertOpenDoorRoomToProto(output.RevealedRoom),
		CombatState:  convertCombatStateToProto(output.CombatState, gridType, hexOrientation),
		MonsterTurns: convertMonsterTurnsToProto(output.MonsterTurns, gridType, hexOrientation),
		Doors:        convertDoorInfoSliceToProto(output.NewDoors),
	}, nil
}

// GetCombatState retrieves current combat state.
//
// Deprecated: Use GetEncounterState for full snapshot with event sync.
func (h *Handler) GetCombatState(
	_ context.Context,
	_ *dnd5ev1alpha1.GetCombatStateRequest,
) (*dnd5ev1alpha1.GetCombatStateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetCombatState is deprecated, use GetEncounterState instead")
}

// GetEncounterState retrieves full encounter snapshot for load-then-stream pattern
func (h *Handler) GetEncounterState(
	ctx context.Context,
	req *dnd5ev1alpha1.GetEncounterStateRequest,
) (*dnd5ev1alpha1.GetEncounterStateResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetPlayerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "player_id is required")
	}

	// 2. Create service input
	input := &encounter.GetEncounterStateInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    req.GetPlayerId(),
	}

	// 3. Call service
	output, err := h.encounterService.GetEncounterState(ctx, input)
	if err != nil {
		if errors.Is(err, encounter.ErrEncounterNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		if errors.Is(err, encounter.ErrPlayerNotInEncounter) {
			return nil, status.Error(codes.PermissionDenied, "player not in encounter")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert to proto response
	response := &dnd5ev1alpha1.GetEncounterStateResponse{
		EncounterId: output.EncounterID,
		State:       convertEncounterStateToProto(output.State),
		JoinCode:    output.JoinCode,
		HostId:      output.HostID,
		LastEventId: output.LastEventID,
	}

	// Convert party members
	response.Party = convertPartyToProto(output.Party)

	// Extract grid info for coordinate conversion
	gridType, hexOrientation := extractGridInfo(output.Room)

	// Convert combat state if present
	if output.CombatState != nil {
		response.CombatState = convertCombatStateToProto(output.CombatState, gridType, hexOrientation)
	}

	// Convert room and add walls (same pattern as StartCombat)
	if output.Room != nil {
		response.Room = convertRoomDataToProto(output.Room)
		if response.Room != nil {
			response.Room.Walls = convertWallsToProto(output.Walls)
		}
	}

	// Convert monsters
	for _, m := range output.Monsters {
		response.Monsters = append(response.Monsters, &dnd5ev1alpha1.MonsterCombatState{
			MonsterId:        m.MonsterID,
			MonsterName:      m.MonsterName,
			CurrentHitPoints: int32(m.CurrentHitPoints),
			MaxHitPoints:     int32(m.MaxHitPoints),
			MonsterType:      m.MonsterType,
		})
	}

	// Convert doors and dungeon ID
	response.Doors = convertDoorInfoSliceToProto(output.Doors)
	response.DungeonId = output.DungeonID

	return response, nil
}

// MoveCharacter handles character movement
func (h *Handler) MoveCharacter(
	ctx context.Context,
	req *dnd5ev1alpha1.MoveCharacterRequest,
) (*dnd5ev1alpha1.MoveCharacterResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "entity_id is required")
	}
	if len(req.GetPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}

	// 2. Pass cube coordinates directly - the client sends cube coords (X, Y, Z)
	// and the orchestrator uses CubeEntities for hex grids
	lastPos := req.GetPath()[len(req.GetPath())-1]
	input := &encounter.MoveCharacterInput{
		EncounterID: req.GetEncounterId(),
		EntityID:    req.GetEntityId(),
		TargetPosition: &encounter.Position{
			X: lastPos.GetX(),
			Y: lastPos.GetY(),
			Z: lastPos.GetZ(),
		},
	}

	// 4. Call service
	output, err := h.encounterService.MoveCharacter(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 5. Convert to proto response
	response := &dnd5ev1alpha1.MoveCharacterResponse{
		Success:           output.Success,
		MovementRemaining: output.MovementRemaining,
	}

	// 6. Convert room data if present (will output cube coordinates)
	if output.UpdatedRoom != nil {
		response.UpdatedRoom = convertRoomDataToProto(output.UpdatedRoom)
		if response.UpdatedRoom != nil {
			response.UpdatedRoom.Walls = convertWallsToProto(output.Walls)
		}
	}

	return response, nil
}

// EndTurn handles turn ending
func (h *Handler) EndTurn(
	ctx context.Context,
	req *dnd5ev1alpha1.EndTurnRequest,
) (*dnd5ev1alpha1.EndTurnResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	// Note: entity_id is no longer required - server determines active entity from encounter state

	// 2. Get authenticated player ID
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "player not authenticated")
	}

	// 3. Create service input
	input := &encounter.EndTurnInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    playerID,
	}

	// 4. Call service
	output, err := h.encounterService.EndTurn(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Use default hex grid settings (D&D 5e default: hex pointy-top)
	// TODO: In the future, could fetch room data from encounter state for accuracy
	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	// 5. Convert to proto response with cube coordinates for hex grids
	response := &dnd5ev1alpha1.EndTurnResponse{
		Success:     true,
		CombatState: convertCombatStateToProto(output.CombatState, gridType, hexOrientation),
		TurnChange:  convertTurnChangeToProto(output.TurnChange),
	}

	// 6. Add monster turns if any were executed
	if len(output.MonsterTurns) > 0 {
		response.MonsterTurns = convertMonsterTurnsToProto(output.MonsterTurns, gridType, hexOrientation)
	}

	// 7. Add encounter result if combat ended
	if output.EncounterResult != nil {
		response.EncounterResult = convertEncounterResultToProto(output.EncounterResult)
	}

	return response, nil
}

// ActivateFeature handles combat feature activation (e.g., Rage)
func (h *Handler) ActivateFeature(
	ctx context.Context,
	req *dnd5ev1alpha1.ActivateFeatureRequest,
) (*dnd5ev1alpha1.ActivateFeatureResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetCharacterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}
	if req.GetFeatureId() == dnd5ev1alpha1.FeatureId_FEATURE_ID_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "feature_id is required")
	}

	// Convert enum to toolkit feature ID string
	featureID := featureEnumToID(req.GetFeatureId())
	if featureID == "" {
		return nil, status.Error(codes.InvalidArgument, "unsupported feature_id")
	}

	// 2. Create service input
	input := &encounter.ActivateFeatureInput{
		EncounterID: req.GetEncounterId(),
		CharacterID: req.GetCharacterId(),
		FeatureID:   featureID,
	}

	// 3. Call service
	output, err := h.encounterService.ActivateFeature(ctx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert character data to proto
	var updatedCharacter *dnd5ev1alpha1.Character
	if charData, ok := output.CharacterData.(*toolkitchar.Data); ok && charData != nil {
		updatedCharacter = characterhandler.ConvertCharacterDataToProto(charData)
	}

	// 5. Return response
	return &dnd5ev1alpha1.ActivateFeatureResponse{
		Success:          output.Success,
		Message:          output.Message,
		UpdatedCharacter: updatedCharacter,
		// TODO: Add UpdatedCombatState when needed
	}, nil
}

// CreateEncounter creates a new multiplayer encounter lobby
func (h *Handler) CreateEncounter(
	ctx context.Context,
	req *dnd5ev1alpha1.CreateEncounterRequest,
) (*dnd5ev1alpha1.CreateEncounterResponse, error) {
	// 1. Validate request
	if len(req.GetCharacterIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "character_ids is required")
	}

	// 2. Get player ID from auth context
	playerID := auth.GetPlayerID(ctx)

	// 3. Call orchestrator
	output, err := h.encounterService.CreateEncounter(ctx, &encounter.CreateEncounterInput{
		PlayerID:     playerID,
		CharacterIDs: req.GetCharacterIds(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert response
	return &dnd5ev1alpha1.CreateEncounterResponse{
		EncounterId: output.EncounterID,
		JoinCode:    output.JoinCode,
		Room:        convertRoomDataToProto(output.Room),
	}, nil
}

// JoinEncounter joins an existing encounter via join code
func (h *Handler) JoinEncounter(
	ctx context.Context,
	req *dnd5ev1alpha1.JoinEncounterRequest,
) (*dnd5ev1alpha1.JoinEncounterResponse, error) {
	// 1. Validate request
	if req.GetJoinCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "join_code is required")
	}
	if len(req.GetCharacterIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "character_ids is required")
	}

	// 2. Get player ID from auth context
	playerID := auth.GetPlayerID(ctx)

	// 3. Call orchestrator
	output, err := h.encounterService.JoinEncounter(ctx, &encounter.JoinEncounterInput{
		JoinCode:     req.GetJoinCode(),
		PlayerID:     playerID,
		CharacterIDs: req.GetCharacterIds(),
	})
	if err != nil {
		if errors.Is(err, encounter.ErrEncounterNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert response
	return &dnd5ev1alpha1.JoinEncounterResponse{
		EncounterId: output.EncounterID,
		Room:        convertRoomDataToProto(output.Room),
		Party:       convertPartyToProto(output.Party),
		State:       convertEncounterStateToProto(output.State),
	}, nil
}

// SetReady marks a player as ready to start combat
func (h *Handler) SetReady(
	ctx context.Context,
	req *dnd5ev1alpha1.SetReadyRequest,
) (*dnd5ev1alpha1.SetReadyResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetPlayerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "player_id is required")
	}

	// 2. Call orchestrator
	output, err := h.encounterService.SetReady(ctx, &encounter.SetReadyInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    req.GetPlayerId(),
		IsReady:     req.GetIsReady(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 3. Return response
	return &dnd5ev1alpha1.SetReadyResponse{
		Success: output.Success,
	}, nil
}

// StartCombat begins combat (host only, all players must be ready)
func (h *Handler) StartCombat(
	ctx context.Context,
	req *dnd5ev1alpha1.StartCombatRequest,
) (*dnd5ev1alpha1.StartCombatResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	// 2. Get player ID from auth context
	playerID := auth.GetPlayerID(ctx)

	// 3. Call orchestrator with dungeon config from request
	output, err := h.encounterService.StartCombat(ctx, &encounter.StartCombatInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    playerID,
		ThemeID:     protoDungeonThemeToString(req.GetTheme()),
		Difficulty:  protoDungeonDifficultyToString(req.GetDifficulty()),
		Length:      protoDungeonLengthToString(req.GetLength()),
	})
	if err != nil {
		if errors.Is(err, encounter.ErrNotHost) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if errors.Is(err, encounter.ErrPlayersNotReady) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		if errors.Is(err, encounter.ErrEncounterNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 4. Convert response
	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	// Convert room and add walls
	room := convertRoomDataToProto(output.Room)
	if room != nil {
		room.Walls = convertWallsToProto(output.Walls)
	}

	return &dnd5ev1alpha1.StartCombatResponse{
		CombatState: convertCombatStateToProto(output.CombatState, gridType, hexOrientation),
		Room:        room,
		Doors:       convertDoorInfoSliceToProto(output.Doors),
		DungeonId:   output.DungeonID,
	}, nil
}

// LeaveEncounter removes a player from the encounter
func (h *Handler) LeaveEncounter(
	ctx context.Context,
	req *dnd5ev1alpha1.LeaveEncounterRequest,
) (*dnd5ev1alpha1.LeaveEncounterResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetPlayerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "player_id is required")
	}

	// 2. Call orchestrator
	output, err := h.encounterService.LeaveEncounter(ctx, &encounter.LeaveEncounterInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    req.GetPlayerId(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 3. Return response
	return &dnd5ev1alpha1.LeaveEncounterResponse{
		Success: output.Success,
	}, nil
}

// GetEncounterHistory retrieves historical events for late join/reconnect
func (h *Handler) GetEncounterHistory(
	ctx context.Context,
	req *dnd5ev1alpha1.GetEncounterHistoryRequest,
) (*dnd5ev1alpha1.GetEncounterHistoryResponse, error) {
	// 1. Validate request
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	// 2. Call orchestrator
	output, err := h.encounterService.GetEncounterHistory(ctx, &encounter.GetEncounterHistoryInput{
		EncounterID: req.GetEncounterId(),
		UpToEventID: req.GetUpToEventId(),
		Limit:       int(req.GetLimit()),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 3. Convert events to proto
	protoEvents := make([]*dnd5ev1alpha1.EncounterEvent, 0, len(output.Events))
	for _, event := range output.Events {
		protoEvent, convertErr := h.convertToProtoEvent(event)
		if convertErr != nil {
			log.Printf("Failed to convert event %s: %v", event.ID, convertErr)
			continue // Skip malformed events
		}
		protoEvents = append(protoEvents, protoEvent)
	}

	return &dnd5ev1alpha1.GetEncounterHistoryResponse{
		Events:      protoEvents,
		HasMore:     output.HasMore,
		LastEventId: output.LastEventID,
	}, nil
}

// StreamEncounterEvents subscribes to real-time encounter events
func (h *Handler) StreamEncounterEvents(
	req *dnd5ev1alpha1.StreamEncounterEventsRequest,
	stream dnd5ev1alpha1.EncounterService_StreamEncounterEventsServer,
) error {
	// 1. Validate publisher is configured
	if h.publisher == nil {
		return status.Error(codes.Unavailable, "event streaming not configured")
	}

	// 2. Validate request
	if req.GetEncounterId() == "" {
		return status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	ctx := stream.Context()
	encounterID := req.GetEncounterId()
	playerID := req.GetPlayerId()

	log.Printf("Player %s subscribing to encounter %s events", playerID, encounterID)

	// 3. Subscribe to encounter events
	subOutput, err := h.publisher.Subscribe(ctx, &encounterpub.SubscribeInput{
		EncounterID: encounterID,
	})
	if err != nil {
		return status.Errorf(codes.Internal, "failed to subscribe to events: %v", err)
	}

	// Ensure we unsubscribe when the stream ends
	defer func() {
		if _, unsubErr := h.publisher.Unsubscribe(context.Background(), &encounterpub.UnsubscribeInput{
			SubscriptionID: subOutput.SubscriptionID,
		}); unsubErr != nil {
			log.Printf("Failed to unsubscribe %s: %v", subOutput.SubscriptionID, unsubErr)
		}
		log.Printf("Player %s disconnected from encounter %s", playerID, encounterID)
		// TODO: Call PlayerDisconnected on the orchestrator
	}()

	// 4. Stream events to the client
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return nil
		case err := <-subOutput.Errors:
			log.Printf("Subscription error for encounter %s: %v", encounterID, err)
			return status.Errorf(codes.Internal, "subscription error: %v", err)
		case event, ok := <-subOutput.Events:
			if !ok {
				// Channel closed
				return nil
			}

			// Convert internal event to proto event
			protoEvent, convertErr := h.convertToProtoEvent(event)
			if convertErr != nil {
				log.Printf("Failed to convert event: %v", convertErr)
				continue // Skip malformed events
			}

			// Send to client
			if sendErr := stream.Send(protoEvent); sendErr != nil {
				// Client likely disconnected
				return sendErr
			}
		}
	}
}

// convertToProtoEvent converts an internal EncounterEvent to a proto EncounterEvent
// Uses typed fields (oneof pattern) for type-safe event data access
func (h *Handler) convertToProtoEvent(event *entities.EncounterEvent) (*dnd5ev1alpha1.EncounterEvent, error) {
	protoEvent := &dnd5ev1alpha1.EncounterEvent{
		EventId:   event.ID,
		Timestamp: event.Timestamp.UnixMilli(),
	}

	// Set the appropriate oneof based on event type using typed fields
	switch event.Type {
	case entities.EventTypePlayerJoined:
		if event.PlayerJoined == nil {
			return nil, fmt.Errorf("missing PlayerJoined data for PlayerJoinedEvent")
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_PlayerJoined{
			PlayerJoined: &dnd5ev1alpha1.PlayerJoinedEvent{
				Member: &dnd5ev1alpha1.PartyMember{
					PlayerId:  event.PlayerJoined.PlayerID,
					Character: characterhandler.ConvertCharacterDataToProto(event.PlayerJoined.CharacterData),
				},
			},
		}

	case entities.EventTypePlayerLeft:
		if event.PlayerLeft == nil {
			return nil, fmt.Errorf("missing PlayerLeft data for PlayerLeftEvent")
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_PlayerLeft{
			PlayerLeft: &dnd5ev1alpha1.PlayerLeftEvent{
				PlayerId:    event.PlayerLeft.PlayerID,
				CharacterId: event.PlayerLeft.CharacterID,
			},
		}

	case entities.EventTypePlayerReady:
		if event.PlayerReady == nil {
			return nil, fmt.Errorf("missing PlayerReady data for PlayerReadyEvent")
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_PlayerReady{
			PlayerReady: &dnd5ev1alpha1.PlayerReadyEvent{
				PlayerId: event.PlayerReady.PlayerID,
				IsReady:  event.PlayerReady.Ready,
			},
		}

	case entities.EventTypeCombatStarted:
		if event.CombatStarted == nil {
			return nil, fmt.Errorf("missing CombatStarted data for CombatStartedEvent")
		}
		// Convert party members (including character data)
		var protoParty []*dnd5ev1alpha1.PartyMember
		if event.CombatStarted.Party != nil {
			protoParty = make([]*dnd5ev1alpha1.PartyMember, len(event.CombatStarted.Party))
			for i, p := range event.CombatStarted.Party {
				protoParty[i] = &dnd5ev1alpha1.PartyMember{
					PlayerId:    p.PlayerID,
					IsHost:      p.IsHost,
					IsReady:     p.IsReady,
					IsConnected: p.IsConnected,
					Character:   characterhandler.ConvertCharacterDataToProto(p.CharacterData),
				}
			}
		}
		// Convert monsters for event
		var protoMonsters []*dnd5ev1alpha1.MonsterCombatState
		if event.CombatStarted.Monsters != nil {
			protoMonsters = make([]*dnd5ev1alpha1.MonsterCombatState, len(event.CombatStarted.Monsters))
			for i, m := range event.CombatStarted.Monsters {
				protoMonsters[i] = &dnd5ev1alpha1.MonsterCombatState{
					MonsterId:        m.MonsterID,
					MonsterName:      m.MonsterName,
					CurrentHitPoints: int32(m.CurrentHitPoints),
					MaxHitPoints:     int32(m.MaxHitPoints),
					MonsterType:      dungeon.MonsterTypeFromID(m.MonsterType),
				}
			}
		}

		// Use default grid settings for event conversion
		gridType := spatial.GridTypeHex
		hexOrientation := spatial.HexOrientationPointyTop
		// Convert room and add walls
		combatStartedRoom := convertRoomDataToProto(event.CombatStarted.Room)
		if combatStartedRoom != nil {
			combatStartedRoom.Walls = convertDungeonWallsToProto(event.CombatStarted.Walls)
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_CombatStarted{
			CombatStarted: &dnd5ev1alpha1.CombatStartedEvent{
				CombatState:  convertCombatStateToProto(event.CombatStarted.CombatState, gridType, hexOrientation),
				Room:         combatStartedRoom,
				Party:        protoParty,
				Monsters:     protoMonsters,
				Doors:        convertEntityDoorsToProto(event.CombatStarted.Doors),
				DungeonId:    event.CombatStarted.DungeonID,
				MonsterTurns: convertEntityMonsterTurnsToProto(event.CombatStarted.MonsterTurns, gridType, hexOrientation),
			},
		}

	case entities.EventTypeMovementCompleted:
		if event.MovementCompleted == nil {
			return nil, fmt.Errorf("missing MovementCompleted data for MovementCompletedEvent")
		}
		var finalPos *apiv1alpha1.Position
		if event.MovementCompleted.FinalPosition != nil {
			finalPos = &apiv1alpha1.Position{
				X: event.MovementCompleted.FinalPosition.X,
				Y: event.MovementCompleted.FinalPosition.Y,
				Z: event.MovementCompleted.FinalPosition.Z,
			}
		}
		// Convert room and add walls
		updatedRoom := convertRoomDataToProto(event.MovementCompleted.UpdatedRoom)
		if updatedRoom != nil {
			updatedRoom.Walls = convertDungeonWallsToProto(event.MovementCompleted.Walls)
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_MovementCompleted{
			MovementCompleted: &dnd5ev1alpha1.MovementCompletedEvent{
				EntityId:          event.MovementCompleted.EntityID,
				MovementRemaining: event.MovementCompleted.MovementRemaining,
				StopReason:        event.MovementCompleted.StopReason,
				FinalPosition:     finalPos,
				UpdatedRoom:       updatedRoom,
			},
		}

	case entities.EventTypeAttackResolved:
		if event.AttackResolved == nil {
			return nil, fmt.Errorf("missing AttackResolved data for AttackResolvedEvent")
		}
		// Convert AttackResult - now a concrete type, no type assertion needed
		var attackResult *dnd5ev1alpha1.AttackResult
		if event.AttackResolved.Result != nil {
			attackResult = convertAttackResultToProto(event.AttackResolved.Result)
		}
		// Convert room and add walls
		attackResolvedRoom := convertRoomDataToProto(event.AttackResolved.Room)
		if attackResolvedRoom != nil {
			attackResolvedRoom.Walls = convertDungeonWallsToProto(event.AttackResolved.Walls)
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_AttackResolved{
			AttackResolved: &dnd5ev1alpha1.AttackResolvedEvent{
				AttackerId:  event.AttackResolved.AttackerID,
				TargetId:    event.AttackResolved.TargetID,
				Result:      attackResult,
				UpdatedRoom: attackResolvedRoom,
			},
		}

	case entities.EventTypeFeatureActivated:
		if event.FeatureActivated == nil {
			return nil, fmt.Errorf("missing FeatureActivated data for FeatureActivatedEvent")
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_FeatureActivated{
			FeatureActivated: &dnd5ev1alpha1.FeatureActivatedEvent{
				CharacterId: event.FeatureActivated.CharacterID,
				FeatureId:   event.FeatureActivated.FeatureID,
				Message:     event.FeatureActivated.Message,
				// Note: Updated character would require conversion
			},
		}

	case entities.EventTypeTurnEnded:
		if event.TurnEnded == nil {
			return nil, fmt.Errorf("missing TurnEnded data for TurnEndedEvent")
		}
		// Extract grid info from room data for coordinate conversion
		gridType, hexOrientation := extractGridInfo(event.TurnEnded.Room)
		// Convert room and add walls
		turnEndedRoom := convertRoomDataToProto(event.TurnEnded.Room)
		if turnEndedRoom != nil {
			turnEndedRoom.Walls = convertDungeonWallsToProto(event.TurnEnded.Walls)
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_TurnEnded{
			TurnEnded: &dnd5ev1alpha1.TurnEndedEvent{
				TurnChange: &dnd5ev1alpha1.TurnChangeEvent{
					PreviousEntityId: event.TurnEnded.PreviousEntityID,
					NextEntityId:     event.TurnEnded.NextEntityID,
					Round:            int32(event.TurnEnded.Round),
					NewRound:         event.TurnEnded.NewRound,
				},
				CombatState: convertCombatStateToProto(event.TurnEnded.CombatState, gridType, hexOrientation),
				UpdatedRoom: turnEndedRoom,
			},
		}

	case entities.EventTypeMonsterTurnCompleted:
		if event.MonsterTurnCompleted == nil {
			return nil, fmt.Errorf("missing MonsterTurnCompleted data for MonsterTurnCompletedEvent")
		}
		// Convert actions
		actions := make([]*dnd5ev1alpha1.MonsterExecutedAction, len(event.MonsterTurnCompleted.Actions))
		for i, action := range event.MonsterTurnCompleted.Actions {
			actions[i] = convertMonsterExecutedActionToProto(&action)
		}
		// Convert movement path (already cube coordinates)
		movementPath := make([]*apiv1alpha1.Position, len(event.MonsterTurnCompleted.Movement))
		for i, pos := range event.MonsterTurnCompleted.Movement {
			movementPath[i] = &apiv1alpha1.Position{
				X: pos.X,
				Y: pos.Y,
				Z: pos.Z,
			}
		}
		// Convert updated characters to proto
		var updatedCharacters []*dnd5ev1alpha1.Character
		for _, charData := range event.MonsterTurnCompleted.UpdatedCharacters {
			if charData != nil {
				updatedCharacters = append(updatedCharacters, characterhandler.ConvertCharacterDataToProto(charData))
			}
		}
		// Convert room and add walls
		monsterTurnRoom := convertRoomDataToProto(event.MonsterTurnCompleted.Room)
		if monsterTurnRoom != nil {
			monsterTurnRoom.Walls = convertDungeonWallsToProto(event.MonsterTurnCompleted.Walls)
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_MonsterTurnCompleted{
			MonsterTurnCompleted: &dnd5ev1alpha1.MonsterTurnCompletedEvent{
				MonsterTurn: &dnd5ev1alpha1.MonsterTurnResult{
					MonsterId:    event.MonsterTurnCompleted.MonsterID,
					MonsterName:  event.MonsterTurnCompleted.MonsterName,
					Actions:      actions,
					MovementPath: movementPath,
				},
				UpdatedCharacters: updatedCharacters,
				UpdatedRoom:       monsterTurnRoom,
			},
		}

	case entities.EventTypeCombatEnded:
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_CombatEnded{
			CombatEnded: &dnd5ev1alpha1.CombatEndedEvent{
				// TODO: Convert EncounterResult from event.CombatEnded
			},
		}

	case entities.EventTypePlayerDisconnected:
		if event.PlayerDisconnected == nil {
			return nil, fmt.Errorf("missing PlayerDisconnected data for PlayerDisconnectedEvent")
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_PlayerDisconnected{
			PlayerDisconnected: &dnd5ev1alpha1.PlayerDisconnectedEvent{
				PlayerId:    event.PlayerDisconnected.PlayerID,
				CharacterId: event.PlayerDisconnected.CharacterID,
			},
		}

	case entities.EventTypePlayerReconnected:
		if event.PlayerReconnected == nil {
			return nil, fmt.Errorf("missing PlayerReconnected data for PlayerReconnectedEvent")
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_PlayerReconnected{
			PlayerReconnected: &dnd5ev1alpha1.PlayerReconnectedEvent{
				PlayerId: event.PlayerReconnected.PlayerID,
				// Note: Full member state would require loading from repo
			},
		}

	case entities.EventTypeCombatPaused:
		if event.CombatPaused == nil {
			return nil, fmt.Errorf("missing CombatPaused data for CombatPausedEvent")
		}
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_CombatPaused{
			CombatPaused: &dnd5ev1alpha1.CombatPausedEvent{
				Reason:               event.CombatPaused.Reason,
				DisconnectedPlayerId: event.CombatPaused.PausedBy,
			},
		}

	case entities.EventTypeCombatResumed:
		// CombatResumed data is optional for basic event
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_CombatResumed{
			CombatResumed: &dnd5ev1alpha1.CombatResumedEvent{
				// TODO: Include full CombatState from event.CombatResumed when resuming
			},
		}

	case entities.EventTypeRoomRevealed:
		if event.RoomRevealed == nil {
			return nil, fmt.Errorf("missing RoomRevealed data for RoomRevealedEvent")
		}
		// Extract grid info from room data for coordinate conversion
		gridType, hexOrientation := extractGridInfo(event.RoomRevealed.RevealedRoom)
		protoEvent.Event = &dnd5ev1alpha1.EncounterEvent_RoomRevealed{
			RoomRevealed: &dnd5ev1alpha1.RoomRevealedEvent{
				DungeonId:    event.RoomRevealed.DungeonID,
				ConnectionId: event.RoomRevealed.ConnectionID,
				Room:         convertRoomDataToProto(event.RoomRevealed.RevealedRoom),
				CombatState:  convertCombatStateToProto(event.RoomRevealed.CombatState, gridType, hexOrientation),
				Doors:        convertEntityDoorsToProto(event.RoomRevealed.NewDoors),
			},
		}

	default:
		return nil, fmt.Errorf("unknown event type: %s", event.Type)
	}

	return protoEvent, nil
}
