package dungeons_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

// CreatureFactsWalkSuite proves the TWO NEW DEED READINGS reach the engine
// through the API's own path (rpg-project#498, rpg-toolkit#1883/#1885): the
// registry compiles the authored room exactly as `PutDungeon` does, so what
// this asserts is what a published document would get.
//
// IT PATROLS A CROSS-REPO SEAM, which is why it lives here rather than in the
// toolkit: the toolkit proves its evaluator reads `on: ally` and `as: actor`;
// the toolkit proves its dialect carries them; nothing before this proved a
// DOCUMENT authored with them compiles through the server a World Builder
// publish actually talks to. The scopes are the whole claim — a consumer that
// dropped them in lowering would still compile, and every toolkit test would
// still pass.
type CreatureFactsWalkSuite struct {
	suite.Suite
	ctx      context.Context
	registry *dungeons.FileRegistry
	raw      []byte
}

func TestCreatureFactsWalkSuite(t *testing.T) { suite.Run(t, new(CreatureFactsWalkSuite)) }

func (s *CreatureFactsWalkSuite) SetupTest() {
	s.ctx = context.Background()
	s.registry, _ = dungeonstest.Scratch(s.T())
	s.raw = dungeonstest.CreatureFactsWalkYAML(s.T())
}

// TestTheWalkDocumentCompiles is the control the scope assertions need: the
// room is a legal document, so a failure below is about the scopes and not
// about the file.
func (s *CreatureFactsWalkSuite) TestTheWalkDocumentCompiles() {
	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{
		Key: dungeonstest.CreatureFactsWalkKey, YAML: s.raw, ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Empty(out.Errors, "the authored room compiles")
	s.Require().NotNil(out.Entry)
}

// TestBothDeedScopesReachTheCompiledTable is the point of the fixture: a
// document authored with `on: ally` and `as: actor` compiles, and the scopes
// are ON THE COMPILED CONDITIONS rather than lost on the way in.
//
// The assertions read the STORED BYTES rather than a Go struct, deliberately:
// the registry keeps the author's YAML verbatim, so a scope that reached the
// compiler but not the stored file would be a document that plays differently
// after a save/reload cycle — the same class of gap the consumer module found
// in its own persistence layer.
func (s *CreatureFactsWalkSuite) TestBothDeedScopesReachTheCompiledTable() {
	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{
		Key: dungeonstest.CreatureFactsWalkKey, YAML: s.raw, ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Require().Empty(out.Errors)

	stored := string(out.Entry.YAML)
	s.Contains(stored, "as: actor", "the pause's reading survives the compile")
	s.Contains(stored, "on: ally", "the ally reading survives the compile")
}

// TestANeutralPairCarriesAnUntil records the rule the World Builder still
// refuses (rpg-dnd5e-web#1197): an `until` on a NEUTRAL pair is legal — the
// engine turns the pair to the OTHER of hostile and neutral (rpg-project#493
// R1) — and only an allied pair has nothing to become.
//
// THE REFUSAL IS ASSERTED TOO, because "neutral is accepted" is only half the
// rule: the other half is that allied is not, and a test that proved only the
// permissive half would pass if the check were deleted entirely.
func (s *CreatureFactsWalkSuite) TestANeutralPairCarriesAnUntil() {
	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{
		Key: dungeonstest.CreatureFactsWalkKey, YAML: s.raw, ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Empty(out.Errors, "a neutral pair with an `until` is a legal document")

	allied := strings.Replace(
		string(s.raw),
		"- {between: [wolves, party], stance: neutral, until: {round: 3}}",
		"- {between: [wolves, party], stance: allied, until: {round: 3}}",
		1,
	)
	s.Require().NotEqual(string(s.raw), allied, "the edit landed on the fixture")

	refused, err := s.registry.Put(s.ctx, &dungeons.PutInput{
		Key: dungeonstest.CreatureFactsWalkKey, YAML: []byte(allied), ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(refused.Errors, "an allied pair has nothing to become")

	var said bool
	for _, e := range refused.Errors {
		if strings.Contains(e.Message, "nothing to become") {
			said = true
		}
	}
	s.True(said, "in the engine's own sentence")
}
