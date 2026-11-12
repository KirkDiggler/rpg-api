package session_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterdata "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
)

type LinkedMaintenanceAcceptanceSuite struct{ suite.Suite }

func TestLinkedMaintenanceAcceptanceSuite(t *testing.T) {
	suite.Run(t, new(LinkedMaintenanceAcceptanceSuite))
}

// The caster is created/finalized through the public character path. No active
// linked condition or class resource is seeded to make the spell work.
func (s *LinkedMaintenanceAcceptanceSuite) TestNativeCreationCastReloadAndConcentrationEnd() {
	h, ctx, id := nativeClericCombatScene(s.T())
	row := castRowFor(ctx, s.T(), h, id, spells.Bless)
	s.Require().True(row.GetAvailable())
	_, err := h.handler.Cast(ctx, &sessionpb.CastRequest{Session: castSessionID, Member: id, DeclarationId: row.GetId(), Targets: []string{id, "alice"}})
	s.Require().NoError(err)
	paid := storedSheetOf(s.T(), h.charRepo, id)
	s.Equal(1, paid.Resources[resources.SpellSlotLevel1].Current)
	var application string
	var ownerBytes []byte
	for _, raw := range paid.Conditions {
		loaded, loadErr := conditions.LoadJSON(raw)
		s.Require().NoError(loadErr)
		if linked, ok := loaded.(*conditions.LinkedCondition); ok && linked.LinkedApplicationInfo().Role == conditions.LinkedCoordinator {
			application = linked.LinkedApplicationInfo().ID
			ownerBytes = append([]byte(nil), raw...)
			s.Empty(linked.LinkedApplicationInfo().PendingMembers)
			s.Len(linked.LinkedApplicationInfo().LiveMembers, 2)
		}
	}
	s.Require().NotEmpty(application)
	reopenClericHost(s.T(), h, sdk.StaleTargetRefuse)
	private := newCharacterCreationHandler(s.T(), h)
	view, err := private.GetCharacter(ctx, &characterpb.GetCharacterRequest{CharacterId: id})
	s.Require().NoError(err)
	found := false
	for _, condition := range view.GetCharacter().GetActiveConditions() {
		if condition.GetName() == "Concentrating on Bless" {
			found = true
			s.Equal(refs.Spells.Bless().String(), condition.GetSource())
			s.Equal(ownerBytes, condition.GetConditionData())
			s.Contains(condition.GetNotes(), "turn ends")
		}
	}
	s.True(found, "the owner-private read must not drop an object-ref coordinator")
	current, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(s.T(), h)})
	s.Require().NoError(err)
	ownerView, err := current.GetCharacterData(ctx, &characterdata.GetCharacterDataRequest{CharacterId: id})
	s.Require().NoError(err)
	named := false
	for _, condition := range ownerView.GetCharacter().GetConditions() {
		if condition.GetName() == "Concentrating on Bless" {
			named = true
			s.Contains(condition.GetDetail(), "turn ends")
			s.Equal(id, condition.GetSourceMember(), "private source member is the caster, never the application key")
			s.NotEqual(application, condition.GetSourceMember())
		}
	}
	s.True(named, "the native v2 private sheet must receive the same provider-owned maintenance description")
	ally := storedSheetOf(s.T(), h.charRepo, "alice")
	recipientFound := false
	for _, raw := range ally.Conditions {
		loaded, loadErr := conditions.LoadJSON(raw)
		s.Require().NoError(loadErr)
		if linked, ok := loaded.(*conditions.LinkedCondition); ok && linked.LinkedApplicationInfo().ID == application {
			recipientFound = true
			s.Equal(id, linked.ConditionOrigin().EntityID)
		}
	}
	s.True(recipientFound)
	_, err = h.handler.End(ctx, &sessionpb.EndRequest{Session: castSessionID, Ending: sdk.EndingWithdrawn})
	s.Require().NoError(err)
	for _, member := range []string{id, "alice"} {
		data := storedSheetOf(s.T(), h.charRepo, member)
		for _, raw := range data.Conditions {
			loaded, err := conditions.LoadJSON(raw)
			s.Require().NoError(err)
			if linked, ok := loaded.(*conditions.LinkedCondition); ok {
				s.NotEqual(application, linked.LinkedApplicationInfo().ID, "departure persists exact linked cleanup on every holder")
			}
		}
	}
}
