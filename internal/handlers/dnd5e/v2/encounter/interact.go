package encounter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	corepb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha2/core"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// Interact handles the Interact RPC. It validates the request envelope,
// delegates door routing + dispatch to the v2 orchestrator, and translates the
// orchestrator's result (and sentinel errors) back to proto.
//
// The orchestrator owns the load → classify door → AttemptUnlock/OpenDoor →
// persist flow (#582 carve-out). This handler stays thin: proto↔input mapping
// plus interactStatusError for the sentinel→gRPC code mapping.
//
// Behavior contract (preserved across the carve):
//   - missing auth → Unauthenticated
//   - empty encounter_id / target_entity_id → InvalidArgument
//   - encounter not in repo → NotFound
//   - target is not a known door in this encounter → NotFound
//   - door is locked → response carries InputRequired{skill_check}; the
//     pending prompt is persisted so it survives reconnect; no broker event
//     (caller-private)
//   - door is unlocked → OpenDoor publishes per-viewer DoorOpened +
//     GeometryRevealed; response is empty by proto design
//   - toolkit OpenDoor / AttemptUnlock refuses (player not in encounter,
//     door already open, prompt already pending, etc.) → FailedPrecondition
//     per pat-v2-status-code-mapping
//   - save failure → Internal
func (h *Handler) Interact(ctx context.Context, req *encounterv2pb.InteractRequest) (*encounterv2pb.InteractResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}
	if req.GetTargetEntityId() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_entity_id is required")
	}

	out, err := h.orch.Interact(ctx, &encounterorch.InteractInput{
		EncounterID:    req.GetEncounterId(),
		PlayerID:       core.PlayerID(playerID),
		TargetEntityID: core.EntityID(req.GetTargetEntityId()),
	})
	if err != nil {
		return nil, interactStatusError(err)
	}

	resp := &encounterv2pb.InteractResponse{}
	if out.Prompt != nil {
		resp.InputRequired = buildSkillCheckPrompt(*out.Prompt)
	}
	return resp, nil
}

// interactStatusError maps the orchestrator's Interact errors onto gRPC status
// codes. Orchestrator sentinels carry the entity-layer classification; this
// proto-layer mapper assigns the gRPC code per pat-v2-status-code-mapping.
//
//   - ErrEncounterNotFound / ErrTargetNotADoor → NotFound
//   - ErrPlayerNotInEncounter → PermissionDenied
//   - ErrDoorNotLocked (door state inconsistent with the just-read snapshot) →
//     Internal — we routed on a Locked door but the toolkit reported unlocked
//   - any other AttemptUnlock / OpenDoor refusal (player not in encounter at the
//     verb level, door already open, prompt already pending) → FailedPrecondition
//   - load / save / wrapped internal failures → Internal
func interactStatusError(err error) error {
	switch {
	case errors.Is(err, encounterorch.ErrEncounterNotFound):
		return status.Error(codes.NotFound, "encounter not found")
	case errors.Is(err, encounterorch.ErrTargetNotADoor):
		return status.Error(codes.NotFound, "target entity is not a door, or door does not exist")
	case errors.Is(err, encounterorch.ErrPlayerNotInEncounter):
		return status.Error(codes.PermissionDenied, "player is not in this encounter")
	case errors.Is(err, encounter.ErrPromptAlreadyPending):
		return status.Error(codes.FailedPrecondition,
			"resolve the pending prompt before issuing another action")
	case errors.Is(err, encounter.ErrDoorNotLocked):
		// Defensive: the orchestrator routed on door.Locked == true, so the
		// toolkit reporting "not locked" means the persisted snapshot is
		// inconsistent with the loaded encounter.
		return status.Errorf(codes.Internal, "locked-door dispatch refused as not-locked: %v", err)
	case errors.Is(err, encounterorch.ErrDoorVerbRefused):
		// Player not in encounter / door not in encounter / door already open —
		// state-dependent verb refusals map to FailedPrecondition.
		return status.Errorf(codes.FailedPrecondition, "%v", err)
	}
	return status.Errorf(codes.Internal, "interact: %v", err)
}

// buildSkillCheckPrompt translates the toolkit's PromptIssued verb-return
// into the proto wire shape InputRequired{skill_check}. The tool ref is
// optional (empty when no tool proficiency applies); we surface it as a
// nullable Ref so the client can render thieves'-tools-or-not without a
// separate flag. Proto translation — stays handler-side.
func buildSkillCheckPrompt(issued encounter.PromptIssued) *encounterv2pb.InputRequired {
	prompt := &encounterv2pb.SkillCheckPrompt{
		Dc:      int32(issued.DC), //nolint:gosec // DC is bounded by ruleset, never overflows int32
		Ability: issued.Ability,
	}
	if issued.Tool != "" {
		prompt.Tool = parseToolkitRef(issued.Tool)
	}
	return &encounterv2pb.InputRequired{
		Kind: &encounterv2pb.InputRequired_SkillCheck{SkillCheck: prompt},
	}
}

// parseToolkitRef converts a toolkit ref string (e.g.
// "dnd5e:item:thieves-tools") into a proto Ref. Reuses the package's
// existing splitRef helper (translate.go) so ref parsing semantics stay
// consistent across projection / translation / prompt translation —
// splitRef requires exactly two colons and returns nil otherwise. Falls
// back to a generic {dnd5e, item, raw} encoding if the ref isn't in the
// canonical "module:type:id" shape so unknown refs still round-trip
// something the client can display.
func parseToolkitRef(ref string) *corepb.Ref {
	if parts := splitRef(ref); parts != nil {
		return &corepb.Ref{Module: parts[0], Type: parts[1], Id: parts[2]}
	}
	return &corepb.Ref{Module: refModuleDnd5e, Type: "item", Id: ref}
}
