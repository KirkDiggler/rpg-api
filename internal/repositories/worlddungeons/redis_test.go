package worlddungeons

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
)

const (
	worldOne  = "w1"
	worldTwo  = "w2"
	alice     = "alice"
	bob       = "bob"
	myCastle  = "my-castle"
	myKeep    = "my-keep"
	bobKeep   = "bob-keep"
	worldHash = "dungeon:world:w1"
)

var castleBytes = []byte("version: 4\nkey: my-castle   \n# a comment that must survive\nname: My Castle  \n")

type RedisSuite struct {
	suite.Suite
	server *miniredis.Miniredis
	client *goredis.Client
	repo   Repository
	ctx    context.Context
}

func TestRedisSuite(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

func (s *RedisSuite) SetupTest() {
	s.server = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	repo, err := NewRedis(&RedisConfig{Client: s.client})
	s.Require().NoError(err)
	s.repo = repo
	s.ctx = context.Background()
}

func (s *RedisSuite) TearDownTest() {
	_ = s.client.Close()
}

func (s *RedisSuite) put(world, owner, key string, yaml []byte) error {
	_, err := s.repo.Put(s.ctx, &PutInput{WorldID: world, Owner: owner, Key: key, YAML: yaml})
	return err
}

func (s *RedisSuite) get(world, key string) Record {
	out, err := s.repo.Get(s.ctx, &GetInput{WorldID: world, Key: key})
	s.Require().NoError(err)
	return out.Record
}

func (s *RedisSuite) hashSnapshot(world string) map[string]string {
	fields, err := s.client.HGetAll(s.ctx, "dungeon:world:"+world).Result()
	s.Require().NoError(err)
	return fields
}

func keepRewrite(old []byte) ([]byte, error) {
	return append(old, []byte("# renamed\n")...), nil
}

func (s *RedisSuite) TestPutGetIsVerbatimWithOwner() {
	in := append([]byte(nil), castleBytes...)
	s.Require().NoError(s.put(worldOne, alice, myCastle, in))
	in[0] = 'X' // the store copied the bytes in

	got := s.get(worldOne, myCastle)
	s.Equal(Record{Key: myCastle, Owner: alice, YAML: castleBytes}, got)
	got.YAML[0] = 'Y' // and copies them out
	s.Equal(castleBytes, s.get(worldOne, myCastle).YAML)
}

func (s *RedisSuite) TestPutReplacesTheOwnersOwn() {
	s.Require().NoError(s.put(worldOne, alice, myCastle, castleBytes))
	next := []byte("version: 4\nkey: my-castle\nname: Second\n")
	s.Require().NoError(s.put(worldOne, alice, myCastle, next))
	s.Equal(Record{Key: myCastle, Owner: alice, YAML: next}, s.get(worldOne, myCastle))
}

func (s *RedisSuite) TestKeysAreUniquePerWorld() {
	s.Require().NoError(s.put(worldOne, alice, myCastle, castleBytes))

	err := s.put(worldOne, bob, myCastle, []byte("bob's"))
	s.ErrorIs(err, ErrOwnedByOther)
	s.Equal(Record{Key: myCastle, Owner: alice, YAML: castleBytes}, s.get(worldOne, myCastle))

	s.Require().NoError(s.put(worldTwo, bob, myCastle, []byte("bob's")))
	s.Equal(Record{Key: myCastle, Owner: alice, YAML: castleBytes}, s.get(worldOne, myCastle))
	s.Equal(Record{Key: myCastle, Owner: bob, YAML: []byte("bob's")}, s.get(worldTwo, myCastle))
}

func (s *RedisSuite) TestHashShape() {
	_, err := s.repo.Put(s.ctx, &PutInput{WorldID: worldOne, Owner: alice, Key: myCastle, Name: "My Castle", YAML: castleBytes})
	s.Require().NoError(err)
	raw := s.server.HGet(worldHash, myCastle)
	var decoded map[string]string
	s.Require().NoError(json.Unmarshal([]byte(raw), &decoded))
	s.Equal(map[string]string{"owner": alice, "name": "My Castle", "yaml": string(castleBytes)}, decoded)
	s.Equal("My Castle", s.get(worldOne, myCastle).Name, "the stored name reads back")

	s.Require().NoError(s.put(worldOne, alice, myKeep, []byte("k")))
	s.NotContains(s.server.HGet(worldHash, myKeep), `"name"`, "an empty name is stored as absent")
	s.Empty(s.get(worldOne, myKeep).Name)
}

func (s *RedisSuite) TestListSortedAndEmptyForNewWorld() {
	s.Require().NoError(s.put(worldOne, alice, myCastle, []byte("c")))
	s.Require().NoError(s.put(worldOne, bob, bobKeep, []byte("b")))
	s.Require().NoError(s.put(worldOne, alice, "a-first", []byte("a")))

	out, err := s.repo.List(s.ctx, &ListInput{WorldID: worldOne})
	s.Require().NoError(err)
	s.Equal([]Record{
		{Key: "a-first", Owner: alice, YAML: []byte("a")},
		{Key: bobKeep, Owner: bob, YAML: []byte("b")},
		{Key: myCastle, Owner: alice, YAML: []byte("c")},
	}, out.Records)

	empty, err := s.repo.List(s.ctx, &ListInput{WorldID: "new-world"})
	s.Require().NoError(err)
	s.NotNil(empty.Records)
	s.Empty(empty.Records)
}

func (s *RedisSuite) TestDeleteOwnAnotherOwnersAndAbsent() {
	s.Require().NoError(s.put(worldOne, alice, myCastle, castleBytes))
	s.Require().NoError(s.put(worldOne, bob, bobKeep, []byte("b")))

	_, err := s.repo.Delete(s.ctx, &DeleteInput{WorldID: worldOne, Owner: alice, Key: bobKeep})
	s.ErrorIs(err, ErrOwnedByOther)
	s.Equal(Record{Key: bobKeep, Owner: bob, YAML: []byte("b")}, s.get(worldOne, bobKeep))

	_, err = s.repo.Delete(s.ctx, &DeleteInput{WorldID: worldOne, Owner: alice, Key: "absent"})
	s.ErrorIs(err, ErrNotFound)

	_, err = s.repo.Delete(s.ctx, &DeleteInput{WorldID: worldOne, Owner: alice, Key: myCastle})
	s.Require().NoError(err)
	_, err = s.repo.Get(s.ctx, &GetInput{WorldID: worldOne, Key: myCastle})
	s.ErrorIs(err, ErrNotFound)
}

func (s *RedisSuite) TestRenameMovesRewrittenBytesAndKeepsTheOwner() {
	_, err := s.repo.Put(s.ctx, &PutInput{WorldID: worldOne, Owner: alice, Key: myCastle, Name: "My Castle", YAML: castleBytes})
	s.Require().NoError(err)
	var seen []byte
	_, err = s.repo.Rename(s.ctx, &RenameInput{
		WorldID: worldOne, Owner: alice, Key: myCastle, NewKey: myKeep,
		Rewrite: func(old []byte) ([]byte, error) {
			seen = append([]byte(nil), old...)
			return []byte("rewritten"), nil
		},
	})
	s.Require().NoError(err)
	s.Equal(castleBytes, seen)
	s.Equal(Record{Key: myKeep, Owner: alice, Name: "My Castle", YAML: []byte("rewritten")}, s.get(worldOne, myKeep),
		"the owner and the stored name move with the record")
	_, err = s.repo.Get(s.ctx, &GetInput{WorldID: worldOne, Key: myCastle})
	s.ErrorIs(err, ErrNotFound)
}

func (s *RedisSuite) TestRenameRefusals() {
	s.Require().NoError(s.put(worldOne, alice, myCastle, castleBytes))
	s.Require().NoError(s.put(worldOne, alice, myKeep, []byte("keep")))
	s.Require().NoError(s.put(worldOne, bob, bobKeep, []byte("b")))
	before := s.hashSnapshot(worldOne)

	cases := []struct {
		name   string
		owner  string
		key    string
		newKey string
		want   error
	}{
		{"own new key", alice, myCastle, myKeep, ErrKeyExists},
		{"same key", alice, myCastle, myCastle, ErrKeyExists},
		{"another owner's new key", alice, myCastle, bobKeep, ErrOwnedByOther},
		{"another owner's source", alice, bobKeep, "fresh", ErrOwnedByOther},
		{"absent source", alice, "absent", "fresh", ErrNotFound},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			_, err := s.repo.Rename(s.ctx, &RenameInput{
				WorldID: worldOne, Owner: tc.owner, Key: tc.key, NewKey: tc.newKey, Rewrite: keepRewrite,
			})
			s.ErrorIs(err, tc.want)
			s.Equal(before, s.hashSnapshot(worldOne))
		})
	}
}

