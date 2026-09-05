package composition

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

var fixedNow = time.Date(2026, time.September, 5, 12, 34, 56, 789, time.UTC)

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type queuedGenerator struct {
	mu  sync.Mutex
	ids []string
}

func (g *queuedGenerator) Generate() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.ids) == 0 {
		return ""
	}
	id := g.ids[0]
	g.ids = g.ids[1:]
	return id
}

func TestRedisConfigRequiresEveryDependency(t *testing.T) {
	t.Parallel()

	validClient := redisclient.Client(goredis.NewClient(&goredis.Options{Addr: "unused:6379"}))
	t.Cleanup(func() { _ = validClient.Close() })
	validClock := clock.Clock(fixedClock{now: fixedNow})
	validGenerator := idgen.Generator(&queuedGenerator{ids: []string{"id"}})

	tests := map[string]*RedisConfig{
		"nil config":    nil,
		"nil client":    {Clock: validClock, IDGenerator: validGenerator},
		"nil clock":     {Client: validClient, IDGenerator: validGenerator},
		"nil generator": {Client: validClient, Clock: validClock},
	}
	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewRedis(config)
			require.Error(t, err)
			require.True(t, apierr.IsInvalidArgument(err), "got %v", err)
		})
	}
}

func TestRedisCreateGetAndListRoundTrip(t *testing.T) {
	server, client, repo := newTestRepository(t, "definition-b", "revision-b", "definition-a", "revision-a")
	ctx := context.Background()

	first, err := repo.CreateDefinition(ctx, CreateDefinitionInput{
		GuildID: "guild-one", CreatedByPlayerID: "player-one", Source: validSource("first"),
	})
	require.NoError(t, err)
	require.Equal(t, "definition-b", first.Definition.ID)
	require.Equal(t, "revision-b", first.Definition.HeadRevisionID)
	require.Equal(t, fixedNow, first.Definition.CreatedAt)
	require.Equal(t, first.Definition.ID, first.Revision.DefinitionID)
	require.Equal(t, "dnd5e:props:table", first.Revision.Source.Items[0].AssetRef)

	secondSource := validSource("second")
	secondSource.Items[0].AssetRef = "synty:props:dungeon:table:stretch"
	second, err := repo.CreateDefinition(ctx, CreateDefinitionInput{
		GuildID: "guild-one", CreatedByPlayerID: "player-two", Source: secondSource,
	})
	require.NoError(t, err)
	require.Equal(t, "synty:props:dungeon:table:stretch", second.Revision.Source.Items[0].AssetRef)

	got, err := repo.GetRevision(ctx, GetRevisionInput{
		GuildID: "guild-one", DefinitionID: first.Definition.ID, RevisionID: first.Revision.ID,
	})
	require.NoError(t, err)
	require.Equal(t, first.Revision, got.Revision)

	listed, err := repo.ListDefinitions(ctx, ListDefinitionsInput{GuildID: "guild-one"})
	require.NoError(t, err)
	require.Equal(t, []string{"definition-a", "definition-b"}, []string{
		listed.Definitions[0].Definition.ID,
		listed.Definitions[1].Definition.ID,
	})
	require.Equal(t, "revision-a", listed.Definitions[0].Revision.ID)
	require.Equal(t, "revision-b", listed.Definitions[1].Revision.ID)

	for _, key := range []string{
		definitionKey("guild-one", first.Definition.ID),
		revisionKey("guild-one", first.Revision.ID),
		definitionIndexKey("guild-one"),
	} {
		require.Equal(t, time.Duration(0), server.TTL(key), "%s must not expire", key)
	}
	members, err := client.SMembers(ctx, definitionIndexKey("guild-one")).Result()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"definition-a", "definition-b"}, members)
}

func TestRedisGuildKeyFamilyUsesOneSafeHashTagPerGuild(t *testing.T) {
	t.Parallel()

	index := definitionIndexKey("guild{unsafe}")
	definition := definitionKey("guild{unsafe}", "definition")
	revision := revisionKey("guild{unsafe}", "revision")
	require.NotContains(t, index, "guild{unsafe}")
	require.Equal(t, hashTag(index), hashTag(definition))
	require.Equal(t, hashTag(index), hashTag(revision))
	require.NotEqual(t, hashTag(index), hashTag(definitionIndexKey("other-guild")))
}

