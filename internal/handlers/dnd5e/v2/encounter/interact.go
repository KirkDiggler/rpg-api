package encounter

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// Interact handles the Interact RPC. Wave 2.7 wired the unlocked-door path
// (toolkit OpenDoor verb dispatched directly). Wave 2.9 adds the locked-door
// path: when the target door has Locked: true, the handler issues a
// per-player skill-check prompt via the toolkit's AttemptUnlock verb and
// returns the prompt to the caller as InputRequired{skill_check} on the
// response. The caller then resolves the prompt by calling SubmitCheck with
// a d20 roll.
//
// Future waves extend the dispatch to chests, levers, NPCs, and traps; the
// optional interaction_kind field is plumbed through the proto for that
// future routing but is unused today.
//
// Behavior contract:
//   - missing auth → Unauthenticated
//   - empty encounter_id / target_entity_id → InvalidArgument
//   - encounter not in repo → NotFound
//   - target is not a known door in this encounter → NotFound
//   - door is locked → AttemptUnlock issues a prompt; response carries
//     InputRequired{skill_check}; persistence captures PendingPrompts so
//     the prompt survives reconnect. Stream sees no event (caller-private).
//   - door is unlocked → OpenDoor publishes per-viewer DoorOpened +
//     GeometryRevealed; response is empty by proto design.
//   - toolkit OpenDoor / AttemptUnlock refuses (player not in encounter,
//     door already open, prompt already pending, etc.) → FailedPrecondition
//     per pat-v2-status-code-mapping.
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

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter %q: %v", req.GetEncounterId(), err)
	}

	// Wave 2.7/2.9 dispatch: only door interactions are wired. Future waves
	// add chests, levers, NPCs, traps via additional lookups + dispatch arms.
	targetID := core.EntityID(req.GetTargetEntityId())
	door, ok := data.Doors[targetID]
	if !ok {
		return nil, status.Error(codes.NotFound, "target entity is not a door, or door does not exist")
	}

	enc, err := encounter.LoadFromData(data, h.broker, encounter.WithCharacterResolver(h.resolver), encounter.WithCombatResolver(h.buildCombatResolver(data)))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data %q: %v", req.GetEncounterId(), err)
	}

	// Locked-door branch (Wave 2.9): issue a per-player skill-check prompt
	// via the toolkit's AttemptUnlock verb. The prompt is persisted to
	// data.PendingPrompts so the caller can resolve it later via SubmitCheck;
	// no broker event is emitted (prompts are caller-private).
	if door.Locked {
		issued, unlockErr := enc.AttemptUnlock(core.PlayerID(playerID), targetID)
		if unlockErr != nil {
			switch {
			case errors.Is(unlockErr, encounter.ErrPromptAlreadyPending):
				return nil, status.Error(codes.FailedPrecondition,
					"resolve the pending prompt before issuing another action")
			case errors.Is(unlockErr, encounter.ErrDoorNotLocked):
				// Defensive: we just checked door.Locked. If toolkit reports
				// unlocked the persisted snapshot is inconsistent with the
				// loaded encounter — surface as Internal.
				return nil, status.Errorf(codes.Internal,
					"locked-door dispatch refused as not-locked: %v", unlockErr)
			default:
				// Player not in encounter, door not in encounter, etc. — these
				// are state-dependent and map to FailedPrecondition per
				// pat-v2-status-code-mapping. AttemptUnlock wraps these with
				// fmt.Errorf rather than sentinels, so the default arm covers
				// the long tail.
				return nil, status.Errorf(codes.FailedPrecondition, "attempt unlock: %v", unlockErr)
			}
		}

		if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
			return nil, status.Errorf(codes.Internal,
				"save encounter %q after attempt unlock: %v", req.GetEncounterId(), err)
		}

		return &encounterv2pb.InteractResponse{
			InputRequired: buildSkillCheckPrompt(issued),
		}, nil
	}

	// Unlocked-door branch (Wave 2.7): dispatch directly to OpenDoor.
	if err := enc.OpenDoor(core.PlayerID(playerID), targetID); err != nil {
		// Toolkit OpenDoor errors are state-dependent (player not in
		// encounter, door already open) — these are gRPC FailedPrecondition
		// per pat-v2-status-code-mapping. The request is syntactically
		// valid but the world state forbids the action.
		return nil, status.Errorf(codes.FailedPrecondition, "open door: %v", err)
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter %q: %v", req.GetEncounterId(), err)
	}

	return &encounterv2pb.InteractResponse{}, nil
}

// buildSkillCheckPrompt translates the toolkit's PromptIssued verb-return
// into the proto wire shape InputRequired{skill_check}. The tool ref is
// optional (empty when no tool proficiency applies); we surface it as a
// nullable Ref so the client can render thieves'-tools-or-not without a
// separate flag.
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
func parseToolkitRef(ref string) *encounterv2pb.Ref {
	if parts := splitRef(ref); parts != nil {
		return &encounterv2pb.Ref{Module: parts[0], Type: parts[1], Id: parts[2]}
	}
	return &encounterv2pb.Ref{Module: refModuleDnd5e, Type: "item", Id: ref}
}
