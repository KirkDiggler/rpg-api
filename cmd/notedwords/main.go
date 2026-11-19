// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// notedwords is a one-shot converter for dungeons written before the
// noted words (rpg-project#1005): it rewrites the retired content words
// (`recognized`, `recognize` with its `holder`, `holds <prop>`, a
// state's `holds`, a guard's `topic:`) into the new ones, changing only those
// tokens and leaving every other byte as it was. A converted file is a no-op,
// so it is safe to run twice.
//
// A content folder (what RPG_CONTENT_DIR mounts), or single files:
//
//	notedwords [--dry-run] <file-or-directory>...
//
// Each changed file is rewritten in place; the original is kept beside it as
// <name>.yaml.before-noted-words.
//
// The dungeons authors saved through the studio (the world store in Redis):
//
//	notedwords --world-store --backup-dir <dir> [--redis-addr host:port] [--dry-run]
//
// Each changed record's original is written to <dir>/<world>/<key>.yaml
// BEFORE the record is replaced, and a record whose backup file already
// exists is refused rather than overwritten, so the true original is never
// lost to a second run.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/notedwords"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

const (
	yamlExt             = ".yaml"
	backupSuffix        = ".before-noted-words"
	envRedisAddr        = "REDIS_ADDR"
	defaultRedisAddress = "localhost:6379"
	backupFileMode      = 0o600
	backupDirMode       = 0o700
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, openWorldStore))
}

// worldStoreHandle is the world store and the client it rides on.
type worldStoreHandle struct {
	repo   worlddungeons.Repository
	client redisclient.Client
}

// openWorldStore connects to Redis at addr.
func openWorldStore(addr string) (*worldStoreHandle, error) {
	client, err := redisclient.NewClient(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to redis at %s: %w", addr, err)
	}
	if pingErr := client.Ping(context.Background()).Err(); pingErr != nil {
		return nil, fmt.Errorf("connect to redis at %s: %w", addr, pingErr)
	}
	repo, err := worlddungeons.NewRedis(&worlddungeons.RedisConfig{Client: client})
	if err != nil {
		return nil, err
	}
	return &worldStoreHandle{repo: repo, client: client}, nil
}

// config is the parsed command line.
type config struct {
	dryRun     bool
	worldStore bool
	backupDir  string
	redisAddr  string
	paths      []string
}

func parse(args []string, stderr io.Writer) (*config, error) {
	fs := flag.NewFlagSet("notedwords", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := &config{}
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "print what would change, file and line; change nothing")
	fs.BoolVar(&cfg.worldStore, "world-store", false, "convert the dungeons authors saved (Redis), not files")
	fs.StringVar(&cfg.backupDir, "backup-dir", "", "world store: where each original is written before it is replaced")
	addr := os.Getenv(envRedisAddr)
	if addr == "" {
		addr = defaultRedisAddress
	}
	fs.StringVar(&cfg.redisAddr, "redis-addr", addr, "world store: the Redis address ($REDIS_ADDR)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	cfg.paths = fs.Args()
	switch {
	case cfg.worldStore && len(cfg.paths) > 0:
		return nil, errors.New("--world-store takes no paths")
	case cfg.worldStore && cfg.backupDir == "" && !cfg.dryRun:
		return nil, errors.New("--world-store needs --backup-dir: each original is written there before it is replaced")
	case !cfg.worldStore && len(cfg.paths) == 0:
		return nil, errors.New("name a dungeon file or a content directory, or pass --world-store")
	}
	return cfg, nil
}

// run is the command; the exit code is 1 when anything was refused.
func run(ctx context.Context, args []string, stdout, stderr io.Writer,
	open func(addr string) (*worldStoreHandle, error)) int {
	cfg, err := parse(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "notedwords:", err)
		return 2
	}
	r := &reporter{out: stdout, errOut: stderr, dryRun: cfg.dryRun}
	if cfg.worldStore {
		store, openErr := open(cfg.redisAddr)
		if openErr != nil {
			_, _ = fmt.Fprintln(stderr, "notedwords:", openErr)
			return 1
		}
		r.worldStore(ctx, store, cfg.backupDir)
	} else {
		for _, p := range cfg.paths {
			r.path(p)
		}
	}
	r.summary()
	if r.failed {
		return 1
	}
	return 0
}

// reporter converts and says what it did.
type reporter struct {
	out, errOut io.Writer
	dryRun      bool
	failed      bool
	changed     int
	unchanged   int
	ambiguous   int
}

func (r *reporter) failf(format string, args ...any) {
	r.failed = true
	_, _ = fmt.Fprintf(r.errOut, "notedwords: "+format+"\n", args...)
}

func (r *reporter) sayf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.out, format+"\n", args...)
}

