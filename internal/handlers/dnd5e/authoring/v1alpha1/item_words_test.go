package authoringv1alpha1_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/magicitems"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	dungeonsmock "github.com/KirkDiggler/rpg-api/internal/dungeons/mock"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// ItemWordsSuite proves the item words cross the wire as the rules publish
// them: every word, every field, nothing reworded (rpg-project#850).
type ItemWordsSuite struct {
	wordsReflect

	handler *authoringhandler.Handler
}

func TestItemWordsSuite(t *testing.T) { suite.Run(t, new(ItemWordsSuite)) }

// SetupSuite builds one handler; the registry is a mock that expects no call
// because the words read no dungeon.
func (s *ItemWordsSuite) SetupSuite() {
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: dungeonsmock.NewMockRegistry(gomock.NewController(s.T()))})
	s.Require().NoError(err)
	s.handler, err = authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
}

func (s *ItemWordsSuite) words() *authoringpb.GetItemWordsResponse {
	resp, err := s.handler.GetItemWords(worldCtx("alice"), &authoringpb.GetItemWordsRequest{})
	s.Require().NoError(err)
	return resp
}

// find is the word of a name written in a place: the table has a `charges` in
// two places, and `charges.max` is the word `max` of place `charges`.
func (s *ItemWordsSuite) find(words []*authoringpb.ItemWord, place, name string) *authoringpb.ItemWord {
	for _, w := range words {
		if w.GetWord() == name && w.GetPlace() == place {
			return w
		}
	}
	s.FailNowf("no such word", "%q in %q", name, place)
	return nil
}

// TestEveryWordAndEveryFieldCrosses: the response equals magicitems.Words,
// field by field, by reflection over magicitems.Word, Value and Bounds,
// through Value.With to the words inside it.
func (s *ItemWordsSuite) TestEveryWordAndEveryFieldCrosses() {
	resp := s.words()

	table := magicitems.Words(&magicitems.WordsInput{}).Words
	s.Require().NotEmpty(table)
	field := resp.ProtoReflect().Descriptor().Fields().ByName("words")
	s.Require().NotNil(field)
	s.requireValue("Words", reflect.ValueOf(table), resp.ProtoReflect().Get(field), field)
}

// TestTheTableIsTheRulesOwn: the words the studio's form leans on, by content.
func (s *ItemWordsSuite) TestTheTableIsTheRulesOwn() {
	resp := s.words()

	slot := s.find(resp.GetWords(), "item", "slot")
	s.Equal("item", slot.GetPlace())
	slots := make([]string, 0, len(slot.GetValues()))
	for _, v := range slot.GetValues() {
		slots = append(slots, v.GetWord())
	}
	s.Equal([]string{"cloak", "amulet", "ring", "belt", "helm", "boots"}, slots)

	use := s.find(resp.GetWords(), "lets entry", "use")
	s.Equal("lets entry", use.GetPlace())
	var secondWind *authoringpb.ItemWordValue
	for _, v := range use.GetValues() {
		if v.GetWord() == "second wind" {
			secondWind = v
		}
	}
	s.Require().NotNil(secondWind, "use has the value second wind")
	s.Require().Len(secondWind.GetWith(), 1)
	level := secondWind.GetWith()[0]
	s.Equal("level", level.GetWord())
	s.Equal("with", level.GetPlace())
	s.Require().NotNil(level.GetBounds())
	s.Equal(int32(1), level.GetBounds().GetMin())
	s.True(level.GetBounds().GetNoMax())

	charges := s.find(resp.GetWords(), "charges", "max")
	s.Require().NotNil(charges.GetBounds())
	s.Equal(int32(1), charges.GetBounds().GetMin())
	s.Equal(int32(1000), charges.GetBounds().GetMax())
	s.False(charges.GetBounds().GetNoMax())
}

// TestRulebookVersionIsFilled: the rulebook release comes from the binary's
// build info, as the status words' does, which a test binary carries too.
func (s *ItemWordsSuite) TestRulebookVersionIsFilled() {
	s.Regexp(`^v\d+\.\d+\.\d+`, s.words().GetRulebookVersion())
}

// TestGatedOnSignIn: like every RPC on this service, a caller with no player
// is refused.
func (s *ItemWordsSuite) TestGatedOnSignIn() {
	_, err := s.handler.GetItemWords(s.T().Context(), &authoringpb.GetItemWordsRequest{})
	s.Require().Error(err)
}
