package devcombat_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/pkg/devcombat"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// InjectSuite covers devcombat.Inject: the shared load/AddMonster/SetMode/save
// logic behind cmd/devseed's --inject-combat flag AND the lobby integration
// test's combat-entry step (rpg-api#634).
type InjectSuite struct {
	suite.Suite
	ctx  context.Context
	repo encountersv2.Repository
}

func TestInjectSuite(t *testing.T) {
	suite.Run(t, new(InjectSuite))
}

func (s *InjectSuite) SetupTest() {
	s.ctx = context.Background()
	s.repo = encountersv2.NewInMemory()
}

// seedLobbyLikeEncounter writes a two-player encounter matching what the
// lobby StartEncounter RPC produces post-#634 Part 1: real HP/AC, FreeRoam
// mode, no monsters, no AttackBonus/DamageDice/DamageType/DataJSON.
func (s *InjectSuite) seedLobbyLikeEncounter(encID string) {
	transport := tkenc.NewInMemoryTransport()
	defer func() { _ = transport.Close() }()
	broker := tkenc.NewBroker(transport)
	defer func() { _ = broker.Close() }()

	enc := tkenc.New(s.ctx, core.EncounterID(encID), broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14,
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "bob", EntityID: "char-bob",
		Position: core.Hex{Q: 1, R: 0, S: -1}, SightRange: 10,
		HP: 14, MaxHP: 14, AC: 13,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))
}

// TestInject_AddsGoblinAndFlipsToTurnBased is the headline #634 Part 2 proof:
// injection adds a real, honest goblin (HP/AC/DataJSON all set) and rolls
// initiative, without rebuilding or mutating the encounter's existing
// players (which stay in their honest #634 Part 1 shape: no
// AttackBonus/DamageDice/DamageType, no DataJSON).
func (s *InjectSuite) TestInject_AddsGoblinAndFlipsToTurnBased() {
	const encID = "lobby-enc-1"
	s.seedLobbyLikeEncounter(encID)

	out, err := devcombat.Inject(s.ctx, s.repo, devcombat.InjectInput{EncounterID: encID})
	s.Require().NoError(err)
	s.Require().Equal(core.EntityID("goblin-1"), out.GoblinID)
	s.Require().Equal(core.ModeTurnBased, out.Mode)
	s.Require().False(out.AlreadySetTurnBased)
	s.Require().Equal(2, out.PlayerCount)
	s.Require().Equal(1, out.MonsterCount)

	data, err := s.repo.Get(s.ctx, encID)
	s.Require().NoError(err)
	s.Require().Equal(core.ModeTurnBased, data.Mode)
	s.Require().Contains(data.Initiative, out.GoblinID, "the goblin must be in the rolled initiative order, or it can never take a turn")
	s.Require().Contains(data.Initiative, core.EntityID("char-alice"))
	s.Require().Contains(data.Initiative, core.EntityID("char-bob"))

	goblin, ok := data.Monsters["goblin-1"]
	s.Require().True(ok)
	s.Require().Positive(goblin.HP)
	s.Require().Positive(goblin.AC)
	s.Require().NotEmpty(goblin.DataJSON, "goblin must carry DataJSON — monster.NewGoblin, not a hand-rolled stub")
	// alice (Q:0) and bob (Q:1) are the party's leading edge; the goblin must
	// land one hex past it (Q:2), not two (the off-1 bug Copilot review
	// caught on rpg-api#635), and not stacked on either player.
	s.Require().Equal(2, goblin.Position.Q, "goblin must spawn adjacent to the party's leading edge, not two hexes past it")

	alice, ok := data.Players["alice"]
	s.Require().True(ok, "existing player must survive injection")
	s.Require().Equal(14, alice.AC)
	s.Require().Equal(12, alice.HP)
	s.Require().Zero(alice.AttackBonus, "injection must not mutate the existing honest #634 Part 1 snapshot")
	s.Require().Empty(alice.DamageDice)
}

// TestInject_AlreadyTurnBased_ReRollsInitiativeForNewMonster proves a second
// injection against an in-progress fight doesn't hit the toolkit's "mode is
// already turn_based" SetMode rejection — Inject round-trips FREE_ROAM ->
// TURN_BASED to force a fresh roll — and, critically, that the added goblin
// (goblin-2, keyed off the current monster count) actually lands in the
// re-rolled Initiative order. AddMonster alone never touches Initiative, so
// without the round-trip a mid-fight monster would sit in the encounter
// forever without ever getting a turn.
func (s *InjectSuite) TestInject_AlreadyTurnBased_ReRollsInitiativeForNewMonster() {
	const encID = "lobby-enc-2"
	s.seedLobbyLikeEncounter(encID)

	_, err := devcombat.Inject(s.ctx, s.repo, devcombat.InjectInput{EncounterID: encID})
	s.Require().NoError(err)

	out, err := devcombat.Inject(s.ctx, s.repo, devcombat.InjectInput{EncounterID: encID})
	s.Require().NoError(err, "a second injection must not fail on the toolkit's redundant-SetMode rejection")
	s.Require().Equal(core.EntityID("goblin-2"), out.GoblinID)
	s.Require().True(out.AlreadySetTurnBased)
	s.Require().Equal(2, out.MonsterCount)

	data, err := s.repo.Get(s.ctx, encID)
	s.Require().NoError(err)
	s.Require().Len(data.Monsters, 2)
	s.Require().Equal(core.ModeTurnBased, data.Mode)
	s.Require().Contains(data.Initiative, out.GoblinID, "the newly injected goblin must be in the re-rolled initiative order, or it can never take a turn")
	s.Require().Contains(data.Initiative, core.EntityID("goblin-1"), "the first goblin must still be in the re-rolled initiative order")
}

// TestInject_MissingEncounter_ReturnsErrNotFound proves --inject-combat
// against a nonexistent encounter ID fails loudly (and identifiably via
// errors.Is) instead of silently seeding nothing.
func (s *InjectSuite) TestInject_MissingEncounter_ReturnsErrNotFound() {
	_, err := devcombat.Inject(s.ctx, s.repo, devcombat.InjectInput{EncounterID: "no-such-encounter"})
	s.Require().Error(err)
	s.Require().True(errors.Is(err, encountersv2.ErrNotFound))
}
