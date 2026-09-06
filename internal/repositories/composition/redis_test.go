package composition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	redismocks "github.com/KirkDiggler/rpg-api/internal/redis/mocks"
)

type RedisSuite struct {
	suite.Suite
	server *miniredis.Miniredis
	client *goredis.Client
	repo   Repository
}

func (s *RedisSuite) SetupTest() {
	s.server = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	repo, err := NewRedis(&RedisConfig{Client: s.client})
	s.Require().NoError(err)
	s.repo = repo
}

func (s *RedisSuite) TearDownTest() {
	s.Require().NoError(s.client.Close())
}

func (s *RedisSuite) TestCreateGetListRoundTripAndNoTTL() {
	ctx := context.Background()
	first := data("world-a", "composition-b", `{"name":"table","count":2}`)
	second := data("world-a", "composition-a", `{"name":"chairs"}`)

	created, err := s.repo.Create(ctx, &CreateInput{Composition: first})
	s.Require().NoError(err)
	s.Equal(first, created.Composition)
	_, err = s.repo.Create(ctx, &CreateInput{Composition: second})
	s.Require().NoError(err)

	got, err := s.repo.Get(ctx, &GetInput{WorldID: first.WorldID, ID: first.ID})
	s.Require().NoError(err)
	s.Equal(first, got.Composition)

	listed, err := s.repo.List(ctx, &ListInput{WorldID: first.WorldID})
	s.Require().NoError(err)
	s.Equal([]*worldcomposition.Data{second, first}, listed.Compositions)

	keys := s.server.Keys()
	s.Require().Len(keys, 1)
	s.Equal(time.Duration(0), s.server.TTL(keys[0]))
	fields, err := s.server.HKeys(keys[0])
	s.Require().NoError(err)
	s.ElementsMatch([]string{first.ID, second.ID}, fields)
}

func (s *RedisSuite) TestWorldIsolationAllowsTheSameCompositionID() {
	ctx := context.Background()
	worldA := data("world-a", "shared-id", `{"world":"a"}`)
	worldB := data("world-b", "shared-id", `{"world":"b"}`)

	_, err := s.repo.Create(ctx, &CreateInput{Composition: worldA})
	s.Require().NoError(err)
	_, err = s.repo.Create(ctx, &CreateInput{Composition: worldB})
	s.Require().NoError(err)

	gotA, err := s.repo.Get(ctx, &GetInput{WorldID: worldA.WorldID, ID: worldA.ID})
	s.Require().NoError(err)
	gotB, err := s.repo.Get(ctx, &GetInput{WorldID: worldB.WorldID, ID: worldB.ID})
	s.Require().NoError(err)
	s.Equal(worldA, gotA.Composition)
	s.Equal(worldB, gotB.Composition)

	listA, err := s.repo.List(ctx, &ListInput{WorldID: worldA.WorldID})
	s.Require().NoError(err)
	listB, err := s.repo.List(ctx, &ListInput{WorldID: worldB.WorldID})
	s.Require().NoError(err)
	s.Equal([]*worldcomposition.Data{worldA}, listA.Compositions)
	s.Equal([]*worldcomposition.Data{worldB}, listB.Compositions)
	s.Len(s.server.Keys(), 2)
}

func (s *RedisSuite) TestDuplicateCreateDoesNotOverwrite() {
	ctx := context.Background()
	original := data("world-a", "composition-a", `{"value":"original"}`)
	replacement := data("world-a", "composition-a", `{"value":"replacement"}`)

	_, err := s.repo.Create(ctx, &CreateInput{Composition: original})
	s.Require().NoError(err)
	_, err = s.repo.Create(ctx, &CreateInput{Composition: replacement})
	s.Require().Error(err)
	s.True(apierr.IsAlreadyExists(err), "got %v", err)

	got, err := s.repo.Get(ctx, &GetInput{WorldID: original.WorldID, ID: original.ID})
	s.Require().NoError(err)
	s.Equal(original, got.Composition)
}

func (s *RedisSuite) TestAbsentGetAndEmptyList() {
	got, err := s.repo.Get(context.Background(), &GetInput{WorldID: "world-a", ID: "missing"})
	s.Require().Error(err)
	s.Nil(got)
	s.True(apierr.IsNotFound(err), "got %v", err)

	listed, err := s.repo.List(context.Background(), &ListInput{WorldID: "world-a"})
	s.Require().NoError(err)
	s.NotNil(listed.Compositions)
	s.Empty(listed.Compositions)
}

func (s *RedisSuite) TestPersistedSnapshotsDoNotAliasCallerOrOutputs() {
	ctx := context.Background()
	input := data("world-a", "composition-a", `{"name":"original"}`)
	created, err := s.repo.Create(ctx, &CreateInput{Composition: input})
	s.Require().NoError(err)

	input.JSON[9] = 'X'
	created.Composition.JSON[9] = 'Y'
	firstGet, err := s.repo.Get(ctx, &GetInput{WorldID: "world-a", ID: "composition-a"})
	s.Require().NoError(err)
	s.JSONEq(`{"name":"original"}`, string(firstGet.Composition.JSON))

	firstGet.Composition.JSON[9] = 'Z'
	secondGet, err := s.repo.Get(ctx, &GetInput{WorldID: "world-a", ID: "composition-a"})
	s.Require().NoError(err)
	s.JSONEq(`{"name":"original"}`, string(secondGet.Composition.JSON))
}

