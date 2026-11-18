package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/suite"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

const beforeCastle = "../../internal/holdingwords/testdata/the-mad-king-before-holding-words.yaml"

var shippedCastle = filepath.Join("..", "..", "content", "the-mad-king.yaml")

type CommandSuite struct {
	suite.Suite

	ctx    context.Context
	before []byte
	after  []byte
	store  *worldStoreHandle
}

func TestCommandSuite(t *testing.T) {
	suite.Run(t, new(CommandSuite))
}

func (s *CommandSuite) SetupTest() {
	s.ctx = context.Background()
	var err error
	s.before, err = os.ReadFile(beforeCastle)
	s.Require().NoError(err)
	s.after, err = os.ReadFile(shippedCastle) //nolint:gosec // the repository's own content
	s.Require().NoError(err)

	mr := miniredis.RunT(s.T())
	client, err := redisclient.NewClient(mr.Addr(), nil)
	s.Require().NoError(err)
	repo, err := worlddungeons.NewRedis(&worlddungeons.RedisConfig{Client: client})
	s.Require().NoError(err)
	s.store = &worldStoreHandle{repo: repo, client: client}
}

func (s *CommandSuite) run(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(s.ctx, args, &out, &errOut, func(string) (*worldStoreHandle, error) { return s.store, nil })
	return code, out.String(), errOut.String()
}

// TestAContentFolder: the old castle in a folder converts in place, its
// original kept beside it; a dry run first writes nothing; a second run is a
// no-op.
func (s *CommandSuite) TestAContentFolder() {
	dir := s.T().TempDir()
	castle := filepath.Join(dir, "the-mad-king.yaml")
	s.Require().NoError(os.WriteFile(castle, s.before, 0o600))

	code, out, _ := s.run("--dry-run", dir)
	s.Equal(0, code)
	s.Contains(out, castle+":1589:17: recognized -> holds: recognized -> holds")
	s.Contains(out, "would convert (dry run, nothing written) 1")
	onDisk, err := os.ReadFile(castle) //nolint:gosec // a temp dir
	s.Require().NoError(err)
	s.Equal(s.before, onDisk, "a dry run writes nothing")

	code, out, errOut := s.run(dir)
	s.Equal(0, code, errOut)
	s.Contains(out, castle+": 46 changes")
	onDisk, err = os.ReadFile(castle) //nolint:gosec // a temp dir
	s.Require().NoError(err)
	s.NotEqual(s.before, onDisk)
	kept, err := os.ReadFile(castle + backupSuffix) //nolint:gosec // a temp dir
	s.Require().NoError(err)
	s.Equal(s.before, kept, "the original is kept beside it")

	code, out, _ = s.run(dir)
	s.Equal(0, code)
	s.Contains(out, "converted 0, already in the new words 1")
}

// TestTheWorldStore: an author's saved old-words dungeon is converted where
// it is stored, owner and name kept, its original written to the backup
// directory first; a new-words one is untouched.
func (s *CommandSuite) TestTheWorldStore() {
	s.put("w1", "alice", "my-castle", s.before)
	s.put("w2", "bob", "his-castle", s.after)
	backups := s.T().TempDir()

	code, out, errOut := s.run("--world-store", "--backup-dir", backups)
	s.Equal(0, code, errOut)
	s.Contains(out, "world w1, dungeon my-castle (owner alice): 46 changes")
	s.Contains(out, "converted 1, already in the new words 1")

	got, err := s.store.repo.Get(s.ctx, &worlddungeons.GetInput{WorldID: "w1", Key: "my-castle"})
	s.Require().NoError(err)
	s.Equal("alice", got.Record.Owner)
	s.Equal("My Castle", got.Record.Name)
	s.NotEqual(s.before, got.Record.YAML)
	kept, err := os.ReadFile(filepath.Join(backups, "w1", "my-castle.yaml")) //nolint:gosec // a temp dir
	s.Require().NoError(err)
	s.Equal(s.before, kept)
	_, err = os.Stat(filepath.Join(backups, "w2"))
	s.True(os.IsNotExist(err), "nothing to back up for a file already in the new words")

	code, out, _ = s.run("--world-store", "--backup-dir", backups)
	s.Equal(0, code)
	s.Contains(out, "converted 0, already in the new words 2")
}

// TestTheWorldStoreNeedsABackupDir: nothing is replaced without somewhere to
// keep the original.
func (s *CommandSuite) TestTheWorldStoreNeedsABackupDir() {
	code, _, errOut := s.run("--world-store")
	s.Equal(2, code)
	s.Contains(errOut, "--world-store needs --backup-dir")
}

func (s *CommandSuite) put(world, owner, key string, raw []byte) {
	_, err := s.store.repo.Put(s.ctx, &worlddungeons.PutInput{
		WorldID: world, Owner: owner, Key: key, Name: "My Castle", YAML: raw,
	})
	s.Require().NoError(err)
}
