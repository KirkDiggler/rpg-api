package sessionv1alpha2

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
)

type ConvertActSuite struct {
	suite.Suite
}

func TestConvertActSuite(t *testing.T) {
	suite.Run(t, new(ConvertActSuite))
}

func i32(v int32) *int32 { return &v }
func bp(v bool) *bool    { return &v }
func ip(v int) *int      { return &v }
func fp(v float64) *float64 {
	return &v
}

var (
	alice   = sdk.Party{Kind: sdk.PartyMember, ID: "alice"}
	gate1   = sdk.Party{Kind: sdk.PartyMember, ID: "gate-1"}
	chest   = sdk.Party{Kind: sdk.PartyProp, ID: "chest"}
	guards  = sdk.Party{Kind: sdk.PartyFaction, ID: "guards"}
	aliceP  = &sessionpb.Party{Kind: sessionpb.PartyKind_PARTY_KIND_MEMBER, Id: "alice"}
	gate1P  = &sessionpb.Party{Kind: sessionpb.PartyKind_PARTY_KIND_MEMBER, Id: "gate-1"}
	chestP  = &sessionpb.Party{Kind: sessionpb.PartyKind_PARTY_KIND_PROP, Id: "chest"}
	guardsP = &sessionpb.Party{Kind: sessionpb.PartyKind_PARTY_KIND_FACTION, Id: "guards"}
	cell    = spatial.Position{X: 3, Y: -2}
	cellP   = &sessionpb.Position{X: 3, Y: -2}
	d20     = &sdk.Roll{
		Expression: "1d20+5", Dice: []int{15}, Modifiers: []sdk.RollModifier{{Source: "Strength", Value: 3}, {Source: "Proficiency", Value: 2}},
		Total: 20, Against: ip(13), Beaten: bp(true),
		Influences: []sdk.RollInfluence{
			{Kind: sdk.InfluenceAdvantage, SourceRef: "dnd5e:conditions:helped", SourceID: "bob", Name: "Helped"},
			{Kind: sdk.InfluenceDisadvantage, SourceRef: "dnd5e:conditions:prone"},
		},
		Terms: []sdk.RollDiceTerm{
			{Count: 1, DieSize: 20, Faces: []int{15}, SourceRef: "dnd5e:weapons:longsword", SourceName: "Longsword", SourceID: "alice"},
			{Count: 1, DieSize: 4, Faces: []int{2}, Subtract: true, SourceRef: "dnd5e:spells:bane", SourceName: "Bane", SourceID: "cultist"},
		},
	}
	d20P = &sessionpb.Roll{
		Expression: "1d20+5", Dice: []int32{15},
		Modifiers: []*sessionpb.RollModifier{{Source: "Strength", Value: 3}, {Source: "Proficiency", Value: 2}},
		Total:     20, Against: i32(13), Beaten: bp(true),
		Influences: []*sessionpb.RollInfluence{
			{Kind: sessionpb.InfluenceKind_INFLUENCE_KIND_ADVANTAGE, SourceRef: "dnd5e:conditions:helped", SourceId: "bob", Name: "Helped"},
			{Kind: sessionpb.InfluenceKind_INFLUENCE_KIND_DISADVANTAGE, SourceRef: "dnd5e:conditions:prone"},
		},
		Terms: []*sessionpb.RollDiceTerm{
			{Count: 1, DieSize: 20, Faces: []int32{15}, SourceRef: "dnd5e:weapons:longsword", SourceName: "Longsword", SourceId: "alice"},
			{Count: 1, DieSize: 4, Faces: []int32{2}, Subtract: true, SourceRef: "dnd5e:spells:bane", SourceName: "Bane", SourceId: "cultist"},
		},
	}
	tableDie  = &sdk.Roll{Dice: []int{4}, Total: 4, Of: 6}
	tableDieP = &sessionpb.Roll{Dice: []int32{4}, Total: 4, Of: 6}
	bless     = sdk.SpellRef{Ref: "dnd5e:spells:bless", Name: "Bless"}
	blessP    = &sessionpb.SpellRef{Ref: "dnd5e:spells:bless", Name: "Bless"}
	oa        = sdk.ReactionRef{Ref: "dnd5e:conditions:opportunity_attack", Name: "Opportunity Attack"}
	oaP       = &sessionpb.ReactionRef{Ref: "dnd5e:conditions:opportunity_attack", Name: "Opportunity Attack"}
)

func answer() sdk.TableAnswer {
	return sdk.TableAnswer{
		Who: gate1, Option: "stand-aside", Key: "persuade", Word: "yield", Selector: "calm", Say: "Go on, then.",
		Temperament: "wary", Entry: 2, Candidates: []sdk.TableCandidate{{Entry: 1, Weight: 2, Percent: 40, Loaded: 1}},
		Roll: tableDie, Dealt: &sdk.Temper{Faction: guards, Temper: "wary", Roll: *tableDie},
	}
}

func answerP() *sessionpb.TableAnswer {
	return &sessionpb.TableAnswer{
		Who: gate1P, Option: "stand-aside", Key: "persuade", Word: "yield", Selector: "calm", Say: "Go on, then.",
		Temperament: "wary", Entry: 2, Candidates: []*sessionpb.TableCandidate{{Entry: 1, Weight: 2, Percent: 40, Loaded: 1}},
		Roll: tableDieP, Dealt: &sessionpb.Temper{Faction: guardsP, Temper: "wary", Roll: tableDieP},
	}
}

// bodyCase is one SDK body, the act family it rides, and the act it spells.
type bodyCase struct {
	family sdk.Family
	body   sdk.ActBody
	want   *sessionpb.Act
}

