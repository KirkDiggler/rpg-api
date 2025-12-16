package encounter

import (
	"fmt"
	"math/rand"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// MonsterSelector selects monsters based on role composition and CR budget
type MonsterSelector interface {
	SelectMonsters(input *SelectInput) *SelectOutput
}

// SelectInput defines parameters for monster selection
type SelectInput struct {
	// Pool contains available regular monsters
	Pool []dungeon.MonsterRef
	// BossPool contains available boss monsters
	BossPool []dungeon.MonsterRef
	// Budget is the CR to spend
	Budget float64
	// IsBoss indicates if this is a boss encounter
	IsBoss bool
	// Seed for deterministic random selection
	Seed int64
}

// SelectOutput contains the selected monsters
type SelectOutput struct {
	// Monsters contains the selected monster placements
	Monsters []dungeon.MonsterPlacement
	// TotalCR is the sum of selected monster CRs
	TotalCR float64
}

// selector implements MonsterSelector
type selector struct{}

// NewMonsterSelector creates a new MonsterSelector
func NewMonsterSelector() MonsterSelector {
	return &selector{}
}

// SelectMonsters performs role-based monster selection within CR budget
func (s *selector) SelectMonsters(input *SelectInput) *SelectOutput {
	if input == nil || len(input.Pool) == 0 {
		return &SelectOutput{
			Monsters: []dungeon.MonsterPlacement{},
			TotalCR:  0,
		}
	}

	// nolint:gosec // Using math/rand for deterministic dungeon generation, not cryptography
	rng := rand.New(rand.NewSource(input.Seed))

	if input.IsBoss {
		return s.selectBossEncounter(input, rng)
	}

	return s.selectRegularEncounter(input, rng)
}

// selectBossEncounter picks a boss and fills remaining budget with minions
func (s *selector) selectBossEncounter(input *SelectInput, rng *rand.Rand) *SelectOutput {
	monsters := []dungeon.MonsterPlacement{}
	totalCR := 0.0

	// Select boss from BossPool
	if len(input.BossPool) == 0 {
		return &SelectOutput{
			Monsters: monsters,
			TotalCR:  totalCR,
		}
	}

	boss := input.BossPool[rng.Intn(len(input.BossPool))]
	monsters = append(monsters, dungeon.MonsterPlacement{
		ID:        generateMonsterID("boss", len(monsters)),
		MonsterID: boss.ID,
		Role:      dungeon.RoleBoss,
		CR:        boss.CR,
	})
	totalCR += boss.CR

	// Fill remaining budget with minions from regular pool
	remainingBudget := input.Budget - totalCR

	// Get minions (prefer melee for boss support)
	meleeMonsters := filterByRole(input.Pool, dungeon.RoleMelee)
	for remainingBudget > 0 && len(meleeMonsters) > 0 {
		// Select random melee monster
		monster := meleeMonsters[rng.Intn(len(meleeMonsters))]

		if monster.CR > remainingBudget {
			// Can't afford any more
			break
		}

		monsters = append(monsters, dungeon.MonsterPlacement{
			ID:        generateMonsterID("minion", len(monsters)),
			MonsterID: monster.ID,
			Role:      monster.Role,
			CR:        monster.CR,
		})
		totalCR += monster.CR
		remainingBudget -= monster.CR
	}

	return &SelectOutput{
		Monsters: monsters,
		TotalCR:  totalCR,
	}
}

// selectRegularEncounter selects monsters based on CR-dependent role distribution
func (s *selector) selectRegularEncounter(input *SelectInput, rng *rand.Rand) *SelectOutput {
	monsters := []dungeon.MonsterPlacement{}
	totalCR := 0.0
	remainingBudget := input.Budget

	// Determine role distribution based on CR budget
	roleDistribution := s.getRoleDistribution(input.Budget)

	// Get monsters by role
	meleeMonsters := filterByRole(input.Pool, dungeon.RoleMelee)
	rangedMonsters := filterByRole(input.Pool, dungeon.RoleRanged)
	supportMonsters := filterByRole(input.Pool, dungeon.RoleSupport)

	// Allocate budget by role
	meleeBudget := remainingBudget * roleDistribution.meleePercent
	rangedBudget := remainingBudget * roleDistribution.rangedPercent
	supportBudget := remainingBudget * roleDistribution.supportPercent

	// Select melee monsters
	monsters, totalCR = s.fillRoleSlot(monsters, totalCR, meleeMonsters, meleeBudget, rng)

	// Select ranged monsters
	monsters, totalCR = s.fillRoleSlot(monsters, totalCR, rangedMonsters, rangedBudget, rng)

	// Select support monsters
	monsters, totalCR = s.fillRoleSlot(monsters, totalCR, supportMonsters, supportBudget, rng)

	return &SelectOutput{
		Monsters: monsters,
		TotalCR:  totalCR,
	}
}

// roleDistribution defines percentage allocation for each role
type roleDistribution struct {
	meleePercent   float64
	rangedPercent  float64
	supportPercent float64
}

// getRoleDistribution returns role percentages based on CR budget
func (s *selector) getRoleDistribution(budget float64) roleDistribution {
	if budget <= 2.0 {
		// Low CR: mostly melee (70%), some ranged (30%)
		return roleDistribution{
			meleePercent:   0.7,
			rangedPercent:  0.3,
			supportPercent: 0.0,
		}
	}

	if budget <= 5.0 {
		// Medium CR: melee (50%), ranged (30%), support (20%)
		return roleDistribution{
			meleePercent:   0.5,
			rangedPercent:  0.3,
			supportPercent: 0.2,
		}
	}

	// High CR: melee (40%), ranged (30%), support (30%)
	return roleDistribution{
		meleePercent:   0.4,
		rangedPercent:  0.3,
		supportPercent: 0.3,
	}
}

// fillRoleSlot fills a role slot with monsters up to the budget
func (s *selector) fillRoleSlot(
	monsters []dungeon.MonsterPlacement,
	currentCR float64,
	pool []dungeon.MonsterRef,
	budget float64,
	rng *rand.Rand,
) ([]dungeon.MonsterPlacement, float64) {
	if len(pool) == 0 || budget <= 0 {
		return monsters, currentCR
	}

	remainingBudget := budget

	for remainingBudget > 0 {
		// Find affordable monsters
		affordable := []dungeon.MonsterRef{}
		for _, m := range pool {
			if m.CR <= remainingBudget {
				affordable = append(affordable, m)
			}
		}

		if len(affordable) == 0 {
			// Can't afford any more
			break
		}

		// Select random affordable monster
		monster := affordable[rng.Intn(len(affordable))]

		monsters = append(monsters, dungeon.MonsterPlacement{
			ID:        generateMonsterID(monsterRoleString(monster.Role), len(monsters)),
			MonsterID: monster.ID,
			Role:      monster.Role,
			CR:        monster.CR,
		})
		currentCR += monster.CR
		remainingBudget -= monster.CR
	}

	return monsters, currentCR
}

// filterByRole returns monsters matching the specified role
func filterByRole(pool []dungeon.MonsterRef, role dungeon.MonsterRole) []dungeon.MonsterRef {
	result := []dungeon.MonsterRef{}
	for _, m := range pool {
		if m.Role == role {
			result = append(result, m)
		}
	}
	return result
}

// generateMonsterID creates a unique ID for a monster placement
func generateMonsterID(prefix string, index int) string {
	return fmt.Sprintf("%s-%d", prefix, index)
}

// monsterRoleString returns the string representation of MonsterRole
func monsterRoleString(r dungeon.MonsterRole) string {
	switch r {
	case dungeon.RoleMelee:
		return "melee"
	case dungeon.RoleRanged:
		return "ranged"
	case dungeon.RoleSupport:
		return "support"
	case dungeon.RoleBoss:
		return "boss"
	default:
		return "unknown"
	}
}
