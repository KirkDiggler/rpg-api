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
		Hit:         result.Hit,
		AttackRoll:  int32(result.AttackRoll),
		AttackTotal: int32(result.TotalAttack),
		TargetAc:    int32(result.AttackBonus), // TODO: Fix when orchestrator exposes TargetAC
		Damage:      int32(result.TotalDamage),
		DamageType:  result.DamageType,
		Critical:    result.Critical,
		// TODO: Add damage_breakdown when proto supports it
	}
}