func (s *RedisSuite) TestCorruptStoredDataReturnsInternal() {
	ctx := context.Background()
	key := compositionKey("world-a")

	s.server.HSet(key, "composition-a", `{not-json`)
	_, err := s.repo.Get(ctx, &GetInput{WorldID: "world-a", ID: "composition-a"})
	s.Require().Error(err)
	s.True(apierr.IsInternal(err), "got %v", err)

	s.server.HSet(key, "composition-a", `{"id":"different-id","world_id":"world-a","json":{}}`)
	_, err = s.repo.List(ctx, &ListInput{WorldID: "world-a"})
	s.Require().Error(err)
	s.True(apierr.IsInternal(err), "got %v", err)
}

func TestRedisSuite(t *testing.T) {
	suite.Run(t, new(RedisSuite))
}

func TestNewRedisValidation(t *testing.T) {
	t.Parallel()

	_, err := NewRedis(nil)
	require.Error(t, err)
	require.True(t, apierr.IsInvalidArgument(err), "got %v", err)

	_, err = NewRedis(&RedisConfig{})
	require.Error(t, err)
	require.True(t, apierr.IsInvalidArgument(err), "got %v", err)
}

func TestOperationInputValidationAndMarshalError(t *testing.T) {
	t.Parallel()

	client := goredis.NewClient(&goredis.Options{Addr: "unused:6379"})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	repo, err := NewRedis(&RedisConfig{Client: client})
	require.NoError(t, err)

	tests := map[string]func() error{
		"create input nil": func() error {
			_, callErr := repo.Create(context.Background(), nil)
			return callErr
		},
		"create composition nil": func() error {
			_, callErr := repo.Create(context.Background(), &CreateInput{})
			return callErr
		},
		"create world missing": func() error {
			_, callErr := repo.Create(context.Background(), &CreateInput{Composition: data("", "id", `{}`)})
			return callErr
		},
		"create id missing": func() error {
			_, callErr := repo.Create(context.Background(), &CreateInput{Composition: data("world", "", `{}`)})
			return callErr
		},
		"get input nil": func() error {
			_, callErr := repo.Get(context.Background(), nil)
			return callErr
		},
		"get world missing": func() error {
			_, callErr := repo.Get(context.Background(), &GetInput{ID: "id"})
			return callErr
		},
		"get id missing": func() error {
			_, callErr := repo.Get(context.Background(), &GetInput{WorldID: "world"})
			return callErr
		},
		"list input nil": func() error {
			_, callErr := repo.List(context.Background(), nil)
			return callErr
		},
		"list world missing": func() error {
			_, callErr := repo.List(context.Background(), &ListInput{})
			return callErr
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.True(t, apierr.IsInvalidArgument(test()), name)
		})
	}

	_, err = repo.Create(context.Background(), &CreateInput{Composition: data("world", "id", `{`)})
	require.Error(t, err)
	require.True(t, apierr.IsInternal(err), "got %v", err)
}

func TestRedisCommandErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	redisErr := errors.New("redis unavailable")

	tests := map[string]struct {
		expect func(*redismocks.MockClient)
		call   func(Repository) error
	}{
		"create": {
			expect: func(client *redismocks.MockClient) {
				client.EXPECT().
					HSetNX(gomock.Any(), gomock.Any(), "id", gomock.Any()).
					Return(goredis.NewBoolResult(false, redisErr))
			},
			call: func(repo Repository) error {
				_, err := repo.Create(ctx, &CreateInput{Composition: data("world", "id", `{}`)})
				return err
			},
		},
		"get": {
			expect: func(client *redismocks.MockClient) {
				client.EXPECT().HGet(gomock.Any(), gomock.Any(), "id").Return(goredis.NewStringResult("", redisErr))
			},
			call: func(repo Repository) error {
				_, err := repo.Get(ctx, &GetInput{WorldID: "world", ID: "id"})
				return err
			},
		},
		"list": {
			expect: func(client *redismocks.MockClient) {
				client.EXPECT().
					HGetAll(gomock.Any(), gomock.Any()).
					Return(goredis.NewMapStringStringResult(nil, redisErr))
			},
			call: func(repo Repository) error {
				_, err := repo.List(ctx, &ListInput{WorldID: "world"})
				return err
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := redismocks.NewMockClient(ctrl)
			test.expect(client)
			repo, err := NewRedis(&RedisConfig{Client: redisclient.Client(client)})
			require.NoError(t, err)
			err = test.call(repo)
			require.ErrorIs(t, err, redisErr)
			require.True(t, apierr.IsInternal(err), "got %v", err)
		})
	}
}

func data(worldID, id, raw string) *worldcomposition.Data {
	return &worldcomposition.Data{WorldID: worldID, ID: id, JSON: []byte(raw)}
}
