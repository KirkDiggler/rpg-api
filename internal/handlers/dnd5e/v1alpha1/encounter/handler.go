// Package encounter handles the gRPC service interface for encounter management
package encounter

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// HandlerConfig holds dependencies for the handler
type HandlerConfig struct {
	EncounterService encounter.Service
}

// Validate ensures all required dependencies are present
func (c *HandlerConfig) Validate() error {
	if c.EncounterService == nil {
		return errors.InvalidArgument("encounter service is required")
	}
	return nil
}

// Handler implements the D&D 5e Encounter gRPC service
type Handler struct {
	dnd5ev1alpha1.UnimplementedEncounterServiceServer
	encounterService encounter.Service
}

// New creates a new handler with the given configuration
func New(cfg *HandlerConfig) (*Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Handler{
		encounterService: cfg.EncounterService,
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
		WeaponID:    req.GetWeaponId(), // Optional
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
		Room:         convertRoomDataToProto(output.Room),
		CombatState:  convertCombatStateToProto(output.CombatState, gridType, hexOrientation),
		MonsterTurns: convertMonsterTurnsToProto(output.MonsterTurns, gridType, hexOrientation),
	}, nil
}

// GetCombatState retrieves current combat state
func (h *Handler) GetCombatState(
	_ context.Context,
	_ *dnd5ev1alpha1.GetCombatStateRequest,
) (*dnd5ev1alpha1.GetCombatStateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetCombatState endpoint not yet implemented")
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

	// 2. Use default hex grid settings for coordinate conversion
	// The client sends cube coordinates; we need to convert to offset for internal use
	// TODO: In the future, could fetch room data from encounter state for accuracy
	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	// 3. Create service input - convert cube coords to offset for internal use
	lastPos := req.GetPath()[len(req.GetPath())-1]
	offsetPos := convertProtoPositionToOffset(lastPos, gridType, hexOrientation)
	input := &encounter.MoveCharacterInput{
		EncounterID: req.GetEncounterId(),
		EntityID:    req.GetEntityId(),
		TargetPosition: &encounter.Position{
			X: offsetPos.X,
			Y: offsetPos.Y,
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

	// 2. Create service input
	// TODO: Get player_id from authentication context or request field once available.
	// Currently, ownership validation is skipped when player_id is empty.
	// When auth is implemented:
	//   - Option A: Add player_id field to EndTurnRequest proto
	//   - Option B: Extract player_id from gRPC metadata/auth context
	playerID := "" // Placeholder until auth is implemented
	input := &encounter.EndTurnInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    playerID,
	}

	// 3. Call service
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
	if req.GetFeatureId() == "" {
		return nil, status.Error(codes.InvalidArgument, "feature_id is required")
	}

	// 2. Create service input
	input := &encounter.ActivateFeatureInput{
		EncounterID: req.GetEncounterId(),
		CharacterID: req.GetCharacterId(),
		FeatureID:   req.GetFeatureId(),
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

	// TODO: Get player_id from authentication context
	// For now, use first character ID as player ID placeholder
	playerID := "player-" + req.GetCharacterIds()[0]

	// 2. Call orchestrator
	output, err := h.encounterService.CreateEncounter(ctx, &encounter.CreateEncounterInput{
		PlayerID:     playerID,
		CharacterIDs: req.GetCharacterIds(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 3. Convert response
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

	// TODO: Get player_id from authentication context
	playerID := "player-" + req.GetCharacterIds()[0]

	// 2. Call orchestrator
	output, err := h.encounterService.JoinEncounter(ctx, &encounter.JoinEncounterInput{
		JoinCode:     req.GetJoinCode(),
		PlayerID:     playerID,
		CharacterIDs: req.GetCharacterIds(),
	})
	if err != nil {
		if err.Error() == "encounter not found" {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 3. Convert response
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

	// TODO: Get player_id from authentication context
	// For now, we need a way to identify the caller
	playerID := "" // Will fail if not host - handled by orchestrator

	// 2. Call orchestrator
	output, err := h.encounterService.StartCombat(ctx, &encounter.StartCombatInput{
		EncounterID: req.GetEncounterId(),
		PlayerID:    playerID,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// 3. Convert response
	gridType := spatial.GridTypeHex
	hexOrientation := spatial.HexOrientationPointyTop

	return &dnd5ev1alpha1.StartCombatResponse{
		CombatState: convertCombatStateToProto(output.CombatState, gridType, hexOrientation),
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

// StreamEncounterEvents subscribes to real-time encounter events
func (h *Handler) StreamEncounterEvents(
	_ *dnd5ev1alpha1.StreamEncounterEventsRequest,
	_ dnd5ev1alpha1.EncounterService_StreamEncounterEventsServer,
) error {
	return status.Error(codes.Unimplemented, "StreamEncounterEvents endpoint not yet implemented")
}
