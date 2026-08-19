// Package ports manages the pool of host ports available for server containers.
package ports

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrPortPoolFull indicates every port in the configured range is in use.
var ErrPortPoolFull = errors.New("port pool full")

// Pool allocates and inspects host ports in an inclusive range backed by the
// servers table's host_port column.
type Pool struct {
	db    *sql.DB
	start int
	end   int
}

// NewPool returns a Pool over the inclusive [start, end] range.
func NewPool(db *sql.DB, start, end int) *Pool {
	return &Pool{db: db, start: start, end: end}
}

// Allocate returns the lowest free port in the range, or ErrPortPoolFull when
// every port is already reserved.
func (p *Pool) Allocate(ctx context.Context) (int, error) {
	used, err := p.used(ctx)
	if err != nil {
		return 0, err
	}
	port, ok := lowestFree(used, p.start, p.end)
	if !ok {
		return 0, ErrPortPoolFull
	}
	return port, nil
}

// Release frees a port. Ports are naturally released when the owning server row
// is deleted, so this is a no-op that exists to keep the pool API explicit.
func (p *Pool) Release(ctx context.Context, port int) error {
	_, err := p.db.ExecContext(ctx, `SELECT 1`)
	return err
}

// Available returns all currently free ports in the range.
func (p *Pool) Available(ctx context.Context) ([]int, error) {
	used, err := p.used(ctx)
	if err != nil {
		return nil, err
	}
	var free []int
	for port := p.start; port <= p.end; port++ {
		if !contains(used, port) {
			free = append(free, port)
		}
	}
	return free, nil
}

func (p *Pool) used(ctx context.Context) ([]int, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT host_port FROM servers WHERE host_port BETWEEN ? AND ?`, p.start, p.end)
	if err != nil {
		return nil, fmt.Errorf("query used ports: %w", err)
	}
	defer rows.Close()

	var used []int
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, err
		}
		used = append(used, port)
	}
	return used, rows.Err()
}

func lowestFree(used []int, start, end int) (int, bool) {
	for port := start; port <= end; port++ {
		if !contains(used, port) {
			return port, true
		}
	}
	return 0, false
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