func TestRedisGuildIsolationIncludesIdenticalGeneratedIDs(t *testing.T) {
	_, _, repo := newTestRepository(t,
		"definition-same", "revision-same",
		"definition-same", "revision-same",
	)
	ctx := context.Background()

	for _, guild := range []string{"guild-a", "guild-b"} {
		_, err := repo.CreateDefinition(ctx, CreateDefinitionInput{
			GuildID: guild, CreatedByPlayerID: "creator", Source: validSource(guild),
		})
		require.NoError(t, err)
	}

	for _, guild := range []string{"guild-a", "guild-b"} {
		got, err := repo.GetRevision(ctx, GetRevisionInput{
			GuildID: guild, DefinitionID: "definition-same", RevisionID: "revision-same",
		})
		require.NoError(t, err)
		require.Equal(t, guild, got.Revision.GuildID)
		require.Equal(t, guild, got.Revision.Source.Name)
	}

	_, err := repo.GetRevision(ctx, GetRevisionInput{
		GuildID: "guild-c", DefinitionID: "definition-same", RevisionID: "revision-same",
	})
	require.True(t, apierr.IsNotFound(err), "got %v", err)

	_, err = repo.GetRevision(ctx, GetRevisionInput{
		GuildID: "guild-a", DefinitionID: "wrong-definition", RevisionID: "revision-same",
	})
	require.True(t, apierr.IsNotFound(err), "wrong definition scope must not expose the revision: %v", err)
}

