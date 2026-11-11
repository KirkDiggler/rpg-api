package character

import (
	"encoding/json"

	generic "github.com/KirkDiggler/rpg-toolkit/mechanics/effects"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	dnd5ecombat "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	effects "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/effects"
	dndevents "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

func (s *ConvertersTestSuite) currentBlessedJSON(member string) json.RawMessage {
	config, err := json.Marshal(conditions.LinkedConditionConfig{ApplicationID: "api-bless", Role: conditions.LinkedRecipient, AddressSourceID: "caster", Bindings: &conditions.LinkedBindings{Name: "Blessed", MemberKey: member, Owner: &dndevents.ConditionAddress{MemberID: "caster", ConditionRef: refs.Conditions.LinkedEffects().String(), SourceID: "api-bless"}}})
	s.Require().NoError(err)
	created, err := conditions.CreateFromRef(&conditions.CreateFromRefInput{Ref: refs.Conditions.Blessed().String(), MemberID: member, Origin: &dndevents.ConditionOrigin{Ref: *refs.Spells.Bless(), EntityID: "caster"}, Config: config})
	s.Require().NoError(err)
	raw, err := created.Condition.ToJSON()
	s.Require().NoError(err)
	return raw
}

func (s *ConvertersTestSuite) TestObsoleteLinkedConditionDataIsRefused() {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"ref":"dnd5e:conditions:blessed","member_id":"recipient","source_id":"caster"}`),
		json.RawMessage(`{"ref":"dnd5e:conditions:baned","member_id":"recipient","source_id":"caster"}`),
		json.RawMessage(`{"ref":"dnd5e:conditions:concentrating","member_id":"caster","turn_ends_left":10}`),
	} {
		_, err := ConvertCharacterDataToProto(&CharacterProtoInput{Data: &toolkitchar.Data{ID: "recipient", Name: "Recipient", Conditions: []json.RawMessage{raw}}, ArmorClass: &dnd5ecombat.ACBreakdown{Total: 13}})
		s.Require().Error(err)
	}
}

func (s *ConvertersTestSuite) TestConvertLinkedConditionPreservesBytesSourceAndDescription() {
	prepared, err := conditions.PrepareApplication(&conditions.PrepareApplicationInput{ID: "cast-1", CoordinatorID: "caster", Origin: dndevents.ConditionOrigin{Ref: *refs.Spells.Bless(), EntityID: "caster"},
		Existing: []conditions.ParticipantConditions{{MemberID: "caster"}, {MemberID: "recipient"}}, Spec: conditions.ApplicationSpec{Name: "Bless", CoordinatorParts: []effects.PartDeclaration{
			{Key: "hold", Kind: effects.KindConcentration, OnTerminal: generic.EndApplication},
			{Key: "clock", Kind: effects.KindDuration, Params: json.RawMessage(`{"turn_ends":10,"skip_first":true}`), OnTerminal: generic.EndApplication},
		}, Recipients: []conditions.RecipientSpec{{MemberID: "recipient", Ref: refs.Conditions.Blessed().String(), AddressSourceID: "caster", Lifetime: conditions.LifetimeShared}}}})
	s.Require().NoError(err)
	owner, err := conditions.CreateApplication(&conditions.CreateApplicationInput{Prepared: prepared})
	s.Require().NoError(err)
	raw, err := owner.Condition.ToJSON()
	s.Require().NoError(err)
	converted := s.convertWithArmorClass(&toolkitchar.Data{ID: "caster", Name: "Caster", Conditions: []json.RawMessage{raw}})
	s.Require().Len(converted.ActiveConditions, 1, "an object ref must not silently disappear")
	got := converted.ActiveConditions[0]
	s.Equal("Concentrating on Bless", got.Name)
	s.Contains(got.Notes, "10")
	s.Contains(got.Notes, "turn ends")
	s.Equal(refs.Spells.Bless().String(), got.Source)
	s.Equal([]byte(raw), got.ConditionData, "API custody does not re-encode the provider blob")
	s.Zero(got.Duration, "the current linked description owns its clock and units")
	recipient, err := conditions.CreateRecipient(&conditions.CreateRecipientInput{Prepared: prepared, SpecIndex: 0})
	s.Require().NoError(err)
	childRaw, err := recipient.Condition.ToJSON()
	s.Require().NoError(err)
	child := s.convertWithArmorClass(&toolkitchar.Data{ID: "recipient", Name: "Recipient", Conditions: []json.RawMessage{childRaw}})
	s.Require().Len(child.ActiveConditions, 1)
	s.Equal("Blessed", child.ActiveConditions[0].Name)
	s.Equal(refs.Spells.Bless().String(), child.ActiveConditions[0].Source)
	s.Zero(child.ActiveConditions[0].Duration, "local payload does not invent the maintainer clock")
	s.Equal([]byte(childRaw), child.ActiveConditions[0].ConditionData)
}
