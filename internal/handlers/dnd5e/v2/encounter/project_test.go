package encounter_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
)

// ProjectSuite covers ProjectFor's per-viewer projection of toolkit
// encounter.Data into a v1alpha2 proto Encounter. Wave 2.8 added Mode +
// TurnState + monster entities — previously ProjectFor hardcoded
// FREE_ROAM and only emitted player entities.
type ProjectSuite struct {
	suite.Suite
	now          time.Time
	broker       *tkenc.Broker
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
}

func (s *ProjectSuite) SetupTest() {
	s.now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
}

func (s *ProjectSuite) TearDownTest() {
	s.ctrl.Finish()
}

// originHex is the cube origin (0,0,0); used everywhere viewers stand at
// the center of their sight radius for fixture simplicity.
var originHex = core.Hex{Q: 0, R: 0, S: 0}

// newPlayerData builds a minimal *tkenc.PlayerData for a viewer standing at
// pos with the given sight range. RevealedHexes is seeded to whatever the
// stub LoS at sightRange returns from pos.
func newPlayerData(pid core.PlayerID, eid core.EntityID, pos core.Hex, sightRange int) *tkenc.PlayerData {
	view := perception.NewView(pid, pos, sightRange)
	// nil room: these fixtures never call InitRoom, matching the pre-wave-1
	// pure-radius LoS behavior (perception.VisibleHexesAt's documented nil-room
	// fallback).
	view.ApplyReveal(perception.VisibleHexesAt(pos, sightRange, nil))
	return &tkenc.PlayerData{
		ID:       pid,
		EntityID: eid,
		View:     view,
	}
}

func (s *ProjectSuite) TestProjectFor_TurnBased_EmitsModeTurnStateAndMonsters() {
	// Build a turn-based encounter with two players (alice+bob) and one
	// monster (goblin-1) standing within alice's LOS. Initiative is
	// [alice, bob, goblin-1] with alice currently active on round 1.
	data := tkenc.NewData("enc-turnbased")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"char-alice", "char-bob", "goblin-1"}

	// Alice at origin with sight 3; Bob right next door so he's visible too.
	alice := newPlayerData("player-alice", "char-alice", originHex, 3)
	bob := newPlayerData("player-bob", "char-bob", core.Hex{Q: 1, R: -1, S: 0}, 3)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{
		"player-alice": alice,
		"player-bob":   bob,
	}

	// Goblin within alice's sight (distance 2 from origin).
	data.Monsters = map[core.EntityID]*tkenc.MonsterData{
		"goblin-1": {
			ID:         "goblin-1",
			Position:   core.Hex{Q: 2, R: -2, S: 0},
			HP:         7,
			MaxHP:      7,
			AC:         15,
			Speed:      30,
			MonsterRef: "dnd5e:monsters:goblin",
		},
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	s.Require().NotNil(pb)
	s.Require().Equal("enc-turnbased", pb.GetId())

	// Mode round-trips to TURN_BASED.
	s.Require().Equal(
		encounterv2pb.EncounterMode_ENCOUNTER_MODE_TURN_BASED,
		pb.GetMode(),
		"data.Mode == ModeTurnBased must project to ENCOUNTER_MODE_TURN_BASED",
	)

	// TurnState carries initiative, active entity, and round number.
	ts := pb.GetTurnState()
	s.Require().NotNil(ts, "TurnState must be present in TURN_BASED mode")
	s.Require().Equal([]string{"char-alice", "char-bob", "goblin-1"}, ts.GetInitiativeOrder())
	s.Require().Equal("char-alice", ts.GetActiveEntityId())
	s.Require().Equal(int32(1), ts.GetRound())
	// Economy + AvailableActions are out-of-scope server-only state for this PR.
	s.Require().Nil(ts.GetEconomy(), "ActionEconomy is server-only state for this PR")
	s.Require().Empty(ts.GetAvailableActions(), "AvailableActions deferred to follow-up")

	// Space.Entities contains alice (self) + bob (visible player) + goblin-1
	// (visible monster). Order: players first (player-id sort), then monsters
	// (entity-id sort).
	ents := pb.GetSpace().GetEntities()
	s.Require().Len(ents, 3)

	byID := make(map[string]*encounterv2pb.Entity, 3)
	for _, e := range ents {
		byID[e.GetId()] = e
	}

	// Players were emitted as before — viewer always sees self.
	s.Require().NotNil(byID["char-alice"], "viewer must always see their own entity")
	s.Require().NotNil(byID["char-bob"], "bob is in alice's sight range and must be emitted")

	// Monster is the new wire-shape: TYPE_MONSTER + HP + MonsterData oneof.
	gob := byID["goblin-1"]
	s.Require().NotNil(gob, "goblin-1 is within alice's LOS and must be emitted")
	s.Require().Equal(encounterv2pb.EntityType_ENTITY_TYPE_MONSTER, gob.GetType())
	s.Require().NotNil(gob.GetHp())
	s.Require().Equal(int32(7), gob.GetHp().GetCurrent())
	s.Require().Equal(int32(7), gob.GetHp().GetMax())
	s.Require().NotNil(gob.GetPosition())
	s.Require().Equal(int32(2), gob.GetPosition().GetX())
	s.Require().Equal(int32(-2), gob.GetPosition().GetY())
	s.Require().Equal(int32(0), gob.GetPosition().GetZ())
	md := gob.GetMonster()
	s.Require().NotNil(md, "MonsterData oneof must be set on monster entities")
	ref := md.GetMonsterRef()
	s.Require().NotNil(ref)
	s.Require().Equal("dnd5e", ref.GetModule())
	s.Require().Equal("monsters", ref.GetType())
	s.Require().Equal("goblin", ref.GetId())

	// ArmorClass is populated from MonsterData.AC (#562).
	s.Require().NotNil(gob.ArmorClass, "monster entity must carry armor_class when MonsterData.AC > 0")
	s.Require().Equal(int32(15), gob.GetArmorClass(), "armor_class must match MonsterData.AC")
}

