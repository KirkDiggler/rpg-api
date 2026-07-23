// Package integration_test's crypt monster-seed suite is rpg-api#689's
// real-Redis end-to-end proof: StartEncounter's archetype-driven crypt
// monster composition (one deterministic-anchor entrance skeleton, zero
// corridor monsters, one deterministic-anchor non-wight skeleton-captain
// boss — rpg-toolkit#816) and its seed-determinism guarantee, exercised
// against REAL Redis-backed lobby + encounter repositories, not the
// in-memory LobbySuite (internal/orchestrators/lobby's unit tests) or the
// shared harness.TestServer (whose LobbyRepo/EncRepoV2 are in-memory by
// design — see harness.go's own comment). Reuses harness.NewWithRedis
// against this package's shared Redis container (rpg-api#699, see
// main_test.go) purely for a real Redis-backed CharacterRepo
// (srv.RedisClient()), then wires a SECOND, independent lobbyorch.
// Orchestrator directly onto Redis-backed lobbyrepo.NewRedis/encountersv2.
// NewRedis — real production repository constructors, no test-only
// backdoor, no proto changes (StartEncounterInput.RandomSeed is an
// orchestrator-level field with no proto surface yet, so this calls the
// orchestrator directly rather than through LobbyClient).
package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	encounterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

const cryptMonsterSeedTestTTL = time.Hour

// CryptMonsterSeedRedisSuite is rpg-api#689's real-Redis integration gate.
type CryptMonsterSeedRedisSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	srv     *harness.TestServer
	release func()

	lobbyRepo lobbyrepo.Repository
	encRepo   encountersv2.Repository
	orch      *lobbyorch.Orchestrator
}

func (s *CryptMonsterSeedRedisSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	// Lease the package's shared Redis container (rpg-api#699) — see
	// main_test.go. Released in TearDownTest.
	s.release = sharedRedis.Lease()

	var err error
	s.srv, err = harness.NewWithRedis(s.ctx, nil, sharedRedis.Addr)
	s.Require().NoError(err, "failed to create test server against shared redis container")
	s.Require().NoError(s.srv.FlushRedis(s.ctx), "failed to flush shared redis")

	// Real Redis-backed lobby + encounter repositories — production
	// constructors, sharing the harness's own Redis container/client.
	s.lobbyRepo = lobbyrepo.NewRedis(s.srv.RedisClient(), cryptMonsterSeedTestTTL)
	s.encRepo = encountersv2.NewRedis(s.srv.RedisClient(), cryptMonsterSeedTestTTL)

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:         s.lobbyRepo,
		LobbyBroker:       lobbyorch.NewBroker(),
		CharacterRepo:     s.srv.CharacterRepo, // real Redis-backed, from the harness
		EncounterRepo:     s.encRepo,
		EncounterBroker:   tkenc.NewBroker(tkenc.NewInMemoryTransport()),
		CharacterResolver: encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewUUID("lobby"),
		JoinRefGenerator:     idgen.NewUUID("join"),
		EncounterIDGenerator: idgen.NewUUID("enc"),
	})
	s.Require().NoError(err)
	s.orch = orch
}

func (s *CryptMonsterSeedRedisSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.release != nil {
		s.release()
	}
}

// seedRedisCharacter creates a minimal playable character in the REAL
// Redis-backed character repo.
func (s *CryptMonsterSeedRedisSuite) seedRedisCharacter(characterID, playerID, name string, hp int) {
	_, err := s.srv.CharacterRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{
			Data: &toolkitchar.Data{
				ID: characterID, PlayerID: playerID, Name: name,
				HitPoints: hp, MaxHitPoints: hp,
			},
		},
	})
	s.Require().NoError(err)
}

// seedRedisReadyLobby writes a ready-to-start lobby directly into the real
// Redis-backed lobby repo, bypassing CreateLobby/JoinLobby (this suite
// tests StartEncounter's monster-seeding, not lobby assembly — same
// pattern as LobbySuite.seedReadyLobby in the unit suite).
func (s *CryptMonsterSeedRedisSuite) seedRedisReadyLobby(id, host string) {
	s.Require().NoError(s.lobbyRepo.Save(s.ctx, &lobbyrepo.Data{
		ID: id, HostPlayerID: host, Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			host: {PlayerID: host, CharacterID: "char-" + host, IsHost: true, IsReady: true},
		},
		MemberOrder: []string{host},
	}))
}

func regionArchetypeAtRedis(space *tkenc.SpaceData, hex core.Hex) (tkenc.RegionArchetype, bool) {
	for _, r := range space.Regions {
		if r.Hexes.Has(hex) {
			return r.Archetype, true
		}
	}
	return "", false
}

