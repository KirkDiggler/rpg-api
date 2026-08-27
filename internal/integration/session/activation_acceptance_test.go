package session_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// ragingBarbarian is a level-1 barbarian carrying Rage, with a weapon so the
// Attack declaration compiles beside the activations.
func ragingBarbarian(id, playerID string) *tkcharacter.Data {
	return &tkcharacter.Data{
		ID: id, PlayerID: playerID, Name: id, Level: 1,
		ClassID: classes.Barbarian, RaceID: races.Human,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 14, abilities.CON: 16,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 8,
		},
		HitPoints: 15, MaxHitPoints: 15, ArmorClass: 15, ProficiencyBonus: 2,
		Inventory: []tkcharacter.InventoryItemData{
			{Type: shared.EquipmentTypeWeapon, ID: weapons.Greataxe, Quantity: 1},
		},
		EquipmentSlots: tkcharacter.EquipmentSlots{
			tkcharacter.SlotMainHand: weapons.Greataxe,
		},
		Resources: map[coreResources.ResourceKey]tkcharacter.RecoverableResourceData{
			resources.RageCharges: {
				Current: 2, Maximum: 2, ResetType: coreResources.ResetLongRest,
			},
		},
		Features: []json.RawMessage{json.RawMessage(
			`{"ref":{"module":"dnd5e","type":"features","id":"rage"},` +
				`"id":"rage","name":"Rage","level":1}`)},
	}
}

// inAFightWith seeds one character, walks them into sight of a skeleton, and
// leaves them on a turn clock — the only place activations exist.
func inAFightWith(
	t *testing.T, sheet *tkcharacter.Data,
) (*acceptanceHarness, context.Context) {
	t.Helper()
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), sheet.PlayerID)

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: sheet},
	})
	require.NoError(t, err)

	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "acceptance-run", Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "acceptance-run", ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: at(19, 3),
	})
	require.NoError(t, err)

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "acceptance-run", Member: sheet.ID, Position: pbAt(1, 1),
	})
	require.NoError(t, err)
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "acceptance-run", Member: sheet.ID, Path: tombRoute(),
	})
	require.NoError(t, err)

	return h, ctx
}

func activationsFor(
	t *testing.T, h *acceptanceHarness, ctx context.Context, member string,
) map[string]*sessionpb.Declaration {
	t.Helper()
	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{
		Session: "acceptance-run", Member: member,
	})
	require.NoError(t, err)
	found := map[string]*sessionpb.Declaration{}
	for _, d := range out.GetDeclarations() {
		if d.GetVerb() != sessionpb.Verb_VERB_ACTIVATE {
			continue
		}
		ref := d.GetAbility().GetRef()
		// FAIL RATHER THAN OVERWRITE. Keying by ref is only sound because one
		// ability compiles one offer; a second row for the same ref is a
		// producer defect, and quietly keeping the last one would make every
		// assertion in this file test whichever copy happened to arrive
		// second.
		_, dup := found[ref]
		require.False(t, dup, "Afford offered %q twice", ref)
		found[ref] = d
	}
	return found
}

func storedSheetOf(
	t *testing.T, repo characterrepo.Repository, id string,
) *tkcharacter.Data {
	t.Helper()
	got, err := repo.Get(context.Background(), characterrepo.GetInput{ID: id})
	require.NoError(t, err)
	return got.Character.Data
}

func conditionRefsOf(t *testing.T, repo characterrepo.Repository, id string) []string {
	t.Helper()
	out := make([]string, 0)
	for _, raw := range storedSheetOf(t, repo, id).Conditions {
		var envelope struct {
			Ref string `json:"ref"`
		}
		require.NoError(t, json.Unmarshal(raw, &envelope))
		out = append(out, envelope.Ref)
	}
	return out
}

