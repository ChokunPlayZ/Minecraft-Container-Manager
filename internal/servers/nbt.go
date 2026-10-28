package servers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/nbt"
)

var (
	ErrNBTBusy    = errors.New("stop the server and wait for active tasks to finish before saving NBT data")
	ErrNBTChanged = errors.New("this file changed or was removed on the server; reload it before saving")
)

type NBTFile struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Editable bool   `json:"editable"`
	nbt.Document
	BackupPath string `json:"backup_path,omitempty"`
}

type PlayerDataFile struct {
	UUID       string    `json:"uuid"`
	Name       string    `json:"name"`
	World      string    `json:"world"`
	Path       string    `json:"path"`
	ModifiedAt time.Time `json:"modified_at"`
}

func (s *Store) nbtStopped(ctx context.Context, srv Server) bool {
	if srv.State != StateStopped && srv.State != StateError {
		return false
	}
	if srv.ContainerID != "" {
		if isDockerNil(s.docker) {
			return false
		}
		inspect, err := s.docker.Inspect(ctx, srv.ContainerID)
		if err != nil || (inspect.Status != "exited" && inspect.Status != "dead" && inspect.Status != "created") {
			return false
		}
	}
	if task := s.GetTaskProgress(srv.ID); task != nil && task.Stage != "completed" && task.Stage != "failed" {
		return false
	}
	return true
}

