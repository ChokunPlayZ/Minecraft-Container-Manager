package servers

import (
	"archive/zip"
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ModManifest contains metadata extracted from a mod or plugin .jar archive.
type ModManifest struct {
	ModID       string `json:"mod_id,omitempty"`
	Title       string `json:"title,omitempty"`
	Version     string `json:"version,omitempty"`
	SHA1        string `json:"sha1,omitempty"`
	Description string `json:"description,omitempty"`
}

type manifestCacheEntry struct {
	modTime  time.Time
	size     int64
	manifest ModManifest
}

var (
	manifestCache   = make(map[string]manifestCacheEntry)
	manifestCacheMu sync.RWMutex
)

// inspectModJar reads a jar archive and returns its SHA1 and manifest metadata.
// Results are cached based on file path, modification time, and size.
func inspectModJar(filePath string) (ModManifest, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return ModManifest{}, err
	}

	manifestCacheMu.RLock()
	if cached, ok := manifestCache[filePath]; ok {
		if cached.modTime.Equal(fi.ModTime()) && cached.size == fi.Size() {
			manifestCacheMu.RUnlock()
			return cached.manifest, nil
		}
	}
	manifestCacheMu.RUnlock()

	manifest, err := readModJarManifest(filePath)
	if err != nil {
		return ModManifest{}, err
	}

	manifestCacheMu.Lock()
	manifestCache[filePath] = manifestCacheEntry{
		modTime:  fi.ModTime(),
		size:     fi.Size(),
		manifest: manifest,
	}
	manifestCacheMu.Unlock()

	return manifest, nil
}

func readModJarManifest(filePath string) (ModManifest, error) {
	manifest := ModManifest{}

	// Compute SHA1
	f, err := os.Open(filePath)
	if err != nil {
		return manifest, err
	}
	h := sha1.New()
	if _, err := io.Copy(h, f); err == nil {
		manifest.SHA1 = hex.EncodeToString(h.Sum(nil))
	}
	f.Close()

	// Open as zip archive
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		// Not a valid zip or corrupted; return whatever SHA1 we computed
		return manifest, nil
	}
	defer zr.Close()

	// 1. Fabric mod
	if zf := findZipFile(zr, "fabric.mod.json"); zf != nil {
		if rc, err := zf.Open(); err == nil {
			var fab struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Version     string `json:"version"`
				Description string `json:"description"`
			}
			dec := json.NewDecoder(io.LimitReader(rc, 256*1024))
			if err := dec.Decode(&fab); err == nil {
				manifest.ModID = strings.TrimSpace(fab.ID)
				manifest.Title = strings.TrimSpace(fab.Name)
				manifest.Version = strings.TrimSpace(fab.Version)
				manifest.Description = strings.TrimSpace(fab.Description)
			}
			rc.Close()
			if manifest.ModID != "" {
				return manifest, nil
			}
		}
	}

	// 2. Quilt mod
	if zf := findZipFile(zr, "quilt.mod.json"); zf != nil {
		if rc, err := zf.Open(); err == nil {
			var q struct {
				QuiltLoader struct {
					ID       string `json:"id"`
					Metadata struct {
						Name        string `json:"name"`
						Version     string `json:"version"`
						Description string `json:"description"`
					} `json:"metadata"`
				} `json:"quilt_loader"`
			}
			dec := json.NewDecoder(io.LimitReader(rc, 256*1024))
			if err := dec.Decode(&q); err == nil {
				manifest.ModID = strings.TrimSpace(q.QuiltLoader.ID)
				manifest.Title = strings.TrimSpace(q.QuiltLoader.Metadata.Name)
				manifest.Version = strings.TrimSpace(q.QuiltLoader.Metadata.Version)
				manifest.Description = strings.TrimSpace(q.QuiltLoader.Metadata.Description)
			}
			rc.Close()
			if manifest.ModID != "" {
				return manifest, nil
			}
		}
	}

	// 3. Paper / Bukkit / Spigot plugin
	for _, pFile := range []string{"paper-plugin.yml", "plugin.yml", "bungee.yml", "velocity-plugin.json"} {
		if zf := findZipFile(zr, pFile); zf != nil {
			if strings.HasSuffix(pFile, ".json") {
				if rc, err := zf.Open(); err == nil {
					var vel struct {
						ID          string `json:"id"`
						Name        string `json:"name"`
						Version     string `json:"version"`
						Description string `json:"description"`
					}
					dec := json.NewDecoder(io.LimitReader(rc, 256*1024))
					if err := dec.Decode(&vel); err == nil {
						manifest.ModID = strings.TrimSpace(vel.ID)
						manifest.Title = strings.TrimSpace(vel.Name)
						manifest.Version = strings.TrimSpace(vel.Version)
						manifest.Description = strings.TrimSpace(vel.Description)
					}
					rc.Close()
					if manifest.ModID != "" {
						return manifest, nil
					}
				}
			} else {
				if rc, err := zf.Open(); err == nil {
					id, title, ver, desc := parsePluginYML(rc)
					rc.Close()
					if id != "" {
						manifest.ModID = id
						manifest.Title = title
						manifest.Version = ver
						manifest.Description = desc
						return manifest, nil
					}
				}
			}
		}
	}

	// 4. Forge / NeoForge mod
	for _, mFile := range []string{"META-INF/neoforge.mods.toml", "META-INF/mods.toml"} {
		if zf := findZipFile(zr, mFile); zf != nil {
			if rc, err := zf.Open(); err == nil {
				id, title, ver, desc := parseModsTOML(rc)
				rc.Close()
				if id != "" {
					manifest.ModID = id
					manifest.Title = title
					manifest.Version = ver
					manifest.Description = desc
					return manifest, nil
				}
			}
		}
	}

	return manifest, nil
}

