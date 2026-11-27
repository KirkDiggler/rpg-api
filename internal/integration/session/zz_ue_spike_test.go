package session_test

// The engine side of the Unreal-client spike. It is not part of any build:
// it lives here as the recipe (see ../README.md), and ran once as
// internal/integration/session/zz_ue_spike_test.go in a worktree of rpg-api
// (`dev` at ed4bfc65), approved by Kirk 2026-11-23.
//
// To re-run, copy it there, inside a worktree of rpg-api:
//
//	cp <this file> internal/integration/session/zz_ue_spike_test.go
//	UE_SPIKE_ADDR=127.0.0.1:50151 UE_SPIKE_INFO=<spike>/run/engine.json \
//	  go test -vet=off -count=1 -v -run '^TestUESpikeServe$' \
//	  ./internal/integration/session -timeout 3h
//
// Skipped unless UE_SPIKE_ADDR and UE_SPIKE_INFO are set. When set, it stands
// up the record-reader suite's run (same fixture, same launch) and serves
// RecordService + SessionService on a real TCP listener, behind the PRODUCTION
// auth interceptors with the Dev scheme allowed, until the test is killed.
// The TCP listener forwards to the suite's in-process bufconn listener.
//
// Teardown note: `go test` spawns the test binary (session.test); killing the
// `go test` pid alone leaves that binary serving the port. Kill by port:
//	ss -ltnp | grep ':50151 '   → pid=…

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	recordhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	recordpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
)

