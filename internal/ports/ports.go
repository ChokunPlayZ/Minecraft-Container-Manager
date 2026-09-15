// Package ports manages the pool of host ports available for server containers.
package ports

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ErrPortPoolFull indicates every port in the configured range is in use.
var ErrPortPoolFull = errors.New("port pool full")

// Pool allocates and inspects host ports in an inclusive range backed by the
// servers table's host_port column and the settings table's port_pool key.
type Pool struct {
	db    *sql.DB
	start int
	end   int
}

// NewPool returns a Pool over the default inclusive [start, end] range.
func NewPool(db *sql.DB, start, end int) *Pool {
	return &Pool{db: db, start: start, end: end}
}

// ParsePortPool parses a port pool string such as "25565-25575", "25565, 25567",
// or "25565, 25570-25575". It returns a sorted, deduplicated slice of valid ports.
func ParsePortPool(s string) ([]int, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, errors.New("port pool string is empty")
	}

	seen := make(map[int]bool)
	var result []int

	parts := strings.Split(trimmed, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			startStr := strings.TrimSpace(rangeParts[0])
			endStr := strings.TrimSpace(rangeParts[1])
			start, err := strconv.Atoi(startStr)
			if err != nil {
				return nil, fmt.Errorf("invalid range start %q: %w", startStr, err)
			}
			end, err := strconv.Atoi(endStr)
			if err != nil {
				return nil, fmt.Errorf("invalid range end %q: %w", endStr, err)
			}
			if start < 1 || end > 65535 || start > end {
				return nil, fmt.Errorf("invalid port range %d-%d (must be 1-65535 and start <= end)", start, end)
			}
			for p := start; p <= end; p++ {
				if !seen[p] {
					seen[p] = true
					result = append(result, p)
				}
			}
		} else {
			port, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid port %q: %w", part, err)
			}
			if port < 1 || port > 65535 {
				return nil, fmt.Errorf("port %d out of valid range (1-65535)", port)
			}
			if !seen[port] {
				seen[port] = true
				result = append(result, port)
			}
		}
	}

	if len(result) == 0 {
		return nil, errors.New("no valid ports found in pool")
	}

	sort.Ints(result)
	return result, nil
}

// ConfiguredPorts returns all ports defined in the usable pool. It first checks
// the settings table for a custom 'port_pool' key. If unset or invalid, it falls
// back to the default [start, end] range.
func (p *Pool) ConfiguredPorts(ctx context.Context) ([]int, error) {
	if p.db != nil {
		var val string
		err := p.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'port_pool'`).Scan(&val)
		if err == nil && strings.TrimSpace(val) != "" {
			ports, perr := ParsePortPool(val)
			if perr == nil && len(ports) > 0 {
				return ports, nil
			}
		}
	}

	var ports []int
	for port := p.start; port <= p.end; port++ {
		ports = append(ports, port)
	}
	return ports, nil
}

// Allocate returns the lowest free port in the pool, or ErrPortPoolFull when
// every port is already reserved.
func (p *Pool) Allocate(ctx context.Context) (int, error) {
	free, err := p.Available(ctx)
	if err != nil {
		return 0, err
	}
	if len(free) == 0 {
		return 0, ErrPortPoolFull
	}
	return free[0], nil
}

// Release frees a port. Ports are naturally released when the owning server row
// is deleted, so this is a no-op that exists to keep the pool API explicit.
func (p *Pool) Release(ctx context.Context, port int) error {
	_, err := p.db.ExecContext(ctx, `SELECT 1`)
	return err
}

// Available returns all currently free ports from the configured pool.
func (p *Pool) Available(ctx context.Context) ([]int, error) {
	configured, err := p.ConfiguredPorts(ctx)
	if err != nil {
		return nil, err
	}
	used, err := p.used(ctx)
	if err != nil {
		return nil, err
	}
	var free []int
	for _, port := range configured {
		if !contains(used, port) {
			free = append(free, port)
		}
	}
	return free, nil
}

type extraPortEntry struct {
	HostPort int `json:"host_port"`
}

func (p *Pool) used(ctx context.Context) ([]int, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT host_port, COALESCE(extra_ports, '[]') FROM servers`)
	if err != nil {
		return nil, fmt.Errorf("query used ports: %w", err)
	}
	defer rows.Close()

	usedMap := make(map[int]bool)
	for rows.Next() {
		var primaryPort int
		var extraRaw string
		if err := rows.Scan(&primaryPort, &extraRaw); err != nil {
			return nil, err
		}
		if primaryPort > 0 {
			usedMap[primaryPort] = true
		}
		var extras []extraPortEntry
		if err := json.Unmarshal([]byte(extraRaw), &extras); err == nil {
			for _, ep := range extras {
				if ep.HostPort > 0 {
					usedMap[ep.HostPort] = true
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	used := make([]int, 0, len(usedMap))
	for port := range usedMap {
		used = append(used, port)
	}
	sort.Ints(used)
	return used, nil
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
