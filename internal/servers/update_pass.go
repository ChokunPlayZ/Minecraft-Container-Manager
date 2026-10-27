package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/mcm-panel/mcm/internal/checkcache"
	"github.com/mcm-panel/mcm/internal/proxy"
)

type hashUpdateCache struct {
	mu       sync.Mutex
	versions map[string]modrinthVersion
	checked  map[string]bool
	errors   map[string]error
}

// Reuse each hash independently of the other mods installed on a server. The
// game version and loaders remain part of the key, so incompatible builds never
// cross between servers. Missing results and failures are cached for this pass.
func fetchModrinthHashUpdates(ctx context.Context, px *proxy.Service, payload map[string]any, force bool) (map[string]modrinthVersion, error) {
	fetch := func(body map[string]any) (map[string]modrinthVersion, error) {
		b, _ := json.Marshal(body)
		resp, err := px.Do(ctx, http.MethodPost, "https://api.modrinth.com/v2/version_files/update", b, nil, force)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK || resp.CacheStatus == "STALE" {
			return nil, fmt.Errorf("Modrinth update check returned status %d (%s)", resp.StatusCode, resp.CacheStatus)
		}
		var versions map[string]modrinthVersion
		if err := json.Unmarshal(resp.Body, &versions); err != nil {
			return nil, err
		}
		return versions, nil
	}
	if !checkcache.Active(ctx) {
		return fetch(payload)
	}
	qualifiers := make(map[string]any, len(payload))
	for k, v := range payload {
		if k != "hashes" {
			qualifiers[k] = v
		}
	}
	key, _ := json.Marshal(qualifiers)
	c, _ := checkcache.Do(ctx, "modrinth-hashes:"+string(key), func() (*hashUpdateCache, error) {
		return &hashUpdateCache{versions: map[string]modrinthVersion{}, checked: map[string]bool{}, errors: map[string]error{}}, nil
	})
	if c == nil {
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	hashes := payload["hashes"].([]string)
	missingSet := map[string]bool{}
	for _, h := range hashes {
		if !c.checked[h] {
			missingSet[h] = true
		}
	}
	missing := make([]string, 0, len(missingSet))
	for h := range missingSet {
		missing = append(missing, h)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		qualifiers["hashes"] = missing
		versions, err := fetch(qualifiers)
		for _, h := range missing {
			c.checked[h] = true
			c.errors[h] = err
			if v, ok := versions[h]; ok {
				c.versions[h] = v
			}
		}
	}
	out := map[string]modrinthVersion{}
	for _, h := range hashes {
		if err := c.errors[h]; err != nil {
			return nil, err
		}
		if v, ok := c.versions[h]; ok {
			out[h] = v
		}
	}
	return out, nil
}