func (s *RedisSuite) TestRenameRewriteErrorWritesNothing() {
	s.Require().NoError(s.put(worldOne, alice, myCastle, castleBytes))
	before := s.hashSnapshot(worldOne)
	refusal := errors.New("no key line")
	_, err := s.repo.Rename(s.ctx, &RenameInput{
		WorldID: worldOne, Owner: alice, Key: myCastle, NewKey: myKeep,
		Rewrite: func([]byte) ([]byte, error) { return nil, refusal },
	})
	s.ErrorIs(err, refusal)
	s.Equal(before, s.hashSnapshot(worldOne))
}

func (s *RedisSuite) TestWritesRetryWhenTheHashChanges() {
	s.Require().NoError(s.put(worldOne, alice, myCastle, castleBytes))
	other := goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	defer func() { _ = other.Close() }()

	calls := 0
	_, err := s.repo.Rename(s.ctx, &RenameInput{
		WorldID: worldOne, Owner: alice, Key: myCastle, NewKey: myKeep,
		Rewrite: func(old []byte) ([]byte, error) {
			calls++
			if calls == 1 {
				value, encodeErr := encodeRecord(bob, "", []byte("interloper"))
				s.Require().NoError(encodeErr)
				s.Require().NoError(other.HSet(s.ctx, worldHash, "unrelated", value).Err())
			}
			return old, nil
		},
	})
	s.Require().NoError(err)
	s.Equal(2, calls)
	s.Equal(Record{Key: myKeep, Owner: alice, YAML: castleBytes}, s.get(worldOne, myKeep))
	s.Equal(Record{Key: "unrelated", Owner: bob, YAML: []byte("interloper")}, s.get(worldOne, "unrelated"))
	_, err = s.repo.Get(s.ctx, &GetInput{WorldID: worldOne, Key: myCastle})
	s.ErrorIs(err, ErrNotFound)
}