func (s *ProjectSuite) TestProjectFor_TurnBased_SkipsMonsterOutsideLOS() {
	// Goblin parked far outside alice's sight — must NOT appear on the wire.
	data := tkenc.NewData("enc-los")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"char-alice", "goblin-far"}

	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}
	data.Monsters = map[core.EntityID]*tkenc.MonsterData{
		"goblin-far": {
			ID: "goblin-far", Position: core.Hex{Q: 10, R: -10, S: 0},
			HP: 5, MaxHP: 5, MonsterRef: "dnd5e:monsters:goblin",
		},
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	for _, e := range pb.GetSpace().GetEntities() {
		s.Require().NotEqual("goblin-far", e.GetId(), "out-of-sight monster must not be emitted")
	}
}

func (s *ProjectSuite) TestProjectFor_FreeRoam_NoTurnState() {
	// FREE_ROAM mode: TurnState must be nil even when initiative-shaped fields
	// happen to be set (defensive — the caller should not have set them, but
	// we don't want to leak server state in the wrong mode).
	data := tkenc.NewData("enc-freeroam")
	data.Mode = core.ModeFreeRoam

	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	s.Require().Equal(
		encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
		pb.GetMode(),
	)
	s.Require().Nil(pb.GetTurnState(), "TurnState must be omitted outside TURN_BASED mode")
}

// TestProjectFor_ModeEnded_NoTurnStateAndOmitsRemovedMonsters verifies the
// Wave 2.10 snapshot replay contract for terminal-state encounters: a player
// who reconnects after the encounter ended must see (a) no TurnState (so the
// UI doesn't render "your turn" / "alice acting" stale prompts), and (b) no
// trace of monsters removed by the kill chain (the toolkit's killEntity
// deletes them from data.Monsters before publishing EntityRemoved). The
// dedicated EncounterEndedEvent — replayed via the broker's per-stream queue
// — carries the "this is over" wire signal; the snapshot's Mode field is
// mapped to FREE_ROAM since the proto has no _ENDED variant.
func (s *ProjectSuite) TestProjectFor_ModeEnded_NoTurnStateAndOmitsRemovedMonsters() {
	data := tkenc.NewData("enc-ended")
	data.Mode = core.ModeEnded
	// Surviving fields that ModeEnded clears in the toolkit kill path:
	// Initiative, ActiveIdx, Round are reset by checkEncounterEnd. Mirror
	// that here so the fixture reflects post-end persisted state.
	data.Initiative = nil
	data.ActiveIdx = 0
	data.Round = 0

	alice := newPlayerData("player-alice", "char-alice", originHex, 5)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}
	// data.Monsters intentionally empty — killEntity removed the last hostile
	// before flipping mode to ModeEnded. Snapshot must not synthesize one.

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)

	// Mode maps to FREE_ROAM because the proto has no _ENDED variant; the
	// dedicated EncounterEndedEvent is the canonical wire signal that the
	// encounter has terminated.
	s.Require().Equal(
		encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
		pb.GetMode(),
		"ModeEnded must map to FREE_ROAM (proto has no _ENDED variant); EncounterEndedEvent carries the terminal-state signal",
	)
	// No TurnState — buildTurnState gates on ModeTurnBased and ModeEnded
	// is neither, so the UI doesn't render stale "your turn" prompts.
	s.Require().Nil(pb.GetTurnState(), "TurnState must be omitted post-end")
	// Space remains populated for the viewer's revealed hexes + their own
	// entity, but no monster entities should appear (data.Monsters is empty).
	s.Require().NotNil(pb.GetSpace())
	for _, e := range pb.GetSpace().GetEntities() {
		s.Require().NotEqual(
			encounterv2pb.EntityType_ENTITY_TYPE_MONSTER, e.GetType(),
			"removed monster %q must not appear in post-end snapshot", e.GetId(),
		)
	}
}