func act(family sessionpb.Family, body any) *sessionpb.Act {
	out := &sessionpb.Act{Session: "sess-1", Id: "id-9", At: 4, Act: "attack:id-1", Verb: "attack", Family: family, Segment: 1}
	switch b := body.(type) {
	case *sessionpb.WalkAct:
		out.Body = &sessionpb.Act_Walk{Walk: b}
	case *sessionpb.AttackAct:
		out.Body = &sessionpb.Act_Attack{Attack: b}
	case *sessionpb.CastAct:
		out.Body = &sessionpb.Act_Cast{Cast: b}
	case *sessionpb.ActivateAct:
		out.Body = &sessionpb.Act_Activate{Activate: b}
	case *sessionpb.SpeakAct:
		out.Body = &sessionpb.Act_Speak{Speak: b}
	case *sessionpb.DeathSaveAct:
		out.Body = &sessionpb.Act_DeathSave{DeathSave: b}
	case *sessionpb.LootAct:
		out.Body = &sessionpb.Act_Loot{Loot: b}
	case *sessionpb.HoldAct:
		out.Body = &sessionpb.Act_Hold{Hold: b}
	case *sessionpb.DropAct:
		out.Body = &sessionpb.Act_Drop{Drop: b}
	case *sessionpb.DoorAct:
		out.Body = &sessionpb.Act_Door{Door: b}
	case *sessionpb.EquipAct:
		out.Body = &sessionpb.Act_Equip{Equip: b}
	case *sessionpb.StayAct:
		out.Body = &sessionpb.Act_Stay{Stay: b}
	case *sessionpb.SneakAct:
		out.Body = &sessionpb.Act_Sneak{Sneak: b}
	case *sessionpb.RestAct:
		out.Body = &sessionpb.Act_Rest{Rest: b}
	case *sessionpb.TableAnswer:
		out.Body = &sessionpb.Act_TableAnswer{TableAnswer: b}
	case *sessionpb.StateChange:
		out.Body = &sessionpb.Act_State{State: b}
	case *sessionpb.Perception:
		out.Body = &sessionpb.Act_Perception{Perception: b}
	case *sessionpb.ClockEntry:
		out.Body = &sessionpb.Act_Clock{Clock: b}
	}
	return out
}

