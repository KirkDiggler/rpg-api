package encounter

// hydrate_players_test.go — rpg-api#612: white-box tests for
// reconciledCharacterHP and its wiring into attachPlayerCharacterData /
// attachActorCharacterData. These live in package encounter (not
// encounter_test) so they can call the unexported reconciledCharacterHP
// directly — the crispest proof of the pure-function contract — mirroring
// dnd5e_combat_resolver_weapon_test.go's white-box precedent.
//
// reconciledCharacterHP mirrors syncMonsterDataFromSnapshot's role for
// monsters (rpg-toolkit encounter/npc.go): once the encounter's own
// PlayerData.HP/MaxHP is seeded (seedPlayerHP at AddPlayer time), the
// ENCOUNTER's snapshot is authoritative over the character-store blob on
// every subsequent load — never the reverse, and never mutating the
// store's own *character.Data in place.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	tkenccore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/events"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// ---------------------------------------------------------------------------
// reconciledCharacterHP — pure-function unit tests
// ---------------------------------------------------------------------------

func TestReconciledCharacterHP_SeededSnapshot_OverridesCharacterData(t *testing.T) {
	charData := &tkcharacter.Data{ID: "char-A", HitPoints: 20, MaxHitPoints: 20}
	pd := &tkenc.PlayerData{HP: 7, MaxHP: 20}

	got := reconciledCharacterHP(charData, pd)

	if got.HitPoints != 7 {
		t.Fatalf("HitPoints = %d, want 7 (from the encounter snapshot, not the store's 20)", got.HitPoints)
	}
	if got.MaxHitPoints != 20 {
		t.Fatalf("MaxHitPoints = %d, want 20", got.MaxHitPoints)
	}
	if charData.HitPoints != 20 {
		t.Fatalf("original charData.HitPoints mutated to %d — reconciledCharacterHP must return a copy, "+
			"not mutate the store's own *character.Data in place", charData.HitPoints)
	}
}

func TestReconciledCharacterHP_UnseededSnapshot_LeavesCharacterDataUntouched(t *testing.T) {
	// pd.MaxHP == 0 means this snapshot was never seeded (a pre-#612 encounter,
	// or a seat added before any character existed) — the store's value must
	// survive, not get zeroed out.
	charData := &tkcharacter.Data{ID: "char-A", HitPoints: 15, MaxHitPoints: 20}
	pd := &tkenc.PlayerData{HP: 0, MaxHP: 0}

	got := reconciledCharacterHP(charData, pd)

	if got.HitPoints != 15 || got.MaxHitPoints != 20 {
		t.Fatalf("got HP=%d/%d, want unchanged 15/20 — an unseeded snapshot must not zero out the store's HP",
			got.HitPoints, got.MaxHitPoints)
	}
}

func TestReconciledCharacterHP_NilCharData_ReturnsNil(t *testing.T) {
	if got := reconciledCharacterHP(nil, &tkenc.PlayerData{HP: 5, MaxHP: 10}); got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestReconciledCharacterHP_NilPlayerData_ReturnsCharDataUnchanged(t *testing.T) {
	charData := &tkcharacter.Data{ID: "char-A", HitPoints: 15, MaxHitPoints: 20}
	got := reconciledCharacterHP(charData, nil)
	if got != charData {
		t.Fatalf("got a different pointer, want the same charData returned unchanged when pd is nil")
	}
}

// ---------------------------------------------------------------------------
// attachPlayerCharacterData / attachActorCharacterData — wiring integration
// ---------------------------------------------------------------------------

// HydratePlayersReconcileSuite proves the reconciliation is actually wired
// into both hydration call sites: the marshaled DataJSON blob, once loaded
// via tkcharacter.LoadFromData (mirroring what the SDK's LoadFromData
// cascade does internally), reflects the encounter snapshot's HP — not the
// store's diverged value.
type HydratePlayersReconcileSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	ctx          context.Context
}

func TestHydratePlayersReconcileSuite(t *testing.T) {
	suite.Run(t, new(HydratePlayersReconcileSuite))
}

func (s *HydratePlayersReconcileSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.ctx = context.Background()
}

