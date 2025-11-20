package encounter

import (
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter"
)

// convertAttackResultToProto converts orchestrator's AttackResult to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertAttackResultToProto(result *encounter.AttackResult) *dnd5ev1alpha1.AttackResult {
	if result == nil {
		return nil
	}

	return &dnd5ev1alpha1.AttackResult{
		Hit:             result.Hit,
		AttackRoll:      int32(result.AttackRoll),
		AttackTotal:     int32(result.TotalAttack),
		TargetAc:        int32(result.TargetAC),
		Damage:          int32(result.TotalDamage),
		DamageType:      result.DamageType,
		Critical:        result.Critical,
		DamageBreakdown: convertDamageBreakdownToProto(result.Breakdown),
	}
}

// convertDamageBreakdownToProto converts orchestrator's DamageBreakdown to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertDamageBreakdownToProto(breakdown *encounter.DamageBreakdown) *dnd5ev1alpha1.DamageBreakdown {
	if breakdown == nil {
		return nil
	}

	components := make([]*dnd5ev1alpha1.DamageComponent, len(breakdown.Components))
	for i, comp := range breakdown.Components {
		components[i] = convertDamageComponentToProto(&comp)
	}

	return &dnd5ev1alpha1.DamageBreakdown{
		Components:  components,
		AbilityUsed: breakdown.AbilityUsed,
		TotalDamage: int32(breakdown.TotalDamage),
	}
}

// convertDamageComponentToProto converts orchestrator's DamageComponent to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertDamageComponentToProto(comp *encounter.DamageComponent) *dnd5ev1alpha1.DamageComponent {
	if comp == nil {
		return nil
	}

	// Convert original dice rolls to int32
	originalRolls := make([]int32, len(comp.OriginalDiceRolls))
	for i, roll := range comp.OriginalDiceRolls {
		originalRolls[i] = int32(roll)
	}

	// Convert final dice rolls to int32
	finalRolls := make([]int32, len(comp.FinalDiceRolls))
	for i, roll := range comp.FinalDiceRolls {
		finalRolls[i] = int32(roll)
	}

	// Convert reroll events
	rerolls := make([]*dnd5ev1alpha1.RerollEvent, len(comp.Rerolls))
	for i, r := range comp.Rerolls {
		rerolls[i] = convertRerollEventToProto(&r)
	}

	return &dnd5ev1alpha1.DamageComponent{
		Source:            comp.Source,
		OriginalDiceRolls: originalRolls,
		FinalDiceRolls:    finalRolls,
		Rerolls:           rerolls,
		FlatBonus:         int32(comp.FlatBonus),
		DamageType:        comp.DamageType,
		IsCritical:        comp.IsCritical,
	}
}

// convertRerollEventToProto converts orchestrator's RerollEvent to proto
//
//nolint:gosec // G115: Game values are bounded by D&D rules, no overflow risk
func convertRerollEventToProto(event *encounter.RerollEvent) *dnd5ev1alpha1.RerollEvent {
	if event == nil {
		return nil
	}

	return &dnd5ev1alpha1.RerollEvent{
		DieIndex: int32(event.DieIndex),
		Before:   int32(event.Before),
		After:    int32(event.After),
		Reason:   event.Reason,
	}
}