// THE WHOLE SURFACE, ON THE WIRE, IN ONE READ.
//
// This is the test that says "the panel has something to draw". Everything
// below checks one ability; this checks that a level-1 barbarian's complete
// activatable surface crosses the seam with the four things a button needs —
// a label, a shape, a selector, and an availability.
func TestAcceptance_TheWholeActivatableSurfaceCrossesTheWire(t *testing.T) {
	h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))

	offers := activationsFor(t, h, ctx, "alice")

	want := map[string]sessionpb.Slot{
		"dnd5e:combat_abilities:dash":      sessionpb.Slot_SLOT_ACTION,
		"dnd5e:combat_abilities:disengage": sessionpb.Slot_SLOT_ACTION,
		"dnd5e:combat_abilities:dodge":     sessionpb.Slot_SLOT_ACTION,
		"dnd5e:combat_abilities:help":      sessionpb.Slot_SLOT_ACTION,
		"dnd5e:combat_abilities:hide":      sessionpb.Slot_SLOT_ACTION,
		"dnd5e:features:rage":              sessionpb.Slot_SLOT_BONUS,
	}
	require.Len(t, offers, len(want), "the level-1 barbarian's whole surface")

	seen := map[string]string{}
	for ref, slot := range want {
		offer, ok := offers[ref]
		require.True(t, ok, "no offer for %s", ref)
		require.Equal(t, slot, offer.GetSlot(), "slot for %s", ref)
		require.NotEmpty(t, offer.GetAbility().GetName(), "%s must carry a label", ref)
		// Availability is an ANSWER, so it is asserted rather than assumed:
		// every one of these is reachable on a fresh turn except Help, which
		// has nobody adjacent to help in a solo fight and says so.
		if ref == "dnd5e:combat_abilities:help" {
			require.False(t, offer.GetAvailable(), "%s: no ally to help", ref)
			require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_TARGET_IN_REACH,
				offer.GetWhy().GetReason())
		} else {
			require.True(t, offer.GetAvailable(), "%s must be available on a fresh turn", ref)
			require.Nil(t, offer.GetWhy(), "%s is available, so nothing ran out", ref)
		}
		require.NotEmpty(t, offer.GetId(), "%s must carry a selector", ref)
		prior, clash := seen[offer.GetId()]
		require.False(t, clash, "%s and %s share a selector", prior, ref)
		seen[offer.GetId()] = ref
	}

	// The Attack combat ability every character carries is NOT among them:
	// swinging is VERB_ATTACK, and two buttons for one thing is how a player
	// spends an action banking swings the seam already banked.
	require.NotContains(t, offers, "dnd5e:combat_abilities:attack")
}

// ONE VERB, TWO SHAPES, AT THE SAME MOMENT — the case that forces many rows
// per verb rather than making it a preference, asserted where a client would
// see it.
func TestAcceptance_RageAndDodgeAreLiveTogetherOnDifferentShapes(t *testing.T) {
	h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))

	offers := activationsFor(t, h, ctx, "alice")
	rage := offers["dnd5e:features:rage"]
	dodge := offers["dnd5e:combat_abilities:dodge"]

	require.True(t, rage.GetAvailable())
	require.True(t, dodge.GetAvailable())
	require.Equal(t, sessionpb.Slot_SLOT_BONUS, rage.GetSlot())
	require.Equal(t, sessionpb.Slot_SLOT_ACTION, dodge.GetSlot())

	// And spending one leaves the other alone, which is the whole point of two
	// shapes: raging does not cost the barbarian her action.
	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: rage.GetId(),
	})
	require.NoError(t, err)

	after := activationsFor(t, h, ctx, "alice")
	require.False(t, after["dnd5e:features:rage"].GetAvailable(), "the bonus action is spent")
	require.True(t, after["dnd5e:combat_abilities:dodge"].GetAvailable(), "the action is not")
}

// Rage end to end: the condition reaches Redis, the charge comes off, and a
// second rage in the same fight is refused in a currency of its own.
//
// The last clause used to be a claim this test did not make (Copilot's finding
// on rpg-api#851). It makes it now, and the refusal is the interesting half:
// the bonus action is what runs out first, so the second attempt is refused on
// the SHAPE rather than the charges — which is the correct answer and not the
// one the sentence implied.
func TestAcceptance_RageSpendsACharge(t *testing.T) {
	h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))

	rage := activationsFor(t, h, ctx, "alice")["dnd5e:features:rage"]
	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: rage.GetId(),
	})
	require.NoError(t, err)

	require.Contains(t, conditionRefsOf(t, h.charRepo, "alice"), "dnd5e:conditions:raging")
	stored := storedSheetOf(t, h.charRepo, "alice")
	require.Equal(t, 1, stored.Resources[resources.RageCharges].Current)
	require.Equal(t, 0, stored.ActionEconomy.BonusActionsRemaining)
	require.Equal(t, 1, stored.ActionEconomy.ActionsRemaining)

	// And the second rage this turn is refused, with the panel told which
	// ledger ran out. One charge is left, so this is the bonus action talking.
	again := activationsFor(t, h, ctx, "alice")["dnd5e:features:rage"]
	require.False(t, again.GetAvailable())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_BUDGET,
		again.GetWhy().GetReason())
	require.Equal(t, sessionpb.Currency_CURRENCY_BONUS, again.GetWhy().GetCurrency())

	_, err = h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: again.GetId(),
	})
	require.Error(t, err, "a spent bonus action refuses the second rage")
}