// TestStartEncounter_RealRedis_CryptComposition proves the composition
// done bar (one entrance skeleton, zero corridor, one boss skeleton-
// captain — no goblins) against REAL Redis persistence, not in-memory.
func (s *CryptMonsterSeedRedisSuite) TestStartEncounter_RealRedis_CryptComposition() {
	s.seedRedisReadyLobby("lobby-redis-crypt", "alice")
	s.seedRedisCharacter("char-alice", "alice", "Alice", 12)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-redis-crypt",
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(out.EncounterID)

	// Read back from REAL Redis — proves persist-then-emit landed durably,
	// not just in an in-process cache.
	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Len(encData.Monsters, 2, "exactly one entrance skeleton + one boss skeleton-captain")

	counts := map[tkenc.RegionArchetype]int{}
	for id, m := range encData.Monsters {
		archetype, ok := regionArchetypeAtRedis(encData.Space, m.Position)
		s.Require().True(ok, "monster %q must be in a tagged region", id)
		counts[archetype]++
		switch archetype {
		case tkenc.ArchetypeEntrance:
			s.Require().Equal(refs.Monsters.Skeleton().String(), m.MonsterRef)
		case tkenc.ArchetypeBoss:
			s.Require().Equal(refs.Monsters.SkeletonCaptain().String(), m.MonsterRef)
		}
	}
	s.Require().Equal(1, counts[tkenc.ArchetypeEntrance])
	s.Require().Equal(1, counts[tkenc.ArchetypeBoss])
	s.Require().Zero(counts[tkenc.ArchetypeCorridor])
}

// TestStartEncounter_RealRedis_SameSeedByteIdenticalPositions is
// rpg-api#689's determinism done bar against real Redis: two independent
// StartEncounter calls with the same explicit RandomSeed must produce
// byte-identical monster positions once read back from Redis.
func (s *CryptMonsterSeedRedisSuite) TestStartEncounter_RealRedis_SameSeedByteIdenticalPositions() {
	s.seedRedisReadyLobby("lobby-redis-seed-a", "alice")
	s.seedRedisCharacter("char-alice", "alice", "Alice", 12)
	s.seedRedisReadyLobby("lobby-redis-seed-b", "bob")
	s.seedRedisCharacter("char-bob", "bob", "Bob", 10)

	outA, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-redis-seed-a", RandomSeed: 4242,
	})
	s.Require().NoError(err)
	outB, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-redis-seed-b", RandomSeed: 4242,
	})
	s.Require().NoError(err)

	dataA, err := s.encRepo.Get(s.ctx, outA.EncounterID)
	s.Require().NoError(err)
	dataB, err := s.encRepo.Get(s.ctx, outB.EncounterID)
	s.Require().NoError(err)

	byArchetype := func(data *tkenc.Data) map[tkenc.RegionArchetype]core.Hex {
		out := map[tkenc.RegionArchetype]core.Hex{}
		for _, m := range data.Monsters {
			archetype, ok := regionArchetypeAtRedis(data.Space, m.Position)
			s.Require().True(ok)
			out[archetype] = m.Position
		}
		return out
	}
	s.Require().Equal(byArchetype(dataA), byArchetype(dataB),
		"the same explicit seed must produce byte-identical monster positions through real Redis persistence")
}

// TestStartEncounter_RealRedis_PartySizeInvariant proves party size 1..4
// never changes monster positions, against real Redis.
func (s *CryptMonsterSeedRedisSuite) TestStartEncounter_RealRedis_PartySizeInvariant() {
	names := []string{"alice", "bob", "carol", "dave"}
	var reference map[tkenc.RegionArchetype]core.Hex

	for partySize := 1; partySize <= 4; partySize++ {
		lobbyID := "lobby-redis-party-" + names[partySize-1]
		members := map[string]*lobbyrepo.Member{}
		order := make([]string, 0, partySize)
		for i := 0; i < partySize; i++ {
			p := names[i]
			charID := "char-" + p + "-" + lobbyID
			s.seedRedisCharacter(charID, p, p, 10)
			members[p] = &lobbyrepo.Member{PlayerID: p, CharacterID: charID, IsHost: i == 0, IsReady: true}
			order = append(order, p)
		}
		s.Require().NoError(s.lobbyRepo.Save(s.ctx, &lobbyrepo.Data{
			ID: lobbyID, HostPlayerID: names[0], Status: lobbyrepo.StatusWaiting,
			Members: members, MemberOrder: order,
		}))

		out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
			PlayerID: names[0], LobbyID: lobbyID, RandomSeed: 9001,
		})
		s.Require().NoError(err, "party size %d must place monsters with zero errors", partySize)

		data, err := s.encRepo.Get(s.ctx, out.EncounterID)
		s.Require().NoError(err)
		got := map[tkenc.RegionArchetype]core.Hex{}
		for _, m := range data.Monsters {
			archetype, ok := regionArchetypeAtRedis(data.Space, m.Position)
			s.Require().True(ok)
			got[archetype] = m.Position
		}
		if reference == nil {
			reference = got
		} else {
			s.Require().Equal(reference, got, "party size %d must not change monster positions", partySize)
		}
	}
}

func TestCryptMonsterSeedRedisSuite(t *testing.T) {
	suite.Run(t, new(CryptMonsterSeedRedisSuite))
}
