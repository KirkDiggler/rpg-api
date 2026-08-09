package authoring

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// keyedMutex serializes updates for one dungeon key without making an
// unrelated dungeon wait. Entries are reference-counted so author-created keys
// do not accumulate in the process for the lifetime of the server.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyLock
}

type keyLock struct {
	mu   sync.Mutex
	refs int
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[string]*keyLock)}
}

func (m *keyedMutex) lock(key string) func() {
	m.mu.Lock()
	lock := m.locks[key]
	if lock == nil {
		lock = &keyLock{}
		m.locks[key] = lock
	}
	lock.refs++
	m.mu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()

		m.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
}

type replaceSourceInput struct {
	Path string
	Data []byte
	Mode fs.FileMode
}

type durableFile interface {
	Write([]byte) (int, error)
	Sync() error
	Chmod(fs.FileMode) error
	Close() error
	Name() string
}

type durableFileOps interface {
	CreateTemp(string, string) (durableFile, error)
	Open(string) (durableFile, error)
	Rename(string, string) error
	Remove(string) error
}

type osDurableFileOps struct{}

func (osDurableFileOps) CreateTemp(dir, pattern string) (durableFile, error) {
	return os.CreateTemp(dir, pattern)
}

func (osDurableFileOps) Open(path string) (durableFile, error) {
	return os.Open(path) //nolint:gosec // ContentDir is operator-controlled.
}

func (osDurableFileOps) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (osDurableFileOps) Remove(path string) error {
	return os.Remove(path)
}

func replaceSourceDurably(in *replaceSourceInput) error {
	return replaceSourceDurablyWithOps(in, osDurableFileOps{})
}

// replaceSourceDurablyWithOps implements the Linux/POSIX durability contract
// used by the production deployment: write a same-directory temporary file,
// sync its bytes and metadata, close it, rename it over the live source, then
// sync the containing directory so an acknowledged rename survives restart.
//
// A crash before rename leaves the old source intact (an unindexed temporary
// file may remain). A crash after rename but before the directory sync may
// expose either the complete old file or the complete new file after restart,
// never a torn mixture. The function returns success only after both syncs.
func replaceSourceDurablyWithOps(in *replaceSourceInput, ops durableFileOps) error {
	if in == nil {
		return errors.New("replace source: input is required")
	}
	if in.Path == "" {
		return errors.New("replace source: path is required")
	}
	if ops == nil {
		return errors.New("replace source: file operations are required")
	}

	dir := filepath.Dir(in.Path)
	pattern := "." + filepath.Base(in.Path) + ".tmp-*"
	temp, err := ops.CreateTemp(dir, pattern)
	if err != nil {
		return fmt.Errorf("create same-directory temporary file: %w", err)
	}
	tempPath := temp.Name()
	tempClosed := false
	renamed := false
	defer func() {
		if !tempClosed {
			_ = temp.Close()
		}
		if !renamed {
			_ = ops.Remove(tempPath)
		}
	}()

	chmodErr := temp.Chmod(in.Mode)
	if chmodErr != nil {
		return fmt.Errorf("set temporary file permissions: %w", chmodErr)
	}
	n, writeErr := temp.Write(in.Data)
	if writeErr != nil {
		return fmt.Errorf("write temporary file: %w", writeErr)
	}
	if n != len(in.Data) {
		return fmt.Errorf("write temporary file: wrote %d of %d bytes: %w", n, len(in.Data), io.ErrShortWrite)
	}
	syncErr := temp.Sync()
	if syncErr != nil {
		return fmt.Errorf("sync temporary file: %w", syncErr)
	}
	closeErr := temp.Close()
	if closeErr != nil {
		return fmt.Errorf("close temporary file: %w", closeErr)
	}
	tempClosed = true

	// Open the directory before rename so an open failure is still a
	// pre-commit failure that leaves the original source untouched.
	directory, err := ops.Open(dir)
	if err != nil {
		return fmt.Errorf("open source directory for sync: %w", err)
	}
	directoryClosed := false
	defer func() {
		if !directoryClosed {
			_ = directory.Close()
		}
	}()

	if err := ops.Rename(tempPath, in.Path); err != nil {
		return fmt.Errorf("rename temporary file over source: %w", err)
	}
	renamed = true
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync source directory after rename: %w", err)
	}
	// Directory descriptors carry no buffered file data after a successful
	// Sync. Closing releases the descriptor; it cannot invalidate the durable
	// rename, so a close error is deliberately not reported as a failed update.
	_ = directory.Close()
	directoryClosed = true
	return nil
}
