package servers

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

var regionPattern = regexp.MustCompile(`^r\.(-?[0-9]+)\.(-?[0-9]+)\.mca$`)

type WorldRegion struct {
	X         int   `json:"x"`
	Z         int   `json:"z"`
	Chunks    int   `json:"chunks"`
	SizeBytes int64 `json:"size_bytes"`
}
type WorldRegionList struct {
	Regions []WorldRegion `json:"regions"`
	Total   int           `json:"total"`
	Offset  int           `json:"offset"`
}
type WorldChunk struct {
	X         int    `json:"x"`
	Z         int    `json:"z"`
	Timestamp uint32 `json:"timestamp"`
}
type WorldChunkSelection struct {
	Dimension string `json:"dimension"`
	RegionX   int    `json:"region_x"`
	RegionZ   int    `json:"region_z"`
	Chunks    []int  `json:"chunks"` // local z*32+x indices
}
type WorldChunkReset struct {
	Reset        int    `json:"reset"`
	RecoveryPath string `json:"recovery_path"`
}

func (s *Store) worldDimension(id, name, dimension string) (string, error) {
	roots, err := s.worldRoots(id, name)
	if err != nil {
		return "", err
	}
	// A chunk request checks only its own path, avoiding a full world-size scan
	// for every region page and selection.
	valid := false
	for _, marker := range []string{"level.dat", worldConfigFile} {
		p, pathErr := s.worldPath(id, name+"/"+marker)
		if pathErr != nil {
			return "", pathErr
		}
		if info, statErr := os.Stat(p); statErr == nil && info.Mode().IsRegular() {
			valid = true
		}
	}
	if !valid {
		return "", ErrPathNotFound
	}
	belongs := false
	for _, root := range roots {
		if dimension == root || strings.HasPrefix(dimension, root+"/") {
			belongs = true
			break
		}
	}
	if !belongs {
		return "", worldInvalid("Choose a dimension belonging to this world.")
	}
	dir, err := s.worldPath(id, dimension)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrPathNotFound
	}
	return dir, nil
}
func regionHeader(filename string) ([]byte, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	h := make([]byte, 8192)
	if _, err = io.ReadFull(f, h); err != nil {
		return nil, 0, worldInvalid("A region file has a truncated header. Restore a backup before editing chunks.")
	}
	used := map[uint32]bool{}
	for i := 0; i < 1024; i++ {
		loc := binary.BigEndian.Uint32(h[i*4:])
		if loc == 0 {
			continue
		}
		offset, count := loc>>8, loc&255
		if offset < 2 || count == 0 || int64(offset+count)*4096 > info.Size() {
			return nil, 0, worldInvalid("A region file contains invalid chunk locations. Restore a backup before editing chunks.")
		}
		for sector := offset; sector < offset+count; sector++ {
			if used[sector] {
				return nil, 0, worldInvalid("A region file contains overlapping chunks. Restore a backup before editing chunks.")
			}
			used[sector] = true
		}
	}
	return h, info.Size(), nil
}
func regionName(x, z int) (string, error) {
	// World coordinates are bounded by Minecraft's 30-million-block limit.
	if x < -58594 || x > 58593 || z < -58594 || z > 58593 {
		return "", worldInvalid("Region coordinates are outside the Minecraft world boundary.")
	}
	return fmt.Sprintf("r.%d.%d.mca", x, z), nil
}
func (s *Store) ListWorldRegions(ctx context.Context, id, name, dimension string, offset int) (WorldRegionList, error) {
	unlockWorld := s.lockWorld(id, false)
	defer unlockWorld()
	result := WorldRegionList{Regions: []WorldRegion{}, Offset: offset}
	if _, err := s.worldServer(ctx, id, false); err != nil {
		return result, err
	}
	_, err := s.worldDimension(id, name, dimension)
	if err != nil {
		return result, err
	}
	regionDir, err := s.worldPath(id, filepath.ToSlash(filepath.Join(dimension, "region")))
	if err != nil {
		return result, err
	}
	entries, err := os.ReadDir(regionDir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	names := []string{}
	for _, e := range entries {
		if regionPattern.MatchString(e.Name()) && !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	result.Total = len(names)
	if offset < 0 || offset > len(names) {
		return result, worldInvalid("Invalid region page.")
	}
	end := min(offset+100, len(names))
	for _, n := range names[offset:end] {
		p, err := s.worldPath(id, dimension+"/region/"+n)
		if err != nil {
			return result, err
		}
		h, size, err := regionHeader(p)
		if err != nil {
			return result, err
		}
		m := regionPattern.FindStringSubmatch(n)
		x, err := strconv.Atoi(m[1])
		if err != nil {
			return result, ErrInvalidPath
		}
		z, err := strconv.Atoi(m[2])
		if err != nil {
			return result, ErrInvalidPath
		}
		count := 0
		for i := 0; i < 1024; i++ {
			if binary.BigEndian.Uint32(h[i*4:]) != 0 {
				count++
			}
		}
		result.Regions = append(result.Regions, WorldRegion{X: x, Z: z, Chunks: count, SizeBytes: size})
	}
	return result, nil
}
func (s *Store) ListWorldChunks(ctx context.Context, id, name, dimension string, x, z int) ([]WorldChunk, error) {
	unlockWorld := s.lockWorld(id, false)
	defer unlockWorld()
	if _, err := s.worldServer(ctx, id, false); err != nil {
		return nil, err
	}
	if _, err := s.worldDimension(id, name, dimension); err != nil {
		return nil, err
	}
	n, err := regionName(x, z)
	if err != nil {
		return nil, err
	}
	p, err := s.worldPath(id, dimension+"/region/"+n)
	if err != nil {
		return nil, err
	}
	h, _, err := regionHeader(p)
	if err != nil {
		return nil, err
	}
	chunks := []WorldChunk{}
	for i := 0; i < 1024; i++ {
		if binary.BigEndian.Uint32(h[i*4:]) != 0 {
			chunks = append(chunks, WorldChunk{X: x*32 + i%32, Z: z*32 + i/32, Timestamp: binary.BigEndian.Uint32(h[4096+i*4:])})
		}
	}
	return chunks, nil
}

// Reset clears location/timestamp entries, allowing Minecraft to regenerate the
// selected chunks. Unselected sectors remain byte-for-byte intact. Backups of
// every affected region and external payload are complete before the first edit.
func (s *Store) ResetWorldChunks(ctx context.Context, id, name string, in WorldChunkSelection) (WorldChunkReset, error) {
	unlockWorld := s.lockWorld(id, true)
	defer unlockWorld()
	result := WorldChunkReset{}
	if _, err := s.worldServer(ctx, id, true); err != nil {
		return result, err
	}
	if _, err := s.worldDimension(id, name, in.Dimension); err != nil {
		return result, err
	}
	n, err := regionName(in.RegionX, in.RegionZ)
	if err != nil {
		return result, err
	}
	if len(in.Chunks) == 0 || len(in.Chunks) > 1024 {
		return result, worldInvalid("Select between 1 and 1,024 generated chunks.")
	}
	selected := map[int]bool{}
	for _, i := range in.Chunks {
		if i < 0 || i >= 1024 || selected[i] {
			return result, worldInvalid("Chunk indices must be unique and between 0 and 1,023.")
		}
		selected[i] = true
	}
	terrain, err := s.worldPath(id, in.Dimension+"/region/"+n)
	if err != nil {
		return result, err
	}
	main, _, err := regionHeader(terrain)
	if err != nil {
		return result, err
	}
	for i := range selected {
		if binary.BigEndian.Uint32(main[i*4:]) == 0 {
			return result, worldInvalid("The selection includes chunks that are no longer generated. Refresh and select again.")
		}
	}
	type change struct {
		rel    string
		header []byte
	}
	changes := []change{}
	files := []string{}
	for _, kind := range []string{"region", "entities", "poi"} {
		rel := in.Dimension + "/" + kind + "/" + n
		p, err := s.worldPath(id, rel)
		if err != nil {
			return result, err
		}
		h, _, err := regionHeader(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		changed := false
		for i := range selected {
			if binary.BigEndian.Uint32(h[i*4:]) == 0 {
				continue
			}
			clear(h[i*4 : i*4+4])
			clear(h[4096+i*4 : 4096+i*4+4])
			changed = true
			external := fmt.Sprintf("%s/%s/c.%d.%d.mcc", in.Dimension, kind, in.RegionX*32+i%32, in.RegionZ*32+i/32)
			ext, err := s.worldPath(id, external)
			if err != nil {
				return result, err
			}
			if _, err = os.Stat(ext); err == nil {
				files = append(files, external)
			} else if !os.IsNotExist(err) {
				return result, err
			}
		}
		if changed {
			changes = append(changes, change{rel, h})
			files = append(files, rel)
		}
	}
	recovery := ".mcm-world-backups/" + uuid.NewString()
	dest, err := s.worldPath(id, recovery)
	if err != nil {
		return result, err
	}
	if err = os.MkdirAll(dest, 0755); err != nil {
		return result, err
	}
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		source, _ := s.worldPath(id, rel)
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return result, err
		}
		src, err := os.Open(source)
		if err != nil {
			return result, err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			src.Close()
			return result, err
		}
		_, copyErr := io.Copy(out, src)
		src.Close()
		syncErr := out.Sync()
		closeErr := out.Close()
		if copyErr != nil {
			return result, copyErr
		}
		if syncErr != nil {
			return result, syncErr
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	manifest, _ := json.Marshal(struct {
		World     string              `json:"world"`
		Selection WorldChunkSelection `json:"selection"`
		Files     []string            `json:"files"`
	}{name, in, files})
	if err = atomicWorldFile(filepath.Join(dest, "reset.json"), manifest); err != nil {
		return result, err
	}
	// Roll back all region headers on an I/O failure. External payloads are left in
	// place and are ignored once their header entries are removed.
	for _, c := range changes {
		p, _ := s.worldPath(id, c.rel)
		f, openErr := os.OpenFile(p, os.O_WRONLY, 0)
		err = openErr
		if err == nil {
			_, err = f.WriteAt(c.header, 0)
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil {
			for _, restore := range changes {
				backup, _ := os.Open(filepath.Join(dest, filepath.FromSlash(restore.rel)))
				if backup == nil {
					continue
				}
				h := make([]byte, 8192)
				_, readErr := io.ReadFull(backup, h)
				backup.Close()
				if readErr == nil {
					original, e := os.OpenFile(filepath.Join(s.dataPath(id), filepath.FromSlash(restore.rel)), os.O_WRONLY, 0)
					if e == nil {
						_, _ = original.WriteAt(h, 0)
						_ = original.Sync()
						original.Close()
					}
				}
			}
			return result, fmt.Errorf("reset failed; recovery copy is at %s: %w", recovery, err)
		}
	}
	result.Reset = len(selected)
	result.RecoveryPath = recovery
	return result, nil
}
