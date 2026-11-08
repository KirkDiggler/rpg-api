package character

import (
	"encoding/json"

	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	dndevents "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

func (s *ConvertersTestSuite) currentRagingJSON(memberID string) json.RawMessage {
	condition, err := conditions.CreateFromRef(&conditions.CreateFromRefInput{
		Ref: refs.Conditions.Raging().String(), MemberID: memberID,
		Origin: &dndevents.ConditionOrigin{Ref: *refs.Features.Rage(), EntityID: memberID},
		Config: json.RawMessage(`{"damage_bonus":{"barbarian_level":true}}`),
	})
	s.Require().NoError(err)
	raw, err := condition.Condition.ToJSON()
	s.Require().NoError(err)
	return raw
}

func (s *ConvertersTestSuite) TestConvertCharacterDataToProto_WithComposedRaging() {
	raw := s.currentRagingJSON("barbarian")
	data := &toolkitchar.Data{ID: "barbarian", Name: "Ragnar", Conditions: []json.RawMessage{raw}}
	result := s.convertWithArmorClass(data)
	s.Require().Len(result.ActiveConditions, 1)
	got := result.ActiveConditions[0]
	display, known := conditions.DisplayFor(*refs.Conditions.Raging())
	s.Require().True(known)
	s.Equal(display.Name, got.Name)
	s.Equal(display.Detail, got.Notes, "the rulebook owns descriptive text")
	s.Equal(refs.Features.Rage().String(), got.Source, "origin ref is metadata, not stacking identity")
	s.Equal([]byte(raw), got.ConditionData, "immutable inputs and runtime pass through unchanged")
	s.Equal(int32(0), got.Duration, "the API must not derive time remaining from the runtime")
}