func TestRedisAppendPreservesOldRevisionAndRejectsStaleHead(t *testing.T) {
	_, _, repo := newTestRepository(t, "definition", "revision-1", "revision-2", "revision-stale")
	ctx := context.Background()
	created := mustCreate(t, repo, "guild", validSource("one"))

	appended, err := repo.AppendRevision(ctx, AppendRevisionInput{
		GuildID: "guild", DefinitionID: created.Definition.ID,
		ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player-two",
		Source: validSource("two"),
	})
	require.NoError(t, err)
	require.Equal(t, "revision-2", appended.Definition.HeadRevisionID)
	require.Equal(t, "player-one", appended.Definition.CreatedByPlayerID)
	require.Equal(t, fixedNow, appended.Definition.CreatedAt)
	require.Equal(t, "player-two", appended.Revision.CreatedByPlayerID)
	require.Equal(t, fixedNow, appended.Revision.CreatedAt)

	_, err = repo.AppendRevision(ctx, AppendRevisionInput{
		GuildID: "guild", DefinitionID: created.Definition.ID,
		ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player-three",
		Source: validSource("stale"),
	})
	require.True(t, apierr.IsAborted(err), "got %v", err)

	old, err := repo.GetRevision(ctx, GetRevisionInput{
		GuildID: "guild", DefinitionID: created.Definition.ID, RevisionID: created.Revision.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "one", old.Revision.Source.Name)
	current, err := repo.GetRevision(ctx, GetRevisionInput{
		GuildID: "guild", DefinitionID: created.Definition.ID, RevisionID: appended.Revision.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "two", current.Revision.Source.Name)

	listed, err := repo.ListDefinitions(ctx, ListDefinitionsInput{GuildID: "guild"})
	require.NoError(t, err)
	require.Equal(t, "revision-2", listed.Definitions[0].Definition.HeadRevisionID)
	require.Equal(t, "revision-2", listed.Definitions[0].Revision.ID)
}

func TestRedisConcurrentExpectedHeadAllowsExactlyOneWriter(t *testing.T) {
	_, _, repo := newTestRepository(t, "definition", "revision-1", "revision-a", "revision-b")
	ctx := context.Background()
	created := mustCreate(t, repo, "guild", validSource("one"))

	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, name := range []string{"two-a", "two-b"} {
		go func() {
			<-start
			_, err := repo.AppendRevision(ctx, AppendRevisionInput{
				GuildID: "guild", DefinitionID: created.Definition.ID,
				ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player",
				Source: validSource(name),
			})
			errs <- err
		}()
	}
	close(start)

	var successes, aborted int
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case apierr.IsAborted(err):
			aborted++
		default:
			t.Fatalf("unexpected append result: %v", err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, aborted)
}

func TestRedisAppendRejectsValidatedReadSetChangedBeforeScript(t *testing.T) {
	tests := map[string]struct {
		targetKey func(*CreateDefinitionOutput) string
		corrupt   func(*CreateDefinitionOutput) any
	}{
		"definition invalid creator": {
			targetKey: func(created *CreateDefinitionOutput) string {
				return definitionKey("guild", created.Definition.ID)
			},
			corrupt: func(created *CreateDefinitionOutput) any {
				stored := *created.Definition
				stored.CreatedByPlayerID = " invalid"
				return &stored
			},
		},
		"definition zero time": {
			targetKey: func(created *CreateDefinitionOutput) string {
				return definitionKey("guild", created.Definition.ID)
			},
			corrupt: func(created *CreateDefinitionOutput) any {
				stored := *created.Definition
				stored.CreatedAt = time.Time{}
				return &stored
			},
		},
		"head invalid creator": {
			targetKey: func(created *CreateDefinitionOutput) string {
				return revisionKey("guild", created.Revision.ID)
			},
			corrupt: func(created *CreateDefinitionOutput) any {
				stored := *created.Revision
				stored.CreatedByPlayerID = " invalid"
				return &stored
			},
		},
		"head zero time": {
			targetKey: func(created *CreateDefinitionOutput) string {
				return revisionKey("guild", created.Revision.ID)
			},
			corrupt: func(created *CreateDefinitionOutput) any {
				stored := *created.Revision
				stored.CreatedAt = time.Time{}
				return &stored
			},
		},
		"head invalid source": {
			targetKey: func(created *CreateDefinitionOutput) string {
				return revisionKey("guild", created.Revision.ID)
			},
			corrupt: func(created *CreateDefinitionOutput) any {
				stored := *created.Revision
				stored.Source.Version = sourceVersion + 1
				return &stored
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			server := miniredis.RunT(t)
			appendClient := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
			mutationClient := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
			t.Cleanup(func() {
				require.NoError(t, appendClient.Close())
				require.NoError(t, mutationClient.Close())
			})
			repo := mustNewRedis(t, appendClient, &queuedGenerator{
				ids: []string{"definition", "revision-1", "revision-2"},
			})
			created := mustCreate(t, repo, "guild", validSource("one"))
			ctx := context.Background()
			definitionBefore := mustRedisGet(t, appendClient, definitionKey("guild", created.Definition.ID))
			headBefore := mustRedisGet(t, appendClient, revisionKey("guild", created.Revision.ID))
			indexBefore := appendClient.SMembers(ctx, definitionIndexKey("guild")).Val()

			targetKey := test.targetKey(created)
			injectedJSON, err := json.Marshal(test.corrupt(created))
			require.NoError(t, err)
			hook := &beforeScriptHook{before: func() error {
				return mutationClient.Set(ctx, targetKey, injectedJSON, 0).Err()
			}}
			appendClient.AddHook(hook)

			_, err = repo.AppendRevision(ctx, AppendRevisionInput{
				GuildID: "guild", DefinitionID: created.Definition.ID,
				ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player-two",
				Source: validSource("two"),
			})
			require.True(t, hook.ran, "test hook did not intercept the append script")
			require.NoError(t, hook.err)
			require.True(t, apierr.IsInternal(err), "append accepted a changed validated read set: %v", err)
			require.Equal(t, string(injectedJSON), mustRedisGet(t, appendClient, targetKey))
			if targetKey == definitionKey("guild", created.Definition.ID) {
				require.Equal(t, headBefore, mustRedisGet(t, appendClient, revisionKey("guild", created.Revision.ID)))
			} else {
				require.Equal(t, definitionBefore, mustRedisGet(t, appendClient, definitionKey("guild", created.Definition.ID)))
			}
			require.ElementsMatch(t, indexBefore, appendClient.SMembers(ctx, definitionIndexKey("guild")).Val())
			require.Zero(t, appendClient.Exists(ctx, revisionKey("guild", "revision-2")).Val())
		})
	}
}

func TestRedisAppendKeepsCompetingValidAppendAsAborted(t *testing.T) {
	server := miniredis.RunT(t)
	originalClient := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	competingClient := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, originalClient.Close())
		require.NoError(t, competingClient.Close())
	})
	originalRepo := mustNewRedis(t, originalClient, &queuedGenerator{
		ids: []string{"definition", "revision-1", "revision-original"},
	})
	competingRepo := mustNewRedis(t, competingClient, &queuedGenerator{ids: []string{"revision-competing"}})
	created := mustCreate(t, originalRepo, "guild", validSource("one"))
	ctx := context.Background()

	var competingOutput *AppendRevisionOutput
	hook := &beforeScriptHook{before: func() error {
		var err error
		competingOutput, err = competingRepo.AppendRevision(ctx, AppendRevisionInput{
			GuildID: "guild", DefinitionID: created.Definition.ID,
			ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player-competing",
			Source: validSource("competing"),
		})
		return err
	}}
	originalClient.AddHook(hook)

	_, err := originalRepo.AppendRevision(ctx, AppendRevisionInput{
		GuildID: "guild", DefinitionID: created.Definition.ID,
		ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player-original",
		Source: validSource("original"),
	})
	require.True(t, hook.ran, "test hook did not intercept the append script")
	require.NoError(t, hook.err)
	require.True(t, apierr.IsAborted(err), "legitimate competing append must remain a CAS conflict: %v", err)
	require.Equal(t, "revision-competing", competingOutput.Definition.HeadRevisionID)
	require.Zero(t, originalClient.Exists(ctx, revisionKey("guild", "revision-original")).Val())
	storedDefinition := mustRedisGet(t, originalClient, definitionKey("guild", created.Definition.ID))
	require.JSONEq(t, string(mustJSON(t, competingOutput.Definition)), storedDefinition)
}

