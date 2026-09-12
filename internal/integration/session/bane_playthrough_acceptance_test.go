package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	dnd5epb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	characterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	diceorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

const (
	baneSpellRef              = "dnd5e:spells:bane"
	thunderwaveSpellRef       = "dnd5e:spells:thunderwave"
	dissonantWhispersSpellRef = "dnd5e:spells:dissonant-whispers"
	commandSpellRef           = "dnd5e:spells:command"
)

func newCharacterCreationHandler(t *testing.T, h *acceptanceHarness) *characterhandler.Handler {
	t.Helper()

	drafts, err := characterdraft.NewRedis(&characterdraft.Config{
		Client: h.redis, Clock: clock.New(), IDGenerator: idgen.NewSequential("stored-draft"),
	})
	require.NoError(t, err)
	diceSessions, err := dicesession.NewRedisRepository(&dicesession.Config{Client: h.redis, Clock: clock.New()})
	require.NoError(t, err)
	diceService, err := diceorch.NewOrchestrator(&diceorch.Config{
		DiceSessionRepo: diceSessions, IDGenerator: idgen.NewSequential("roll"),
	})
	require.NoError(t, err)
	characters, err := characterorch.New(&characterorch.Config{
		DraftRepo: drafts, CharacterRepo: h.charRepo, DiceService: diceService,
		IDGenerator: idgen.NewSequential("character"), DraftIDGenerator: idgen.NewSequential("draft"),
		// This playthrough is about a spell, not about who was told a
		// weapon moved — said out loud, because the capability is required
		// so that "nobody is told" is a choice rather than a nil.
		AppearanceNotifier: characterorch.NoAppearanceNotifier{},
	})
	require.NoError(t, err)
	handler, err := characterhandler.NewHandler(&characterhandler.HandlerConfig{CharacterService: characters})
	require.NoError(t, err)
	return handler
}

func createFinalizedBaneBard(t *testing.T, h *acceptanceHarness, playerID string) string {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), playerID)
	handler := newCharacterCreationHandler(t, h)

	created, err := handler.CreateDraft(ctx, &dnd5epb.CreateDraftRequest{})
	require.NoError(t, err)
	draftID := created.GetDraft().GetId()
	_, err = handler.UpdateName(ctx, &dnd5epb.UpdateNameRequest{DraftId: draftID, Name: "Bella"})
	require.NoError(t, err)
	_, err = handler.UpdateRace(ctx, &dnd5epb.UpdateRaceRequest{
		DraftId: draftID, Race: dnd5epb.Race_RACE_HUMAN,
		RaceChoices: []*dnd5epb.ChoiceData{{
			Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
			Source:   dnd5epb.ChoiceSource_CHOICE_SOURCE_RACE,
			Selection: &dnd5epb.ChoiceData_Languages{Languages: &dnd5epb.LanguageSelection{
				Languages: []dnd5epb.Language{dnd5epb.Language_LANGUAGE_ELVISH},
			}},
		}},
	})
	require.NoError(t, err)
	_, err = handler.UpdateClass(ctx, &dnd5epb.UpdateClassRequest{
		DraftId: draftID, Class: dnd5epb.Class_CLASS_BARD,
		ClassChoices: []*dnd5epb.ChoiceData{
			{
				Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5epb.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-skills",
				Selection: &dnd5epb.ChoiceData_Skills{Skills: &dnd5epb.SkillSelection{Skills: []dnd5epb.Skill{
					dnd5epb.Skill_SKILL_PERFORMANCE, dnd5epb.Skill_SKILL_PERSUASION, dnd5epb.Skill_SKILL_DECEPTION,
				}}},
			},
			{
				Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5epb.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-instruments",
				Selection: &dnd5epb.ChoiceData_Tools{Tools: &dnd5epb.ToolSelection{Tools: []dnd5epb.Tool{
					dnd5epb.Tool_TOOL_LUTE, dnd5epb.Tool_TOOL_FLUTE, dnd5epb.Tool_TOOL_DRUM,
				}}},
			},
			{
				Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, Source: dnd5epb.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-weapons-primary", OptionId: "bard-weapon-a",
				Selection: &dnd5epb.ChoiceData_Equipment{Equipment: &dnd5epb.EquipmentSelection{}},
			},
			{
				Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, Source: dnd5epb.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-pack", OptionId: "bard-pack-b",
				Selection: &dnd5epb.ChoiceData_Equipment{Equipment: &dnd5epb.EquipmentSelection{}},
			},
			{
				Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT, Source: dnd5epb.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-instrument", OptionId: "bard-instrument-a",
				Selection: &dnd5epb.ChoiceData_Equipment{Equipment: &dnd5epb.EquipmentSelection{}},
			},
			{
				Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS, Source: dnd5epb.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-cantrips-1",
				Selection: &dnd5epb.ChoiceData_Spells{Spells: &dnd5epb.SpellSelection{SpellRefs: []string{
					"dnd5e:spells:true-strike", "dnd5e:spells:vicious-mockery",
				}}},
			},
			{
				Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_SPELLS, Source: dnd5epb.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "bard-spells-1",
				// ALL OF THEM, because the level-1 pick takes every
				// supported spell (rpg-toolkit#1661) and validates the count
				// exactly. This test is about Bane; the other three ride
				// along only because the pick refuses a bard who left a slot
				// empty.
				Selection: &dnd5epb.ChoiceData_Spells{Spells: &dnd5epb.SpellSelection{SpellRefs: []string{
					baneSpellRef, thunderwaveSpellRef, dissonantWhispersSpellRef, commandSpellRef,
				}}},
			},
		},
	})
	require.NoError(t, err)
	_, err = handler.UpdateBackground(ctx, &dnd5epb.UpdateBackgroundRequest{
		DraftId: draftID, Background: dnd5epb.Background_BACKGROUND_OUTLANDER,
		BackgroundChoices: []*dnd5epb.ChoiceData{{
			Category: dnd5epb.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
			Source:   dnd5epb.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
			ChoiceId: "outlander-instrument",
			Selection: &dnd5epb.ChoiceData_Tools{Tools: &dnd5epb.ToolSelection{
				Tools: []dnd5epb.Tool{dnd5epb.Tool_TOOL_LUTE},
			}},
		}},
	})
	require.NoError(t, err)
	_, err = handler.UpdateAbilityScores(ctx, &dnd5epb.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5epb.UpdateAbilityScoresRequest_AbilityScores{AbilityScores: &dnd5epb.AbilityScores{
			Strength: 8, Dexterity: 14, Constitution: 12, Intelligence: 10, Wisdom: 12, Charisma: 16,
		}},
	})
	require.NoError(t, err)

	validated, err := handler.GetDraft(ctx, &dnd5epb.GetDraftRequest{DraftId: draftID})
	require.NoError(t, err)
	require.True(t, validated.GetDraft().GetValidation().GetIsValid(),
		"normal draft validation must accept the Bane choice: %s", validated.GetDraft().GetValidation())
	finalized, err := handler.FinalizeDraft(ctx, &dnd5epb.FinalizeDraftRequest{DraftId: draftID})
	require.NoError(t, err)
	require.Contains(t, finalized.GetCharacter().GetKnownSpells(), baneSpellRef,
		"the spell this playthrough is about reached the finalized character")
	return finalized.GetCharacter().GetId()
}