func TestUESpikeServe(t *testing.T) {
	addr, infoPath := os.Getenv("UE_SPIKE_ADDR"), os.Getenv("UE_SPIKE_INFO")
	if addr == "" || infoPath == "" {
		t.Skip("ue spike: set UE_SPIKE_ADDR and UE_SPIKE_INFO")
	}
	if err := os.MkdirAll(filepath.Dir(infoPath), 0o700); err != nil {
		t.Fatal(err)
	}

	// The record-reader suite's setup, verbatim in shape.
	h := newAcceptanceHarnessWithDice(t, alwaysTheLowestFace{})
	projector := dungeonstest.ProjectorFor(h.manager.Manager)
	raw, err := os.ReadFile(dungeonstest.SitePath(t, hashSiteKey)) //nolint:gosec // repository fixture
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dungeons.DefaultKey+".yaml"),
		dungeonstest.AsKey(t, raw, hashSiteKey, dungeons.DefaultKey), 0o600); err != nil {
		t.Fatal(err)
	}
	shipped, err := dungeons.NewFileRegistry(dir, projector)
	if err != nil {
		t.Fatal(err)
	}
	lobbies := lobbyrepo.NewInMemory()
	lobby, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo: lobbies, LobbyBroker: lobbyorch.NewBroker(), CharacterRepo: h.charRepo,
		LobbyIDGenerator: idgen.NewSequential("lobby"), JoinRefGenerator: idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"), SessionManager: h.manager.Manager,
		Dungeons: dungeonstest.Join(t, shipped, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	member, host := "reader-alice", "player-reader-alice"
	if _, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter(member, host)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := lobbies.Save(context.Background(), &lobbyrepo.Data{
		ID: "lobby-reader", HostPlayerID: host, Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{host: {PlayerID: host, CharacterID: member, IsHost: true, IsReady: true}},
		MemberOrder: []string{host},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := lobby.StartEncounter(context.Background(), &lobbyorch.StartEncounterInput{
		WorldID: dungeonstest.Caller.WorldID, PlayerID: host, LobbyID: "lobby-reader",
	})
	if err != nil {
		t.Fatal(err)
	}
	run := out.EncounterID

	// A second run, from a shipped dungeon, launched by key the way the lobby
	// does it: the castle. Its atlas is what the Unreal scene is built from.
	castleDir := t.TempDir()
	castleSrc := dungeonstest.FixtureContentDir(t)
	entries, err := os.ReadDir(castleSrc)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(castleSrc, entry.Name())) //nolint:gosec // a fixture entry under the repo
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(castleDir, entry.Name()), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	shippedCastle, err := dungeons.NewFileRegistry(castleDir, projector)
	if err != nil {
		t.Fatal(err)
	}
	castleLobbies := lobbyrepo.NewInMemory()
	castleLobby, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo: castleLobbies, LobbyBroker: lobbyorch.NewBroker(), CharacterRepo: h.charRepo,
		LobbyIDGenerator: idgen.NewSequential("castle-lobby"), JoinRefGenerator: idgen.NewSequential("castle-ref"),
		EncounterIDGenerator: idgen.NewSequential("castle"), SessionManager: h.manager.Manager,
		Dungeons: dungeonstest.Join(t, shippedCastle, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	castleMember, castleHost := "alice", "player-alice"
	if _, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter(castleMember, castleHost)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := castleLobbies.Save(context.Background(), &lobbyrepo.Data{
		ID: "lobby-castle", HostPlayerID: castleHost, Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{castleHost: {PlayerID: castleHost, CharacterID: castleMember, IsHost: true, IsReady: true}},
		MemberOrder: []string{castleHost},
	}); err != nil {
		t.Fatal(err)
	}
	castleOut, err := castleLobby.StartEncounter(context.Background(), &lobbyorch.StartEncounterInput{
		WorldID: dungeonstest.Caller.WorldID, PlayerID: castleHost, LobbyID: "lobby-castle", DungeonKey: "the-mad-king",
	})
	if err != nil {
		t.Fatalf("castle launch: %v", err)
	}
	castle := castleOut.EncounterID
	atlas, err := h.handler.GetAtlas(auth.WithPlayerID(context.Background(), castleHost),
		&sessionpb.GetAtlasRequest{Session: castle, Member: castleMember})
	if err != nil {
		t.Fatalf("castle atlas: %v", err)
	}
	t.Logf("CASTLE %s atlas: grid=%s cells=%d regions=%d boundaries=%d doorways=%d props=%d placed=%d structural walls=%d structural doors=%d exits=%d segments=%d",
		castle, atlas.GetGrid(), len(atlas.GetCells()), len(atlas.GetRegions()), len(atlas.GetBoundaries()),
		len(atlas.GetDoorways()), len(atlas.GetProps()), len(atlas.GetPlaced()), len(atlas.GetStructuralWalls()),
		len(atlas.GetStructuralDoors()), len(atlas.GetExits()), len(atlas.GetSegments()))
	if dumpPath := os.Getenv("UE_SPIKE_ATLAS"); dumpPath != "" {
		document, err := protojson.MarshalOptions{Multiline: true}.Marshal(atlas)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dumpPath, document, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("castle atlas written to %s", dumpPath)
	}

	// Three runs for the Unreal client's verb tests (ideas/unreal-client/spike/
	// round-verbs.md, Task 1), launched the way the acceptance suite launches
	// because this server registers Session and Record only, no lobby. Each
	// holds exactly the geometry one test needs, and no test ends a turn in a
	// run another test reads, so no test depends on another's leftovers:
	//   ue-tomb   a fighter with a skeleton beside it: a swing that the engine
	//             resolves, and a turn to end;
	//   ue-range  a fighter with a skeleton three cells off: a swing refused;
	//   ue-walk   a fighter alone: the world clock, a step out and back.
	for _, seeded := range []struct{ member, host string }{
		{"ue-fighter", "player-ue-fighter"}, {"ue-archer", "player-ue-archer"}, {"ue-walker", "player-ue-walker"},
	} {
		if _, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(seeded.member, seeded.host)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	skeleton := refs.Monsters.Skeleton().String()
	if placed := memberIDs(h.launch(t, "ue-tomb",
		withMonsters(buildThreeRoomTomb(t), monsterAt("ue-dummy", skeleton, 2, 1)),
		seatAt("ue-fighter", 1, 1),
	)); len(placed) != 2 {
		t.Fatalf("ue-tomb: want the fighter and the skeleton on the board, got %v", placed)
	}
	h.launch(t, "ue-range",
		withMonsters(buildThreeRoomTomb(t), monsterAt("ue-far", skeleton, 4, 1)),
		seatAt("ue-archer", 1, 1))
	h.launch(t, "ue-walk", buildThreeRoomTomb(t), seatAt("ue-walker", 1, 1))

	// The production auth boundary, Dev scheme allowed: a client must send
	// `authorization: Dev <player_id>`.
	authCfg := &auth.InterceptorConfig{DevMode: true}
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(auth.UnaryAuthInterceptor(nil, nil, authCfg)),
		grpc.ChainStreamInterceptor(auth.StreamAuthInterceptor(nil, nil, authCfg)),
	)
	record, err := recordhandler.New(&recordhandler.HandlerConfig{
		Manager: h.manager.Manager, Broker: h.manager.RecordBroker, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	recordpb.RegisterRecordServiceServer(srv, record)
	sessionpb.RegisterSessionServiceServer(srv, h.handler)

	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	tcp, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tcp.Close() })
	go func() {
		for {
			c, err := tcp.Accept()
			if err != nil {
				return
			}
			b, err := lis.Dial()
			if err != nil {
				_ = c.Close()
				continue
			}
			go func() {
				defer c.Close()
				defer b.Close()
				go func() { _, _ = io.Copy(b, c) }()
				_, _ = io.Copy(c, b)
			}()
		}
	}()

	// Live beats: the host ends her turn a few times (Afford → EndTurn, the
	// way a client does on the turn clock; this fixture opens in a fight) so
	// the record grows while a client is watching. Never fatal: the server
	// must outlive a refused beat.
	go func() {
		ctx := auth.WithPlayerID(context.Background(), host)
		for i := 0; i < 12; i++ {
			time.Sleep(20 * time.Second)
			out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{Session: run, Member: member})
			if err != nil {
				t.Logf("live beat %d: Afford refused: %v", i, err)
				continue
			}
			var decl string
			for _, d := range out.GetDeclarations() {
				if d.GetVerb() == sessionpb.Verb_VERB_END_TURN && d.GetAvailable() {
					decl = d.GetId()
					break
				}
			}
			if decl == "" {
				t.Logf("live beat %d: no available END_TURN declaration (clock=%v)", i, out.GetClock())
				continue
			}
			resp, err := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{Session: run, Member: member, DeclarationId: decl})
			if err != nil {
				t.Logf("live beat %d: EndTurn refused: %v", i, err)
				continue
			}
			t.Logf("live beat %d: EndTurn ok, next=%q round_wrapped=%v", i, resp.GetNext(), resp.GetRoundWrapped())
		}
	}()

	info, _ := json.MarshalIndent(map[string]string{
		"addr": addr, "session": run, "member": member, "host": host, "onlooker": readerPlayer,
		"castle": castle, "castle_member": castleMember, "castle_host": castleHost,
		"tomb": "ue-tomb", "fighter": "ue-fighter", "fighter_host": "player-ue-fighter", "dummy": "ue-dummy",
		"range": "ue-range", "archer": "ue-archer", "archer_host": "player-ue-archer", "far": "ue-far",
		"walk": "ue-walk", "walker": "ue-walker", "walker_host": "player-ue-walker",
		"auth": "authorization: Dev <player id>",
	}, "", "  ")
	if err := os.WriteFile(infoPath, info, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("UE SPIKE READY %s", info)

	time.Sleep(2 * time.Hour)
}