func (s *ProjectSuite) TestProjectFor_Unspecified_TreatedAsFreeRoam() {
	// Toolkit doc convention: ModeUnspecified is treated as ModeFreeRoam. Make
	// sure ProjectFor reflects that on the wire so late-joining clients don't
	// see ENCOUNTER_MODE_UNSPECIFIED.
	data := tkenc.NewData("enc-unspec")
	// data.Mode left as zero value (ModeUnspecified)

	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	s.Require().Equal(
		encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
		pb.GetMode(),
	)
	s.Require().Nil(pb.GetTurnState())
}

func (s *ProjectSuite) TestProjectFor_TurnBased_ActiveIdxOutOfRange_NoActive() {
	// Defensive: a corrupt ActiveIdx must not panic the projection. Field
	// should be empty when the index points outside the initiative slice.
	data := tkenc.NewData("enc-bad-active")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 99 // out of range
	data.Initiative = []core.EntityID{"char-alice"}

	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	ts := pb.GetTurnState()
	s.Require().NotNil(ts)
	s.Require().Equal("", ts.GetActiveEntityId(), "out-of-range ActiveIdx -> empty active id")
}

func (s *ProjectSuite) TestProjectFor_MonsterRef_MalformedFallsBackToDefaults() {
	// A bare-string MonsterRef (not a fully-qualified module:type:id) must
	// not blow up the projection. We mirror conditionRefFor's fallback in
	// translate.go — default module=dnd5e, type=monster, id=raw — so the
	// parsing contract is consistent across the v2 encounter wire.
	data := tkenc.NewData("enc-bad-ref")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"char-alice", "weird-1"}

	alice := newPlayerData("player-alice", "char-alice", originHex, 3)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}
	data.Monsters = map[core.EntityID]*tkenc.MonsterData{
		"weird-1": {
			ID: "weird-1", Position: core.Hex{Q: 1, R: -1, S: 0},
			HP: 3, MaxHP: 3, MonsterRef: "no-colons-here",
		},
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	var found bool
	for _, e := range pb.GetSpace().GetEntities() {
		if e.GetId() != "weird-1" {
			continue
		}
		found = true
		ref := e.GetMonster().GetMonsterRef()
		s.Require().NotNil(ref)
		s.Require().Equal("dnd5e", ref.GetModule())
		s.Require().Equal("monster", ref.GetType())
		s.Require().Equal("no-colons-here", ref.GetId())
	}
	s.Require().True(found, "weird-1 must be projected (in alice's LOS)")
}

