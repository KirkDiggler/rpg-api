package authoringv1alpha1_test

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/scenarios"
	"google.golang.org/grpc/codes"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
)

// TestListScenarios_Unauthenticated: authoring is a signed-in verb, ungated
// or not.
func (s *HandlerSuite) TestListScenarios_Unauthenticated() {
	_, err := s.handler.ListScenarios(s.T().Context(), &authoringpb.ListScenariosRequest{})
	s.requireCode(err, codes.Unauthenticated)
}

// TestListScenarios_NeverTouchesTheRegistry is the ungating, made
// mechanical: the answer is a property of the SERVER'S rulebook build, not
// of any stored content, so a build with no dungeons at all still has one.
// The registry mock expects nothing -- a call to it fails the test.
func (s *HandlerSuite) TestListScenarios_NeverTouchesTheRegistry() {
	resp, err := s.handler.ListScenarios(s.ctx, &authoringpb.ListScenariosRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
}

// TestListScenarios_IsTheRulebooksOwnDescriptorVerbatim runs over All()
// rather than over the one scenario somebody remembered, so the second
// scenario cannot skip it (the toolkit registry's own reason for existing).
//
// Every string on the wire is compared to the string the rulebook exports.
// There is no rpg-api copy of a label, a key or a guidance sentence, and
// this is what says so: a handler that "improved" a label, sorted the
// fields, or filled in a default would fail here.
func (s *HandlerSuite) TestListScenarios_IsTheRulebooksOwnDescriptorVerbatim() {
	resp, err := s.handler.ListScenarios(s.ctx, &authoringpb.ListScenariosRequest{})
	s.Require().NoError(err)

	all := scenarios.All()
	s.Require().NotEmpty(all, "this build offers no scenario at all -- the registry is broken, not the wire")
	s.Require().Len(resp.GetScenarios(), len(all))

	for i, want := range all {
		got := resp.GetScenarios()[i]
		s.Require().Equal(want.ID(), got.GetId())
		s.Require().Equal(want.Name(), got.GetName())

		wantFields := want.Fields()
		s.Require().Len(got.GetFields(), len(wantFields), "scenario %q", want.ID())
		for j, wf := range wantFields {
			gf := got.GetFields()[j]
			s.Require().Equal(wf.Key, gf.GetKey())
			s.Require().Equal(wf.Label, gf.GetLabel())
			s.Require().Equal(wf.Kind, gf.GetKind())
			s.Require().Equal(wf.Guidance, gf.GetGuidance())
			s.Require().NotEqual(authoringpb.FieldType_FIELD_TYPE_UNSPECIFIED, gf.GetType(),
				"field %q of %q has a widget shape this build's protos cannot name",
				wf.Key, want.ID())
		}
	}
}

// TestListScenarios_GuidanceIsTheRefusal is the load-bearing half, and the
// one a field-for-field copy test cannot give: the sentence the builder shows
// while a blank is EMPTY has to be the sentence the author gets back when
// they get it WRONG. One string, two jobs -- two would drift, and the one
// that drifts is always the one nobody reads.
//
// So this fills in every blank of every scenario with something the rulebook
// would accept, then empties one blank at a time and checks the refusal
// carries exactly the guidance that travelled the wire. It runs over All(),
// so a scenario added later cannot skip it.
//
// A kind this test cannot satisfy FAILS rather than skips. `kind` is an open
// string on purpose -- a rulebook may grow a new family of bindable thing
// without a proto release -- and a silent skip here is how such a scenario
// would arrive with its guidance unpinned.
func (s *HandlerSuite) TestListScenarios_GuidanceIsTheRefusal() {
	resp, err := s.handler.ListScenarios(s.ctx, &authoringpb.ListScenariosRequest{})
	s.Require().NoError(err)

	for _, got := range resp.GetScenarios() {
		scenario, known := scenarios.Lookup(got.GetId())
		s.Require().True(known, "the wire named a scenario this build does not have: %q", got.GetId())

		// A dungeon that places one satisfying thing per blank, named after
		// the blank. The ids are this test's own invention, which is fine
		// precisely because the scenario binds by id and never looks at what
		// an id spells.
		facts := &scenarios.DungeonFacts{
			Props: map[string]bool{}, Exits: map[string]bool{},
		}
		filled := map[string]string{}
		for _, field := range got.GetFields() {
			s.Require().Equal(authoringpb.FieldType_FIELD_TYPE_ENTITY_REF, field.GetType(),
				"this test can only satisfy entity_ref blanks; %q.%q wants %s",
				got.GetId(), field.GetKey(), field.GetType())
			filled[field.GetKey()] = field.GetKey()
			switch field.GetKind() {
			case "prop":
				facts.Props[field.GetKey()] = true // holdable: the only prop a binding accepts
			case "exit":
				facts.Exits[field.GetKey()] = true
			default:
				s.Require().Failf("unsatisfiable kind",
					"%q.%q binds kind %q, which this test does not know how to place -- "+
						"add it here rather than letting the guidance go unpinned",
					got.GetId(), field.GetKey(), field.GetKind())
			}
		}

		// Filled in, it constructs. Without this the loop below would pass
		// on a scenario that refuses everything for some unrelated reason.
		_, err := scenario.New(filled, facts)
		s.Require().NoError(err, "scenario %q refuses a fully bound form", got.GetId())

		for _, field := range got.GetFields() {
			blanked := map[string]string{}
			for k, v := range filled {
				blanked[k] = v
			}
			delete(blanked, field.GetKey())

			_, newErr := scenario.New(blanked, facts)
			s.Require().Error(newErr,
				"%q with %q left blank must refuse -- nothing is defaulted",
				got.GetId(), field.GetKey())
			s.Require().Contains(newErr.Error(), field.GetGuidance(),
				"the guidance the builder shows for %q.%q is not the sentence its constructor refuses with",
				got.GetId(), field.GetKey())
		}
	}
}