func (s *RedisSuite) TestConcurrentPutsOfOneNewKey() {
	second := goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	defer func() { _ = second.Close() }()
	bobRepo, err := NewRedis(&RedisConfig{Client: second})
	s.Require().NoError(err)

	for round := 0; round < 50; round++ {
		s.server.FlushAll()
		var wg sync.WaitGroup
		var aliceErr, bobErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, aliceErr = s.repo.Put(s.ctx, &PutInput{WorldID: worldOne, Owner: alice, Key: myCastle, YAML: []byte("a")})
		}()
		go func() {
			defer wg.Done()
			_, bobErr = bobRepo.Put(s.ctx, &PutInput{WorldID: worldOne, Owner: bob, Key: myCastle, YAML: []byte("b")})
		}()
		wg.Wait()

		holder := s.get(worldOne, myCastle).Owner
		switch holder {
		case alice:
			s.Require().NoError(aliceErr, "round %d", round)
			s.Require().ErrorIs(bobErr, ErrOwnedByOther, "round %d", round)
		case bob:
			s.Require().NoError(bobErr, "round %d", round)
			s.Require().ErrorIs(aliceErr, ErrOwnedByOther, "round %d", round)
		default:
			s.FailNow("unexpected holder", "round %d holder %q", round, holder)
		}
	}
}

func (s *RedisSuite) TestValidationDecodeAndCommandErrors() {
	_, err := s.repo.Put(s.ctx, &PutInput{Owner: alice, Key: myCastle})
	s.True(apierr.IsInvalidArgument(err), "empty world: %v", err)
	_, err = s.repo.Put(s.ctx, &PutInput{WorldID: worldOne, Key: myCastle})
	s.True(apierr.IsInvalidArgument(err), "empty owner: %v", err)
	_, err = s.repo.Put(s.ctx, &PutInput{WorldID: worldOne, Owner: alice})
	s.True(apierr.IsInvalidArgument(err), "empty key: %v", err)
	_, err = s.repo.Get(s.ctx, &GetInput{WorldID: worldOne})
	s.True(apierr.IsInvalidArgument(err), "get empty key: %v", err)
	_, err = s.repo.List(s.ctx, &ListInput{})
	s.True(apierr.IsInvalidArgument(err), "list empty world: %v", err)
	_, err = s.repo.Delete(s.ctx, &DeleteInput{WorldID: worldOne, Key: myCastle})
	s.True(apierr.IsInvalidArgument(err), "delete empty owner: %v", err)
	_, err = s.repo.Rename(s.ctx, &RenameInput{WorldID: worldOne, Owner: alice, Key: myCastle, Rewrite: keepRewrite})
	s.True(apierr.IsInvalidArgument(err), "rename empty new key: %v", err)
	_, err = s.repo.Put(s.ctx, &PutInput{WorldID: worldOne, Owner: alice, Key: myCastle, YAML: []byte{0xff, 0xfe}})
	s.True(apierr.IsInvalidArgument(err), "non-UTF-8 bytes: %v", err)

	s.server.HSet(worldHash, "broken", "not json")
	_, err = s.repo.Get(s.ctx, &GetInput{WorldID: worldOne, Key: "broken"})
	s.True(apierr.IsInternal(err), "decode: %v", err)
	s.ErrorContains(err, "broken")
	s.ErrorContains(err, worldOne)
	_, err = s.repo.List(s.ctx, &ListInput{WorldID: worldOne})
	s.True(apierr.IsInternal(err), "list decode: %v", err)

	s.server.Close()
	_, err = s.repo.Get(s.ctx, &GetInput{WorldID: worldOne, Key: myCastle})
	s.Error(err)
	s.ErrorContains(err, myCastle)
	err = s.put(worldOne, alice, myCastle, castleBytes)
	s.Error(err)
}