// Each of the self-affecting abilities lands its own condition, read back out
// of Redis. One test per ability would be four copies of one sentence.
func TestAcceptance_EachSelfAffectingAbilityLandsItsCondition(t *testing.T) {
	cases := []struct {
		ref  string
		want string
	}{
		{"dnd5e:combat_abilities:dodge", "dnd5e:conditions:dodging"},
		{"dnd5e:combat_abilities:disengage", "dnd5e:conditions:disengaging"},
	}

	for _, tc := range cases {
		t.Run(tc.ref, func(t *testing.T) {
			h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))
			offer := activationsFor(t, h, ctx, "alice")[tc.ref]
			require.NotNil(t, offer)

			_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
				Session: "acceptance-run", Member: "alice", DeclarationId: offer.GetId(),
			})
			require.NoError(t, err)
			require.Contains(t, conditionRefsOf(t, h.charRepo, "alice"), tc.want)
		})
	}
}

// Dash banks movement rather than applying a condition, so its proof is the
// ledger: the action goes, and the economy says more feet.
func TestAcceptance_DashBanksMovementRatherThanACondition(t *testing.T) {
	h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))

	before := storedSheetOf(t, h.charRepo, "alice")
	beforeFeet := 0
	if before.ActionEconomy != nil {
		beforeFeet = before.ActionEconomy.MovementRemaining
	}

	dash := activationsFor(t, h, ctx, "alice")["dnd5e:combat_abilities:dash"]
	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: dash.GetId(),
	})
	require.NoError(t, err)

	after := storedSheetOf(t, h.charRepo, "alice")
	require.Equal(t, 0, after.ActionEconomy.ActionsRemaining, "dash costs the action")
	require.Greater(t, after.ActionEconomy.MovementRemaining, beforeFeet,
		"dash's whole effect is on the ledger")
}

// A SPENT SELECTOR IS REFUSED, NOT REPLAYED — with the code a client branches
// on, and without having applied anything a second time.
func TestAcceptance_ASpentSelectorIsRefusedWithFailedPrecondition(t *testing.T) {
	h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))

	dodge := activationsFor(t, h, ctx, "alice")["dnd5e:combat_abilities:dodge"]
	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: dodge.GetId(),
	})
	require.NoError(t, err)

	_, err = h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: dodge.GetId(),
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "a refusal must be a status error")
	require.Equal(t, codes.FailedPrecondition, st.Code())

	// And the panel is told why, in a currency it can name.
	after := activationsFor(t, h, ctx, "alice")["dnd5e:combat_abilities:dodge"]
	require.False(t, after.GetAvailable())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_BUDGET,
		after.GetWhy().GetReason())
	require.Equal(t, sessionpb.Currency_CURRENCY_ACTION, after.GetWhy().GetCurrency())
}

// Second Wind heals its own fighter, which is the one activation whose effect
// is neither a condition nor the ledger.
func TestAcceptance_SecondWindHealsTheFighter(t *testing.T) {
	// armedFighter carries no features, so this one gets its own sheet rather
	// than mutating a fixture five other acceptance tests read.
	fighter := armedFighter("alice", "player-alice")
	fighter.HitPoints = 10
	fighter.Features = []json.RawMessage{json.RawMessage(
		`{"ref":{"module":"dnd5e","type":"features","id":"second_wind"},` +
			`"id":"second-wind-1","name":"Second Wind","level":3,` +
			`"character_id":"alice","uses":1,"max_uses":1}`)}

	h, ctx := inAFightWith(t, fighter)

	offers := activationsFor(t, h, ctx, "alice")
	secondWind, ok := offers["dnd5e:features:second_wind"]
	require.True(t, ok, "a fighter carrying Second Wind must be offered it")
	require.Equal(t, sessionpb.Slot_SLOT_BONUS, secondWind.GetSlot())

	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: secondWind.GetId(),
	})
	require.NoError(t, err)

	// The one activation of the seven whose effect is neither a condition nor
	// the ledger — it heals, and the healing has to reach the stored sheet the
	// same way everything else does.
	require.Greater(t, storedSheetOf(t, h.charRepo, "alice").HitPoints, 10,
		"Second Wind heals its own fighter")
}

