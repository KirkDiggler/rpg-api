package dungeonstest

import (
	"fmt"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

// CompileShelf compiles dir as a shipped store, the way the server compiles
// its content directory at boot, for a test binary to share: a TestMain
// calls it once and every test reads the one store. Its projector runs over
// a miniredis that lives until closeFn is called, so the store stays usable
// for every test after the one that built it. Use it when a package's suites
// only READ a shelf they stage themselves; ShippedStore is the shared
// content/ shelf, and a test about construction itself builds its own
// registry (rpg-project#911).
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
	p, err := projectorOver(client)
	if err != nil {
		closeFn()
		return nil, nil, err
	}
	store, err = dungeons.NewFileRegistry(dir, p)
	if err != nil {
		closeFn()
		return nil, nil, err
	}

	return store, closeFn, nil
}
