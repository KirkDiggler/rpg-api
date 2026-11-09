package dungeons_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

const (
	castleKitchen = dungeonstest.CastleKitchenKey
	storeMyCastle = "my-castle"
	storeMyKeep   = "my-keep"
	storeBobKeep  = "bob-keep"
	// storeTomb is the version 2 reference on the test shelf (the retired
	// tomb under testdata/sites): resolvable by key, never listed (R23).
	storeTomb = "reference-tomb"
)

var (
	alice = dungeons.Caller{WorldID: "w1", PlayerID: "alice"}
	bob   = dungeons.Caller{WorldID: "w1", PlayerID: "bob"}
	carol = dungeons.Caller{WorldID: "w2", PlayerID: "carol"}
)

// StoresSuite is the two-store registry over a real shipped store (the
// package's shared shelf: content/, the retired walk sites and the castle
// kitchen fixture staged in as a reference, compiled once per binary; see
// shelf_test.go) and a real world store over miniredis, fresh per test.
type StoresSuite struct {
	suite.Suite
	ctx     context.Context
	dir     string
	world   worlddungeons.Repository
	kitchen []byte
}

func TestStoresSuite(t *testing.T) { suite.Run(t, new(StoresSuite)) }

func (s *StoresSuite) SetupTest() {
	s.ctx = context.Background()
	s.dir = shelf.dir
	s.world = dungeonstest.WorldStore(s.T())
	s.kitchen = dungeonstest.CastleKitchenYAML(s.T())
}

func (s *StoresSuite) registry(authoring bool) dungeons.Registry {
	return joinShelf(s.T(), s.world, authoring)
}

// file is the castle kitchen as an author's own encounter under key.
func (s *StoresSuite) file(key string) []byte {
	return dungeonstest.AsKey(s.T(), s.kitchen, castleKitchen, key)
}

func (s *StoresSuite) save(r dungeons.Registry, caller dungeons.Caller, key string) []byte {
	raw := s.file(key)
	out, err := r.Put(s.ctx, &dungeons.PutInput{Caller: caller, Key: key, YAML: raw})
	s.Require().NoError(err)
	s.Require().Empty(out.Errors)
	return raw
}

func (s *StoresSuite) stored(worldID, key string) worlddungeons.Record {
	out, err := s.world.Get(s.ctx, &worlddungeons.GetInput{WorldID: worldID, Key: key})
	s.Require().NoError(err)
	return out.Record
}

// dirSnapshot is the content directory as it stands, every file with its
// bytes.
func (s *StoresSuite) dirSnapshot() map[string][]byte {
	files, err := readShelf(s.dir)
	s.Require().NoError(err)
	return files
}

func (s *StoresSuite) TestPutWritesTheWorldStoreAndNotTheDirectory() {
	r := s.registry(true)
	s.Require().Equal(shelf.staged, s.dirSnapshot(), "the directory starts as staged")

	raw := s.save(r, alice, storeMyCastle)

	s.Equal(shelf.staged, s.dirSnapshot(), "the directory is unchanged")
	s.Equal(worlddungeons.Record{Key: storeMyCastle, Owner: alice.PlayerID, Name: "The Castle Kitchen", YAML: raw},
		s.stored(alice.WorldID, storeMyCastle), "the compiled name is stored with the record")
}

// TestListDoesNotCompile: List names world records from the name stored at
// the write, never by compiling; a record whose bytes would not compile is
// still listed by its stored name, and one stored without a name by its key.
func (s *StoresSuite) TestListDoesNotCompile() {
	r := s.registry(true)
	for _, in := range []*worlddungeons.PutInput{
		{WorldID: alice.WorldID, Owner: alice.PlayerID, Key: "broken-keep", Name: "The Broken Keep", YAML: []byte("version: 4\nkey: broken-keep\nvoid: nonsense\n")},
		{WorldID: alice.WorldID, Owner: alice.PlayerID, Key: "nameless", YAML: []byte("not a dungeon")},
	} {
		_, err := s.world.Put(s.ctx, in)
		s.Require().NoError(err)
	}

	list, err := r.List(s.ctx, alice)
	s.Require().NoError(err)
	s.Equal(dungeons.Summary{Key: "broken-keep", Name: "The Broken Keep", Owner: alice.PlayerID, Source: dungeons.SourceWorld}, list[0])
	s.Equal(dungeons.Summary{Key: "nameless", Name: "nameless", Owner: alice.PlayerID, Source: dungeons.SourceWorld}, list[1])
}