// HIDE REACHES THE SHEET AND ITS CHECK IS VACUOUS, which are two different
// facts and this test asserts both.
//
// Hide is the one of the seven that is not a pure self-activation: its Stealth
// check is rolled against the passive Perception of whoever could notice. But
// ActivateRequest carries no observers, and rpg-api passes none — so the check
// is made against an empty list and cannot fail. A player hides in plain sight
// of a skeleton, in its line of sight, and succeeds.
//
// The activation half is correct and worth pinning: the condition lands, the
// action is spent, the whole seam works. The difficulty half is filed as
// rpg-project#300's follow-up. When observers are wired this test should start
// needing a roll, which is the signal it exists to give.
func TestAcceptance_HideLandsButItsCheckHasNothingToBeat(t *testing.T) {
	h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))

	hide := activationsFor(t, h, ctx, "alice")["dnd5e:combat_abilities:hide"]
	require.NotNil(t, hide)
	require.True(t, hide.GetAvailable())

	_, err := h.handler.Activate(ctx, &sessionpb.ActivateRequest{
		Session: "acceptance-run", Member: "alice", DeclarationId: hide.GetId(),
	})
	require.NoError(t, err)

	// The seam works.
	require.Contains(t, conditionRefsOf(t, h.charRepo, "alice"), "dnd5e:conditions:hidden")
	require.Equal(t, 0, storedSheetOf(t, h.charRepo, "alice").ActionEconomy.ActionsRemaining)

	// And it worked while a skeleton was looking straight at her, which is the
	// part that is not finished.
}

// --- Help's candidate universe, over the wire (rpg-toolkit#1274) ---

// HELP OFFERS THE ALLY STANDING NEXT TO YOU, and the panel gets everything it
// needs to draw the prompt: a row per ally, each with its own availability.
//
// This is the case rpg-dnd5e-web#835 declines to render today, because before
// #1274 the declaration said TARGET_KIND_MEMBER and carried no candidates at
// all — a control nothing could drive.
func TestAcceptance_HelpOffersTheAdjacentAlly(t *testing.T) {
	alice := ragingBarbarian("alice", "player-alice")
	h, ctx := inAFightWith(t, alice)

	// Bob joins next to her, through the real service and as his own player.
	bob := ragingBarbarian("bob", "player-bob")
	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: bob},
	})
	require.NoError(t, err)

	alicePos, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{
		Session: "acceptance-run", Member: "alice",
	})
	require.NoError(t, err)

	bobCtx := auth.WithPlayerID(context.Background(), "player-bob")
	_, err = h.handler.Join(bobCtx, &sessionpb.JoinRequest{
		Session: "acceptance-run", Member: "bob",
		Position: &sessionpb.Position{
			X: alicePos.GetPosition().GetX() + 1,
			Y: alicePos.GetPosition().GetY(),
		},
	})
	require.NoError(t, err)

	help := activationsFor(t, h, ctx, "alice")["dnd5e:combat_abilities:help"]
	require.NotNil(t, help)
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_MEMBER, help.GetTargetKind())

	require.NotEmpty(t, help.GetCandidates(), "a declaration that asks for a target must say who")
	var bobRow *sessionpb.TargetCandidate
	for _, c := range help.GetCandidates() {
		if c.GetMember() == "bob" {
			bobRow = c
		}
	}
	require.NotNil(t, bobRow, "the ally standing next to her")
	require.True(t, bobRow.GetAvailable())
	require.True(t, help.GetAvailable())
}

// A MONSTER IS NEVER A HELP CANDIDATE, asserted over the wire because the
// filter is a rules decision the client must never be asked to make. The
// skeleton is adjacent, in sight, and absent from the list.
func TestAcceptance_HelpNeverOffersAMonster(t *testing.T) {
	h, ctx := inAFightWith(t, ragingBarbarian("alice", "player-alice"))

	help := activationsFor(t, h, ctx, "alice")["dnd5e:combat_abilities:help"]
	require.NotNil(t, help)

	for _, c := range help.GetCandidates() {
		require.NotEqual(t, "skel-1", c.GetMember(), "a monster is not an ally")
	}

	// With nobody to help, the declaration goes dark with the reason a panel
	// can render — and keeps its selector, so the button is drawn disabled
	// rather than dropped.
	require.False(t, help.GetAvailable())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_TARGET_IN_REACH,
		help.GetWhy().GetReason())
	require.NotEmpty(t, help.GetId())
	require.Equal(t, "Help", help.GetAbility().GetName())
}