//nolint:funlen // the table is the test
func everyBody() map[string]bodyCase {
	ans := answer()
	return map[string]bodyCase{
		"walk": {sdk.FamilyAct,
			sdk.WalkAct{Mover: alice, Path: []sdk.WalkStep{{Cell: cell, Doors: []string{"gate"}}, {Cell: spatial.Position{X: 4, Y: -2}}}, Cause: "shoved"},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.WalkAct{
				Mover: aliceP, Path: []*sessionpb.WalkStep{{Cell: cellP, Doors: []string{"gate"}}, {Cell: &sessionpb.Position{X: 4, Y: -2}}}, Cause: "shoved",
			})},
		"attack": {sdk.FamilyReaction,
			sdk.AttackAct{
				By: gate1, To: alice, Attack: sdk.AttackRef{Ref: "dnd5e:weapons:spear", Name: "Spear", DamageType: sdk.DamagePiercing},
				Outcome: sdk.AttackHit, Critical: true, Damage: 7,
				DamageComponents: []sdk.DamageComponent{{
					Source: "Spear", SourceRef: "dnd5e:weapons:spear", DamageType: sdk.DamagePiercing, Multiplier: fp(0.5),
					Roll: sdk.RollComponent{Source: sdk.RollSource{Name: "Strength"}, Dice: &sdk.DiceTrace{DieSize: 6, FinalRolls: []int{2, 5}, Subtotal: 7}, Modifier: ip(2)},
				}},
				Roll: d20, Ward: &sdk.WardSave{Source: alice, Ability: "wis", Roll: *d20},
				Sequence: &sdk.SequenceRef{Ref: "dnd5e:actions:multiattack", Name: "Multiattack"},
				Reaction: &oa, PresentationID: "presentation-1",
			},
			act(sessionpb.Family_FAMILY_REACTION, &sessionpb.AttackAct{
				By: gate1P, To: aliceP, Attack: &sessionpb.AttackRef{Ref: "dnd5e:weapons:spear", Name: "Spear", DamageType: sessionpb.DamageType_DAMAGE_TYPE_PIERCING},
				Outcome: sessionpb.AttackOutcome_ATTACK_OUTCOME_HIT, Critical: true, Damage: 7,
				DamageComponents: []*sessionpb.DamageComponent{{
					Source: "Spear", SourceRef: "dnd5e:weapons:spear", DamageType: sessionpb.DamageType_DAMAGE_TYPE_PIERCING, Multiplier: fp(0.5),
					Roll: &sessionpb.Roll{
						Expression: "2d6+2", Dice: []int32{2, 5}, Modifiers: []*sessionpb.RollModifier{{Source: "Strength", Value: 2}}, Total: 9,
						Terms: []*sessionpb.RollDiceTerm{{Count: 2, DieSize: 6, Faces: []int32{2, 5}, SourceName: "Strength"}},
					},
				}},
				Roll: d20P, Ward: &sessionpb.WardSave{Source: aliceP, Ability: "wis", Roll: d20P},
				Sequence: &sessionpb.SequenceRef{Ref: "dnd5e:actions:multiattack", Name: "Multiattack"},
				Reaction: oaP, PresentationId: "presentation-1",
			})},
		"cast": {sdk.FamilyAct,
			sdk.CastAct{By: alice, Spell: bless, Targets: []sdk.Party{gate1}, Results: []sdk.CastResult{
				{Target: gate1, Outcome: sdk.CastFailed, Ability: "cha", Save: d20, Source: alice},
			}},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.CastAct{By: aliceP, Spell: blessP, Targets: []*sessionpb.Party{gate1P}, Results: []*sessionpb.CastResult{
				{Target: gate1P, Outcome: sessionpb.CastOutcome_CAST_OUTCOME_FAILED, Ability: "cha", Save: d20P, Source: aliceP},
			}})},
		"activate": {sdk.FamilyAct,
			sdk.ActivateAct{By: alice, Ability: sdk.AbilityRef{Ref: "dnd5e:features:second-wind", Name: "Second Wind"}, Target: gate1,
				Results: []sdk.ActivationResult{{
					Kind: "healed", Target: alice, Ref: "r", Name: "n", SourceID: "s", Amount: 6, Requested: 7, Before: 3, After: 9,
					Roll: d20, DamageType: "fire", Detail: "d", Description: "desc", Moved: 10, StoppedBy: "wall",
					Stabilization: &sdk.Stabilization{Before: "dying", After: "stable", HitPoints: 0, Successes: 3, Failures: 1, SuccessesNeeded: 0, FailuresRemaining: 2, Stabilized: true},
				}},
				Saves: []sdk.SaveResult{{Saver: gate1, Ability: "con", Source: bless, Roll: *d20}},
			},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.ActivateAct{By: aliceP, Ability: &sessionpb.AbilityRef{Ref: "dnd5e:features:second-wind", Name: "Second Wind"}, Target: gate1P,
				Results: []*sessionpb.ActivationResult{{
					Kind: "healed", Target: aliceP, Ref: "r", Name: "n", SourceId: "s", Amount: 6, Requested: 7, Before: 3, After: 9,
					Roll: d20P, DamageType: sessionpb.DamageType_DAMAGE_TYPE_FIRE, Detail: "d", Description: "desc", Moved: 10, StoppedBy: "wall",
					Stabilization: &sessionpb.Stabilization{Before: "dying", After: "stable", Successes: 3, Failures: 1, FailuresRemaining: 2, Stabilized: true},
				}},
				Saves: []*sessionpb.SaveResult{{Saver: gate1P, Ability: "con", Source: blessP, Roll: d20P}},
			})},
		"speak": {sdk.FamilyAct,
			sdk.SpeakAct{By: alice, To: gate1, Option: "stand-aside", Line: "Stand aside.", Skill: "persuasion", Roll: d20, Answer: &ans},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.SpeakAct{By: aliceP, To: gate1P, Option: "stand-aside", Line: "Stand aside.", Skill: "persuasion", Roll: d20P, Answer: answerP()})},
		"death_save": {sdk.FamilyAct,
			sdk.DeathSaveAct{By: alice, Roll: d20, Outcome: "success", SuccessesAdded: 1, FailuresAdded: 0, Successes: 2, Failures: 1,
				SuccessesNeeded: 1, FailuresRemaining: 2, Stabilized: false, Dead: false, Recovered: true, HPRestored: 1, Continuation: "end_turn", PresentationID: "p"},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.DeathSaveAct{By: aliceP, Roll: d20P, Outcome: "success", SuccessesAdded: 1, Successes: 2, Failures: 1,
				SuccessesNeeded: 1, FailuresRemaining: 2, Recovered: true, HpRestored: 1, Continuation: "end_turn", PresentationId: "p"})},
		"loot": {sdk.FamilyAct, sdk.LootAct{By: alice, From: gate1},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.LootAct{By: aliceP, From: gate1P})},
		"hold": {sdk.FamilyAct, sdk.HoldAct{By: alice, Prop: chest},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.HoldAct{By: aliceP, Prop: chestP})},
		"drop": {sdk.FamilyAct, sdk.DropAct{By: alice, Prop: chest, Cell: cell},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.DropAct{By: aliceP, Prop: chestP, Cell: cellP})},
		"door": {sdk.FamilyAct, sdk.DoorAct{By: alice, Door: "gate", State: "unlocked", Roll: d20},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.DoorAct{By: aliceP, Door: "gate", State: "unlocked", Roll: d20P})},
		"equip": {sdk.FamilyAct, sdk.EquipAct{By: alice, Slot: "main_hand", Item: "longsword", Change: "draw"},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.EquipAct{By: aliceP, Slot: "main_hand", Item: "longsword", Change: "draw"})},
		"stay": {sdk.FamilyAct, sdk.StayAct{Member: gate1, Cause: "driven", Why: "no target"},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.StayAct{Member: gate1P, Cause: "driven", Why: "no target"})},
		"sneak": {sdk.FamilyAct, sdk.SneakAct{By: alice, Watcher: gate1, Roll: d20},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.SneakAct{By: aliceP, Watcher: gate1P, Roll: d20P})},
		"rest": {sdk.FamilyAct,
			sdk.RestAct{By: alice, Kind: "short", HitPointsRestored: 5, HitPoints: 12, HitDiceSpent: 1, HitDiceReturned: 0, HitDiceRemaining: 2,
				ResourcesRefilled: []string{"second-wind"}, Roll: d20},
			act(sessionpb.Family_FAMILY_ACT, &sessionpb.RestAct{By: aliceP, Kind: "short", HitPointsRestored: 5, HitPoints: 12, HitDiceSpent: 1, HitDiceRemaining: 2,
				ResourcesRefilled: []string{"second-wind"}, Roll: d20P})},
		"table_answer": {sdk.FamilyTableAnswer, ans, act(sessionpb.Family_FAMILY_TABLE_ANSWER, answerP())},
		"state": {sdk.FamilyState,
			sdk.StateChange{Party: alice, Change: sdk.StateExperience,
				Downed: &sdk.Downed{Kind: "creature"}, Stance: &sdk.StanceTurn{Between: []sdk.Party{guards, alice}, Stance: "hostile", Cause: "insult"},
				Concentration: &sdk.ConcentrationEnded{Spell: bless, Reason: "rest"},
				Condition:     &sdk.ConditionLifted{Ref: "dnd5e:conditions:blessed", Name: "Blessed", SourceID: "alice", Reason: "rest"},
				Experience:    []sdk.ExperienceGrant{{Character: "alice", Amount: 25, Total: 325}},
			},
			act(sessionpb.Family_FAMILY_STATE, &sessionpb.StateChange{Party: aliceP, Change: sessionpb.StateKind_STATE_KIND_EXPERIENCE,
				Downed: &sessionpb.Downed{Kind: "creature"}, Stance: &sessionpb.StanceTurn{Between: []*sessionpb.Party{guardsP, aliceP}, Stance: "hostile", Cause: "insult"},
				Concentration: &sessionpb.ConcentrationEnded{Spell: blessP, Reason: "rest"},
				Condition:     &sessionpb.ConditionLifted{Ref: "dnd5e:conditions:blessed", Name: "Blessed", SourceId: "alice", Reason: "rest"},
				Experience:    []*sessionpb.ExperienceGrant{{Character: "alice", Amount: 25, Total: 325}},
			})},
		"perception": {sdk.FamilyPerception, perceptionSDK(), act(sessionpb.Family_FAMILY_PERCEPTION, perceptionProto())},
		"clock": {sdk.FamilyClock,
			sdk.ClockEntry{Mark: sdk.ClockWindowOpened, Tick: 9,
				Turn:      &sdk.TurnPassed{Member: alice, Next: gate1},
				Fight:     &sdk.FightFormed{Order: []sdk.Party{alice, gate1}, Surprised: []sdk.Party{gate1}},
				Dissolved: &sdk.FightDissolved{Members: []sdk.Party{alice}, Cause: "fled"},
				Member:    alice, Departure: &sdk.Departure{Member: alice, Holding: []string{"crown"}, Exit: "postern"},
				Arrival: &sdk.Arrival{ID: "guard-3", Kind: "creature", Cell: cell}, Ending: "escaped",
				Window: &sdk.WindowOpened{Member: alice, From: cell, To: spatial.Position{X: 4, Y: -2},
					Windows: []sdk.WindowOffer{{Audience: gate1, Reaction: oa}}},
				RollWindow: &sdk.RollWindowOpened{PresentationID: "p", Audience: alice, Offer: oa, Roll: d20},
			},
			act(sessionpb.Family_FAMILY_CLOCK, &sessionpb.ClockEntry{Mark: sessionpb.ClockMark_CLOCK_MARK_WINDOW_OPENED, Tick: 9,
				Turn:      &sessionpb.TurnPassed{Member: aliceP, Next: gate1P},
				Fight:     &sessionpb.FightFormed{Order: []*sessionpb.Party{aliceP, gate1P}, Surprised: []*sessionpb.Party{gate1P}},
				Dissolved: &sessionpb.FightDissolved{Members: []*sessionpb.Party{aliceP}, Cause: "fled"},
				Member:    aliceP, Departure: &sessionpb.Departure{Member: aliceP, Holding: []string{"crown"}, Exit: "postern"},
				Arrival: &sessionpb.Arrival{Id: "guard-3", Kind: "creature", Cell: cellP}, Ending: "escaped",
				Window: &sessionpb.WindowOpened{Member: aliceP, From: cellP, To: &sessionpb.Position{X: 4, Y: -2},
					Windows: []*sessionpb.WindowOffer{{Audience: gate1P, Reaction: oaP}}},
				RollWindow: &sessionpb.RollWindowOpened{PresentationId: "p", Audience: aliceP, Offer: oaP, Roll: d20P},
			})},
	}
}