func (s *ProjectSuite) TestProjectFor_PlayerEntitiesCarryTypeHpAndCharacterData() {
	// Symmetric to monster emission: both the viewer's own entity and visible
	// other-player entities must carry Type=CHARACTER, HP, and CharacterData
	// with PlayerId populated. Previously these emit paths produced bare
	// {id, position} entities which rendered as UNSPECIFIED with no HP on the
	// client (issue #511).
	data := tkenc.NewData("enc-players")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"char-alice", "char-bob"}

	alice := newPlayerData("player-alice", "char-alice", originHex, 3)
	alice.HP = 12
	alice.MaxHP = 15

	bob := newPlayerData("player-bob", "char-bob", core.Hex{Q: 1, R: -1, S: 0}, 3)
	bob.HP = 8
	bob.MaxHP = 10

	data.Players = map[core.PlayerID]*tkenc.PlayerData{
		"player-alice": alice,
		"player-bob":   bob,
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)

	byID := make(map[string]*encounterv2pb.Entity)
	for _, e := range pb.GetSpace().GetEntities() {
		byID[e.GetId()] = e
	}

	// Viewer's own entity (alice).
	a := byID["char-alice"]
	s.Require().NotNil(a, "viewer's own entity must be emitted")
	s.Require().Equal(
		encounterv2pb.EntityType_ENTITY_TYPE_CHARACTER, a.GetType(),
		"viewer's own entity must carry ENTITY_TYPE_CHARACTER",
	)
	s.Require().NotNil(a.GetHp(), "viewer's own entity must carry HP")
	s.Require().Equal(int32(12), a.GetHp().GetCurrent())
	s.Require().Equal(int32(15), a.GetHp().GetMax())
	ac := a.GetCharacter()
	s.Require().NotNil(ac, "viewer's own entity must carry CharacterData oneof")
	s.Require().Equal("player-alice", ac.GetPlayerId(),
		"CharacterData.PlayerId must equal the toolkit PlayerData.ID")
	// This test passes a nil charRepo (see the ProjectFor call below), so
	// characterIdentityFor has nothing to resolve display_name/class_ref
	// from — see TestProjectFor_PlayerEntityCarriesDisplayNameAndClassRef for
	// the populated case (rpg-api#664). RaceRef stays nil regardless — see
	// playerEntity's doc: unlike class, nothing reads it yet.
	s.Require().Empty(a.GetDisplayName(), "no charRepo wired in this test")
	s.Require().Nil(ac.GetClassRef(), "no charRepo wired in this test")
	s.Require().Nil(ac.GetRaceRef(), "RaceRef has no reader yet — see playerEntity's doc")

	// Visible other-player (bob, in alice's sight).
	b := byID["char-bob"]
	s.Require().NotNil(b, "visible other-player must be emitted")
	s.Require().Equal(
		encounterv2pb.EntityType_ENTITY_TYPE_CHARACTER, b.GetType(),
		"visible other-player must carry ENTITY_TYPE_CHARACTER",
	)
	s.Require().NotNil(b.GetHp(), "visible other-player must carry HP")
	s.Require().Equal(int32(8), b.GetHp().GetCurrent())
	s.Require().Equal(int32(10), b.GetHp().GetMax())
	bc := b.GetCharacter()
	s.Require().NotNil(bc, "visible other-player must carry CharacterData oneof")
	s.Require().Equal("player-bob", bc.GetPlayerId())
}

func (s *ProjectSuite) TestProjectFor_PlayerEntityCarriesArmorClass() {
	// Charli is a L1 monk with AC=15 (UnarmoredDefense). The v2 Entity envelope
	// must carry armor_class=15 so the playtest harness can render it (#562).
	data := tkenc.NewData("enc-ac")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"char-charli"}

	charli := newPlayerData("player-charli", "char-charli", originHex, 3)
	charli.HP = 10
	charli.MaxHP = 10
	charli.AC = 15
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-charli": charli}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-charli", s.broker, nil, s.now)
	s.Require().NoError(err)

	var found *encounterv2pb.Entity
	for _, e := range pb.GetSpace().GetEntities() {
		if e.GetId() == "char-charli" {
			found = e
			break
		}
	}
	s.Require().NotNil(found, "charli must be in the snapshot")
	s.Require().NotNil(found.ArmorClass, "player entity must carry armor_class when PlayerData.AC > 0")
	s.Require().Equal(int32(15), found.GetArmorClass(), "armor_class must equal PlayerData.AC")
}

func (s *ProjectSuite) TestProjectFor_PlayerEntityCarriesActiveConditionsAsStatusEffects() {
	// Rurik is mid-rage. rpg-toolkit#754/#778 (encounter v0.29.1): a hydrated
	// combatant's PlayerData.ActiveConditions carries the ref of every
	// condition that survives the toolkit's structurally-permanent filter —
	// i.e. exactly the set a continuously-connected client would have seen
	// live via ConditionApplied. rpg-api#651: project that onto the wire the
	// same shape the live path already uses (translateConditionAppliedEvent's
	// StatusEffect{Source: conditionRefFor(...)}), so a reconnecting client
	// sees the rage badge instead of nothing until the next live event.
	data := tkenc.NewData("enc-conditions")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"char-rurik"}

	rurik := newPlayerData("player-rurik", "char-rurik", originHex, 3)
	rurik.HP = 20
	rurik.MaxHP = 20
	rurik.ActiveConditions = []string{"dnd5e:conditions:raging"}
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-rurik": rurik}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-rurik", s.broker, nil, s.now)
	s.Require().NoError(err)

	var found *encounterv2pb.Entity
	for _, e := range pb.GetSpace().GetEntities() {
		if e.GetId() == "char-rurik" {
			found = e
			break
		}
	}
	s.Require().NotNil(found, "rurik must be in the snapshot")
	s.Require().Len(found.GetStatusEffects(), 1, "one ActiveConditions ref must become one status_effects entry")
	effect := found.GetStatusEffects()[0]
	s.Require().NotNil(effect.GetSource(), "status effect must carry a Source ref")
	s.Require().Equal("dnd5e", effect.GetSource().GetModule())
	s.Require().Equal("conditions", effect.GetSource().GetType())
	s.Require().Equal("raging", effect.GetSource().GetId())
}