func TestAcceptance_BaneCreationCastPaymentAndAffectedRoll(t *testing.T) {
	h := newAcceptanceHarnessWithDice(t, failedSaveDice{})
	const bardPlayer = "player-bella"
	bardID := createFinalizedBaneBard(t, h, bardPlayer)
	bardCtx := auth.WithPlayerID(context.Background(), bardPlayer)
	fighterCtx := auth.WithPlayerID(context.Background(), "player-fighter")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("fighter", "player-fighter")},
	})
	require.NoError(t, err)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "bane-playthrough", Encounter: "room-encounter", World: buildOpenRoom(t, 12, 6),
	})
	require.NoError(t, err)
	_, err = h.handler.Join(bardCtx, &sessionpb.JoinRequest{Session: "bane-playthrough", Member: bardID, Position: pbAt(2, 0)})
	require.NoError(t, err)
	_, err = h.handler.Join(fighterCtx, &sessionpb.JoinRequest{Session: "bane-playthrough", Member: "fighter", Position: pbAt(3, 0)})
	require.NoError(t, err)
	inCombat(t, h.charRepo, bardID, 1)
	inCombat(t, h.charRepo, "fighter", 1)
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "bane-playthrough", ID: "skel-1", Ref: refs.Monsters.Skeleton().String(), Position: at(4, 0),
	})
	require.NoError(t, err)

	turn, err := h.handler.Turn(bardCtx, &sessionpb.TurnRequest{Session: "bane-playthrough", Member: bardID})
	require.NoError(t, err)
	if turn.GetActive() == "fighter" {
		_, err = h.handler.EndTurn(fighterCtx, &sessionpb.EndTurnRequest{
			Session: "bane-playthrough", Member: "fighter",
			DeclarationId: currentDeclarationID(fighterCtx, t, h.handler, "bane-playthrough", "fighter", sessionpb.Verb_VERB_END_TURN),
		})
		require.NoError(t, err)
	}
	turn, err = h.handler.Turn(bardCtx, &sessionpb.TurnRequest{Session: "bane-playthrough", Member: bardID})
	require.NoError(t, err)
	require.Equal(t, bardID, turn.GetActive())

	row := castRowForSession(bardCtx, t, h, "bane-playthrough", bardID, baneSpellRef)
	require.Equal(t, int32(1), row.GetMinTargets())
	require.Equal(t, int32(3), row.GetMaxTargets())
	require.Equal(t, []*sessionpb.CostComponent{
		{Currency: sessionpb.Currency_CURRENCY_ACTION, Needed: 1},
		{Currency: sessionpb.Currency_CURRENCY_CHARGES, Needed: 1, Label: "1st-level Spell Slots"},
	}, row.GetCost())

	before, err := h.charRepo.Get(context.Background(), characterrepo.GetInput{ID: bardID})
	require.NoError(t, err)
	require.Equal(t, 2, before.Character.Data.Resources[resources.SpellSlotLevel1].Current)
	_, err = h.handler.Cast(bardCtx, &sessionpb.CastRequest{
		Session: "bane-playthrough", Member: bardID, DeclarationId: row.GetId(),
		Target: "fighter", Targets: []string{"fighter"},
	})
	requireGRPCCode(t, err, codes.InvalidArgument)

	_, err = h.handler.Cast(bardCtx, &sessionpb.CastRequest{
		Session: "bane-playthrough", Member: bardID, DeclarationId: row.GetId(), Targets: []string{"fighter"},
	})
	require.NoError(t, err)
	after, err := h.charRepo.Get(context.Background(), characterrepo.GetInput{ID: bardID})
	require.NoError(t, err)
	require.Equal(t, 1, after.Character.Data.Resources[resources.SpellSlotLevel1].Current,
		"one successful cast spends exactly one provider-owned slot")

	story, err := h.handler.GetStory(bardCtx, &sessionpb.GetStoryRequest{Session: "bane-playthrough", Member: bardID})
	require.NoError(t, err)
	var castEvent *sessionpb.Cast
	var saveEvent *sessionpb.Saved
	var baned *sessionpb.ConditionApplied
	for _, event := range story.GetEntries() {
		switch event.GetKind() {
		case sessionpb.EventKind_EVENT_KIND_CAST:
			castEvent = event.GetCast()
		case sessionpb.EventKind_EVENT_KIND_SAVED:
			saveEvent = event.GetSaved()
		case sessionpb.EventKind_EVENT_KIND_ACTIVATION_RESULT:
			if applied := event.GetActivationResult().GetConditionApplied(); applied != nil &&
				applied.GetRef() == refs.Conditions.Baned().String() {
				baned = applied
			}
		}
	}
	require.NotNil(t, castEvent)
	require.Equal(t, []string{"fighter"}, castEvent.GetTargets())
	require.Equal(t, "fighter", castEvent.GetTarget(), "one canonical target has a faithful legacy projection")
	require.NotNil(t, saveEvent)
	require.NotNil(t, saveEvent.GetCalculation(), "the Bane saving throw carries provider-authored calculation facts")
	require.Equal(t, saveEvent.GetTotal(), saveEvent.GetCalculation().GetTotal())

	// WHOSE BANE. Two casters can each land Bane on this fighter, and the
	// condition's address is target plus ref plus source -- so the beat that
	// announces one must name the caster, or a client holding two rows cannot
	// say which is which, and cannot strike the right one when a removal beat
	// arrives. Read here, end to end, because the walk found the typed field
	// empty while the raw payload beside it carried the id (2026-09-12).
	require.NotNil(t, baned, "the failed save attached Bane, and the beat says so")
	require.Equal(t, "fighter", baned.GetTarget())
	require.Equal(t, bardID, baned.GetSourceId(),
		"the caster who spent the slot is the source the beat names")

	_, err = h.handler.EndTurn(bardCtx, &sessionpb.EndTurnRequest{
		Session: "bane-playthrough", Member: bardID,
		DeclarationId: currentDeclarationID(bardCtx, t, h.handler, "bane-playthrough", bardID, sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)
	attackID := currentDeclarationID(
		fighterCtx, t, h.handler, "bane-playthrough", "fighter", sessionpb.Verb_VERB_ATTACK,
	)
	attack, err := h.handler.Attack(fighterCtx, &sessionpb.AttackRequest{
		Session: "bane-playthrough", Attacker: "fighter", Target: "skel-1", DeclarationId: attackID,
	})
	require.NoError(t, err)
	require.NotNil(t, attack.GetCalculation())
	var bane *sessionpb.RollComponent
	for _, component := range attack.GetCalculation().GetComponents() {
		if component.GetSource().GetRef() == baneSpellRef {
			bane = component
			break
		}
	}
	require.NotNil(t, bane, "the affected attack carries the Bane calculation component")
	require.True(t, bane.GetSubtractDice())
	require.Equal(t, bardID, bane.GetSource().GetSourceId(), "responsible source identity crosses the API")
}

func castRowForSession(
	ctx context.Context, t *testing.T, h *acceptanceHarness, sessionID, member, spellRef string,
) *sessionpb.Declaration {
	t.Helper()
	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: sessionID, Member: member})
	require.NoError(t, err)
	for _, row := range out.GetDeclarations() {
		if row.GetVerb() == sessionpb.Verb_VERB_CAST && row.GetSpell().GetRef() == spellRef {
			return row
		}
	}
	require.FailNow(t, "Bane declaration not offered")
	return nil
}
