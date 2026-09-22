package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type ClericPreparationSuite struct{ suite.Suite }

func TestClericPreparationSuite(t *testing.T) { suite.Run(t, new(ClericPreparationSuite)) }

func (s *ClericPreparationSuite) TestOnlyPreparationsAndDomainGrantsReachCast() {
	h, ctx, id := nativeClericCombatScene(s.T())
	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: castSessionID, Member: id})
	s.Require().NoError(err)
	var offered []string
	for _, row := range out.GetDeclarations() {
		if row.GetVerb() == sessionpb.Verb_VERB_CAST {
			offered = append(offered, row.GetSpell().GetRef())
		}
	}
	for _, spell := range []spells.Spell{spells.Bane, spells.Command, spells.HealingWord, spells.Sanctuary, spells.Bless, spells.CureWounds} {
		s.Contains(offered, refs.Spells.ByID(spell).String())
	}
	for _, spell := range []spells.Spell{spells.GuidingBolt, spells.InflictWounds, spells.ShieldOfFaith, spells.Light} {
		s.NotContains(offered, refs.Spells.ByID(spell).String())
	}
}

func (s *ClericPreparationSuite) TestBardCreationRetainsFourKnownSpells() {
	h := newAcceptanceHarness(s.T())
	const playerID = "preparation-bard-player"
	id := createFinalizedBaneBard(s.T(), h, playerID)
	ctx := auth.WithPlayerID(context.Background(), playerID)
	stored, err := h.charRepo.Get(ctx, characterrepo.GetInput{ID: id})
	s.Require().NoError(err)
	expected := make([]string, 0, 4)
	for _, spell := range []spells.Spell{spells.Bane, spells.Thunderwave, spells.DissonantWhispers, spells.Command} {
		expected = append(expected, refs.Spells.ByID(spell).String())
	}
	s.ElementsMatch(expected, stored.Character.Data.KnownSpells)
}