func (s *ProjectSuite) TestProjectFor_PlayerEntityNoActiveConditions_NoStatusEffects() {
	// Negative case: a player with no ActiveConditions must get an empty (not
	// nil-panicking, not spuriously populated) status_effects list.
	data := tkenc.NewData("enc-no-conditions")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"char-plain"}

	plain := newPlayerData("player-plain", "char-plain", originHex, 3)
	plain.HP = 10
	plain.MaxHP = 10
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-plain": plain}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-plain", s.broker, nil, s.now)
	s.Require().NoError(err)

	var found *encounterv2pb.Entity
	for _, e := range pb.GetSpace().GetEntities() {
		if e.GetId() == "char-plain" {
			found = e
			break
		}
	}
	s.Require().NotNil(found, "plain must be in the snapshot")
	s.Require().Empty(found.GetStatusEffects(), "no ActiveConditions must mean no status_effects")
}

func (s *ProjectSuite) TestProjectFor_MonsterEntityCarriesActiveConditionsAsStatusEffects() {
	// Same projection, monster side. A monster's own condition namespace
	// (e.g. a spell-applied dnd5e:conditions:poisoned, not a permanent trait —
	// those are filtered toolkit-side per rpg-toolkit#778) must project the
	// same way as a player's.
	data := tkenc.NewData("enc-monster-conditions")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"goblin-1"}

	alice := newPlayerData("player-alice", "char-alice", originHex, 10)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}
	data.Monsters = map[core.EntityID]*tkenc.MonsterData{
		"goblin-1": {
			ID:               "goblin-1",
			Position:         originHex,
			HP:               7,
			MaxHP:            7,
			ActiveConditions: []string{"dnd5e:conditions:poisoned"},
		},
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)

	var found *encounterv2pb.Entity
	for _, e := range pb.GetSpace().GetEntities() {
		if e.GetId() == "goblin-1" {
			found = e
			break
		}
	}
	s.Require().NotNil(found, "goblin-1 must be in the snapshot")
	s.Require().Len(found.GetStatusEffects(), 1)
	effect := found.GetStatusEffects()[0]
	s.Require().Equal("dnd5e", effect.GetSource().GetModule())
	s.Require().Equal("conditions", effect.GetSource().GetType())
	s.Require().Equal("poisoned", effect.GetSource().GetId())
}

func (s *ProjectSuite) TestProjectFor_MonsterRef_TooManyColonsTreatedAsMalformed() {
	// splitRef requires exactly two colons. A four-part ref (three colons)
	// must fall back to the default rather than misparse, since we share
	// splitRef's strict contract with the rest of the v2 encounter wire.
	data := tkenc.NewData("enc-3colon-ref")
	data.Mode = core.ModeTurnBased
	data.Round = 1
	data.ActiveIdx = 0
	data.Initiative = []core.EntityID{"char-alice", "long-1"}

	alice := newPlayerData("player-alice", "char-alice", originHex, 3)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}
	data.Monsters = map[core.EntityID]*tkenc.MonsterData{
		"long-1": {
			ID: "long-1", Position: core.Hex{Q: 1, R: -1, S: 0},
			HP: 1, MaxHP: 1, MonsterRef: "dnd5e:monsters:goblin:variant",
		},
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	for _, e := range pb.GetSpace().GetEntities() {
		if e.GetId() != "long-1" {
			continue
		}
		ref := e.GetMonster().GetMonsterRef()
		s.Require().Equal("dnd5e", ref.GetModule())
		s.Require().Equal("monster", ref.GetType())
		s.Require().Equal("dnd5e:monsters:goblin:variant", ref.GetId(),
			"3+ colons -> fallback (raw lands in Id), per splitRef's strict contract")
	}
}

