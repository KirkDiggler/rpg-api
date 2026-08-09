package authoring

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

func durabilityCanvasYAML(key, name string, width int, placement string) string {
	return fmt.Sprintf("version: 1\nkey: %s\nname: %s\nheight: 1\ncanvas: { width: %d, height: 2 }\nrooms: []\n%s", key, name, width, placement)
}

func newDurabilityOrchestrator(t *testing.T) (*Orchestrator, *dungeonregistry.Registry, string) {
	t.Helper()
	dir := t.TempDir()
	registry := dungeonregistry.New(nil)
	orch, err := New(&Config{Registry: registry, ContentDir: dir, PartyStartSeatCount: 4})
	require.NoError(t, err)
	return orch, registry, dir
}

func firstFieldErrorMessage(out *PutDungeonOutput) string {
	if out == nil || len(out.FieldErrors) == 0 {
		return "provider validation failed without a field error"
	}
	return out.FieldErrors[0].Message
}

func TestPutDungeon_SameKeyWaiterRunsAfterCommittedWinner(t *testing.T) {
	orch, registry, dir := newDurabilityOrchestrator(t)
	const key = "linear-update"
	initial := durabilityCanvasYAML(key, "Initial", 6, "")
	out, err := orch.PutDungeon(context.Background(), &PutDungeonInput{Key: key, YAML: initial})
	require.NoError(t, err)
	require.True(t, out.Success)

	winner := durabilityCanvasYAML(key, "Winner", 6, "place:\n  - { ref: dnd5e:props:pillar, at: [4, 0] }\n")
	// The complete loser candidate explicitly omits Winner's placement. It is
	// valid standalone and must run only after Winner's durable commit.
	loser := durabilityCanvasYAML(key, "Loser", 4, "")

	productionReplace := orch.replaceSource
	winnerAtWrite := make(chan struct{})
	releaseWinner := make(chan struct{})
	var signalWinner sync.Once
	orch.replaceSource = func(in *replaceSourceInput) error {
		if strings.Contains(string(in.Data), "name: Winner") {
			signalWinner.Do(func() { close(winnerAtWrite) })
			<-releaseWinner
		}
		return productionReplace(in)
	}

	type result struct {
		out *PutDungeonOutput
		err error
	}
	winnerDone := make(chan result, 1)
	go func() {
		resultOut, resultErr := orch.PutDungeon(context.Background(), &PutDungeonInput{Key: key, YAML: winner})
		winnerDone <- result{out: resultOut, err: resultErr}
	}()
	<-winnerAtWrite

	loserStarted := make(chan struct{})
	loserDone := make(chan result, 1)
	go func() {
		close(loserStarted)
		resultOut, resultErr := orch.PutDungeon(context.Background(), &PutDungeonInput{Key: key, YAML: loser})
		loserDone <- result{out: resultOut, err: resultErr}
	}()
	<-loserStarted
	select {
	case got := <-loserDone:
		t.Fatalf("same-key waiter completed before winner committed: out=%+v err=%v", got.out, got.err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseWinner)
	winnerResult := <-winnerDone
	require.NoError(t, winnerResult.err)
	require.True(t, winnerResult.out.Success)

	loserResult := <-loserDone
	require.NoError(t, loserResult.err)
	require.True(t, loserResult.out.Success)

	disk, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
	require.NoError(t, err)
	require.Equal(t, loser, string(disk))
	entry, ok := registry.Get(key)
	require.True(t, ok)
	require.Equal(t, "Loser", entry.Name)
}

func TestPutDungeon_DifferentKeysDoNotWaitForEachOther(t *testing.T) {
	orch, _, _ := newDurabilityOrchestrator(t)
	blockedAtWrite := make(chan struct{})
	releaseBlocked := make(chan struct{})
	productionReplace := orch.replaceSource
	orch.replaceSource = func(in *replaceSourceInput) error {
		if strings.Contains(string(in.Data), "key: blocked-key") {
			close(blockedAtWrite)
			<-releaseBlocked
		}
		return productionReplace(in)
	}

	blockedDone := make(chan error, 1)
	go func() {
		out, err := orch.PutDungeon(context.Background(), &PutDungeonInput{
			Key: "blocked-key", YAML: durabilityCanvasYAML("blocked-key", "Blocked", 4, ""),
		})
		if err == nil && !out.Success {
			err = errors.New(firstFieldErrorMessage(out))
		}
		blockedDone <- err
	}()
	<-blockedAtWrite

	otherDone := make(chan error, 1)
	go func() {
		out, err := orch.PutDungeon(context.Background(), &PutDungeonInput{
			Key: "other-key", YAML: durabilityCanvasYAML("other-key", "Other", 4, ""),
		})
		if err == nil && !out.Success {
			err = errors.New(firstFieldErrorMessage(out))
		}
		otherDone <- err
	}()

	select {
	case err := <-otherDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		close(releaseBlocked)
		t.Fatal("an update to a different key was unnecessarily serialized")
	}
	close(releaseBlocked)
	require.NoError(t, <-blockedDone)
}

func TestPutDungeon_ReplacementFaultsPreservePriorSourceAndRegistry(t *testing.T) {
	for _, fault := range []string{"chmod", "write", "short-write", "file-sync", "file-close", "open-directory", "rename"} {
		t.Run(fault, func(t *testing.T) {
			orch, registry, dir := newDurabilityOrchestrator(t)
			const key = "fault-safe"
			initial := durabilityCanvasYAML(key, "Initial", 4, "")
			out, err := orch.PutDungeon(context.Background(), &PutDungeonInput{Key: key, YAML: initial})
			require.NoError(t, err)
			require.True(t, out.Success)
			beforeEntry, ok := registry.Get(key)
			require.True(t, ok)

			orch.replaceSource = func(in *replaceSourceInput) error {
				return replaceSourceDurablyWithOps(in, &faultDurableOps{fault: fault})
			}
			candidate := durabilityCanvasYAML(key, "Candidate", 5, "")
			out, err = orch.PutDungeon(context.Background(), &PutDungeonInput{Key: key, YAML: candidate})
			require.Nil(t, out)
			require.Error(t, err)

			disk, readErr := os.ReadFile(filepath.Join(dir, key+".yaml"))
			require.NoError(t, readErr)
			require.Equal(t, initial, string(disk), "pre-rename failure must preserve prior bytes")
			afterEntry, ok := registry.Get(key)
			require.True(t, ok)
			require.Equal(t, beforeEntry, afterEntry, "failed durable replacement must not swap the registry")

			entries, readDirErr := os.ReadDir(dir)
			require.NoError(t, readDirErr)
			require.Len(t, entries, 1, "failed replacement must clean its temporary file")
			require.Equal(t, key+".yaml", entries[0].Name())
		})
	}
}

func TestPutDungeon_DirectorySyncFailureReportsIndeterminateCommit(t *testing.T) {
	orch, registry, dir := newDurabilityOrchestrator(t)
	const key = "directory-sync-fault"
	initial := durabilityCanvasYAML(key, "Initial", 4, "")
	out, err := orch.PutDungeon(context.Background(), &PutDungeonInput{Key: key, YAML: initial})
	require.NoError(t, err)
	require.True(t, out.Success)
	beforeEntry, ok := registry.Get(key)
	require.True(t, ok)

	productionReplace := orch.replaceSource
	orch.replaceSource = func(in *replaceSourceInput) error {
		return replaceSourceDurablyWithOps(in, &faultDurableOps{fault: "directory-sync"})
	}
	candidate := durabilityCanvasYAML(key, "Candidate", 5, "")
	out, err = orch.PutDungeon(context.Background(), &PutDungeonInput{Key: key, YAML: candidate})
	require.Nil(t, out)
	require.ErrorContains(t, err, "sync source directory after rename")

	// Rename is already the filesystem commit point. The helper cannot claim a
	// rollback when the following directory sync fails: current lookup sees the
	// complete candidate, while the registry remains at the last durable entry.
	disk, readErr := os.ReadFile(filepath.Join(dir, key+".yaml"))
	require.NoError(t, readErr)
	require.Equal(t, candidate, string(disk))
	afterEntry, ok := registry.Get(key)
	require.True(t, ok)
	require.Equal(t, beforeEntry, afterEntry)

	// A normal retry reconciles the durable source and live registry.
	orch.replaceSource = productionReplace
	out, err = orch.PutDungeon(context.Background(), &PutDungeonInput{Key: key, YAML: candidate})
	require.NoError(t, err)
	require.True(t, out.Success)
	reconciled, ok := registry.Get(key)
	require.True(t, ok)
	require.Equal(t, "Candidate", reconciled.Name)
}

func TestReplaceSourceDurably_SyncsFileAndDirectoryAndAppliesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dungeon.yaml")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	ops := &trackingDurableOps{}

	err := replaceSourceDurablyWithOps(&replaceSourceInput{Path: path, Data: []byte("new"), Mode: 0o600}, ops)
	require.NoError(t, err)
	require.True(t, ops.fileSynced, "temporary bytes must be synced before rename")
	require.True(t, ops.directorySynced, "the containing directory must be synced after rename")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "new", string(got))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