func (s *StoresSuite) TestPutUnderAShippedKeyIsTheReferenceRefusal() {
	r := s.registry(true)
	for _, validateOnly := range []bool{true, false} {
		_, err := r.Put(s.ctx, &dungeons.PutInput{
			Caller: alice, Key: castleKitchen, YAML: s.kitchen, ValidateOnly: validateOnly,
		})
		s.ErrorIsf(err, dungeons.ErrReferenceKey, "validate-only %v", validateOnly)
	}
	_, err := s.world.Get(s.ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: castleKitchen})
	s.ErrorIs(err, worlddungeons.ErrNotFound, "nothing was written")
}

func (s *StoresSuite) TestPutUnderAnotherOwnersKeyIsRefused() {
	r := s.registry(true)
	raw := s.save(r, alice, storeMyCastle)
	bobs := append(s.file(storeMyCastle), []byte("# bob's\n")...)

	for _, validateOnly := range []bool{true, false} {
		_, err := r.Put(s.ctx, &dungeons.PutInput{
			Caller: bob, Key: storeMyCastle, YAML: bobs, ValidateOnly: validateOnly,
		})
		s.ErrorIsf(err, dungeons.ErrOwnedByOther, "validate-only %v", validateOnly)
	}
	s.Equal(worlddungeons.Record{Key: storeMyCastle, Owner: alice.PlayerID, Name: "The Castle Kitchen", YAML: raw},
		s.stored(alice.WorldID, storeMyCastle))
}

func (s *StoresSuite) TestPutReplacesTheCallersOwn() {
	r := s.registry(true)
	s.save(r, alice, storeMyCastle)
	second := append(s.file(storeMyCastle), []byte("# second save\n")...)

	out, err := r.Put(s.ctx, &dungeons.PutInput{Caller: alice, Key: storeMyCastle, YAML: second})
	s.Require().NoError(err)
	s.Require().NotNil(out.Entry)
	s.Equal(dungeons.SourceWorld, out.Entry.Source)
	s.Equal(alice.PlayerID, out.Entry.Owner)
	s.Equal(second, s.stored(alice.WorldID, storeMyCastle).YAML)
}

func (s *StoresSuite) TestPutWithoutAuthoringAnswersTheGradeOnly() {
	r := s.registry(false)

	out, err := r.Put(s.ctx, &dungeons.PutInput{Caller: alice, Key: storeMyCastle, YAML: s.file(storeMyCastle), ValidateOnly: true})
	s.Require().NoError(err)
	s.Require().NotNil(out.Entry, "the grade is answered")
	s.Require().NotNil(out.Entry.Atlas)

	_, err = r.Put(s.ctx, &dungeons.PutInput{Caller: alice, Key: storeMyCastle, YAML: s.file(storeMyCastle)})
	s.ErrorIs(err, dungeons.ErrAuthoringDisabled)
	_, err = s.world.Get(s.ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: storeMyCastle})
	s.ErrorIs(err, worlddungeons.ErrNotFound, "and nothing was written")
}

func (s *StoresSuite) TestGetResolvesWorldThenShipped() {
	r := s.registry(true)
	raw := s.save(r, alice, storeMyCastle)

	for _, caller := range []dungeons.Caller{alice, bob} {
		e, err := r.Get(s.ctx, caller, storeMyCastle)
		s.Require().NoError(err)
		s.Equal(dungeons.SourceWorld, e.Source)
		s.Equal(alice.PlayerID, e.Owner, "the key names one thing in the world, whoever asks")
		s.Equal(raw, e.YAML)
		s.Require().NotNil(e.Atlas)

		ref, err := r.Get(s.ctx, caller, castleKitchen)
		s.Require().NoError(err)
		s.Equal(dungeons.SourceShipped, ref.Source)
		s.Empty(ref.Owner)
		s.Equal(s.kitchen, ref.YAML)
	}

	_, err := r.Get(s.ctx, carol, storeMyCastle)
	s.ErrorIs(err, dungeons.ErrNotFound, "another world does not hold it")
}

