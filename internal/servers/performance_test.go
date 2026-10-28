package servers

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/rcon"
)

func newPerformanceStore(t *testing.T, kind, version, state string) (*Store, Server) {
	t.Helper()
	dir := t.TempDir()
	handle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	_, err = handle.DB.Exec(`INSERT INTO servers (id,name,server_type,version,ram_mb,host_port,container_id,state,created_at,updated_at)
		VALUES ('one','Test',?,?,2048,25565,'container',?,'2026-01-01','2026-01-01')`, kind, version, state)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(handle, nil, nil, 25565, 25575, dir, "")
	t.Cleanup(func() { s.WaitProvisioning(context.Background()) })
	srv, err := s.Get(context.Background(), "one")
	if err != nil {
		t.Fatal(err)
	}
	return s, srv
}

func TestSparkDetection(t *testing.T) {
	for _, test := range []struct {
		kind, version string
		want          bool
	}{
		{"paper", "1.21", false}, {"paper", "1.21.0", false}, {"paper", "1.21.1", true},
		{"paper", "1.21.10", true}, {"Paper", "1.22", true}, {"paper", "1.20.6", false},
		{"paper", "latest", false}, {"paper", "1.21.1-pre1", false}, {"paper", "1.21.bad", false},
		{"fabric", "1.21.1", false}, {"folia", "1.21.4", false},
	} {
		if got := bundledSpark(test.kind, test.version); got != test.want {
			t.Errorf("%s %s: got %v", test.kind, test.version, got)
		}
	}
	for _, test := range []struct {
		mod  Mod
		want bool
	}{
		{Mod{Enabled: true, ModID: "spark", File: "renamed.jar"}, true},
		{Mod{Enabled: true, File: "spark-1.10.119-fabric.jar"}, true},
		{Mod{Enabled: true, File: "spark.jar"}, true},
		{Mod{Enabled: false, ModID: "spark", File: "spark.jar.disabled"}, false},
		{Mod{Enabled: true, ModID: "sparkle", File: "spark-1.0.jar"}, false},
		{Mod{Enabled: true, File: "spark-utils.jar"}, false},
		{Mod{Enabled: true, ModID: "spark", IsClientOnly: true}, false},
	} {
		if got := sparkMod(test.mod); got != test.want {
			t.Errorf("%+v: got %v", test.mod, got)
		}
	}
	// Read the actual mod list and JAR metadata rather than relying on filenames.
	s, srv := newPerformanceStore(t, "fabric", "1.21.1", StateStopped)
	dir := filepath.Join(s.dataPath(srv.ID), "mods")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "renamed.jar"))
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	manifest, err := archive.Create("fabric.mod.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = manifest.Write([]byte(`{"id":"spark","name":"spark","version":"1.10","environment":"server"}`))
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	support, err := s.sparkSupport(context.Background(), srv)
	if err != nil || !support.Available || support.Source != "mod" {
		t.Fatalf("support=%+v, error=%v", support, err)
	}
	if err := os.Rename(filepath.Join(dir, "renamed.jar"), filepath.Join(dir, "renamed.jar.disabled")); err != nil {
		t.Fatal(err)
	}
	support, err = s.sparkSupport(context.Background(), srv)
	if err != nil || support.Available {
		t.Fatalf("disabled support=%+v, error=%v", support, err)
	}
}

const sparkMetricsFixture = "[12:00:00] [Server thread/INFO]: [⚡] TPS from last 5s, 10s, 1m, 5m, 15m:\n" +
	"[12:00:00] [Server thread/INFO]: [⚡] §a*20.00, 19.99, 19.98, 19.97, 19.96\n" +
	"[12:00:00] [Server thread/INFO]: [⚡] Tick durations (min/med/95%ile/max ms) from last 10s, 1m:\n" +
	"[12:00:00] [Server thread/INFO]: [⚡] 1.0/12.34/30.0/60.0; 2.0/15.0/40.0/80.0\n"