// TestProjectFor_PopulatesWalls_WholeRoomVisibility covers rpg-api#644 step
// 7: Space.Walls must be populated from the encounter's persisted room
// snapshot (rpg-toolkit#757's SpaceData), converting each WallSegmentData's
// cube-coordinate Start/End into wire Positions and mapping
// BlocksMovement/BlocksLoS onto WallKind. Wave 1 wall visibility is
// whole-room (design doc) — unlike Hexes/Entities, walls are NOT gated by
// the viewer's RevealedHexes/visibleNow, so even a wall alice has never
// stood near must still appear.
func (s *ProjectSuite) TestProjectFor_PopulatesWalls_WholeRoomVisibility() {
	data := tkenc.NewData("enc-walls")
	data.Mode = core.ModeFreeRoam

	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	// solidHex sits well outside alice's sight range (2) so this test also
	// proves wall projection is whole-room, not LOS-gated like Hexes/Entities.
	solidHex := core.Hex{Q: 8, R: -8, S: 0}
	windowHex := core.Hex{Q: 1, R: -1, S: 0}
	data.Space = &tkenc.SpaceData{
		Width:  10,
		Height: 10,
		Walls: []environments.WallSegmentData{
			{Start: solidHex.ToCube(), End: solidHex.ToCube(), BlocksMovement: true, BlocksLoS: true},
			{Start: windowHex.ToCube(), End: windowHex.ToCube(), BlocksMovement: true, BlocksLoS: false},
		},
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)

	walls := pb.GetSpace().GetWalls()
	s.Require().Len(walls, 2, "both walls must be emitted regardless of alice's sight range")

	byKind := make(map[encounterv2pb.WallKind]*encounterv2pb.Wall, 2)
	for _, w := range walls {
		byKind[w.GetKind()] = w
	}

	solid := byKind[encounterv2pb.WallKind_WALL_KIND_SOLID]
	s.Require().NotNil(solid, "BlocksMovement+BlocksLoS must map to WALL_KIND_SOLID")
	s.Require().Equal(int32(8), solid.GetFrom().GetX())
	s.Require().Equal(int32(-8), solid.GetFrom().GetY())
	s.Require().Equal(int32(0), solid.GetFrom().GetZ())
	s.Require().Equal(solid.GetFrom(), solid.GetTo(), "wave 1 walls are degenerate Start==End segments")

	window := byKind[encounterv2pb.WallKind_WALL_KIND_WINDOW]
	s.Require().NotNil(window, "BlocksMovement without BlocksLoS must map to WALL_KIND_WINDOW")
	s.Require().Equal(int32(1), window.GetFrom().GetX())
	s.Require().Equal(int32(-1), window.GetFrom().GetY())
}

// TestProjectFor_NilSpace_EmptyWalls covers the pre-wave-1 fallback: an
// encounter with no room (InitRoom never called, data.Space == nil) must
// project an empty Walls list, not panic.
func (s *ProjectSuite) TestProjectFor_NilSpace_EmptyWalls() {
	data := tkenc.NewData("enc-no-room")
	data.Mode = core.ModeFreeRoam
	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	s.Require().Empty(pb.GetSpace().GetWalls())
}

