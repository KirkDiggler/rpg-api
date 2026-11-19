package authoringv1alpha1_test

import (
	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
)

// captainHeardStep is the castle captain's step in the hall (rpg-project#1000):
// on `heard`, a failed claim from someone he has noted `admitted` about sends
// him back to his gate.
const captainHeardStep = "behaviorDefinitions.captain-policy.recipe.states.in-hall.transitions[1]"

// TestPutDungeon_ValidateOnly_CarriesAStepsConditions: the castle's
// validate-only check carries the captain's heard step with each condition
// its `when` reads as a `condition` connection, with the value it compares
// and where it sits (rpg-project#997).
func (s *ReadBackWireSuite) TestPutDungeon_ValidateOnly_CarriesAStepsConditions() {
	resp := s.gradedCastle()
	s.Require().Empty(resp.GetErrors())

	var step *authoringpb.ReadBackStatement
	for _, sec := range resp.GetReadBack().GetSections() {
		for _, g := range sec.GetGroups() {
			for _, st := range g.GetStatements() {
				if st.GetPath() == captainHeardStep && st.GetKind() == "routine.wake" {
					step = st
				}
			}
		}
	}
	s.Require().NotNil(step, "the captain's heard step is said back")

	conditions := map[string]*authoringpb.ReadBackConnection{}
	for _, c := range step.GetConnects() {
		if c.GetKind() == "condition" {
			conditions[c.GetId()] = c
		}
	}

	claim := conditions["heard.claim"]
	s.Require().NotNil(claim)
	s.Equal("asks", claim.GetRole())
	s.Equal("failed", claim.GetIs())
	s.False(claim.GetNegated())
	s.Equal("when.all[0]", claim.GetAt())

	noted := conditions["noted:admitted"]
	s.Require().NotNil(noted)
	s.Equal("true", noted.GetIs())
	s.Equal("when.all[1]", noted.GetAt())
}