func (s *StoresSuite) TestTwoWorldsHoldTheSameKey() {
	r := s.registry(true)
	aliceRaw := s.save(r, alice, storeMyCastle)
	carolRaw := append(s.file(storeMyCastle), []byte("# carol's\n")...)
	out, err := r.Put(s.ctx, &dungeons.PutInput{Caller: carol, Key: storeMyCastle, YAML: carolRaw})
	s.Require().NoError(err)
	s.Require().Empty(out.Errors)

	a, err := r.Get(s.ctx, alice, storeMyCastle)
	s.Require().NoError(err)
	s.Equal(aliceRaw, a.YAML)
	s.Equal(alice.PlayerID, a.Owner)
	c, err := r.Get(s.ctx, carol, storeMyCastle)
	s.Require().NoError(err)
	s.Equal(carolRaw, c.YAML)
	s.Equal(carol.PlayerID, c.Owner)
}

func (s *StoresSuite) TestListIsTheWorldThenSingleRoomReferences() {
	r := s.registry(true)
	s.save(r, alice, storeMyCastle)
	s.save(r, bob, storeBobKeep)

	list, err := r.List(s.ctx, alice)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(list), 3)
	s.Equal(dungeons.Summary{Key: storeBobKeep, Name: "The Castle Kitchen", Owner: bob.PlayerID, Source: dungeons.SourceWorld}, list[0])
	s.Equal(dungeons.Summary{Key: storeMyCastle, Name: "The Castle Kitchen", Owner: alice.PlayerID, Source: dungeons.SourceWorld}, list[1])
	s.Contains(list, dungeons.Summary{Key: castleKitchen, Name: "The Castle Kitchen", Source: dungeons.SourceShipped})

	refs := make([]string, 0, len(list)-2)
	for _, sum := range list[2:] {
		s.Equal(dungeons.SourceShipped, sum.Source)
		s.Empty(sum.Owner)
		refs = append(refs, sum.Key)
	}
	s.IsIncreasing(refs, "references sorted by key")
	s.Contains(refs, "reference-raider-camp", "the rebuilt current-format camp is an editable reference")
	for _, v2 := range []string{storeTomb, "reference-front-room", "reference-hearing", "reference-minds", "reference-tomb-heirloom"} {
		s.NotContains(refs, v2, "no version 2 reference is listed (R23)")
	}

	carols, err := r.List(s.ctx, carol)
	s.Require().NoError(err)
	for _, sum := range carols {
		s.Equal(dungeons.SourceShipped, sum.Source, "carol's world holds neither record: %s", sum.Key)
	}
}

func (s *StoresSuite) TestListLeavesOutAReferenceHeldInTheWorld() {
	r := s.registry(true)
	_, err := s.world.Put(s.ctx, &worlddungeons.PutInput{
		WorldID: alice.WorldID, Owner: alice.PlayerID, Key: castleKitchen, YAML: s.kitchen,
	})
	s.Require().NoError(err)

	list, err := r.List(s.ctx, bob)
	s.Require().NoError(err)
	held := 0
	for _, sum := range list {
		if sum.Key == castleKitchen {
			held++
			s.Equal(dungeons.SourceWorld, sum.Source)
			s.Equal(alice.PlayerID, sum.Owner)
		}
	}
	s.Equal(1, held, "the world's record, and not the reference too")
}

func (s *StoresSuite) TestGetStillResolvesAVersionTwoReference() {
	r := s.registry(false)
	e, err := r.Get(s.ctx, alice, storeTomb)
	s.Require().NoError(err)
	s.Equal(dungeons.SourceShipped, e.Source)
	s.False(e.SingleRoom)
}

func (s *StoresSuite) TestRenameRewritesOnlyTheKeyLine() {
	r := s.registry(true)
	raw := s.save(r, alice, storeMyCastle)

	s.Require().NoError(r.Rename(s.ctx, &dungeons.RenameInput{Caller: alice, Key: storeMyCastle, NewKey: storeMyKeep}))

	got := s.stored(alice.WorldID, storeMyKeep)
	s.Equal(alice.PlayerID, got.Owner)
	s.Equal("The Castle Kitchen", got.Name, "the stored name moves with the record")
	want := bytes.Replace(raw, []byte("\nkey: my-castle\n"), []byte("\nkey: my-keep\n"), 1)
	s.Equal(want, got.YAML, "every other byte unchanged")
	_, err := s.world.Get(s.ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: storeMyCastle})
	s.ErrorIs(err, worlddungeons.ErrNotFound)

	e, err := r.Get(s.ctx, alice, storeMyKeep)
	s.Require().NoError(err)
	s.Equal(storeMyKeep, e.Key)
}