// path converts one file, or every *.yaml directly in a directory (the
// registry reads no deeper, so neither does this).
func (r *reporter) path(p string) {
	info, err := os.Stat(p)
	if err != nil {
		r.failf("%v", err)
		return
	}
	if !info.IsDir() {
		r.file(p, info.Mode().Perm())
		return
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		r.failf("%v", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != yamlExt {
			continue
		}
		fi, infoErr := e.Info()
		if infoErr != nil {
			r.failf("%v", infoErr)
			continue
		}
		r.file(filepath.Join(p, e.Name()), fi.Mode().Perm())
	}
}

func (r *reporter) file(p string, mode os.FileMode) {
	raw, err := os.ReadFile(filepath.Clean(p))
	if err != nil {
		r.failf("%v", err)
		return
	}
	out, ok := r.convert(p, raw)
	if !ok || len(out.Changes) == 0 || r.dryRun {
		return
	}
	backup := p + backupSuffix
	if writeErr := writeNew(backup, raw, mode); writeErr != nil {
		r.failf("%s: not converted: keeping the original at %s: %v", p, backup, writeErr)
		return
	}
	if writeErr := replaceFile(p, out.Out, mode); writeErr != nil {
		r.failf("%s: not converted: %v (the original is also at %s)", p, writeErr, backup)
		return
	}
	r.sayf("%s: converted; the original is %s", p, backup)
}

// worldStore converts every saved dungeon in every world.
func (r *reporter) worldStore(ctx context.Context, store *worldStoreHandle, backupDir string) {
	worlds, err := worlddungeons.ListWorldIDs(ctx, &worlddungeons.ListWorldIDsInput{Client: store.client})
	if err != nil {
		r.failf("%v", err)
		return
	}
	for _, world := range worlds.WorldIDs {
		listed, listErr := store.repo.List(ctx, &worlddungeons.ListInput{WorldID: world})
		if listErr != nil {
			r.failf("world %s: %v", world, listErr)
			continue
		}
		for _, rec := range listed.Records {
			r.record(ctx, store.repo, backupDir, world, rec)
		}
	}
}

func (r *reporter) record(ctx context.Context, repo worlddungeons.Repository, backupDir, world string,
	rec worlddungeons.Record) {
	where := fmt.Sprintf("world %s, dungeon %s (owner %s)", world, rec.Key, rec.Owner)
	out, ok := r.convert(where, rec.YAML)
	if !ok || len(out.Changes) == 0 || r.dryRun {
		return
	}
	backup := filepath.Join(backupDir, world, rec.Key+yamlExt)
	if err := os.MkdirAll(filepath.Dir(backup), backupDirMode); err != nil {
		r.failf("%s: not converted: %v", where, err)
		return
	}
	if err := writeNew(backup, rec.YAML, backupFileMode); err != nil {
		r.failf("%s: not converted: keeping the original at %s: %v", where, backup, err)
		return
	}
	_, err := repo.Put(ctx, &worlddungeons.PutInput{
		WorldID: world, Owner: rec.Owner, Key: rec.Key, Name: rec.Name, YAML: out.Out,
	})
	if err != nil {
		r.failf("%s: not converted: %v (the original is at %s)", where, err, backup)
		return
	}
	r.sayf("%s: converted; the original is %s", where, backup)
}

// convert converts and reports one source; ok is false when it was refused.
func (r *reporter) convert(where string, raw []byte) (*notedwords.ConvertOutput, bool) {
	out, err := notedwords.Convert(&notedwords.ConvertInput{Source: raw})
	if err != nil {
		r.failf("%s: %v", where, err)
		return nil, false
	}
	for _, a := range out.Ambiguous {
		r.ambiguous++
		r.sayf("%s:%d: left as written: %s", where, a.Line, a.Reason)
	}
	if len(out.Changes) == 0 {
		r.unchanged++
		return out, true
	}
	r.changed++
	if r.dryRun {
		for _, c := range out.Changes {
			r.sayf("%s:%d:%d: %s: %s -> %s", where, c.Line, c.Column, c.Kind, c.From, c.To)
		}
	}
	r.sayf("%s: %d changes (%s)", where, len(out.Changes), countsLine(out.Counts()))
	return out, true
}

func (r *reporter) summary() {
	verb := "converted"
	if r.dryRun {
		verb = "would convert (dry run, nothing written)"
	}
	r.sayf("%s %d, already in the new words %d, left for an author %d", verb, r.changed, r.unchanged, r.ambiguous)
}

func countsLine(counts map[notedwords.Kind]int) string {
	parts := make([]string, 0, len(counts))
	for k, n := range counts {
		parts = append(parts, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// writeNew writes p only if nothing is there: a backup is never overwritten.
func writeNew(p string, raw []byte, mode os.FileMode) error {
	f, err := os.OpenFile(filepath.Clean(p), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// replaceFile writes p through a temp file and a rename, so a reader never
// sees half a dungeon.
func replaceFile(p string, raw []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, p); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