func TestRedisGeneratedIDCollisionsLeaveStateUnchanged(t *testing.T) {
	t.Run("definition collision", func(t *testing.T) {
		_, client, repo := newTestRepository(t, "definition", "revision-1", "definition", "revision-unused")
		ctx := context.Background()
		created := mustCreate(t, repo, "guild", validSource("one"))
		before := mustRedisGet(t, client, definitionKey("guild", created.Definition.ID))

		_, err := repo.CreateDefinition(ctx, CreateDefinitionInput{
			GuildID: "guild", CreatedByPlayerID: "player", Source: validSource("other"),
		})
		require.True(t, apierr.IsAlreadyExists(err), "got %v", err)
		require.Equal(t, before, mustRedisGet(t, client, definitionKey("guild", created.Definition.ID)))
		require.Zero(t, client.Exists(ctx, revisionKey("guild", "revision-unused")).Val())
	})

	t.Run("revision collision", func(t *testing.T) {
		_, client, repo := newTestRepository(t, "definition", "revision-1", "revision-1")
		ctx := context.Background()
		created := mustCreate(t, repo, "guild", validSource("one"))
		before := mustRedisGet(t, client, definitionKey("guild", created.Definition.ID))

		_, err := repo.AppendRevision(ctx, AppendRevisionInput{
			GuildID: "guild", DefinitionID: created.Definition.ID,
			ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player",
			Source: validSource("two"),
		})
		require.True(t, apierr.IsAlreadyExists(err), "got %v", err)
		require.Equal(t, before, mustRedisGet(t, client, definitionKey("guild", created.Definition.ID)))
	})

	t.Run("wrong-type generated revision collision", func(t *testing.T) {
		_, client, repo := newTestRepository(t, "definition", "revision")
		ctx := context.Background()
		require.NoError(t, client.SAdd(ctx, revisionKey("guild", "revision"), "wrong-type").Err())

		_, err := repo.CreateDefinition(ctx, validCreateInput(validSource("source")))
		require.True(t, apierr.IsAlreadyExists(err), "got %v", err)
		require.Zero(t, client.Exists(ctx, definitionKey("guild", "definition")).Val())
		require.Zero(t, client.Exists(ctx, definitionIndexKey("guild")).Val())
		require.Equal(t, "set", client.Type(ctx, revisionKey("guild", "revision")).Val())
	})
}

func TestRedisInputAndResultsDoNotAliasPersistedSource(t *testing.T) {
	_, _, repo := newTestRepository(t, "definition", "revision-1", "revision-2")
	ctx := context.Background()
	source := validSource("original")
	created := mustCreate(t, repo, "guild", source)

	source.Items[0].Label = "mutated input"
	created.Revision.Source.Items[0].Label = "mutated result"
	created.Definition.HeadRevisionID = "mutated head"

	got, err := repo.GetRevision(ctx, GetRevisionInput{
		GuildID: "guild", DefinitionID: "definition", RevisionID: "revision-1",
	})
	require.NoError(t, err)
	require.Equal(t, "Table", got.Revision.Source.Items[0].Label)
	got.Revision.Source.Items[0].Label = "mutated read"

	again, err := repo.GetRevision(ctx, GetRevisionInput{
		GuildID: "guild", DefinitionID: "definition", RevisionID: "revision-1",
	})
	require.NoError(t, err)
	require.Equal(t, "Table", again.Revision.Source.Items[0].Label)

	listed, err := repo.ListDefinitions(ctx, ListDefinitionsInput{GuildID: "guild"})
	require.NoError(t, err)
	require.Equal(t, "revision-1", listed.Definitions[0].Definition.HeadRevisionID)
}

func TestRedisReadAfterConstructionFromAnotherRepository(t *testing.T) {
	server := miniredis.RunT(t)
	clientOne := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	clientTwo := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, clientOne.Close())
		require.NoError(t, clientTwo.Close())
	})

	repoOne := mustNewRedis(t, clientOne, &queuedGenerator{ids: []string{"definition", "revision"}})
	repoTwo := mustNewRedis(t, clientTwo, &queuedGenerator{ids: []string{"unused"}})
	created := mustCreate(t, repoOne, "guild", validSource("source"))

	got, err := repoTwo.GetRevision(context.Background(), GetRevisionInput{
		GuildID: "guild", DefinitionID: created.Definition.ID, RevisionID: created.Revision.ID,
	})
	require.NoError(t, err)
	require.Equal(t, created.Revision, got.Revision)
}

func TestRedisCreateRefusesWrongTypeIndexWithoutPartialWrites(t *testing.T) {
	_, client, repo := newTestRepository(t, "definition", "revision")
	ctx := context.Background()
	indexKey := definitionIndexKey("guild")
	require.NoError(t, client.Set(ctx, indexKey, "wrong-type", 0).Err())

	_, err := repo.CreateDefinition(ctx, CreateDefinitionInput{
		GuildID: "guild", CreatedByPlayerID: "player", Source: validSource("source"),
	})
	require.True(t, apierr.IsInternal(err), "got %v", err)
	require.Zero(t, client.Exists(ctx, definitionKey("guild", "definition")).Val())
	require.Zero(t, client.Exists(ctx, revisionKey("guild", "revision")).Val())
	require.Equal(t, "wrong-type", client.Get(ctx, indexKey).Val())
}

