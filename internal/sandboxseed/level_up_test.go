package sandboxseed

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	charconv "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
)

// seedFakeStore is a per-id character store: the default fixture set now seeds
// two characters through it, so a single-character fake would not tell them
// apart.
type seedFakeStore struct {
	byID      map[string]*entities.Character
	getErr    error
	updateErr error
	updates   []characterrepo.UpdateInput
}

func newSeedFakeStore(ids ...string) *seedFakeStore {
	store := &seedFakeStore{byID: map[string]*entities.Character{}}
	for _, id := range ids {
		store.byID[id] = &entities.Character{Data: &tkcharacter.Data{
			ID:      id,
			Level:   1,
			ClassID: classes.Fighter,
			Levels: []tkcharacter.LevelEntry{
				{Level: 1, ClassID: classes.Fighter, HitPointMethod: tkcharacter.HitPointMethodMax},
			},
		}}
	}
	return store
}

func (s *seedFakeStore) Get(_ context.Context, input characterrepo.GetInput) (*characterrepo.GetOutput, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	stored, ok := s.byID[input.ID]
	if !ok {
		return nil, characterNotFound(input.ID)
	}
	return &characterrepo.GetOutput{Character: cloneEntity(stored), Version: "version"}, nil
}

func (s *seedFakeStore) Update(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
	s.updates = append(s.updates, characterrepo.UpdateInput{Character: cloneEntity(input.Character)})
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	s.byID[input.Character.Data.ID] = cloneEntity(input.Character)
	return &characterrepo.UpdateOutput{Character: cloneEntity(input.Character)}, nil
}

func characterNotFound(id string) error {
	return &notFoundError{id: id}
}

type notFoundError struct{ id string }

func (e *notFoundError) Error() string { return "character " + e.id + " not found" }

// seedListResponses is the list traffic the five fixtures generate, in order.
// Each identity lists once to find what to delete and once to find what it just
// created; the sandbox fighter lists a third time to confirm its equip.
func seedListResponses() []*dnd5ev1alpha1.ListCharactersResponse {
	named := func(id, name string) *dnd5ev1alpha1.ListCharactersResponse {
		return &dnd5ev1alpha1.ListCharactersResponse{
			Characters: []*dnd5ev1alpha1.Character{{Id: id, Name: name}},
		}
	}
	return []*dnd5ev1alpha1.ListCharactersResponse{
		{}, named("new-fighter", fighterName), named("new-fighter", fighterName),
		{}, named("new-barbarian", barbarianName), named("new-barbarian", barbarianName),
		{}, named("new-bard", bardName),
		{}, named("level-up-fighter-id", levelUpFighterName),
		{}, named("level-up-bard-id", levelUpBardName),
	}
}

// TestSeed_WritesLevelUpExperienceThroughTheRepository is design R4.12 as a
// test: "Seeding a character for a walk means writing experience on the
// persisted sheet through the fixture tool, the way every fixture is written --
// not through the served API, which has no code path that writes it."
//
// The two writes land on the repository, and the three numbers come back
// through GetCharacter -- which this fake answers by running the stored sheet
// through the real projection, so a broken derivation fails here.
func TestSeed_WritesLevelUpExperienceThroughTheRepository(t *testing.T) {
	store := newSeedFakeStore("level-up-fighter-id", "level-up-bard-id")
	client := newGalleryFakeClient()
	client.seedStore = store
	client.listResponses = seedListResponses()

	err := Seed(context.Background(), &SeedInput{Client: client, Store: store})

	require.NoError(t, err)
	require.Len(t, store.updates, 2, "one repository write per level-up fixture, and none for the other three")
	require.Equal(t, 300, store.updates[0].Character.Data.Experience)
	require.Equal(t, 300, store.updates[1].Character.Data.Experience)
	require.Equal(t, 300, store.byID["level-up-fighter-id"].Data.Experience)
	require.Equal(t, 300, store.byID["level-up-bard-id"].Data.Experience)

	// The projection the fixture verified: entitled to 2 while still level 1,
	// with 900 named as what the level after that costs.
	projected := charconv.ConvertCharacterDataToProto(store.byID["level-up-bard-id"].Data)
	require.Equal(t, int32(300), projected.GetExperiencePoints())
	require.Equal(t, int32(2), projected.GetEntitledLevel())
	require.Equal(t, int32(900), projected.GetNextLevelThreshold())
	require.Equal(t, int32(1), projected.GetLevel(), "the gap between 2 and 1 is the level-up signal")
}

// TestSeed_RefusesWithoutAStore. A store-less seed would create two fixtures
// that look correct until someone clicks "level up" and is told they have not
// earned it -- the failure this refusal exists to make loud.
func TestSeed_RefusesWithoutAStore(t *testing.T) {
	err := Seed(context.Background(), &SeedInput{Client: newGalleryFakeClient()})

	require.Error(t, err)
	require.Contains(t, err.Error(), "character store is required")
}

// TestSeed_LevelUpFixturesUseTheirOwnIdentities pins the walk's entry points:
// the identity IS the ?playerId= the environment is opened with, and the three
// sandbox fixtures must keep holding no experience so a freshly created
// character still shows 0 of 300 (done-when 7).
func TestSeed_LevelUpFixturesUseTheirOwnIdentities(t *testing.T) {
	store := newSeedFakeStore("level-up-fighter-id", "level-up-bard-id")
	client := newGalleryFakeClient()
	client.seedStore = store
	client.listResponses = seedListResponses()

	require.NoError(t, Seed(context.Background(), &SeedInput{Client: client, Store: store}))

	require.Contains(t, client.authHeaders, "Dev "+levelUpFighterIdentity)
	require.Contains(t, client.authHeaders, "Dev "+levelUpBardIdentity)
	require.Equal(t, 5, client.createDrafts, "three sandbox fixtures plus two level-up fixtures")

	updatedIDs := []string{
		store.updates[0].Character.Data.ID,
		store.updates[1].Character.Data.ID,
	}
	require.ElementsMatch(t, []string{"level-up-fighter-id", "level-up-bard-id"}, updatedIDs,
		"experience is written to the level-up fixtures and to nothing else")
}

// TestSeed_LevelUpBardIsBuiltByTheSameFunctionAsTheSandboxBard: the level-2
// spell question is "five known minus four known", so the two bards must ask
// for the same four spells. A second copy of that list free to drift would
// quietly change what the level-up screen asks.
func TestSeed_LevelUpBardIsBuiltByTheSameFunctionAsTheSandboxBard(t *testing.T) {
	store := newSeedFakeStore("level-up-fighter-id", "level-up-bard-id")
	client := newGalleryFakeClient()
	client.seedStore = store
	client.listResponses = seedListResponses()

	require.NoError(t, Seed(context.Background(), &SeedInput{Client: client, Store: store}))

	var bardSpellRequests [][]string
	for _, request := range client.updateClassRequests {
		if request.GetClass() != dnd5ev1alpha1.Class_CLASS_BARD {
			continue
		}
		for _, choice := range request.GetClassChoices() {
			if choice.GetCategory() == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS {
				bardSpellRequests = append(bardSpellRequests, choice.GetSpells().GetSpellRefs())
			}
		}
	}

	require.Len(t, bardSpellRequests, 2, "the sandbox bard and the level-up bard")
	require.Equal(t,
		[]string{baneRef, thunderwaveRef, dissonantWhispersRef, commandRef},
		bardSpellRequests[0])
	require.Equal(t, bardSpellRequests[0], bardSpellRequests[1],
		"four known spells is what makes level 2 a one-spell question")
}
