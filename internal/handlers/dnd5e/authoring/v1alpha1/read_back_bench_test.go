package authoringv1alpha1_test

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec/readback"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// BenchmarkReadBack_Castle is what the read-back adds to one check of the
// castle (rpg-project#967): readback.Of alone, the second decode and compile
// included, and the check's encoded size with and without it. Run with
// `go test -run '^$' -bench ReadBack_Castle ./internal/handlers/dnd5e/authoring/v1alpha1/`.
func BenchmarkReadBack_Castle(b *testing.B) {
	path := filepath.Join(dungeonstest.ContentDir(b), dungeons.DefaultKey+".yaml")
	raw, err := os.ReadFile(path) //nolint:gosec // the repository's shipped castle
	if err != nil {
		b.Fatal(err)
	}
	yaml := string(dungeonstest.AsKey(b, raw, dungeons.DefaultKey, readBackCastleKey))

	ctx := worldCtx("alice")
	shipped, err := dungeons.NewFileRegistry(dungeonstest.ContentDir(b), dungeonstest.Projector(b))
	if err != nil {
		b.Fatal(err)
	}
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: dungeonstest.Join(b, shipped, false)})
	if err != nil {
		b.Fatal(err)
	}
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	if err != nil {
		b.Fatal(err)
	}
	resp, err := h.PutDungeon(ctx, &authoringpb.PutDungeonRequest{Key: readBackCastleKey, Yaml: yaml, ValidateOnly: true})
	if err != nil || resp.GetReadBack() == nil {
		b.Fatalf("castle check: %v (read-back %v)", err, resp.GetReadBack() != nil)
	}
	with := encoded(b, resp)
	resp.ReadBack = nil
	without := encoded(b, resp)
	b.Logf("castle check encoded: %d bytes with the read-back, %d without; gzip %d with, %d without",
		len(with), len(without), gz(b, with), gz(b, without))

	castle := []byte(yaml)
	b.Run("readback.Of", func(b *testing.B) {
		for b.Loop() {
			if _, err := readback.Of(castle); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("PutDungeon validate-only", func(b *testing.B) {
		for b.Loop() {
			if _, err := h.PutDungeon(ctx, &authoringpb.PutDungeonRequest{Key: readBackCastleKey, Yaml: yaml, ValidateOnly: true}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func encoded(b *testing.B, m proto.Message) []byte {
	b.Helper()
	out, err := proto.Marshal(m)
	if err != nil {
		b.Fatal(err)
	}
	return out
}

func gz(b *testing.B, raw []byte) int {
	b.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		b.Fatal(err)
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
	}
	return buf.Len()
}