func (s *StoresSuite) TestRenameRefusals() {
	r := s.registry(true)
	s.save(r, alice, storeMyCastle)
	s.save(r, alice, storeMyKeep)
	s.save(r, bob, storeBobKeep)

	cases := []struct {
		name      string
		registry  dungeons.Registry
		key, next string
		want      error
	}{
		{"another owner's source", r, storeBobKeep, "fresh-key", dungeons.ErrOwnedByOther},
		{"own new key", r, storeMyCastle, storeMyKeep, dungeons.ErrKeyExists},
		{"same key", r, storeMyCastle, storeMyCastle, dungeons.ErrKeyExists},
		{"another owner's new key", r, storeMyCastle, storeBobKeep, dungeons.ErrOwnedByOther},
		{"shipped new key", r, storeMyCastle, castleKitchen, dungeons.ErrReferenceKey},
		{"invalid new key", r, storeMyCastle, "My Keep", dungeons.ErrInvalidKey},
		{"shipped source", r, castleKitchen, "fresh-key", dungeons.ErrReferenceReadOnly},
		{"absent source", r, "absent", "fresh-key", dungeons.ErrNotFound},
		{"authoring off", s.registry(false), storeMyCastle, "fresh-key", dungeons.ErrAuthoringDisabled},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			before, err := s.world.List(s.ctx, &worlddungeons.ListInput{WorldID: alice.WorldID})
			s.Require().NoError(err)
			err = tc.registry.Rename(s.ctx, &dungeons.RenameInput{Caller: alice, Key: tc.key, NewKey: tc.next})
			s.ErrorIs(err, tc.want)
			after, err := s.world.List(s.ctx, &worlddungeons.ListInput{WorldID: alice.WorldID})
			s.Require().NoError(err)
			s.Equal(before, after, "nothing moved")
		})
	}
}

func (s *StoresSuite) TestDeleteRefusals() {
	r := s.registry(true)
	s.save(r, alice, storeMyCastle)
	s.save(r, bob, storeBobKeep)

	s.ErrorIs(r.Delete(s.ctx, alice, storeBobKeep), dungeons.ErrOwnedByOther)
	s.Equal(bob.PlayerID, s.stored(bob.WorldID, storeBobKeep).Owner, "bob's record stands")
	s.ErrorIs(r.Delete(s.ctx, alice, castleKitchen), dungeons.ErrReferenceReadOnly)
	s.ErrorIs(r.Delete(s.ctx, alice, "absent"), dungeons.ErrNotFound)
	s.ErrorIs(s.registry(false).Delete(s.ctx, alice, storeMyCastle), dungeons.ErrAuthoringDisabled)

	s.Require().NoError(r.Delete(s.ctx, alice, storeMyCastle))
	_, err := r.Get(s.ctx, alice, storeMyCastle)
	s.ErrorIs(err, dungeons.ErrNotFound, "own record gone")
}

func (s *StoresSuite) TestEveryVerbRefusesAnIncompleteCaller() {
	r := s.registry(true)
	for _, caller := range []dungeons.Caller{{PlayerID: "alice"}, {WorldID: "w1"}} {
		_, err := r.List(s.ctx, caller)
		s.ErrorIs(err, dungeons.ErrNoCaller)
		_, err = r.Get(s.ctx, caller, castleKitchen)
		s.ErrorIs(err, dungeons.ErrNoCaller, "never read as shipped only")
		_, err = r.Put(s.ctx, &dungeons.PutInput{Caller: caller, Key: storeMyCastle, YAML: s.file(storeMyCastle), ValidateOnly: true})
		s.ErrorIs(err, dungeons.ErrNoCaller)
		s.ErrorIs(r.Delete(s.ctx, caller, storeMyCastle), dungeons.ErrNoCaller)
		s.ErrorIs(r.Rename(s.ctx, &dungeons.RenameInput{Caller: caller, Key: storeMyCastle, NewKey: storeMyKeep}), dungeons.ErrNoCaller)
	}
}
