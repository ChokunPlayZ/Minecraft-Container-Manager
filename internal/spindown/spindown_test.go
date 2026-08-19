package spindown

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/servers"
)

type fakeSpin struct {
	list       []servers.Server
	activity   map[string]time.Time
	overrides  map[string]int
	defaultMin int
}

func (f *fakeSpin) List(context.Context) ([]servers.Server, error) { return f.list, nil }
func (f *fakeSpin) LastActivity(_ context.Context, id string) (time.Time, error) {
	// Mimic the real store: a missing/empty column yields a zero time with no
	// error, so the scheduler can seed a grace period.
	return f.activity[id], nil
}
func (f *fakeSpin) SetActivity(_ context.Context, id string, t time.Time) error {
	f.activity[id] = t
	return nil
}
func (f *fakeSpin) IdleTimeoutOverride(_ context.Context, id string) (int, bool, error) {
	if v, ok := f.overrides[id]; ok {
		return v, true, nil
	}
	return 0, false, nil
}
func (f *fakeSpin) DefaultIdleTimeout(_ context.Context, fallback time.Duration) (time.Duration, error) {
	if f.defaultMin > 0 {
		return time.Duration(f.defaultMin) * time.Minute, nil
	}
	return fallback, nil
}

type fakeCtrl struct {
	stops  map[string]int
	starts map[string]int
}

func (c *fakeCtrl) Stop(_ context.Context, id string) (servers.Server, error) {
	c.stops[id]++
	return servers.Server{ID: id, State: servers.StateStopped}, nil
}
func (c *fakeCtrl) Start(_ context.Context, id string) (servers.Server, error) {
	c.starts[id]++
	return servers.Server{ID: id, State: servers.StateRunning}, nil
}

func newFakes(list []servers.Server) (*fakeSpin, *fakeCtrl) {
	spin := &fakeSpin{
		list:     list,
		activity: map[string]time.Time{},
		overrides: map[string]int{},
	}
	return spin, &fakeCtrl{stops: map[string]int{}, starts: map[string]int{}}
}

func TestShouldStopDefaultTimeout(t *testing.T) {
	spin, _ := newFakes(nil)
	spin.defaultMin = 30
	s := New(spin, nil, log.New(io.Discard, "", 0), 30*time.Minute)

	cases := []struct {
		idleSeconds int
		want        bool
	}{
		{0, false},
		{29 * 60, false},
		{30 * 60, true},
		{31 * 60, true},
	}
	for _, c := range cases {
		if got := s.ShouldStop(context.TODO(), "srv1", c.idleSeconds); got != c.want {
			t.Errorf("ShouldStop(idle=%ds) = %v, want %v", c.idleSeconds, got, c.want)
		}
	}
}

func TestShouldStopPerServerOverride(t *testing.T) {
	spin, _ := newFakes(nil)
	spin.overrides["srv1"] = 5
	spin.defaultMin = 60
	s := New(spin, nil, log.New(io.Discard, "", 0), 60*time.Minute)

	if s.ShouldStop(context.TODO(), "srv1", 4*60) {
		t.Error("override 5m should not stop at 4m idle")
	}
	if !s.ShouldStop(context.TODO(), "srv1", 5*60) {
		t.Error("override 5m should stop at 5m idle")
	}
	// Servers without an override fall back to the site-wide default.
	if s.ShouldStop(context.TODO(), "srv2", 30*60) {
		t.Error("no-override server at 30m idle should not stop under 60m default")
	}
}

func TestTickSeedsThenStops(t *testing.T) {
	spin, ctrl := newFakes([]servers.Server{
		{ID: "srv1", State: servers.StateRunning},
	})
	spin.defaultMin = 30
	s := New(spin, ctrl, log.New(io.Discard, "", 0), 30*time.Minute)

	clock := time.Unix(1_000_000, 0)
	s.SetClock(func() time.Time { return clock })

	// First tick has no recorded activity, so it seeds and must not stop.
	s.tick(context.TODO())
	if got := ctrl.stops["srv1"]; got != 0 {
		t.Fatalf("first tick stopped server (%d times); expected seeding only", got)
	}
	if _, ok := spin.activity["srv1"]; !ok {
		t.Fatal("first tick did not seed last activity")
	}

	// Advance past the timeout; the next tick should spin the server down.
	clock = clock.Add(31 * time.Minute)
	s.tick(context.TODO())
	if got := ctrl.stops["srv1"]; got != 1 {
		t.Fatalf("after idle timeout stops = %d, want 1", got)
	}
}

func TestWakeRestartsAndSeeds(t *testing.T) {
	spin, ctrl := newFakes([]servers.Server{
		{ID: "srv1", State: servers.StateStopped},
	})
	s := New(spin, ctrl, log.New(io.Discard, "", 0), 30*time.Minute)
	s.SetClock(func() time.Time { return time.Unix(2_000_000, 0) })

	if _, err := s.Wake(context.TODO(), "srv1"); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	if got := ctrl.starts["srv1"]; got != 1 {
		t.Fatalf("Wake starts = %d, want 1", got)
	}
	if _, ok := spin.activity["srv1"]; !ok {
		t.Fatal("Wake did not seed last activity")
	}
}