type faultDurableOps struct {
	fault string
}

func (o *faultDurableOps) CreateTemp(dir, pattern string) (durableFile, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &faultDurableFile{File: file, fault: o.fault}, nil
}

func (o *faultDurableOps) Open(path string) (durableFile, error) {
	if o.fault == "open-directory" {
		return nil, errors.New("injected directory open failure")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if o.fault == "directory-sync" {
		return &faultDirectoryFile{File: file}, nil
	}
	return file, nil
}

func (o *faultDurableOps) Rename(oldPath, newPath string) error {
	if o.fault == "rename" {
		return errors.New("injected rename failure")
	}
	return os.Rename(oldPath, newPath)
}

func (o *faultDurableOps) Remove(path string) error { return os.Remove(path) }

type faultDurableFile struct {
	*os.File
	fault string
}

func (f *faultDurableFile) Chmod(mode fs.FileMode) error {
	if f.fault == "chmod" {
		return errors.New("injected chmod failure")
	}
	return f.File.Chmod(mode)
}

func (f *faultDurableFile) Write(data []byte) (int, error) {
	switch f.fault {
	case "write":
		return 0, errors.New("injected write failure")
	case "short-write":
		return len(data) / 2, nil
	default:
		return f.File.Write(data)
	}
}

func (f *faultDurableFile) Sync() error {
	if f.fault == "file-sync" {
		return errors.New("injected file sync failure")
	}
	return f.File.Sync()
}

func (f *faultDurableFile) Close() error {
	if f.fault == "file-close" {
		_ = f.File.Close()
		return errors.New("injected file close failure")
	}
	return f.File.Close()
}

type faultDirectoryFile struct {
	*os.File
}

func (f *faultDirectoryFile) Sync() error {
	return errors.New("injected directory sync failure")
}

type trackingDurableOps struct {
	fileSynced      bool
	directorySynced bool
}

func (o *trackingDurableOps) CreateTemp(dir, pattern string) (durableFile, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &trackingDurableFile{File: file, synced: &o.fileSynced}, nil
}

func (o *trackingDurableOps) Open(path string) (durableFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &trackingDurableFile{File: file, synced: &o.directorySynced}, nil
}

func (o *trackingDurableOps) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (o *trackingDurableOps) Remove(path string) error { return os.Remove(path) }

type trackingDurableFile struct {
	*os.File
	synced *bool
}

func (f *trackingDurableFile) Sync() error {
	*f.synced = true
	return f.File.Sync()
}