func (s *HydratePlayersReconcileSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestAttachPlayerCharacterData_ReconcilesFromSnapshot proves the encounter
// snapshot's HP (7/20, simulating mid-combat damage already tracked by the
// encounter) wins over the character store's diverged copy (20/20, as if the
// store were never told about the damage) once attachPlayerCharacterData
// runs — the held character built from the resulting blob reports 7, not 20.
func (s *HydratePlayersReconcileSuite) TestAttachPlayerCharacterData_ReconcilesFromSnapshot() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-A"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &tkcharacter.Data{ID: "char-A", HitPoints: 20, MaxHitPoints: 20},
			},
		}, nil)

	data := tkenc.NewData("enc-1")
	data.Players[tkenccore.PlayerID("player-A")] = &tkenc.PlayerData{
		ID:       tkenccore.PlayerID("player-A"),
		EntityID: tkenccore.EntityID("char-A"),
		HP:       7,
		MaxHP:    20,
	}

	s.Require().NoError(attachPlayerCharacterData(s.ctx, data, s.mockCharRepo))

	pd := data.Players["player-A"]
	s.Require().NotEmpty(pd.DataJSON, "attachPlayerCharacterData must populate the transient DataJSON blob")

	var loaded tkcharacter.Data
	s.Require().NoError(json.Unmarshal(pd.DataJSON, &loaded))
	s.Equal(7, loaded.HitPoints, "the marshaled blob must carry the snapshot's HP, not the store's diverged 20")
	s.Equal(20, loaded.MaxHitPoints)

	// Prove it also survives a real character.LoadFromData round trip — the
	// same construction the SDK's hydration cascade performs.
	char, err := tkcharacter.LoadFromData(s.ctx, &loaded, events.NewEventBus())
	s.Require().NoError(err)
	s.Equal(7, char.GetHitPoints())
}

// TestAttachPlayerCharacterData_UnseededSnapshot_KeepsStoreHP proves a
// pre-#612 encounter (PlayerData.HP/MaxHP never seeded, both 0) does not
// zero out the character store's real HP — the store's value survives.
func (s *HydratePlayersReconcileSuite) TestAttachPlayerCharacterData_UnseededSnapshot_KeepsStoreHP() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-A"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &tkcharacter.Data{ID: "char-A", HitPoints: 12, MaxHitPoints: 15},
			},
		}, nil)

	data := tkenc.NewData("enc-1")
	data.Players[tkenccore.PlayerID("player-A")] = &tkenc.PlayerData{
		ID:       tkenccore.PlayerID("player-A"),
		EntityID: tkenccore.EntityID("char-A"),
		// HP/MaxHP left at the zero value — unseeded snapshot.
	}

	s.Require().NoError(attachPlayerCharacterData(s.ctx, data, s.mockCharRepo))

	var loaded tkcharacter.Data
	s.Require().NoError(json.Unmarshal(data.Players["player-A"].DataJSON, &loaded))
	s.Equal(12, loaded.HitPoints, "an unseeded snapshot must not zero out the store's real HP")
	s.Equal(15, loaded.MaxHitPoints)
}

// TestAttachActorCharacterData_ReconcilesFromSnapshot proves the same
// reconciliation applies on the turn-start actor-menu hydration path, not
// just the full combat-verb load path.
func (s *HydratePlayersReconcileSuite) TestAttachActorCharacterData_ReconcilesFromSnapshot() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-A"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &tkcharacter.Data{ID: "char-A", HitPoints: 20, MaxHitPoints: 20},
			},
		}, nil)

	data := tkenc.NewData("enc-1")
	data.Players[tkenccore.PlayerID("player-A")] = &tkenc.PlayerData{
		ID:       tkenccore.PlayerID("player-A"),
		EntityID: tkenccore.EntityID("char-A"),
		HP:       3,
		MaxHP:    20,
	}

	attached, err := attachActorCharacterData(s.ctx, data, tkenccore.EntityID("char-A"), s.mockCharRepo)
	s.Require().NoError(err)
	s.Require().True(attached)

	var loaded tkcharacter.Data
	s.Require().NoError(json.Unmarshal(data.Players["player-A"].DataJSON, &loaded))
	s.Equal(3, loaded.HitPoints, "the actor-menu hydration path must reconcile HP too")
}