func perceptionSDK() sdk.Perception {
	region := sdk.AtlasRegion{ID: "gatehouse", Name: "Gatehouse", Cells: []spatial.Position{cell}, Archetype: "castle", Lighting: sdk.Lighting{Intensity: 0.5}}
	prop := sdk.AtlasProp{ID: "brazier", Holdable: true, Ref: "props:brazier", At: cell, BlocksMovement: true, BlocksLineOfSight: false, Facing: "north", Offset: [3]float64{0.1, -0.2, 0.3}}
	boundary := sdk.AtlasBoundary{From: cell, To: spatial.Position{X: 4, Y: -2}, BlocksMovement: true, BlocksLineOfSight: true, Height: 2}
	segment := sdk.AtlasSegment{From: sdk.AxialPointF{Q: 0.5, R: 1}, To: sdk.AxialPointF{Q: 1.5, R: 1}, Height: 1.5}
	doorway := sdk.AtlasDoorway{Door: "gate", From: cell, To: spatial.Position{X: 4, Y: -2}}
	opening := sdk.AtlasStructuralOpening{ID: "o1", Position: 2.5, Width: 4}
	wall := sdk.AtlasStructuralWall{ID: "w1", Ref: "walls:stone", From: sdk.FootprintPoint{X: 0, Y: 0}, To: sdk.FootprintPoint{X: 10, Y: 0},
		Height: 12, Thickness: 1, Elevation: 0.5, FootprintCells: []spatial.Position{cell}, Openings: []sdk.AtlasStructuralOpening{opening}}
	door := sdk.AtlasStructuralDoor{ID: "gate", Ref: "doors:oak", From: sdk.FootprintPoint{X: 1, Y: 0}, To: sdk.FootprintPoint{X: 5, Y: 0}, Height: 8, Thickness: 0.5, Elevation: 0}
	replace := sdk.StructuralWallOpeningsReplacement{WallID: "w1", Openings: []sdk.AtlasStructuralOpening{opening}}
	presentation := sdk.PropPresentation{ID: "brazier", Ref: "props:brazier", Origin: sdk.FootprintPoint{X: 2, Y: 3}, Elevation: 1, FacingDegrees: 90, HeightScale: 1.2,
		PointLight: &sdk.PropPointLight{Enabled: true, Offset: sdk.FootprintPoint{X: 0, Y: 1}, OffsetElevation: 2, Color: "#ffaa00", Intensity: 0.8, Range: 15},
		DoorID:     "gate", Label: "Brazier"}
	return sdk.Perception{
		Observer: alice, Sense: sdk.SenseCells,
		Sighted:   &sdk.SightChange{Gained: []string{"gate-1"}, Lost: []string{"gate-2"}, Changed: []string{"captain"}, KnowledgeChanged: true},
		Heard:     &sdk.Sound{Kind: "clang", Cell: cell, Hops: 3},
		Discovery: &sdk.DiscoveryCheck{Ability: "wis", Roll: d20},
		Revealed: &sdk.SecretRevealed{Concealment: "postern", Cells: []spatial.Position{cell}, Props: []sdk.AtlasProp{prop},
			Doors:   []sdk.RevealedDoor{{Door: "postern", State: "locked", Doorways: []sdk.AtlasDoorway{doorway}, Approaches: []sdk.DoorApproach{{Ability: "dex", Tool: "thieves-tools", DC: 15}}}},
			Regions: []sdk.AtlasRegion{region}, Boundaries: []sdk.AtlasBoundary{boundary}, Segments: []sdk.AtlasSegment{segment}, Sealed: []spatial.Position{cell},
			StructuralWalls: []sdk.AtlasStructuralWall{wall}, StructuralDoors: []sdk.AtlasStructuralDoor{door},
			StructuralWallOpeningsReplacements: []sdk.StructuralWallOpeningsReplacement{replace}, PropPresentations: []sdk.PropPresentation{presentation}},
		Cells: &sdk.CellsLearned{Regions: []sdk.AtlasRegion{region}, Scenery: []spatial.Position{cell}, Props: []sdk.AtlasProp{prop},
			Boundaries: []sdk.AtlasBoundary{boundary}, Segments: []sdk.AtlasSegment{segment}, Sealed: []spatial.Position{cell},
			Doorways: []sdk.AtlasDoorway{doorway},
			Placed: []sdk.AtlasPlacedProp{{ID: "table", Placement: sdk.FootprintPlacement{Width: 3, Depth: 6, Origin: sdk.FootprintPoint{X: 1, Y: 2}, Facing: 45, LocalOffset: sdk.FootprintPoint{X: 0.5, Y: 0}},
				BlocksMovement: true, BlocksLineOfSight: false, Holdable: false, Cells: []spatial.Position{cell}}},
			Exits:           []sdk.AtlasExit{{ID: "postern", At: cell}},
			StructuralWalls: []sdk.AtlasStructuralWall{wall}, StructuralDoors: []sdk.AtlasStructuralDoor{door},
			StructuralWallOpeningsReplacements: []sdk.StructuralWallOpeningsReplacement{replace}, PropPresentations: []sdk.PropPresentation{presentation}},
	}
}

