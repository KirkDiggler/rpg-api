package encounter_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
)

// ProjectSuite covers ProjectFor's per-viewer projection of toolkit
// encounter.Data into a v1alpha2 proto Encounter. Wave 2.8 added Mode +
// TurnState + monster entities — previously ProjectFor hardcoded
// FREE_ROAM and only emitted player entities.
type ProjectSuite struct {
	suite.Suite
	now    time.Time
	broker *tkenc.Broker
}

func (s *ProjectSuite) SetupTest() {
	s.now = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
}

// originHex is the cube origin (0,0,0); used everywhere viewers stand at
// the center of their sight radius for fixture simplicity.
var originHex = core.Hex{Q: 0, R: 0, S: 0}

// newPlayerData builds a minimal *tkenc.PlayerData for a viewer standing at
// pos with the given sight range. RevealedHexes is seeded to whatever the
// stub LoS at sightRange returns from pos.
func newPlayerData(pid core.PlayerID, eid core.EntityID, pos core.Hex, sightRange int) *tkenc.PlayerData {
	view := perception.NewView(pid, pos, sightRange)
	view.ApplyReveal(perception.VisibleHexesAt(pos, sightRange))
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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
	// ClassRef / RaceRef intentionally deferred — see issue #511 deferred section.
	s.Require().Nil(ac.GetClassRef(), "ClassRef deferred until PlayerData carries it")
	s.Require().Nil(ac.GetRaceRef(), "RaceRef deferred until PlayerData carries it")

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

	pb, err := v2encounter.ProjectFor(data, "player-charli", s.broker, s.now)
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

	pb, err := v2encounter.ProjectFor(data, "player-alice", s.broker, s.now)
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

func TestProjectSuite(t *testing.T) {
	suite.Run(t, new(ProjectSuite))
}