func TestRedisAppendRefusesCorruptPrerequisitesWithoutPartialWrites(t *testing.T) {
	tests := map[string]func(context.Context, redisclient.Client, *CreateDefinitionOutput){
		"wrong-type index": func(ctx context.Context, client redisclient.Client, _ *CreateDefinitionOutput) {
			key := definitionIndexKey("guild")
			client.Del(ctx, key)
			client.Set(ctx, key, "wrong-type", 0)
		},
		"definition absent from index": func(
			ctx context.Context,
			client redisclient.Client,
			created *CreateDefinitionOutput,
		) {
			client.SRem(ctx, definitionIndexKey("guild"), created.Definition.ID)
		},
		"missing head": func(ctx context.Context, client redisclient.Client, created *CreateDefinitionOutput) {
			client.Del(ctx, revisionKey("guild", created.Revision.ID))
		},
		"corrupt head": func(ctx context.Context, client redisclient.Client, created *CreateDefinitionOutput) {
			client.Set(ctx, revisionKey("guild", created.Revision.ID), "{", 0)
		},
		"corrupt definition": func(ctx context.Context, client redisclient.Client, created *CreateDefinitionOutput) {
			client.Set(ctx, definitionKey("guild", created.Definition.ID), "{", 0)
		},
	}

	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			_, client, repo := newTestRepository(t, "definition", "revision-1", "revision-2")
			ctx := context.Background()
			created := mustCreate(t, repo, "guild", validSource("one"))
			corrupt(ctx, client, created)
			definitionBefore := client.Get(ctx, definitionKey("guild", created.Definition.ID)).Val()
			indexTypeBefore := client.Type(ctx, definitionIndexKey("guild")).Val()
			indexBefore, _ := client.SMembers(ctx, definitionIndexKey("guild")).Result()

			_, err := repo.AppendRevision(ctx, AppendRevisionInput{
				GuildID: "guild", DefinitionID: created.Definition.ID,
				ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player",
				Source: validSource("two"),
			})
			require.True(t, apierr.IsInternal(err), "got %v", err)
			require.Zero(t, client.Exists(ctx, revisionKey("guild", "revision-2")).Val())
			require.Equal(t, definitionBefore, client.Get(ctx, definitionKey("guild", created.Definition.ID)).Val())
			require.Equal(t, indexTypeBefore, client.Type(ctx, definitionIndexKey("guild")).Val())
			if indexTypeBefore == "set" {
				require.ElementsMatch(t, indexBefore, client.SMembers(ctx, definitionIndexKey("guild")).Val())
			}
		})
	}
}

