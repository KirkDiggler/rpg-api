package encounter

// turn_state_project.go projects the toolkit's per-actor turn state
// (encounter.ActorTurnState — economy + the abilities/actions menu) onto the
// proto TurnState for the turn-START snapshot (#601).
//
// This is the SNAPSHOT-path counterpart to translate.go's turnStateSnapshotToProto
// (the LIVE TurnStateChanged push). The live path consumes the toolkit's already-
// flattened, rulebook-agnostic events.TurnStateSnapshot; the snapshot path reads
// encounter.ActorTurnState directly, whose fields are the rulebook's own
// character.{ActionEconomyData,AvailableAbility,AvailableAction} types. The
// toolkit's ActorTurnState is the read surface it built FOR rpg-api to project
// (turn_state.go: "The caller (rpg-api) projects the returned domain types onto
// the wire TurnState"); rpg-api computes nothing here — it copies fields and maps
// the toolkit's string enums onto the proto enums.
//
// This file is the single rulebook-touching spot in the projection path, kept out
// of the depguard-guarded action-handler files. Follow-up (surface to director):
// the toolkit could export its buildTurnStateSnapshot flattener (or an
// Encounter.ActorTurnStateSnapshot method) so BOTH paths consume the agnostic
// events.TurnStateSnapshot and this rulebook import goes away entirely.

import (
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	tkcore "github.com/KirkDiggler/rpg-toolkit/core"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// actorEconomyToProto projects the toolkit's ActionEconomyData (the actor's
// current turn economy) onto the proto ActionEconomy field-for-field. nil
// economy (actor not in combat / no held character) projects to nil so the
// snapshot omits it. The toolkit owns these counters (Invariants 2/3); this is
// a faithful copy, no math.
func actorEconomyToProto(econ *tkcharacter.ActionEconomyData) *encounterv2pb.ActionEconomy {
	if econ == nil {
		return nil
	}
	out := &encounterv2pb.ActionEconomy{
		ActionsRemaining:      int32(econ.ActionsRemaining),      //nolint:gosec // bounded turn-economy counters fit int32
		BonusActionsRemaining: int32(econ.BonusActionsRemaining), //nolint:gosec // bounded turn-economy counters fit int32
		ReactionsRemaining:    int32(econ.ReactionsRemaining),    //nolint:gosec // bounded turn-economy counters fit int32
		MovementRemaining:     int32(econ.MovementRemaining),     //nolint:gosec // movement in feet fits int32
	}
	if len(econ.Granted) > 0 {
		out.Capacities = make(map[string]int32, len(econ.Granted))
		for k, v := range econ.Granted {
			out.Capacities[string(k)] = int32(v) //nolint:gosec // granted-capacity counts fit int32
		}
	}
	return out
}

// actorMenuToProto projects the actor's available menu — abilities (primary
// economy slots) first, then granted-capacity actions, matching the live path's
// ordering (buildTurnStateSnapshot) and the dnd5e two-level model — onto proto
// AvailableActions. Each entry is mapped field-for-field: ref (split via the
// shared actionRefToProto guard), display name, effective availability + reason,
// economy slot enum, target kind enum. The toolkit authored every field
// (including the CanUse verdict and the reason string, and the D17 Move deferral
// baked into ActorTurnState); rpg-api never recomputes availability.
func actorMenuToProto(ts *tkenc.ActorTurnState) []*encounterv2pb.AvailableAction {
	if ts == nil {
		return nil
	}
	total := len(ts.Abilities) + len(ts.Actions)
	if total == 0 {
		return nil
	}
	out := make([]*encounterv2pb.AvailableAction, 0, total)
	for _, a := range ts.Abilities {
		out = append(out, &encounterv2pb.AvailableAction{
			Ref:               actionRefToProto(refOrEmpty(a.Ref)),
			DisplayName:       a.Name,
			Available:         a.CanUse,
			UnavailableReason: a.Reason,
			EconomySlot:       economySlotToProto(string(a.EconomySlot)),
			TargetKind:        targetKindToProto(string(a.TargetKind)),
		})
	}
	for _, a := range ts.Actions {
		out = append(out, &encounterv2pb.AvailableAction{
			Ref:               actionRefToProto(refOrEmpty(a.Ref)),
			DisplayName:       a.Name,
			Available:         a.CanUse,
			UnavailableReason: a.Reason,
			EconomySlot:       economySlotToProto(string(a.EconomySlot)),
			TargetKind:        targetKindToProto(string(a.TargetKind)),
		})
	}
	return out
}

// refOrEmpty renders a toolkit *core.Ref to its canonical "module:type:id"
// string, or "" when nil. (*core.Ref).String() dereferences without a nil check,
// so the explicit guard is required. The shared actionRefToProto then splits the
// string, guarding degenerate input (protos disc-014).
func refOrEmpty(ref *tkcore.Ref) string {
	if ref == nil {
		return ""
	}
	return ref.String()
}