func perceptionProto() *sessionpb.Perception {
	cells := []*sessionpb.Position{cellP}
	region := &sessionpb.AtlasRegion{Id: "gatehouse", Name: "Gatehouse", Cells: cells, Archetype: "castle", Lighting: &sessionpb.Lighting{Intensity: 0.5}}
	prop := &sessionpb.AtlasProp{Id: "brazier", Holdable: true, Ref: "props:brazier", At: cellP, BlocksMovement: true, Facing: "north", OffsetX: 0.1, OffsetY: -0.2, OffsetZ: 0.3}
	boundary := &sessionpb.AtlasBoundary{From: cellP, To: &sessionpb.Position{X: 4, Y: -2}, BlocksMovement: true, BlocksLineOfSight: true, Height: 2}
	segment := &sessionpb.AtlasSegment{From: &sessionpb.AxialPoint{Q: 0.5, R: 1}, To: &sessionpb.AxialPoint{Q: 1.5, R: 1}, Height: 1.5}
	doorway := &sessionpb.AtlasDoorway{Door: "gate", From: cellP, To: &sessionpb.Position{X: 4, Y: -2}}
	opening := &sessionpb.AtlasStructuralOpening{Id: "o1", Position: 2.5, Width: 4}
	wall := &sessionpb.AtlasStructuralWall{Id: "w1", Ref: "walls:stone", From: &sessionpb.FootprintPoint{}, To: &sessionpb.FootprintPoint{X: 10},
		Height: 12, Thickness: 1, Elevation: 0.5, FootprintCells: cells, Openings: []*sessionpb.AtlasStructuralOpening{opening}}
	door := &sessionpb.AtlasStructuralDoor{Id: "gate", Ref: "doors:oak", From: &sessionpb.FootprintPoint{X: 1}, To: &sessionpb.FootprintPoint{X: 5}, Height: 8, Thickness: 0.5}
	replace := &sessionpb.StructuralWallOpeningsReplacement{WallId: "w1", Openings: []*sessionpb.AtlasStructuralOpening{opening}}
	presentation := &sessionpb.PropPresentation{Id: "brazier", Ref: "props:brazier", Origin: &sessionpb.FootprintPoint{X: 2, Y: 3}, Elevation: 1, FacingDegrees: 90, HeightScale: 1.2,
		PointLight: &sessionpb.PropPointLight{Enabled: true, Offset: &sessionpb.FootprintPoint{Y: 1}, OffsetElevation: 2, Color: "#ffaa00", Intensity: 0.8, Range: 15},
		DoorId:     "gate", Label: "Brazier"}
	return &sessionpb.Perception{
		Observer: aliceP, Sense: sessionpb.Sense_SENSE_CELLS,
		Sighted:   &sessionpb.SightChange{Gained: []string{"gate-1"}, Lost: []string{"gate-2"}, Changed: []string{"captain"}, KnowledgeChanged: true},
		Heard:     &sessionpb.Sound{Kind: "clang", Cell: cellP, Hops: 3},
		Discovery: &sessionpb.DiscoveryCheck{Ability: "wis", Roll: d20P},
		Revealed: &sessionpb.SecretRevealed{Concealment: "postern", Cells: cells, Props: []*sessionpb.AtlasProp{prop},
			Doors: []*sessionpb.RevealedDoor{{Door: "postern", State: "locked", Doorways: []*sessionpb.AtlasDoorway{doorway},
				Approaches: []*sessionpb.DoorApproach{{Ability: "dex", Tool: "thieves-tools", Dc: 15}}}},
			Regions: []*sessionpb.AtlasRegion{region}, Boundaries: []*sessionpb.AtlasBoundary{boundary}, Segments: []*sessionpb.AtlasSegment{segment}, Sealed: cells,
			StructuralWalls: []*sessionpb.AtlasStructuralWall{wall}, StructuralDoors: []*sessionpb.AtlasStructuralDoor{door},
			StructuralWallOpeningsReplacements: []*sessionpb.StructuralWallOpeningsReplacement{replace}, PropPresentations: []*sessionpb.PropPresentation{presentation}},
		Cells: &sessionpb.CellsLearned{Regions: []*sessionpb.AtlasRegion{region}, Scenery: cells, Props: []*sessionpb.AtlasProp{prop},
			Boundaries: []*sessionpb.AtlasBoundary{boundary}, Segments: []*sessionpb.AtlasSegment{segment}, Sealed: cells,
			Doorways: []*sessionpb.AtlasDoorway{doorway},
			Placed: []*sessionpb.AtlasPlacedProp{{Id: "table", Placement: &sessionpb.FootprintPlacement{Width: 3, Depth: 6, Origin: &sessionpb.FootprintPoint{X: 1, Y: 2}, Facing: 45, LocalOffset: &sessionpb.FootprintPoint{X: 0.5}},
				BlocksMovement: true, Cells: cells}},
			Exits:           []*sessionpb.AtlasExit{{Id: "postern", At: cellP}},
			StructuralWalls: []*sessionpb.AtlasStructuralWall{wall}, StructuralDoors: []*sessionpb.AtlasStructuralDoor{door},
			StructuralWallOpeningsReplacements: []*sessionpb.StructuralWallOpeningsReplacement{replace}, PropPresentations: []*sessionpb.PropPresentation{presentation}},
	}
}