// TestProjectFor_DoorWall_ProjectsIdKindAndPassageEdge covers The Dungeon
// wave 2 Slice 2 (rpg-api#676): a door in data.Doors must project onto the
// wire as a Wall carrying its entity id (rpg-api-protos#186's additive
// Wall.id) and a DOOR_CLOSED/DOOR_OPEN kind derived from DoorData.Open —
// not the door's own degenerate Start==End segment, but From=the door's
// cell and To=its passage-edge neighbor (design doc §Q2's 6-door-pair
// multiplicity fix), found via SpaceData.RegionAt over the door's six
// neighbors. Like solid walls, door walls are whole-room (unconditional),
// not LOS-gated.
func (s *ProjectSuite) TestProjectFor_DoorWall_ProjectsIdKindAndPassageEdge() {
	data := tkenc.NewData("enc-door")
	data.Mode = core.ModeFreeRoam
	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	doorHex := core.Hex{Q: 0, R: 0, S: 0}
	// Exactly one of the door's six neighbors (perception.HexNeighbors) is
	// tagged into a region — chamber-1's — so the passage-edge pick is
	// unambiguous. The door's OWN cell is deliberately left untagged: it
	// belongs to neither region (rpg-toolkit#806's gate note, mirrored in
	// start_encounter.go's chamberEntryAnchor KNOWN TRAP).
	passageNeighbor := core.Hex{Q: -1, R: 0, S: 1}
	data.Space = &tkenc.SpaceData{
		Width:  10,
		Height: 10,
		Regions: []tkenc.RegionData{
			{ID: tkenc.RegionChamber1, Hexes: core.NewHexSet(passageNeighbor)},
		},
	}
	data.Doors = map[core.EntityID]*tkenc.DoorData{
		"door-1": {ID: "door-1", Position: doorHex, Open: false},
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)

	walls := pb.GetSpace().GetWalls()
	s.Require().Len(walls, 1)
	w := walls[0]
	s.Require().Equal("door-1", w.GetId(), "the wire wall must carry the door's entity id (the click->Interact bridge)")
	s.Require().Equal(encounterv2pb.WallKind_WALL_KIND_DOOR_CLOSED, w.GetKind())
	s.Require().Equal(int32(0), w.GetFrom().GetX())
	s.Require().Equal(int32(0), w.GetFrom().GetY())
	s.Require().Equal(int32(0), w.GetFrom().GetZ())
	s.Require().NotEqual(w.GetFrom(), w.GetTo(),
		"a door wall must NOT be a degenerate Start==End segment — that's the 6-door-pair multiplicity bug")
	s.Require().Equal(int32(-1), w.GetTo().GetX())
	s.Require().Equal(int32(0), w.GetTo().GetY())
	s.Require().Equal(int32(1), w.GetTo().GetZ())

	// Open the door and re-project: same id/edge, kind flips to DOOR_OPEN.
	data.Doors["door-1"].Open = true
	pbOpen, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)
	openWalls := pbOpen.GetSpace().GetWalls()
	s.Require().Len(openWalls, 1)
	s.Require().Equal(encounterv2pb.WallKind_WALL_KIND_DOOR_OPEN, openWalls[0].GetKind())
	s.Require().Equal("door-1", openWalls[0].GetId())
}

// TestProjectFor_DoorWall_NoRegionNeighbor_FallsBackToDegenerateSegment
// covers doorPassageNeighbor's defensive fallback: a door whose neighbors
// are all untagged (a data shape the current two-chamber generator never
// produces) must still project — as a degenerate Start==To segment,
// matching a solid wall's shape — rather than being dropped from the wire
// or panicking.
func (s *ProjectSuite) TestProjectFor_DoorWall_NoRegionNeighbor_FallsBackToDegenerateSegment() {
	data := tkenc.NewData("enc-door-no-region")
	data.Mode = core.ModeFreeRoam
	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	data.Space = &tkenc.SpaceData{Width: 10, Height: 10} // no Regions at all
	data.Doors = map[core.EntityID]*tkenc.DoorData{
		"door-1": {ID: "door-1", Position: core.Hex{Q: 0, R: 0, S: 0}, Open: false},
	}

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	s.Require().NoError(err)

	walls := pb.GetSpace().GetWalls()
	s.Require().Len(walls, 1)
	s.Require().Equal(walls[0].GetFrom(), walls[0].GetTo(), "no tagged neighbor falls back to a degenerate Start==End segment")
}

// TestProjectFor_DoorWall_NilSpace_NoPanic_DegenerateSegment is the
// Copilot-review regression pin (PR #677): other integration/unit fixtures
// build a door via enc.AddDoor without ever calling InitRoom/
// InitTwoChamberRoom (e.g. encounter_v2_test.go's TestInteract_OpenDoor_
// TwoPlayers), which leaves Data.Space nil while Data.Doors is non-empty —
// AddDoor's toolkit implementation only touches Space when it is already
// set. ProjectFor must still project such a door deterministically (a
// degenerate Start==To segment, matching a solid wall's shape) instead of
// panicking. SpaceData.RegionAt is itself nil-receiver safe (rpg-
// toolkit#806), and doorPassageNeighbor now also nil-checks space
// explicitly — this test proves the combination end to end through the
// public ProjectFor entry point, not just the unexported helper.
func (s *ProjectSuite) TestProjectFor_DoorWall_NilSpace_NoPanic_DegenerateSegment() {
	data := tkenc.NewData("enc-door-nil-space")
	data.Mode = core.ModeFreeRoam
	alice := newPlayerData("player-alice", "char-alice", originHex, 2)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	// data.Space is left nil — no InitRoom/InitTwoChamberRoom call.
	data.Doors = map[core.EntityID]*tkenc.DoorData{
		"door-1": {ID: "door-1", Position: core.Hex{Q: 3, R: -3, S: 0}, Open: false},
	}

	var pb *encounterv2pb.Encounter
	var err error
	s.Require().NotPanics(func() {
		pb, err = v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, nil, s.now)
	}, "ProjectFor must not panic on a door-only encounter with nil Data.Space")
	s.Require().NoError(err)

	walls := pb.GetSpace().GetWalls()
	s.Require().Len(walls, 1)
	s.Require().Equal("door-1", walls[0].GetId())
	s.Require().Equal(walls[0].GetFrom(), walls[0].GetTo(),
		"nil space falls back to a degenerate Start==End segment, same as the no-tagged-neighbor case")
}

