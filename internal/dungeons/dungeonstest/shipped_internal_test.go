package dungeonstest

import (
	"path/filepath"
	"testing"
)

// A shelf that cannot compile comes back as an error, never as a nil store
// with no error: ShippedStore caches what compileShipped returns, and every
// test after the first must see the real cause (rpg-api#1200).
func TestCompileShipped_FailureIsAnError(t *testing.T) {
	cases := map[string][]string{
		"missing directory": {filepath.Join(t.TempDir(), "missing")},
		"empty shelf":       {t.TempDir()},
	}
	for name, srcs := range cases {
		t.Run(name, func(t *testing.T) {
			store, err := compileShipped(srcs...)
			if err == nil {
				t.Fatal("compileShipped succeeded on a shelf that cannot compile")
			}
			if store != nil {
				t.Fatalf("compileShipped returned a store alongside its error: %v", err)
			}
		})
	}
}