func TestRedisStoredIdentifiersFailClosedBeforeAppendWrites(t *testing.T) {
	invalidIdentifiers := map[string]string{
		"whitespace": " invalid",
		"control":    "invalid\n",
		"overlong":   strings.Repeat("x", maxIDLength+1),
	}

	for kind, invalid := range invalidIdentifiers {
		t.Run("get revision creator "+kind, func(t *testing.T) {
			_, client, repo := newTestRepository(t, "definition", "revision")
			created := mustCreate(t, repo, "guild", validSource("source"))
			stored := *created.Revision
			stored.CreatedByPlayerID = invalid
			setJSON(t, client, revisionKey("guild", stored.ID), &stored)

			_, err := repo.GetRevision(context.Background(), GetRevisionInput{
				GuildID: "guild", DefinitionID: created.Definition.ID, RevisionID: stored.ID,
			})
			require.True(t, apierr.IsInternal(err), "got %v", err)
		})

		for _, metadata := range []string{"definition creator", "definition head", "revision creator"} {
			t.Run("list "+metadata+" "+kind, func(t *testing.T) {
				_, client, repo := newTestRepository(t, "definition", "revision")
				created := mustCreate(t, repo, "guild", validSource("source"))
				switch metadata {
				case "definition creator":
					stored := *created.Definition
					stored.CreatedByPlayerID = invalid
					setJSON(t, client, definitionKey("guild", stored.ID), &stored)
				case "definition head":
					definition := *created.Definition
					definition.HeadRevisionID = invalid
					setJSON(t, client, definitionKey("guild", definition.ID), &definition)
					revision := *created.Revision
					revision.ID = invalid
					setJSON(t, client, revisionKey("guild", invalid), &revision)
				case "revision creator":
					stored := *created.Revision
					stored.CreatedByPlayerID = invalid
					setJSON(t, client, revisionKey("guild", stored.ID), &stored)
				}

				_, err := repo.ListDefinitions(context.Background(), ListDefinitionsInput{GuildID: "guild"})
				require.True(t, apierr.IsInternal(err), "got %v", err)
			})
		}

		for _, metadata := range []string{"definition creator", "definition head", "revision creator"} {
			t.Run("append "+metadata+" "+kind, func(t *testing.T) {
				_, client, repo := newTestRepository(t, "definition", "revision-1", "revision-2")
				ctx := context.Background()
				created := mustCreate(t, repo, "guild", validSource("source"))
				switch metadata {
				case "definition creator":
					stored := *created.Definition
					stored.CreatedByPlayerID = invalid
					setJSON(t, client, definitionKey("guild", stored.ID), &stored)
				case "definition head":
					stored := *created.Definition
					stored.HeadRevisionID = invalid
					setJSON(t, client, definitionKey("guild", stored.ID), &stored)
				case "revision creator":
					stored := *created.Revision
					stored.CreatedByPlayerID = invalid
					setJSON(t, client, revisionKey("guild", stored.ID), &stored)
				}
				definitionBefore := mustRedisGet(t, client, definitionKey("guild", created.Definition.ID))
				indexBefore := client.SMembers(ctx, definitionIndexKey("guild")).Val()

				_, err := repo.AppendRevision(ctx, AppendRevisionInput{
					GuildID: "guild", DefinitionID: created.Definition.ID,
					ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player-two",
					Source: validSource("two"),
				})
				require.True(t, apierr.IsInternal(err), "got %v", err)
				require.Zero(t, client.Exists(ctx, revisionKey("guild", "revision-2")).Val())
				require.Equal(t, definitionBefore, mustRedisGet(t, client, definitionKey("guild", created.Definition.ID)))
				require.ElementsMatch(t, indexBefore, client.SMembers(ctx, definitionIndexKey("guild")).Val())
			})
		}
	}
}

func TestRedisStoredIdentityAndTimestampsFailClosed(t *testing.T) {
	tests := map[string]func(*CreateDefinitionOutput, redisclient.Client){
		"definition wrong guild": func(created *CreateDefinitionOutput, client redisclient.Client) {
			stored := *created.Definition
			stored.GuildID = "other-guild"
			setJSON(t, client, definitionKey("guild", stored.ID), &stored)
		},
		"definition zero timestamp": func(created *CreateDefinitionOutput, client redisclient.Client) {
			stored := *created.Definition
			stored.CreatedAt = time.Time{}
			setJSON(t, client, definitionKey("guild", stored.ID), &stored)
		},
		"revision wrong guild": func(created *CreateDefinitionOutput, client redisclient.Client) {
			stored := *created.Revision
			stored.GuildID = "other-guild"
			setJSON(t, client, revisionKey("guild", stored.ID), &stored)
		},
		"revision zero timestamp": func(created *CreateDefinitionOutput, client redisclient.Client) {
			stored := *created.Revision
			stored.CreatedAt = time.Time{}
			setJSON(t, client, revisionKey("guild", stored.ID), &stored)
		},
	}

	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			_, client, repo := newTestRepository(t, "definition", "revision")
			created := mustCreate(t, repo, "guild", validSource("source"))
			corrupt(created, client)

			_, err := repo.ListDefinitions(context.Background(), ListDefinitionsInput{GuildID: "guild"})
			require.True(t, apierr.IsInternal(err), "got %v", err)
		})
	}
}

func TestRedisGetRevisionPreservesWrongDefinitionNotFound(t *testing.T) {
	_, client, repo := newTestRepository(t, "definition", "revision")
	created := mustCreate(t, repo, "guild", validSource("source"))

	_, err := repo.GetRevision(context.Background(), GetRevisionInput{
		GuildID: "guild", DefinitionID: "other-definition", RevisionID: created.Revision.ID,
	})
	require.True(t, apierr.IsNotFound(err), "got %v", err)

	stored := *created.Revision
	stored.DefinitionID = " invalid"
	setJSON(t, client, revisionKey("guild", stored.ID), &stored)
	_, err = repo.GetRevision(context.Background(), GetRevisionInput{
		GuildID: "guild", DefinitionID: "other-definition", RevisionID: created.Revision.ID,
	})
	require.True(t, apierr.IsInternal(err), "invalid stored definition identity is corruption: %v", err)
}