func TestSparkMetricsParsing(t *testing.T) {
	tps, tick := parseSparkMetrics(sparkMetricsFixture)
	if tps == nil || *tps != 20 || tick == nil || *tick != 12.34 {
		t.Fatalf("tps=%v tick=%v", tps, tick)
	}
	tps, tick = parseSparkMetrics("[⚡] CPU usage from last 10s, 1m, 15m:\n 10.0%, 15.0%, 20.0%")
	if tps != nil || tick != nil {
		t.Fatal("CPU usage must not be interpreted as TPS")
	}
	if count := playerCount("There are 0 of a max 20 players online:"); count == nil || *count != 0 {
		t.Fatal("zero players must be a valid reading")
	}
	if playerCount("Unknown command") != nil {
		t.Fatal("missing players must not become zero")
	}
	if sparkFailure("The async-profiler engine is not supported for your os/arch, so the built-in Java engine will be used instead.") {
		t.Fatal("a supported Java profiler fallback must not fail the capture")
	}
}

type performanceRuntime struct {
	*fakeRuntime
	logPath   string
	responses map[string]string
	gate      <-chan struct{}
}

func (f *performanceRuntime) SendConsole(ctx context.Context, _ string, command string) error {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consoleCommands = append(f.consoleCommands, command)
	file, err := os.OpenFile(f.logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(f.responses[command])
	return err
}

func performanceLog(t *testing.T, s *Store, srv Server) string {
	t.Helper()
	path := filepath.Join(s.dataPath(srv.ID), "logs", "latest.log")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	// Stale readings and links must never be used as a new command response.
	if err := os.WriteFile(path, []byte("There are 99 of a max 100 players online:\nhttps://spark.lucko.me/stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPerformanceSamplingHistoryAndRetention(t *testing.T) {
	s, srv := newPerformanceStore(t, "paper", "1.21.1", StateRunning)
	s.docker = &performanceRuntime{fakeRuntime: &fakeRuntime{statsResult: docker.ContainerStats{CPUPercent: 150.5, MemoryUsageBytes: 1024}}, logPath: performanceLog(t, s, srv), responses: map[string]string{
		"list": "There are 3 of a max 20 players online: Steve, Alex, Test\n", "spark tps": sparkMetricsFixture,
	}}
	ctx := context.Background()
	if _, err := s.SetPerformanceCollection(ctx, srv.ID, "console"); err != nil {
		t.Fatal(err)
	}
	if err := s.samplePerformance(ctx, srv, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	history, err := s.PerformanceHistory(ctx, srv.ID, "1h")
	if err != nil || len(history.Samples) != 1 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	sample := history.Samples[0]
	if !sample.Online || sample.CPUPercent == nil || *sample.CPUPercent != 150.5 || sample.Players == nil || *sample.Players != 3 || sample.TPS == nil || *sample.TPS != 20 || sample.TickMS == nil || *sample.TickMS != 12.34 {
		t.Fatalf("sample=%+v", sample)
	}
	// Longer windows average resources and retain peak player counts per bucket.
	_, err = s.db.Exec(`INSERT INTO performance_samples (server_id,recorded_at,online,cpu_percent,memory_bytes,players,tps,tick_ms) SELECT server_id,recorded_at+1,1,49.5,2048,8,NULL,NULL FROM performance_samples`)
	if err != nil {
		t.Fatal(err)
	}
	history, err = s.PerformanceHistory(ctx, srv.ID, "7d")
	if err != nil || len(history.Samples) != 1 || *history.Samples[0].CPUPercent != 100 || *history.Samples[0].Players != 8 {
		t.Fatalf("aggregated=%+v err=%v", history, err)
	}
	if _, err := s.PerformanceHistory(ctx, srv.ID, "forever"); !errors.Is(err, ErrPerformanceWindow) {
		t.Fatal(err)
	}
	if _, err := s.PerformanceHistory(ctx, "missing", "1h"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	_, _ = s.db.Exec(`INSERT INTO performance_samples(server_id,recorded_at,online) VALUES(?,?,0)`, srv.ID, time.Now().Add(-8*24*time.Hour).UnixMilli())
	_, _ = s.db.Exec(`UPDATE servers SET state='stopped' WHERE id=?`, srv.ID)
	if err := s.collectPerformance(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM performance_samples WHERE recorded_at < ?`, time.Now().Add(-PerformanceRetention).UnixMilli()).Scan(&count); err != nil || count != 0 {
		t.Fatalf("retention count=%d err=%v", count, err)
	}
	if _, err := s.db.Exec(`DELETE FROM servers WHERE id=?`, srv.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM performance_samples`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade count=%d err=%v", count, err)
	}
}

func TestPerformanceCollectionOptIn(t *testing.T) {
	s, srv := newPerformanceStore(t, "paper", "1.21.1", StateRunning)
	runtime := &performanceRuntime{fakeRuntime: &fakeRuntime{statsResult: docker.ContainerStats{CPUPercent: 25, MemoryUsageBytes: 2048}}, logPath: performanceLog(t, s, srv), responses: map[string]string{
		"list": "There are 2 of a max 20 players online: Steve, Alex\n", "spark tps": sparkMetricsFixture,
	}}
	s.docker = runtime
	ctx := context.Background()
	settings, err := s.PerformanceSettings(ctx, srv.ID)
	if err != nil || settings.CollectionMode != "off" {
		t.Fatalf("default=%+v err=%v", settings, err)
	}
	at := time.Now().Add(-3 * time.Minute).Truncate(time.Minute)
	if err := s.samplePerformance(ctx, srv, at); err != nil {
		t.Fatal(err)
	}
	if len(runtime.consoleCommands) != 0 {
		t.Fatal("default collection sent console commands")
	}
	if _, err := s.SetPerformanceCollection(ctx, srv.ID, "console"); err != nil {
		t.Fatal(err)
	}
	// A newly constructed store reads the persisted choice rather than a cache.
	other := &Store{db: s.db}
	settings, err = other.PerformanceSettings(ctx, srv.ID)
	if err != nil || settings.CollectionMode != "console" {
		t.Fatalf("persisted=%+v err=%v", settings, err)
	}
	if err := s.samplePerformance(ctx, srv, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(runtime.consoleCommands) != 2 {
		t.Fatalf("commands=%v", runtime.consoleCommands)
	}
	if _, err := s.SetPerformanceCollection(ctx, srv.ID, "off"); err != nil {
		t.Fatal(err)
	}
	if err := s.samplePerformance(ctx, srv, at.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(runtime.consoleCommands) != 2 {
		t.Fatal("disabled collection continued sending commands")
	}
	history, err := s.PerformanceHistory(ctx, srv.ID, "1h")
	if err != nil || len(history.Samples) != 3 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	for _, sample := range history.Samples {
		if sample.CPUPercent == nil || *sample.CPUPercent != 25 {
			t.Fatal("quiet CPU history was lost")
		}
	}
	if history.Samples[0].TPS != nil || history.Samples[1].TPS == nil || history.Samples[2].TPS != nil {
		t.Fatal("opt-in gaps or prior history were not preserved")
	}
	if _, err := s.SetPerformanceCollection(ctx, srv.ID, "invalid"); !errors.Is(err, ErrPerformanceCollectionMode) {
		t.Fatal(err)
	}
	if _, err := s.SetPerformanceCollection(ctx, "missing", "off"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.PerformanceSettings(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

type performanceRCONRuntime struct {
	*fakeRuntime
	address string
}

func (f *performanceRCONRuntime) RCONAddresses(context.Context, string, int) ([]string, error) {
	return []string{f.address}, nil
}

func TestPerformanceRCONPolling(t *testing.T) {
	s, srv := newPerformanceStore(t, "paper", "1.21.1", StateRunning)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	commands := make(chan string, 4)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var length int32
			if binary.Read(conn, binary.LittleEndian, &length) != nil || length < 10 || length > 65536 {
				return
			}
			body := make([]byte, length)
			if _, err := io.ReadFull(conn, body); err != nil {
				return
			}
			id := binary.LittleEndian.Uint32(body[:4])
			kind := binary.LittleEndian.Uint32(body[4:8])
			payload := string(body[8 : len(body)-2])
			response := ""
			if kind == rcon.TypeAuth {
				if payload != "secret" {
					return
				}
			} else {
				commands <- payload
				switch payload {
				case "list":
					response = "There are 4 of a max 20 players online: A, B, C, D"
				case "spark tps":
					response = sparkMetricsFixture
				}
			}
			packet := make([]byte, 14+len(response))
			binary.LittleEndian.PutUint32(packet[:4], uint32(len(packet)-4))
			binary.LittleEndian.PutUint32(packet[4:8], id)
			if kind == rcon.TypeAuth {
				binary.LittleEndian.PutUint32(packet[8:12], rcon.TypeAuthResponse)
			}
			copy(packet[12:], response)
			if _, err := conn.Write(packet); err != nil {
				return
			}
		}
	}()
	// Even with a console log available, RCON mode must never send stdin commands.
	_ = performanceLog(t, s, srv)
	props := "enable-rcon=true\nrcon.password=secret\nrcon.port=25575\n"
	if err := os.WriteFile(filepath.Join(s.dataPath(srv.ID), "server.properties"), []byte(props), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &performanceRCONRuntime{fakeRuntime: &fakeRuntime{statsResult: docker.ContainerStats{CPUPercent: 12}}, address: listener.Addr().String()}
	s.docker = runtime
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := s.SetPerformanceCollection(ctx, srv.ID, "rcon"); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-2 * time.Minute)
	if err := s.samplePerformance(ctx, srv, at); err != nil {
		t.Fatal(err)
	}
	history, err := s.PerformanceHistory(ctx, srv.ID, "1h")
	if err != nil || len(history.Samples) != 1 || history.Samples[0].TPS == nil || *history.Samples[0].TPS != 20 || *history.Samples[0].Players != 4 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if len(commands) != 2 || len(runtime.consoleCommands) != 0 || history.Settings.LastError != "" {
		t.Fatalf("commands=%d console=%v settings=%+v", len(commands), runtime.consoleCommands, history.Settings)
	}
	_ = listener.Close()
	if err := s.samplePerformance(ctx, srv, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	settings, err := s.PerformanceSettings(ctx, srv.ID)
	if err != nil || settings.LastError == "" || len(runtime.consoleCommands) != 0 {
		t.Fatalf("failed RCON settings=%+v console=%v err=%v", settings, runtime.consoleCommands, err)
	}
}

func TestPerformanceRCONDisabledIsQuiet(t *testing.T) {
	s, srv := newPerformanceStore(t, "paper", "1.21.1", StateRunning)
	runtime := &fakeRuntime{statsResult: docker.ContainerStats{CPUPercent: 10}}
	s.docker = runtime
	if _, err := s.SetPerformanceCollection(context.Background(), srv.ID, "rcon"); err != nil {
		t.Fatal(err)
	}
	if err := s.samplePerformance(context.Background(), srv, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	history, err := s.PerformanceHistory(context.Background(), srv.ID, "1h")
	if err != nil || history.Settings.LastError == "" || len(runtime.consoleCommands) != 0 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if history.Samples[0].CPUPercent == nil || history.Samples[0].TPS != nil {
		t.Fatal(fmt.Sprint(history.Samples[0]))
	}
}

func TestSparkCaptureLifecycle(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(strconvBool(existing), func(t *testing.T) {
			s, srv := newPerformanceStore(t, "paper", "1.21.1", StateRunning)
			gate := make(chan struct{})
			responses := map[string]string{"spark profiler start --timeout 60": "[spark] https://spark.lucko.me/newReport\n"}
			if existing {
				responses["spark profiler start --timeout 60"] = "[spark] A profiler is already running!\n"
				responses["spark profiler open"] = "[spark] https://spark.lucko.me/activeReport\n"
			}
			s.docker = &performanceRuntime{fakeRuntime: &fakeRuntime{}, logPath: performanceLog(t, s, srv), responses: responses, gate: gate}
			report, err := s.CaptureSparkReport(context.Background(), srv.ID)
			if err != nil || report.Status != "capturing" {
				t.Fatalf("report=%+v err=%v", report, err)
			}
			if _, err := s.CaptureSparkReport(context.Background(), srv.ID); !errors.Is(err, ErrSparkBusy) {
				t.Fatalf("duplicate=%v", err)
			}
			close(gate)
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				result, err := s.latestSparkReport(context.Background(), srv.ID)
				if err != nil {
					t.Fatal(err)
				}
				if result.Status != "capturing" {
					if result.Status != "completed" || result.Existing != existing || strings.Contains(result.URL, "stale") {
						t.Fatalf("result=%+v", result)
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("capture did not complete")
		})
	}
}

type performanceStreamRuntime struct {
	*fakeRuntime
	writer   *io.PipeWriter
	response string
}

func (f *performanceStreamRuntime) LogsSince(_ context.Context, _ string, since time.Time) (io.ReadCloser, error) {
	if since.IsZero() {
		return nil, errors.New("missing log cursor")
	}
	reader, writer := io.Pipe()
	f.writer = writer
	return reader, nil
}

func (f *performanceStreamRuntime) SendConsole(context.Context, string, string) error {
	go func() {
		_, _ = stdcopy.NewStdWriter(f.writer, stdcopy.Stdout).Write([]byte(f.response))
		_ = f.writer.Close()
	}()
	return nil
}

func TestPerformanceCommandContainerOutputFallback(t *testing.T) {
	s, srv := newPerformanceStore(t, "paper", "1.21.1", StateRunning)
	s.docker = &performanceStreamRuntime{fakeRuntime: &fakeRuntime{}, response: sparkMetricsFixture}
	out, err := s.performanceCommand(context.Background(), srv, "spark tps", time.Second, func(out string) bool {
		tps, tick := parseSparkMetrics(out)
		return tps != nil && tick != nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tps, tick := parseSparkMetrics(out)
	if tps == nil || *tps != 20 || tick == nil || *tick != 12.34 {
		t.Fatalf("unexpected response: %s", out)
	}
	var buffer strings.Builder
	buffer.WriteString(strings.Repeat("x", 129*1024))
	trimPerformanceOutput(&buffer)
	if buffer.Len() > 128*1024 {
		t.Fatal("output buffering is unbounded")
	}
}

func TestSparkCaptureFailure(t *testing.T) {
	s, srv := newPerformanceStore(t, "paper", "1.21.1", StateRunning)
	s.docker = &performanceStreamRuntime{fakeRuntime: &fakeRuntime{}, response: "Unknown command. Type /help for help.\n"}
	if _, err := s.CaptureSparkReport(context.Background(), srv.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		report, err := s.latestSparkReport(context.Background(), srv.ID)
		if err != nil {
			t.Fatal(err)
		}
		if report.Status != "capturing" {
			if report.Status != "failed" || report.Error == "" || report.URL != "" {
				t.Fatalf("report=%+v", report)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("failed capture did not finish")
}

func strconvBool(value bool) string {
	if value {
		return "existing"
	}
	return "new"
}

func TestSparkCaptureUnavailableAndInterrupted(t *testing.T) {
	s, srv := newPerformanceStore(t, "vanilla", "1.21.1", StateRunning)
	s.docker = &fakeRuntime{}
	if _, err := s.CaptureSparkReport(context.Background(), srv.ID); !errors.Is(err, ErrSparkUnavailable) {
		t.Fatal(err)
	}
	_, _ = s.db.Exec(`UPDATE servers SET state='stopped' WHERE id=?`, srv.ID)
	if _, err := s.CaptureSparkReport(context.Background(), srv.ID); !errors.Is(err, ErrServerNotRunning) {
		t.Fatal(err)
	}
	_, err := s.db.Exec(`INSERT INTO spark_reports(server_id,id,status,started_at) VALUES (?, 'old', 'capturing', ?)`, srv.ID, time.Now().Add(-2*time.Minute).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.latestSparkReport(context.Background(), srv.ID)
	if err != nil || report.Status != "failed" {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}
