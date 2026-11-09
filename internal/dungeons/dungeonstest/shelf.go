package dungeonstest

import (
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// CompileShelf compiles dir as a shipped store, the way the server compiles
// its content directory at boot, for a test binary to share: a TestMain
// calls it once and every test reads the one store. Its projector runs over
// a miniredis that lives until close is called, so the store stays usable
// for every test after the one that built it. Use it when a package's suites
// only READ a shelf; a test about construction itself builds its own
// registry (rpg-project#907).
func CompileShelf(dir string) (store *dungeons.FileRegistry, closeFn func(), err error) {
	mr, err := miniredis.Run()
	if err != nil {
		return nil, nil, fmt.Errorf("dungeonstest: start miniredis: %w", err)
	}
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	closeFn = func() {
		_ = client.Close()
		mr.Close()
	}
	chars, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	if err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("dungeonstest: character repo: %w", err)
	}
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: chars, TTL: time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	if err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("dungeonstest: session orchestrator: %w", err)
	}
	store, err = dungeons.NewFileRegistry(dir, ProjectorFor(orch.Manager))
	if err != nil {
		closeFn()
		return nil, nil, err
	}

	return store, closeFn, nil
}