func TestRedisHeadReadErrorDistinguishesMissingOrUnreadableHead(t *testing.T) {
	_, client, repo := newTestRepository(t, "definition", "revision-1", "revision-2")
	ctx := context.Background()
	created := mustCreate(t, repo, "guild", validSource("source"))
	require.NoError(t, client.Del(ctx, revisionKey("guild", created.Revision.ID)).Err())
	require.NoError(t, client.SAdd(ctx, revisionKey("guild", created.Revision.ID), "wrong-type").Err())

	_, err := repo.AppendRevision(ctx, AppendRevisionInput{
		GuildID: "guild", DefinitionID: created.Definition.ID,
		ExpectedHeadRevisionID: created.Revision.ID, CreatedByPlayerID: "player-two",
		Source: validSource("two"),
	})
	require.True(t, apierr.IsInternal(err), "got %v", err)
	require.ErrorContains(t, err, "missing or unreadable head revision")
	require.ErrorContains(t, err, "WRONGTYPE")
	require.Zero(t, client.Exists(ctx, revisionKey("guild", "revision-2")).Val())
}

func TestRedisListUntouchedGuildReturnsNonNilEmptyDefinitions(t *testing.T) {
	_, _, repo := newTestRepository(t, "unused")

	listed, err := repo.ListDefinitions(context.Background(), ListDefinitionsInput{GuildID: "untouched"})
	require.NoError(t, err)
	require.NotNil(t, listed.Definitions)
	require.Empty(t, listed.Definitions)
}

func TestRedisCorruptJSONFailsClosed(t *testing.T) {
	t.Run("get malformed revision", func(t *testing.T) {
		_, client, repo := newTestRepository(t, "definition", "revision")
		ctx := context.Background()
		created := mustCreate(t, repo, "guild", validSource("source"))
		require.NoError(t, client.Set(ctx, revisionKey("guild", created.Revision.ID), "{", 0).Err())

		_, err := repo.GetRevision(ctx, GetRevisionInput{
			GuildID: "guild", DefinitionID: created.Definition.ID, RevisionID: created.Revision.ID,
		})
		require.True(t, apierr.IsInternal(err), "got %v", err)
	})

	t.Run("list mismatched head", func(t *testing.T) {
		_, client, repo := newTestRepository(t, "definition", "revision")
		ctx := context.Background()
		created := mustCreate(t, repo, "guild", validSource("source"))
		stored := *created.Revision
		stored.DefinitionID = "other"
		encoded, err := json.Marshal(&stored)
		require.NoError(t, err)
		require.NoError(t, client.Set(ctx, revisionKey("guild", stored.ID), encoded, 0).Err())

		_, err = repo.ListDefinitions(ctx, ListDefinitionsInput{GuildID: "guild"})
		require.True(t, apierr.IsInternal(err), "got %v", err)
	})
}

func TestRedisValidatesTypedBoundedSourceAndAssetRefGrammar(t *testing.T) {
	validQualified := validSource("qualified")
	validQualified.Items[0].AssetRef = "synty:props:dungeon:furniture:stretch-table-v2-with-a-qualified-long-tail"

	tests := map[string]struct {
		input CreateDefinitionInput
		valid bool
	}{
		"legacy ref":      {input: validCreateInput(validSource("legacy")), valid: true},
		"qualified ref":   {input: validCreateInput(validQualified), valid: true},
		"empty guild":     {input: CreateDefinitionInput{CreatedByPlayerID: "player", Source: validSource("source")}},
		"empty creator":   {input: CreateDefinitionInput{GuildID: "guild", Source: validSource("source")}},
		"wrong version":   {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) { s.Version = 2 }))},
		"empty name":      {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) { s.Name = "" }))},
		"no items":        {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) { s.Items = nil }))},
		"wrong item kind": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) { s.Items[0].Kind = "group" }))},
		"bad ref":         {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) { s.Items[0].AssetRef = "two:segments" }))},
		"empty ref part":  {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) { s.Items[0].AssetRef = "dnd5e:props::table" }))},
		"duplicate id": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Groups = []entities.CompositionGroup{{ID: "table", Kind: "group", Label: "Group", Transform: validTransform()}}
		}))},
		"missing group parent": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items[0].ParentID = "missing"
		}))},
		"missing support": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items[0].SupportID = "missing"
		}))},
		"support self cycle": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items[0].SupportID = s.Items[0].ID
		}))},
		"group cycle": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Groups = []entities.CompositionGroup{
				{ID: "g1", Kind: "group", Label: "One", Transform: validTransform(), ParentID: "g2"},
				{ID: "g2", Kind: "group", Label: "Two", Transform: validTransform(), ParentID: "g1"},
			}
		}))},
		"nan": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items[0].Transform.X = math.NaN()
		}))},
		"x over bound": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items[0].Transform.X = 12.01
		}))},
		"negative y": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items[0].Transform.Y = -0.01
		}))},
		"yaw over bound": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items[0].Transform.RotationY = math.Pi*100 + 0.01
		}))},
		"too many items": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items = make([]entities.CompositionItem, maxItems+1)
		}))},
		"too many groups": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Groups = make([]entities.CompositionGroup, maxGroups+1)
		}))},
		"name too long": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Name = strings.Repeat("n", maxNameLength+1)
		}))},
		"ref too long": {input: validCreateInput(mutatedSource(func(s *entities.CompositionSource) {
			s.Items[0].AssetRef = "dnd5e:props:" + strings.Repeat("x", maxRefLength)
		}))},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, repo := newTestRepository(t, "definition", "revision")
			output, err := repo.CreateDefinition(context.Background(), test.input)
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, test.input.Source.Items[0].AssetRef, output.Revision.Source.Items[0].AssetRef)
				return
			}
			require.Error(t, err)
			require.True(t, apierr.IsInvalidArgument(err), "got %v", err)
		})
	}
}