// Use an OS-rooted handle as well as rejecting symlinks, so concurrent path
// changes cannot escape the server directory. Never read devices or FIFOs.
func (s *Store) openNBTRoot(id, rel string) (*os.Root, error) {
	if rel == "" || rel == "." || strings.ContainsRune(rel, '\x00') {
		return nil, ErrInvalidPath
	}
	if _, err := s.worldPath(id, rel); err != nil {
		return nil, err
	}
	return os.OpenRoot(s.dataPath(id))
}
func readNBTBytes(root *os.Root, rel string) ([]byte, fs.FileInfo, error) {
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		return nil, nil, ErrIsDirectory
	}
	if !info.Mode().IsRegular() {
		return nil, nil, ErrInvalidPath
	}
	if info.Size() > nbt.MaxBytes {
		return nil, nil, ErrReadTooLarge
	}
	f, err := root.Open(rel)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, nbt.MaxBytes+1))
	if len(data) > nbt.MaxBytes {
		return nil, nil, ErrReadTooLarge
	}
	return data, info, err
}
func nbtRevision(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func (s *Store) ReadNBT(ctx context.Context, id, rel string) (NBTFile, error) {
	unlock := s.lockWorld(id, false)
	defer unlock()
	srv, err := s.Get(ctx, id)
	if err != nil {
		return NBTFile{}, err
	}
	s.fileEditMu.Lock()
	defer s.fileEditMu.Unlock()
	root, err := s.openNBTRoot(id, rel)
	if err != nil {
		return NBTFile{}, err
	}
	defer root.Close()
	data, _, err := readNBTBytes(root, rel)
	if err != nil {
		return NBTFile{}, err
	}
	doc, err := nbt.Decode(data)
	if err != nil {
		return NBTFile{}, err
	}
	return NBTFile{Path: rel, Revision: nbtRevision(data), Editable: s.nbtStopped(ctx, srv), Document: doc}, nil
}

func writeNBTFile(root *os.Root, rel string, data []byte, mode fs.FileMode) error {
	f, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	return f.Close()
}

// Writes serialize with world changes, startup, and text-editor saves. The
// original file is backed up byte-for-byte before an atomic same-dir rename.
func (s *Store) WriteNBT(ctx context.Context, id, rel, expected string, doc nbt.Document) (NBTFile, error) {
	unlock := s.lockWorld(id, true)
	defer unlock()
	srv, err := s.Get(ctx, id)
	if err != nil {
		return NBTFile{}, err
	}
	if !s.nbtStopped(ctx, srv) {
		return NBTFile{}, ErrNBTBusy
	}
	s.fileEditMu.Lock()
	defer s.fileEditMu.Unlock()
	root, err := s.openNBTRoot(id, rel)
	if err != nil {
		return NBTFile{}, err
	}
	defer root.Close()
	original, info, err := readNBTBytes(root, rel)
	if os.IsNotExist(err) {
		return NBTFile{}, ErrNBTChanged
	}
	if err != nil {
		return NBTFile{}, err
	}
	if expected == "" || nbtRevision(original) != expected {
		return NBTFile{}, ErrNBTChanged
	}
	previous, err := nbt.Decode(original)
	if err != nil {
		return NBTFile{}, err
	}
	doc.Compression = previous.Compression
	data, err := nbt.Encode(doc)
	if err != nil {
		return NBTFile{}, err
	}
	backup := rel + ".mcm-nbt-" + uuid.NewString() + ".bak"
	if err = writeNBTFile(root, backup, original, 0600); err != nil {
		return NBTFile{}, err
	}
	temp := path.Join(path.Dir(rel), ".mcm-nbt-"+uuid.NewString()+".tmp")
	defer root.Remove(temp)
	if err = writeNBTFile(root, temp, data, info.Mode().Perm()); err != nil {
		return NBTFile{}, err
	}
	// Recheck after encoding/backup work, including external editors.
	current, _, err := readNBTBytes(root, rel)
	if os.IsNotExist(err) || (err == nil && nbtRevision(current) != expected) {
		return NBTFile{}, ErrNBTChanged
	}
	if err != nil {
		return NBTFile{}, err
	}
	if err = root.Rename(temp, rel); err != nil {
		return NBTFile{}, err
	}
	return NBTFile{Path: rel, Revision: nbtRevision(data), Editable: true, Document: doc, BackupPath: backup}, nil
}

// Discovery only inspects playerdata directories, not potentially huge region
// trees. It supports world folders nested by modded/multiworld servers.
func (s *Store) ListPlayerData(ctx context.Context, id string) ([]PlayerDataFile, error) {
	unlock := s.lockWorld(id, false)
	defer unlock()
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.dataPath(id))
	if os.IsNotExist(err) {
		return []PlayerDataFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	names := map[string]string{}
	for _, rel := range []string{"usercache.json", "whitelist.json", "ops.json"} {
		if _, err := s.worldPath(id, rel); err != nil {
			continue
		}
		data, _, err := readNBTBytes(root, rel)
		if err != nil || len(data) > 1<<20 {
			continue
		}
		var entries []struct {
			UUID string
			Name string
		}
		if json.Unmarshal(data, &entries) == nil {
			for _, e := range entries {
				if u, err := uuid.Parse(e.UUID); err == nil {
					names[u.String()] = e.Name
				}
			}
		}
	}
	result := []PlayerDataFile{}
	visited := 0
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > 50000 {
			return fmt.Errorf("%w: too many folders to discover player data", nbt.ErrInvalid)
		}
		if !d.IsDir() {
			return nil
		}
		if rel != "." && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if strings.Count(rel, "/") > 5 {
			return fs.SkipDir
		}
		if d.Name() == "playerdata" {
			entries, err := fs.ReadDir(root.FS(), rel)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(strings.ToLower(e.Name()), ".dat") {
					continue
				}
				u, err := uuid.Parse(strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
				if err != nil {
					continue
				}
				info, err := e.Info()
				if err != nil || !info.Mode().IsRegular() {
					continue
				}
				result = append(result, PlayerDataFile{UUID: u.String(), Name: names[u.String()], World: path.Dir(rel), Path: path.Join(rel, e.Name()), ModifiedAt: info.ModTime()})
			}
			return fs.SkipDir
		}
		switch d.Name() {
		case "region", "entities", "poi", "plugins", "mods", "libraries", "backups", ".mcm-trash":
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Name != b.Name {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		return a.Path < b.Path
	})
	return result, nil
}