// TestProjectFor_PlayerEntityCarriesDisplayNameAndClassRef proves rpg-api#664:
// once a charRepo is wired, both the viewer's own entity and a visible
// other-player entity carry display_name (from the character record's Name)
// and CharacterData.class_ref (from ClassID) — the real StartEncounter path
// (which does wire a charRepo, unlike this suite's other nil-charRepo tests)
// no longer falls back to the raw entity id on the web's identity dock.
func (s *ProjectSuite) TestProjectFor_PlayerEntityCarriesDisplayNameAndClassRef() {
	// FreeRoam (no active actor) deliberately: turn-based mode would also
	// exercise ProjectFor's #601 active-actor menu hydration (a SEPARATE
	// charRepo.Get/Update pair for whichever seat is active), which is
	// orthogonal to what this test verifies and would need its own mock
	// expectations to avoid an unrelated gomock failure.
	data := tkenc.NewData("enc-identity")
	data.Mode = core.ModeFreeRoam

	alice := newPlayerData("player-alice", "char-alice", originHex, 3)
	bob := newPlayerData("player-bob", "char-bob", core.Hex{Q: 1, R: -1, S: 0}, 3)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{
		"player-alice": alice,
		"player-bob":   bob,
	}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &tkcharacter.Data{ID: "char-alice", Name: "Alice", ClassID: "rogue"}},
		}, nil)
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-bob"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{Data: &tkcharacter.Data{ID: "char-bob", Name: "Bob", ClassID: "fighter"}},
		}, nil)

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, s.mockCharRepo, s.now)
	s.Require().NoError(err)

	byID := make(map[string]*encounterv2pb.Entity)
	for _, e := range pb.GetSpace().GetEntities() {
		byID[e.GetId()] = e
	}

	a := byID["char-alice"]
	s.Require().NotNil(a, "viewer's own entity must be emitted")
	s.Require().Equal("Alice", a.GetDisplayName())
	aClassRef := a.GetCharacter().GetClassRef()
	s.Require().NotNil(aClassRef)
	s.Require().Equal("dnd5e", aClassRef.GetModule())
	s.Require().Equal("class", aClassRef.GetType())
	s.Require().Equal("rogue", aClassRef.GetId())

	b := byID["char-bob"]
	s.Require().NotNil(b, "visible other-player must be emitted")
	s.Require().Equal("Bob", b.GetDisplayName())
	bClassRef := b.GetCharacter().GetClassRef()
	s.Require().NotNil(bClassRef)
	s.Require().Equal("fighter", bClassRef.GetId())
}

// TestProjectFor_PlayerEntity_CharacterNotFound_LeavesIdentityBlank proves
// the absent-data fallback (rpg-api#664): a character store miss (or any
// other lookup error) must not fail the whole snapshot — it degrades to the
// pre-fix blank display_name/class_ref, exactly like the nil-charRepo case,
// so a data-consistency gap elsewhere never blocks viewing the encounter.
func (s *ProjectSuite) TestProjectFor_PlayerEntity_CharacterNotFound_LeavesIdentityBlank() {
	data := tkenc.NewData("enc-missing-char")
	data.Mode = core.ModeFreeRoam
	alice := newPlayerData("player-alice", "char-alice", originHex, 3)
	data.Players = map[core.PlayerID]*tkenc.PlayerData{"player-alice": alice}

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(nil, apierr.NotFoundf("character with ID %s not found", "char-alice"))

	pb, err := v2encounter.ProjectFor(context.Background(), data, "player-alice", s.broker, s.mockCharRepo, s.now)
	s.Require().NoError(err, "a missing character record must not fail the snapshot")

	var a *encounterv2pb.Entity
	for _, e := range pb.GetSpace().GetEntities() {
		if e.GetId() == "char-alice" {
			a = e
		}
	}
	s.Require().NotNil(a)
	s.Require().Empty(a.GetDisplayName())
	s.Require().Nil(a.GetCharacter().GetClassRef())
}

func TestProjectSuite(t *testing.T) {
	suite.Run(t, new(ProjectSuite))
}