// TestConvertEveryBody: every SDK body spells every one of its fields on the
// wire — a reaction's ReactionRef and a perception's geometry included — and
// the table covers every member of Act's body oneof, so a body added to the
// contract without a case here fails.
func (s *ConvertActSuite) TestConvertEveryBody() {
	cases := everyBody()
	oneof := (&sessionpb.Act{}).ProtoReflect().Descriptor().Oneofs().ByName("body")
	s.Require().NotNil(oneof)
	for i := 0; i < oneof.Fields().Len(); i++ {
		name := string(oneof.Fields().Get(i).Name())
		s.Contains(cases, name, "the body %q has no conversion case", name)
	}
	s.Len(cases, oneof.Fields().Len(), "a case names a body the contract does not have")

	for name, c := range cases {
		s.Run(name, func() {
			got, err := actToProto(sdk.Act{
				Session: "sess-1", ID: "id-9", At: 4, Act: "attack:id-1", Verb: "attack", Recipient: "alice",
				Segment: 1, Family: c.family, Body: c.body,
			})
			s.Require().NoError(err)
			s.True(proto.Equal(c.want, got), "want %v\n got %v", c.want, got)
		})
	}
}

// TestConvertAbsentPartiesStayAbsent: a zero party is an absent field, not a
// party with no kind.
func (s *ConvertActSuite) TestConvertAbsentPartiesStayAbsent() {
	got, err := actToProto(sdk.Act{ID: "id-1", Family: sdk.FamilyAct, Body: sdk.DoorAct{Door: "gate", State: "open"}})
	s.Require().NoError(err)
	s.Nil(got.GetDoor().GetBy())
	s.Nil(got.GetDoor().GetRoll())
}

// TestConvertUnknownValueRefuses: a closed-set value this build cannot spell
// refuses its act — id kept, no body — rather than sending a zero enum that
// would read as a real answer.
func (s *ConvertActSuite) TestConvertUnknownValueRefuses() {
	got, err := actToProto(sdk.Act{ID: "id-1", Act: "attack:id-1", Family: sdk.FamilyAct,
		Body: sdk.AttackAct{By: alice, To: gate1, Outcome: "grazed"}})
	s.Require().ErrorIs(err, errUnknownValue)
	s.True(got.GetRefused())
	s.Equal("id-1", got.GetId())
	s.Contains(got.GetRefusal(), "grazed")
	s.Nil(got.GetBody())

	got, err = actToProto(sdk.Act{ID: "id-2", Family: "roster", Body: sdk.ClockEntry{Mark: sdk.ClockTick}})
	s.Require().ErrorIs(err, errUnknownValue)
	s.True(got.GetRefused())
	s.Equal("id-2", got.GetId())
}

// TestConvertLegacyDamageComponent: a damage component persisted before roll
// traces reads its scalars into the same Roll.
func (s *ConvertActSuite) TestConvertLegacyDamageComponent() {
	got := damageRollToProto(sdk.DamageComponent{Source: "Longsword", Dice: "1d8", FinalRolls: []int{6}, FlatBonus: 3})
	s.True(proto.Equal(&sessionpb.Roll{
		Expression: "1d8+3", Dice: []int32{6}, Modifiers: []*sessionpb.RollModifier{{Source: "Longsword", Value: 3}}, Total: 9,
	}, got), "%v", got)
}

// TestConvertTracedDamageCarriesItsSource is rpg-project#643 A: a traced
// damage component's roll says which source rolled it. Divine Favor's 1d4
// rides a weapon swing; its term names the spell.
func (s *ConvertActSuite) TestConvertTracedDamageCarriesItsSource() {
	got := damageRollToProto(sdk.DamageComponent{
		Source: "spell", DamageType: sdk.DamageRadiant,
		Roll: sdk.RollComponent{
			Source: sdk.RollSource{Ref: "dnd5e:spells:divine-favor", Name: "Divine Favor", SourceID: "cleric"},
			Dice:   &sdk.DiceTrace{Notation: "1d4", DieSize: 4, OriginalRolls: []int{3}, FinalRolls: []int{3}, Subtotal: 3},
		},
	})
	s.True(proto.Equal(&sessionpb.Roll{
		Expression: "1d4", Dice: []int32{3}, Total: 3,
		Terms: []*sessionpb.RollDiceTerm{{
			Count: 1, DieSize: 4, Faces: []int32{3}, Rolled: []int32{3},
			SourceRef: "dnd5e:spells:divine-favor", SourceName: "Divine Favor", SourceId: "cleric",
		}},
	}, got), "%v", got)
}

