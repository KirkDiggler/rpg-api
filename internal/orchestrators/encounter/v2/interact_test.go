package encounter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

type InteractSuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *InteractSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:        s.broker,
		EncounterRepo: s.repo,
		Resolver:      stubCharacterResolver{},
		// No combat / movement resolver behavior is exercised by the door verbs,
		// but the orchestrator requires the builders to be present. Return the
		// toolkit zero-value interfaces (nil) — Interact never invokes them.
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

// stubCharacterResolver satisfies tkenc.CharacterResolver with zero modifiers;
// the door verbs never call it, but Config.Resolver is required.
type stubCharacterResolver struct{}

func (stubCharacterResolver) AbilityModifier(_ core.PlayerID, _ string) (int, bool) { return 0, true }
func (stubCharacterResolver) ToolProficiencyBonus(_ core.PlayerID, _ string) (int, bool) {
	return 0, true
}

// seedUnlockedDoor persists an encounter with player-A adjacent to an unlocked
// door so OpenDoor can publish a per-viewer slice (the viewer must see the
// door's hex).
func (s *InteractSuite) seedUnlockedDoor(encID, doorID string) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   "player-A",
		EntityID:   "char-A",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 4,
	}))
	enc.AddDoor(core.EntityID(doorID), core.Hex{Q: 1, R: 0, S: -1}, false)
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))
}

// seedLockedDoor persists an encounter with player-A adjacent to a locked door.
func (s *InteractSuite) seedLockedDoor(encID, doorID string, dc int, ability, tool string) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   "player-A",
		EntityID:   "char-A",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 4,
	}))
	enc.AddDoor(core.EntityID(doorID), core.Hex{Q: 1, R: 0, S: -1}, false)
	data := enc.ToData()
	door := data.Doors[core.EntityID(doorID)]
	door.Locked = true
	door.LockDC = dc
	door.LockAbility = ability
	door.LockTool = tool
	s.Require().NoError(s.repo.Save(s.ctx, data))
}

func (s *InteractSuite) interact(encID, target string) (*encounterorch.InteractOutput, error) {
	return s.orch.Interact(s.ctx, &encounterorch.InteractInput{
		EncounterID:    encID,
		PlayerID:       "player-A",
		TargetEntityID: core.EntityID(target),
	})
}

// --- load preconditions ---

func (s *InteractSuite) TestInteract_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.interact("nope", "door-east")
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *InteractSuite) TestInteract_PlayerNotInEncounter_ErrPlayerNotInEncounter() {
	enc := tkenc.New(s.ctx, "enc-no-player", s.broker)
	enc.AddDoor("door-east", core.Hex{Q: 1, R: 0, S: -1}, false)
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.interact("enc-no-player", "door-east")
	s.Require().ErrorIs(err, encounterorch.ErrPlayerNotInEncounter)
}

// --- door classification ---

func (s *InteractSuite) TestInteract_TargetNotADoor_ErrTargetNotADoor() {
	s.seedUnlockedDoor("enc-no-door", "door-east")
	_, err := s.interact("enc-no-door", "not-a-door")
	s.Require().ErrorIs(err, encounterorch.ErrTargetNotADoor)
}

// --- unlocked-door happy path ---

func (s *InteractSuite) TestInteract_OpenDoor_OpensAndPersists() {
	s.seedUnlockedDoor("enc-open", "door-east")

	out, err := s.interact("enc-open", "door-east")
	s.Require().NoError(err)
	s.Require().NotNil(out)
	s.Require().Nil(out.Prompt, "unlocked door must not issue a prompt")

	loaded, err := s.repo.Get(s.ctx, "enc-open")
	s.Require().NoError(err)
	door, ok := loaded.Doors[core.EntityID("door-east")]
	s.Require().True(ok, "door must remain in persisted Doors map")
	s.Require().True(door.Open, "door must be persisted as Open")
}

func (s *InteractSuite) TestInteract_DoorAlreadyOpen_ErrDoorVerbRefused() {
	enc := tkenc.New(s.ctx, "enc-already-open", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	enc.AddDoor("door-east", core.Hex{Q: 1, R: 0, S: -1}, true) // already open
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.interact("enc-already-open", "door-east")
	s.Require().ErrorIs(err, encounterorch.ErrDoorVerbRefused)
}

// --- locked-door path ---

func (s *InteractSuite) TestInteract_LockedDoor_IssuesPromptAndPersists() {
	s.seedLockedDoor("enc-locked", "door-east", 15, "DEX", "dnd5e:item:thieves-tools")

	out, err := s.interact("enc-locked", "door-east")
	s.Require().NoError(err)
	s.Require().NotNil(out.Prompt, "locked door must issue a skill-check prompt")
	s.Require().Equal(15, out.Prompt.DC)
	s.Require().Equal("DEX", out.Prompt.Ability)
	s.Require().Equal("dnd5e:item:thieves-tools", out.Prompt.Tool)

	// Pending prompt persisted so SubmitCheck can resolve it; door stays closed.
	loaded, err := s.repo.Get(s.ctx, "enc-locked")
	s.Require().NoError(err)
	s.Require().Contains(loaded.PendingPrompts, core.PlayerID("player-A"))
	door := loaded.Doors[core.EntityID("door-east")]
	s.Require().True(door.Locked)
	s.Require().False(door.Open)
}

func (s *InteractSuite) TestInteract_LockedDoor_PromptCollision_ErrPromptAlreadyPending() {
	s.seedLockedDoor("enc-collide", "door-east", 15, "DEX", "")

	_, err := s.interact("enc-collide", "door-east")
	s.Require().NoError(err)

	// Second call with an outstanding prompt must be refused with the toolkit's
	// ErrPromptAlreadyPending sentinel passed through (the handler maps it
	// distinctly from generic verb refusals).
	_, err = s.interact("enc-collide", "door-east")
	s.Require().ErrorIs(err, tkenc.ErrPromptAlreadyPending)
	s.Require().False(errors.Is(err, encounterorch.ErrDoorVerbRefused),
		"recognized sentinels must NOT also be wrapped as generic verb refusals")
}

func TestInteractSuite(t *testing.T) {
	suite.Run(t, new(InteractSuite))
}