func TestRedisInvalidGeneratedIDsReturnInternalBeforeWrites(t *testing.T) {
	tests := map[string][]string{
		"invalid definition ID": {""},
		"invalid revision ID":   {"definition", "revision\n"},
	}
	for name, ids := range tests {
		t.Run(name, func(t *testing.T) {
			_, client, repo := newTestRepository(t, ids...)
			ctx := context.Background()

			_, err := repo.CreateDefinition(ctx, validCreateInput(validSource("source")))
			require.True(t, apierr.IsInternal(err), "got %v", err)
			keys, keysErr := client.Keys(ctx, compositionKeyPrefix+"*").Result()
			require.NoError(t, keysErr)
			require.Empty(t, keys)
		})
	}
}

func TestRedisAppendValidatesCASInputBeforeGeneratingOrWriting(t *testing.T) {
	_, client, repo := newTestRepository(t, "definition", "revision-1", "revision-unused")
	ctx := context.Background()
	created := mustCreate(t, repo, "guild", validSource("one"))

	_, err := repo.AppendRevision(ctx, AppendRevisionInput{
		GuildID: "guild", DefinitionID: created.Definition.ID,
		CreatedByPlayerID: "player", Source: validSource("two"),
	})
	require.True(t, apierr.IsInvalidArgument(err), "got %v", err)
	require.Zero(t, client.Exists(ctx, revisionKey("guild", "revision-unused")).Val())
}

func newTestRepository(t *testing.T, ids ...string) (*miniredis.Miniredis, redisclient.Client, Repository) {
	t.Helper()
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return server, client, mustNewRedis(t, client, &queuedGenerator{ids: ids})
}

func mustNewRedis(t *testing.T, client redisclient.Client, generator idgen.Generator) Repository {
	t.Helper()
	repo, err := NewRedis(&RedisConfig{
		Client: client, Clock: fixedClock{now: fixedNow}, IDGenerator: generator,
	})
	require.NoError(t, err)
	return repo
}

func mustCreate(
	t *testing.T,
	repo Repository,
	guildID string,
	source entities.CompositionSource,
) *CreateDefinitionOutput {
	t.Helper()
	output, err := repo.CreateDefinition(context.Background(), CreateDefinitionInput{
		GuildID: guildID, CreatedByPlayerID: "player-one", Source: source,
	})
	require.NoError(t, err)
	return output
}

func validCreateInput(source entities.CompositionSource) CreateDefinitionInput {
	return CreateDefinitionInput{GuildID: "guild", CreatedByPlayerID: "player", Source: source}
}

func validSource(name string) entities.CompositionSource {
	return entities.CompositionSource{
		Version: 1,
		Name:    name,
		Items: []entities.CompositionItem{{
			ID: "table", Kind: "prop", AssetRef: "dnd5e:props:table", Label: "Table",
			Transform: validTransform(),
		}},
		Groups: []entities.CompositionGroup{},
	}
}

func validTransform() entities.CompositionTransform {
	return entities.CompositionTransform{X: 1, Y: 0, Z: -1, RotationY: math.Pi / 2}
}

func mutatedSource(mutate func(*entities.CompositionSource)) entities.CompositionSource {
	source := validSource("source")
	mutate(&source)
	return source
}

func hashTag(key string) string {
	start := strings.IndexByte(key, '{')
	end := strings.IndexByte(key, '}')
	if start < 0 || end <= start {
		return ""
	}
	return key[start+1 : end]
}

func mustRedisGet(t *testing.T, client redisclient.Client, key string) string {
	t.Helper()
	value, err := client.Get(context.Background(), key).Result()
	require.NoError(t, err)
	return value
}

func setJSON(t *testing.T, client redisclient.Client, key string, value any) {
	t.Helper()
	require.NoError(t, client.Set(context.Background(), key, mustJSON(t, value), 0).Err())
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

type beforeScriptHook struct {
	before func() error
	once   sync.Once
	ran    bool
	err    error
}

func (h *beforeScriptHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h *beforeScriptHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if cmd.Name() == "eval" || cmd.Name() == "evalsha" {
			h.once.Do(func() {
				h.ran = true
				h.err = h.before()
			})
		}
		return next(ctx, cmd)
	}
}

func (h *beforeScriptHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}