// TestConvertDamageRollCarriesTheGWFReroll is rpg-project#643 B and D on the
// damage path: a Great Weapon Fighting greatsword's 2d6 first rolled [1, 5],
// die 0 rerolled 1 -> 4 by the fighting style, and the Strength modifier
// names its ref and label. The v1 Struck beat carried all of it.
func (s *ConvertActSuite) TestConvertDamageRollCarriesTheGWFReroll() {
	weapon := damageRollToProto(sdk.DamageComponent{
		Source: "weapon", DamageType: sdk.DamageSlashing,
		Roll: sdk.RollComponent{
			Source: sdk.RollSource{Ref: "dnd5e:weapons:greatsword", Name: "Greatsword"},
			Dice: &sdk.DiceTrace{
				Notation: "2d6", DieSize: 6, OriginalRolls: []int{1, 5}, FinalRolls: []int{4, 5}, Subtotal: 9,
				Rerolls: []sdk.DiceReroll{{
					DieIndex: 0, Before: 1, After: 4,
					Source: sdk.RollSource{Ref: "dnd5e:conditions:fighting_style_great_weapon_fighting", Name: "Great Weapon Fighting"},
				}},
			},
		},
	})
	s.Require().Len(weapon.GetTerms(), 1)
	term := weapon.GetTerms()[0]
	s.Equal([]int32{4, 5}, term.GetFaces(), "the faces that counted")
	s.Equal([]int32{1, 5}, term.GetRolled(), "the pool as first rolled")
	s.Require().Len(term.GetRerolls(), 1)
	reroll := term.GetRerolls()[0]
	s.Equal(int32(0), reroll.GetDieIndex())
	s.Equal(int32(1), reroll.GetBefore())
	s.Equal(int32(4), reroll.GetAfter())
	s.Equal("dnd5e:conditions:fighting_style_great_weapon_fighting", reroll.GetSourceRef())
	s.Equal("Great Weapon Fighting", reroll.GetSourceName())
	s.Equal(int32(9), weapon.GetTotal())

	level := damageRollToProto(sdk.DamageComponent{
		Source: "ability", DamageType: sdk.DamageSlashing,
		Roll: sdk.RollComponent{
			Source:   sdk.RollSource{Ref: "dnd5e:classes:fighter", Name: "Fighter", Label: "Fighter level", SourceID: "alice"},
			Modifier: ip(1),
		},
	})
	s.Require().Len(level.GetModifiers(), 1)
	mod := level.GetModifiers()[0]
	s.Equal("Fighter", mod.GetSource())
	s.Equal("dnd5e:classes:fighter", mod.GetSourceRef())
	s.Equal("Fighter level", mod.GetSourceLabel())
	s.Equal("alice", mod.GetSourceId())
	s.Equal(int32(1), mod.GetValue())
}

// TestConvertDamageComponentSourceRef is rpg-project#643 C: a traced
// component's source_ref is its roll's source, not only the legacy scalar.
// A Great Weapon Fighting greatsword and its Strength modifier each name
// their source; the kept faces are the rerolled ones.
func (s *ConvertActSuite) TestConvertDamageComponentSourceRef() {
	out, err := attackToProto(sdk.AttackAct{
		Attack:  sdk.AttackRef{Ref: "dnd5e:weapons:greatsword", Name: "Greatsword", DamageType: sdk.DamageSlashing},
		Outcome: sdk.AttackHit, Damage: 12,
		DamageComponents: []sdk.DamageComponent{
			{Source: "weapon", DamageType: sdk.DamageSlashing, Roll: sdk.RollComponent{
				Source: sdk.RollSource{Ref: "dnd5e:weapons:greatsword", Name: "Greatsword"},
				Dice:   &sdk.DiceTrace{Notation: "2d6", DieSize: 6, OriginalRolls: []int{1, 5}, FinalRolls: []int{4, 5}, Subtotal: 9},
			}},
			{Source: "ability", DamageType: sdk.DamageSlashing, Roll: sdk.RollComponent{
				Source: sdk.RollSource{Ref: "dnd5e:abilities:str", Name: "Strength"}, Modifier: ip(3),
			}},
		},
	})
	s.Require().NoError(err)
	s.Require().Len(out.GetDamageComponents(), 2)
	weapon, ability := out.GetDamageComponents()[0], out.GetDamageComponents()[1]
	s.Equal("dnd5e:weapons:greatsword", weapon.GetSourceRef())
	s.Equal("dnd5e:abilities:str", ability.GetSourceRef())
	s.Require().Len(weapon.GetRoll().GetTerms(), 1)
	s.Equal([]int32{4, 5}, weapon.GetRoll().GetTerms()[0].GetFaces())
	s.Equal("Greatsword", weapon.GetRoll().GetTerms()[0].GetSourceName())
	s.Empty(ability.GetRoll().GetTerms(), "a flat modifier rolls no dice")

	// A legacy component keeps its own scalar.
	legacy, err := attackToProto(sdk.AttackAct{
		Attack: sdk.AttackRef{Ref: "dnd5e:weapons:spear"}, Outcome: sdk.AttackHit,
		DamageComponents: []sdk.DamageComponent{{Source: "Spear", SourceRef: "dnd5e:weapons:spear", DamageType: sdk.DamagePiercing}},
	})
	s.Require().NoError(err)
	s.Equal("dnd5e:weapons:spear", legacy.GetDamageComponents()[0].GetSourceRef())
}

// TestConvertUnknownResultDamageRefuses: an activation result naming a damage
// type this build cannot spell refuses its act rather than reading as none.
func (s *ConvertActSuite) TestConvertUnknownResultDamageRefuses() {
	got, err := actToProto(sdk.Act{ID: "id-3", Family: sdk.FamilyAct, Body: sdk.ActivateAct{
		By: alice, Results: []sdk.ActivationResult{{Kind: "damaged", DamageType: "sonic"}},
	}})
	s.Require().ErrorIs(err, errUnknownValue)
	s.True(got.GetRefused())
	s.Equal("id-3", got.GetId())
}

// TestConvertUnknownInfluenceRefuses: a roll's influence of a kind this build
// cannot spell refuses its act — the card never silently loses a source.
func (s *ConvertActSuite) TestConvertUnknownInfluenceRefuses() {
	got, err := actToProto(sdk.Act{ID: "id-4", Family: sdk.FamilyAct, Body: sdk.SneakAct{
		By: alice, Roll: &sdk.Roll{Total: 12, Influences: []sdk.RollInfluence{{Kind: "inspired", SourceRef: "x"}}},
	}})
	s.Require().ErrorIs(err, errUnknownValue)
	s.True(got.GetRefused())
	s.Equal("id-4", got.GetId())
}