func findZipFile(zr *zip.ReadCloser, name string) *zip.File {
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, name) {
			return f
		}
	}
	return nil
}

var (
	ymlNameRe    = regexp.MustCompile(`^(?:name|id):\s*['"]?([^'"#\r\n]+)['"]?`)
	ymlVersionRe = regexp.MustCompile(`^version:\s*['"]?([^'"#\r\n]+)['"]?`)
	ymlDescRe    = regexp.MustCompile(`^description:\s*['"]?([^'"#\r\n]+)['"]?`)

	tomlModIDRe = regexp.MustCompile(`^modId\s*=\s*['"]([^'"]+)['"]`)
	tomlNameRe  = regexp.MustCompile(`^displayName\s*=\s*['"]([^'"]+)['"]`)
	tomlVerRe   = regexp.MustCompile(`^version\s*=\s*['"]([^'"]+)['"]`)
	tomlDescRe  = regexp.MustCompile(`^description\s*=\s*['"]([^'"]+)['"]`)
)

func parsePluginYML(r io.Reader) (id, title, ver, desc string) {
	scanner := bufio.NewScanner(io.LimitReader(r, 64*1024))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if id == "" {
			if m := ymlNameRe.FindStringSubmatch(line); len(m) > 1 {
				id = strings.TrimSpace(m[1])
				title = id
			}
		}
		if ver == "" {
			if m := ymlVersionRe.FindStringSubmatch(line); len(m) > 1 {
				ver = strings.TrimSpace(m[1])
			}
		}
		if desc == "" {
			if m := ymlDescRe.FindStringSubmatch(line); len(m) > 1 {
				desc = strings.TrimSpace(m[1])
			}
		}
	}
	return
}

func parseModsTOML(r io.Reader) (id, title, ver, desc string) {
	scanner := bufio.NewScanner(io.LimitReader(r, 64*1024))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if id == "" {
			if m := tomlModIDRe.FindStringSubmatch(line); len(m) > 1 {
				id = strings.TrimSpace(m[1])
			}
		}
		if title == "" {
			if m := tomlNameRe.FindStringSubmatch(line); len(m) > 1 {
				title = strings.TrimSpace(m[1])
			}
		}
		if ver == "" {
			if m := tomlVerRe.FindStringSubmatch(line); len(m) > 1 {
				ver = strings.TrimSpace(m[1])
			}
		}
		if desc == "" {
			if m := tomlDescRe.FindStringSubmatch(line); len(m) > 1 {
				desc = strings.TrimSpace(m[1])
			}
		}
	}
	if title == "" && id != "" {
		title = id
	}
	return
}
