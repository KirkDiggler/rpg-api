package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

func TestSanctuaryEventMappings(t *testing.T) {
	modifier := 2
	calculation := &sdk.RollCalculation{Total: 8, Components: []sdk.RollComponent{{Source: sdk.RollSource{Ref: "dnd5e:abilities:wisdom", Name: "Wisdom", SourceID: "attacker"}, Modifier: &modifier}}}
	attack := sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing}
	spell := sdk.SpellRef{Ref: "dnd5e:spells:bane", Name: "Bane"}
	wantCalculation, err := rollCalculationToProto(calculation)
	require.NoError(t, err)
	events := []sdk.Event{
		{Session: "session", Recipient: "witness", Seq: 7, Kind: sdk.EventWarded, Body: sdk.WardedBody{Attacker: "attacker", Target: "target", Source: "cleric", Attack: attack, Ability: "wisdom", Roll: 6, Total: 8, DC: 15, Calculation: calculation}},
		{Session: "session", Recipient: "witness", Seq: 8, Kind: sdk.EventCastWarded, Body: sdk.CastWardedBody{Actor: "caster", Target: "target", Source: "cleric", Spell: spell, Ability: "wisdom", Roll: 0, Total: 0, DC: 15, Calculation: calculation}},
	}
	got, err := eventsToProto(events)
	require.NoError(t, err)
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_WARDED, got[0].GetKind())
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_CAST_WARDED, got[1].GetKind())
	require.True(t, proto.Equal(&sessionpb.Warded{Attacker: "attacker", Target: "target", Source: "cleric", Attack: attackRefToProto(attack), Ability: "wisdom", Roll: 6, Total: 8, Dc: 15, Calculation: wantCalculation}, got[0].GetWarded()))
	require.True(t, proto.Equal(&sessionpb.CastWarded{Actor: "caster", Target: "target", Source: "cleric", Spell: spellRefToProto(spell), Ability: "wisdom", Dc: 15, Calculation: wantCalculation}, got[1].GetCastWarded()))
	for i, event := range events {
		live, convertErr := eventToProto(event)
		require.NoError(t, convertErr)
		require.True(t, proto.Equal(live, got[i]))
		require.Equal(t, event.Seq, live.GetSeq())
		require.Equal(t, event.Recipient, live.GetRecipient())
		require.Nil(t, live.GetMissed())
		require.Nil(t, live.GetCastMissed())
	}
}
