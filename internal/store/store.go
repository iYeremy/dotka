// Package store implements Dotka's content-addressed object store and the
// generation manifests that reference it.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/iYeremy/dotka/internal/fsutil"
)

// ErrNotInitialized is returned when the store directory does not exist.
var ErrNotInitialized = errors.New("dotka store not initialized (run `dotka init`)")

// Store is a directory holding objects, generations and the operation log.
type Store struct {
	Dir string
}

// Open opens an existing store.
func Open(dir string) (*Store, error) {
	if _, err := os.Stat(filepath.Join(dir, "objects")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotInitialized
		}
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// Create creates a new store with private permissions. Fails if it exists.
func Create(dir string) (*Store, error) {
	if _, err := os.Stat(filepath.Join(dir, "objects")); err == nil {
		return nil, fmt.Errorf("store already exists at %s", dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	for _, d := range []string{"objects", "generations"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) objectPath(hash string) string {
	return filepath.Join(s.Dir, "objects", hash[:2], hash[2:])
}

// HashFile returns the sha256 of a file without storing it.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashBytes returns the sha256 of b.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// PutFile copies a file into the store (if not already there) and returns
// its hash. The content is hashed while copying, so a file modified during
// the copy still produces a consistent (hash, object) pair.
func (s *Store) PutFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	tmp, err := os.CreateTemp(filepath.Join(s.Dir, "objects"), ".incoming-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), f); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	hash := hex.EncodeToString(h.Sum(nil))
	dst := s.objectPath(hash)
	if _, err := os.Stat(dst); err == nil {
		return hash, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o400); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	return hash, nil
}

// Has reports whether an object exists.
func (s *Store) Has(hash string) bool {
	if len(hash) < 3 {
		return false
	}
	_, err := os.Stat(s.objectPath(hash))
	return err == nil
}

// Open returns a reader for an object.
func (s *Store) OpenObject(hash string) (*os.File, error) {
	if len(hash) != 64 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}
	return os.Open(s.objectPath(hash))
}

// ReadObject returns the full content of an object.
func (s *Store) ReadObject(hash string) ([]byte, error) {
	f, err := s.OpenObject(hash)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// ---- Generations ----------------------------------------------------------

func (s *Store) genPath(id int) string {
	return filepath.Join(s.Dir, "generations", fmt.Sprintf("%06d.json", id))
}

// SaveGeneration writes a manifest. v must marshal to JSON. Generations are
// immutable: saving an existing id fails.
func (s *Store) SaveGeneration(id int, v any) error {
	p := s.genPath(id)
	if _, err := os.Stat(p); err == nil {
		return fmt.Errorf("generation %d already exists", id)
	}
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	if err := fsutil.AtomicWrite(p, strings.NewReader(string(b)), 0o400); err != nil {
		return err
	}
	return s.bumpLastID(id)
}

// DeleteGeneration removes a generation manifest. Its objects stay in the
// store until GC.
func (s *Store) DeleteGeneration(id int) error {
	err := os.Remove(s.genPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("generation %d does not exist", id)
	}
	return err
}

// lastID is the highest id ever used, so ids are never reused even after
// generations are deleted.
func (s *Store) lastID() int {
	b, err := os.ReadFile(filepath.Join(s.Dir, "LAST_ID"))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func (s *Store) bumpLastID(id int) error {
	if id <= s.lastID() {
		return nil
	}
	return fsutil.AtomicWrite(filepath.Join(s.Dir, "LAST_ID"), strings.NewReader(strconv.Itoa(id)+"\n"), 0o600)
}

// LoadGeneration reads a manifest into v.
func (s *Store) LoadGeneration(id int, v any) error {
	b, err := os.ReadFile(s.genPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("generation %d does not exist", id)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// GenerationIDs returns all generation ids in ascending order.
func (s *Store) GenerationIDs() ([]int, error) {
	ents, err := os.ReadDir(filepath.Join(s.Dir, "generations"))
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, e := range ents {
		name := strings.TrimSuffix(e.Name(), ".json")
		if name == e.Name() {
			continue
		}
		if id, err := strconv.Atoi(name); err == nil {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids, nil
}

// NextID returns the id for a new generation.
func (s *Store) NextID() (int, error) {
	ids, err := s.GenerationIDs()
	if err != nil {
		return 0, err
	}
	next := s.lastID() + 1
	if len(ids) > 0 && ids[len(ids)-1] >= next {
		next = ids[len(ids)-1] + 1
	}
	return next, nil
}

// Head returns the generation the home is considered to be based on.
func (s *Store) Head() (int, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, "HEAD"))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

// SetHead updates HEAD atomically.
func (s *Store) SetHead(id int) error {
	return fsutil.AtomicWrite(filepath.Join(s.Dir, "HEAD"), strings.NewReader(strconv.Itoa(id)+"\n"), 0o600)
}

// ---- Operation log --------------------------------------------------------

// LogEntry is one line of log.jsonl.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Op      string    `json:"op"`
	Gen     int       `json:"gen,omitempty"`
	Target  int       `json:"target,omitempty"`
	Result  string    `json:"result"`
	Details string    `json:"details,omitempty"`
}

// AppendLog appends an entry to the operation log.
func (s *Store) AppendLog(e LogEntry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, "log.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// ---- Locking --------------------------------------------------------------

// Lock takes an exclusive lock on the store so two dotka processes never
// modify it (or $HOME) at the same time. Call the returned func to release.
func (s *Store) Lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.Dir, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("another dotka process is running")
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// Size returns the total size of the store on disk.
func (s *Store) Size() int64 {
	var total int64
	filepath.WalkDir(s.Dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
			}
		}
		return nil
	})
	return total
}

// ---- Garbage collection ---------------------------------------------------

// GC removes objects not present in keep. Returns removed count and bytes.
func (s *Store) GC(keep map[string]bool) (int, int64, error) {
	root := filepath.Join(s.Dir, "objects")
	var n int
	var bytes int64
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		hash := strings.ReplaceAll(rel, string(filepath.Separator), "")
		if strings.HasPrefix(d.Name(), ".incoming-") || !keep[hash] {
			if fi, err := d.Info(); err == nil {
				bytes += fi.Size()
			}
			if err := os.Remove(p); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, bytes, err
}