// TestRollToProtoCarriesTheRerollRecord is rpg-project#643 B on the wire: a
// Great Weapon Fighting damage term keeps its pool as first rolled, its kept
// indices and its sourced reroll. Mutant: omitting rerolls fails it.
func (s *ConvertActSuite) TestRollToProtoCarriesTheRerollRecord() {
	got, err := rollToProto(&sdk.Roll{
		Expression: "2d6", Dice: []int{4, 5}, Total: 9,
		Terms: []sdk.RollDiceTerm{{
			Count: 2, DieSize: 6, Faces: []int{4, 5},
			SourceRef: "dnd5e:weapons:greatsword", SourceName: "Greatsword", SourceID: "fighter", SourceLabel: "weapon",
			Rolled: []int{1, 5}, KeptIndices: []int{0, 1},
			Rerolls: []sdk.RollReroll{{
				DieIndex: 0, Before: 1, After: 4,
				SourceRef: "dnd5e:fighting-styles:great-weapon-fighting", SourceName: "Great Weapon Fighting", SourceID: "fighter",
				SourceLabel: "fighting style",
			}},
		}},
	})
	s.Require().NoError(err)
	s.Require().Len(got.GetTerms(), 1)
	term := got.GetTerms()[0]
	s.Equal([]int32{1, 5}, term.GetRolled())
	s.Equal([]int32{0, 1}, term.GetKeptIndices())
	s.Equal([]int32{4, 5}, term.GetFaces())
	s.Equal("weapon", term.GetSourceLabel())
	s.Require().Len(term.GetRerolls(), 1)
	s.True(proto.Equal(&sessionpb.RollReroll{
		DieIndex: 0, Before: 1, After: 4,
		SourceRef: "dnd5e:fighting-styles:great-weapon-fighting", SourceName: "Great Weapon Fighting", SourceId: "fighter",
		SourceLabel: "fighting style",
	}, term.GetRerolls()[0]), "%v", term.GetRerolls()[0])
}

// TestTraceTermToProtoCarriesTheRerollRecord is the damage path's half of
// rpg-project#643 B: a Great Weapon Fighting greatsword component's raw
// trace spells the same term a folded roll does, with the pool as first
// rolled, its kept faces and the sourced reroll. Mutant: dropping the trace's
// rerolls fails it.
func (s *ConvertActSuite) TestTraceTermToProtoCarriesTheRerollRecord() {
	gwf := sdk.RollSource{Ref: "dnd5e:conditions:fighting_style_great_weapon_fighting", Name: "Great Weapon Fighting", SourceID: "fighter"}
	got := traceTermToProto(sdk.RollComponent{
		Source: sdk.RollSource{Ref: "dnd5e:weapons:greatsword", Name: "Greatsword", SourceID: "fighter"},
		Dice: &sdk.DiceTrace{
			Notation: "2d6", DieSize: 6, OriginalRolls: []int{1, 5}, FinalRolls: []int{4, 5}, Subtotal: 9,
			Rerolls: []sdk.DiceReroll{{DieIndex: 0, Before: 1, After: 4, Source: gwf}},
		},
	})
	s.True(proto.Equal(&sessionpb.RollDiceTerm{
		Count: 2, DieSize: 6, Faces: []int32{4, 5}, Rolled: []int32{1, 5},
		SourceRef: "dnd5e:weapons:greatsword", SourceName: "Greatsword", SourceId: "fighter",
		Rerolls: []*sessionpb.RollReroll{{
			DieIndex: 0, Before: 1, After: 4,
			SourceRef: "dnd5e:conditions:fighting_style_great_weapon_fighting", SourceName: "Great Weapon Fighting", SourceId: "fighter",
		}},
	}, got), "%v", got)
	s.Nil(traceTermToProto(sdk.RollComponent{Source: gwf}), "a component that rolled no dice has no term")
}

// TestSourceModifierToProtoCarriesItsRef is the damage path's half of
// rpg-project#643 D: a traced component's flat contribution names its source
// whole. Mutant: dropping the ref fails it.
func (s *ConvertActSuite) TestSourceModifierToProtoCarriesItsRef() {
	got := sourceModifierToProto(sdk.RollSource{Ref: "dnd5e:abilities:str", Name: "Strength", Label: "ability", SourceID: "fighter"}, 3)
	s.True(proto.Equal(&sessionpb.RollModifier{
		Source: "Strength", Value: 3, SourceRef: "dnd5e:abilities:str", SourceId: "fighter", SourceLabel: "ability",
	}, got), "%v", got)
}

// TestRollModifierToProtoCarriesItsRef is rpg-project#643 D on the wire:
// Second Wind's modifier names its source by ref, id and label, beside its
// name; an influence carries its label. Mutant: omitting source_ref fails it.
func (s *ConvertActSuite) TestRollModifierToProtoCarriesItsRef() {
	got, err := rollToProto(&sdk.Roll{
		Expression: "1d10+1", Total: 7,
		Modifiers: []sdk.RollModifier{{
			Source: "Fighter", Value: 1,
			SourceRef: "dnd5e:classes:fighter", SourceID: "fighter", SourceLabel: "Fighter level",
		}},
		Influences: []sdk.RollInfluence{{Kind: sdk.InfluenceAdvantage, SourceRef: "dnd5e:conditions:helped", Name: "Helped", SourceLabel: "help"}},
	})
	s.Require().NoError(err)
	s.Require().Len(got.GetModifiers(), 1)
	s.True(proto.Equal(&sessionpb.RollModifier{
		Source: "Fighter", Value: 1,
		SourceRef: "dnd5e:classes:fighter", SourceId: "fighter", SourceLabel: "Fighter level",
	}, got.GetModifiers()[0]), "%v", got.GetModifiers()[0])
	s.Require().Len(got.GetInfluences(), 1)
	s.Equal("help", got.GetInfluences()[0].GetSourceLabel())
}
