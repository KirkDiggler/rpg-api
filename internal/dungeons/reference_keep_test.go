package dungeons_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

const referenceKeepKey = "reference-keep"

type ReferenceKeepSuite struct{ suite.Suite }

func TestReferenceKeepSuite(t *testing.T) { suite.Run(t, new(ReferenceKeepSuite)) }

// This exercises the published file through the same reference/world-store
// boundary an author uses when saving a personal copy.
func (s *ReferenceKeepSuite) TestShippedKeepCanBeCopiedWithoutChangingTheReference() {
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(s.T()), referenceKeepKey+".yaml"))
	s.Require().NoError(err)
	// The shared shelf (shelf_test.go) holds the published file: content/.
	registry := joinShelf(s.T(), dungeonstest.WorldStore(s.T()), true)
	original, err := registry.Get(ctx, dungeonstest.Caller, referenceKeepKey)
	s.Require().NoError(err)
	s.Equal(dungeons.SourceShipped, original.Source)
	s.True(original.SingleRoom, "the reference is offered by the regular single-room picker")
	s.NotEmpty(original.Atlas.Cells)
	listed, err := registry.List(ctx, dungeonstest.Caller)
	s.Require().NoError(err)
	s.Contains(listed, dungeons.Summary{Key: referenceKeepKey, Name: original.Name, Source: dungeons.SourceShipped})
	_, err = registry.Put(ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: referenceKeepKey, YAML: raw})
	s.ErrorIs(err, dungeons.ErrReferenceKey)

	const copyKey = "my-keep"
	copied := bytes.Replace(raw, []byte("key: "+referenceKeepKey), []byte("key: "+copyKey), 1)
	copied = bytes.ReplaceAll(copied, []byte(original.Name), []byte("My Keep"))
	saved, err := registry.Put(ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: copyKey, YAML: copied})
	s.Require().NoError(err)
	s.Require().Empty(saved.Errors)
	s.Require().NotNil(saved.Entry)
	mine, err := registry.Get(ctx, dungeonstest.Caller, copyKey)
	s.Require().NoError(err)
	s.Equal(dungeons.SourceWorld, mine.Source)
	s.Equal("My Keep", mine.Name)
	s.Equal(copied, mine.YAML)
	s.Equal(original.Atlas.Cells, mine.Atlas.Cells, "copying retains the playable floor")
	again, err := registry.Get(ctx, dungeonstest.Caller, referenceKeepKey)
	s.Require().NoError(err)
	s.Equal(raw, again.YAML, "saving a personal copy does not rewrite the reference")
	s.Equal(original.Name, again.Name)
}
