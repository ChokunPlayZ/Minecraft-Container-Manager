package servers

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
)

const PerformanceInterval = 30 * time.Second
const PerformanceRetention = 7 * 24 * time.Hour

var ErrSparkUnavailable = errors.New("spark is not available on this server")
var ErrSparkBusy = errors.New("a spark report is already being captured")
var ErrPerformanceCollectionMode = errors.New("collection_mode must be off, rcon, or console")

type PerformanceSettings struct {
	CollectionMode string `json:"collection_mode"`
	LastError      string `json:"last_error,omitempty"`
}

func (s *Store) PerformanceSettings(ctx context.Context, id string) (PerformanceSettings, error) {
	var settings PerformanceSettings
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(p.collection_mode,'off'), COALESCE(p.last_error,'')
		FROM servers s LEFT JOIN performance_settings p ON p.server_id=s.id WHERE s.id=?`, id).
		Scan(&settings.CollectionMode, &settings.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, ErrNotFound
	}
	return settings, err
}

func (s *Store) SetPerformanceCollection(ctx context.Context, id, mode string) (PerformanceSettings, error) {
	if mode != "off" && mode != "rcon" && mode != "console" {
		return PerformanceSettings{}, ErrPerformanceCollectionMode
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO performance_settings(server_id,collection_mode)
		SELECT id, ? FROM servers WHERE id=?
		ON CONFLICT(server_id) DO UPDATE SET collection_mode=excluded.collection_mode, last_error=''`, mode, id)
	if err != nil {
		return PerformanceSettings{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return PerformanceSettings{}, err
	}
	if n == 0 {
		return PerformanceSettings{}, ErrNotFound
	}
	return PerformanceSettings{CollectionMode: mode}, nil
}

type PerformanceSample struct {
	Timestamp   time.Time `json:"timestamp"`
	Online      bool      `json:"online"`
	CPUPercent  *float64  `json:"cpu_percent"`
	MemoryBytes *float64  `json:"memory_bytes"`
	Players     *float64  `json:"players"`
	TPS         *float64  `json:"tps"`
	TickMS      *float64  `json:"tick_ms"`
}

type SparkSupport struct {
	Available bool   `json:"available"`
	Source    string `json:"source"` // bundled, plugin, mod, or unavailable
}

type SparkReport struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	URL       string    `json:"url,omitempty"`
	Error     string    `json:"error,omitempty"`
	Existing  bool      `json:"existing"`
}

type PerformanceHistory struct {
	Samples           []PerformanceSample `json:"samples"`
	Start             time.Time           `json:"start"`
	End               time.Time           `json:"end"`
	ResolutionSeconds int                 `json:"resolution_seconds"`
	IntervalSeconds   int                 `json:"interval_seconds"`
	RetentionDays     int                 `json:"retention_days"`
	Spark             SparkSupport        `json:"spark"`
	Report            *SparkReport        `json:"report"`
	Settings          PerformanceSettings `json:"settings"`
}

// Versions are compared numerically: 1.21.10 is newer than 1.21.1. Unresolved
// aliases and snapshots are not evidence that a particular bundle is installed.
func bundledSpark(serverType, version string) bool {
	if !strings.EqualFold(serverType, "paper") {
		return false
	}
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	v := [3]int{}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return false
		}
		v[i] = n
	}
	return v[0] > 1 || v[0] == 1 && (v[1] > 21 || v[1] == 21 && v[2] >= 1)
}

var sparkFileRE = regexp.MustCompile(`(?i)^spark(?:[-_][0-9v].*)?$`)

func sparkMod(mod Mod) bool {
	if !mod.Enabled || mod.IsClientOnly {
		return false
	}
	for _, value := range []string{mod.ModID, mod.Title, mod.ProjectSlug} {
		if strings.EqualFold(strings.TrimSpace(value), "spark") {
			return true
		}
	}
	// A known identity wins over a filename, avoiding unrelated spark-* plugins.
	if mod.ModID != "" {
		return false
	}
	return sparkFileRE.MatchString(modDisplayBase(mod.File))
}

func (s *Store) sparkSupport(ctx context.Context, srv Server) (SparkSupport, error) {
	mods, err := s.ListMods(ctx, srv.ID)
	if err != nil && !errors.Is(err, ErrUnsupportedMods) {
		return SparkSupport{}, err
	}
	for _, mod := range mods.Items {
		if sparkMod(mod) {
			source := "mod"
			if mods.Type == string(ModTypePlugins) {
				source = "plugin"
			}
			return SparkSupport{Available: true, Source: source}, nil
		}
	}
	if bundledSpark(srv.ServerType, srv.Version) {
		return SparkSupport{Available: true, Source: "bundled"}, nil
	}
	return SparkSupport{Source: "unavailable"}, nil
}

// RunPerformanceMonitor samples independently of browser sessions. Each server
// has a deadline and the worker pool is bounded, so a stuck server cannot stop
// history collection for the rest of the fleet. Cancellation waits for workers.
func (s *Store) RunPerformanceMonitor(ctx context.Context, logger *log.Logger) {
	ticker := time.NewTicker(PerformanceInterval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.collectPerformance(ctx); err != nil && ctx.Err() == nil && logger != nil {
			logger.Printf("performance history: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Store) collectPerformance(ctx context.Context) error {
	servers, err := s.List(ctx)
	if err != nil {
		return err
	}
	jobs := make(chan Server)
	var workers sync.WaitGroup
	var errsMu sync.Mutex
	var errs []error
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for srv := range jobs {
				sampleCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
				err := s.samplePerformance(sampleCtx, srv, time.Now().UTC())
				cancel()
				if err != nil {
					errsMu.Lock()
					errs = append(errs, fmt.Errorf("%s: %w", srv.ID, err))
					errsMu.Unlock()
				}
			}
		}()
	}
dispatch:
	for _, srv := range servers {
		select {
		case jobs <- srv:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	_, err = s.db.ExecContext(ctx, `DELETE FROM performance_samples WHERE recorded_at < ?`, time.Now().Add(-PerformanceRetention).UnixMilli())
	return errors.Join(append(errs, err)...)
}

func floatPointer(v float64) *float64 { return &v }

func (s *Store) samplePerformance(ctx context.Context, srv Server, at time.Time) error {
	settings, err := s.PerformanceSettings(ctx, srv.ID)
	if err != nil {
		return err
	}
	sample := PerformanceSample{Timestamp: at}
	if srv.State != StateRunning && srv.State != StateStarting && srv.State != StateInstalling && srv.State != StateBuilding {
		sample.CPUPercent, sample.MemoryBytes = floatPointer(0), floatPointer(0)
		if settings.CollectionMode != "off" {
			sample.Players = floatPointer(0)
		}
	} else if srv.ContainerID != "" && !isDockerNil(s.docker) {
		stats, err := s.docker.Stats(ctx, srv.ContainerID)
		if err == nil {
			sample.Online = true
			sample.CPUPercent = floatPointer(stats.CPUPercent)
			sample.MemoryBytes = floatPointer(float64(stats.MemoryUsageBytes))
		}
		if srv.State == StateRunning && settings.CollectionMode != "off" {
			metricsCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			message := s.sampleGameMetrics(metricsCtx, srv, settings.CollectionMode, &sample)
			cancel()
			// A setting changed during a poll must not receive the old mode's error.
			_, _ = s.db.ExecContext(ctx, `UPDATE performance_settings SET last_error=? WHERE server_id=? AND collection_mode=?`, message, srv.ID, settings.CollectionMode)
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO performance_samples
		(server_id, recorded_at, online, cpu_percent, memory_bytes, players, tps, tick_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		srv.ID, at.UnixMilli(), sample.Online, sample.CPUPercent, sample.MemoryBytes, sample.Players, sample.TPS, sample.TickMS)
	return err
}

func (s *Store) sampleGameMetrics(ctx context.Context, srv Server, mode string, sample *PerformanceSample) string {
	query := func(command string, done func(string) bool) (string, error) {
		return s.performanceCommand(ctx, srv, command, 3*time.Second, done)
	}
	if mode == "rcon" {
		client, err := s.rconDial(ctx, srv.ID)
		if err != nil || client == nil {
			return "RCON is unavailable. Enable RCON, set a password, and make its TCP port reachable from MCM. Console polling was not used."
		}
		defer client.Close()
		query = func(command string, _ func(string) bool) (string, error) { return client.Command(command) }
	}
	out, _ := query("list", func(out string) bool { return playerCount(out) != nil })
	sample.Players = playerCount(out)
	message := ""
	if sample.Players == nil {
		message = "The server did not return a player count. Check the selected polling connection and server configuration."
	}
	if spark, err := s.sparkSupport(ctx, srv); err == nil && spark.Available {
		out, _ = query(sparkCommand(srv.ServerType)+" tps", func(out string) bool {
			tps, tick := parseSparkMetrics(out)
			return tps != nil && tick != nil
		})
		sample.TPS, sample.TickMS = parseSparkMetrics(out)
		if sample.TPS == nil || sample.TickMS == nil {
			message = "Spark did not return tick readings through the selected polling connection. Check that spark is loaded and supports this connection."
		}
	}
	return message
}

var playerCountRE = regexp.MustCompile(`(?i)There are (\d+) of a max \d+ players online`)
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]|§[0-9a-fk-orxA-FK-ORX]`)
var logPrefixRE = regexp.MustCompile(`(?m)^(?:\[[^\]\r\n]*\][ \t]*:?[ \t]*)+`)
var sparkTPSRE = regexp.MustCompile(`(?is)TPS from last [^\r\n]*:\s*(?:\[[^\]\r\n]*\]\s*)*\*?([0-9]+(?:\.[0-9]+)?)`)
var sparkTickRE = regexp.MustCompile(`(?is)Tick durations \(min/med/95%ile/max ms\) from last [^\r\n]*:\s*(?:\[[^\]\r\n]*\]\s*)*[0-9.]+\s*/\s*([0-9]+(?:\.[0-9]+)?)`)

func playerCount(out string) *float64 {
	m := playerCountRE.FindStringSubmatch(ansiRE.ReplaceAllString(out, ""))
	if m == nil {
		return nil
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseSparkMetrics(out string) (*float64, *float64) {
	out = ansiRE.ReplaceAllString(out, "")
	out = logPrefixRE.ReplaceAllString(out, "")
	parse := func(re *regexp.Regexp) *float64 {
		m := re.FindStringSubmatch(out)
		if m == nil {
			return nil
		}
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return nil
		}
		return &v
	}
	return parse(sparkTPSRE), parse(sparkTickRE)
}

func sparkCommand(serverType string) string {
	switch strings.ToLower(serverType) {
	case "velocity":
		return "sparkv"
	case "bungeecord", "waterfall":
		return "sparkb"
	default:
		return "spark"
	}
}

// Commands use the shared data directory's current log offset. This works with
// console stdin and needs no exposed RCON port. Container output is a fallback
// for servers which do not write latest.log. Reading is bounded, and only bytes
// appended after issuing the command are considered a response.
func (s *Store) performanceCommand(ctx context.Context, srv Server, command string, timeout time.Duration, done func(string) bool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	file, err := os.Open(filepath.Join(s.dataPath(srv.ID), "logs", "latest.log"))
	if err != nil {
		if runtime, ok := s.docker.(interface {
			LogsSince(context.Context, string, time.Time) (io.ReadCloser, error)
		}); ok {
			return s.performanceCommandStream(ctx, srv, command, done, runtime.LogsSince)
		}
		client, dialErr := s.rconDial(ctx, srv.ID)
		if dialErr != nil || client == nil {
			return "", errors.New("server response unavailable; logs/latest.log or RCON is required")
		}
		defer client.Close()
		return client.Command(command)
	}
	defer file.Close()
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return "", err
	}
	if err := s.SendConsoleCommand(ctx, srv.ID, command); err != nil {
		return "", err
	}
	var out strings.Builder
	buffer := make([]byte, 8192)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			out.Write(buffer[:n])
		}
		if done(out.String()) {
			return out.String(), nil
		}
		trimPerformanceOutput(&out)
		if err != nil && !errors.Is(err, io.EOF) {
			return out.String(), err
		}
		select {
		case <-ctx.Done():
			return out.String(), ctx.Err()
		case <-ticker.C:
		}
	}
}

// Keep memory bounded without failing a report merely because a busy server
// writes unrelated logs during the 60-second capture.
func trimPerformanceOutput(out *strings.Builder) {
	if out.Len() > 128*1024 {
		tail := strings.Clone(out.String()[out.Len()-64*1024:])
		out.Reset()
		out.WriteString(tail)
	}
}

func (s *Store) performanceCommandStream(ctx context.Context, srv Server, command string, done func(string) bool, logs func(context.Context, string, time.Time) (io.ReadCloser, error)) (string, error) {
	stream, err := logs(ctx, srv.ContainerID, time.Now())
	if err != nil {
		return "", err
	}
	defer stream.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() { _, err := stdcopy.StdCopy(writer, writer, stream); _ = writer.CloseWithError(err) }()
	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 8192), 128*1024)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()
	if err := s.SendConsoleCommand(ctx, srv.ID, command); err != nil {
		return "", err
	}
	var out strings.Builder
	for {
		select {
		case <-ctx.Done():
			return out.String(), ctx.Err()
		case line, ok := <-lines:
			if !ok {
				return out.String(), io.EOF
			}
			out.WriteString(line)
			out.WriteByte('\n')
			if done(out.String()) {
				return out.String(), nil
			}
			trimPerformanceOutput(&out)
		}
	}
}

func (s *Store) PerformanceHistory(ctx context.Context, id, window string) (PerformanceHistory, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return PerformanceHistory{}, err
	}
	duration, resolution, err := performanceWindow(window)
	if err != nil {
		return PerformanceHistory{}, err
	}
	end := time.Now().UTC()
	result := PerformanceHistory{Samples: []PerformanceSample{}, Start: end.Add(-duration), End: end,
		ResolutionSeconds: int(resolution.Seconds()), IntervalSeconds: int(PerformanceInterval.Seconds()), RetentionDays: 7}
	result.Settings, err = s.PerformanceSettings(ctx, id)
	if err != nil {
		return PerformanceHistory{}, err
	}
	result.Spark, err = s.sparkSupport(ctx, srv)
	if err != nil {
		return PerformanceHistory{}, err
	}
	result.Report, err = s.latestSparkReport(ctx, id)
	if err != nil {
		return PerformanceHistory{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT (recorded_at / ?) * ?, MAX(online), AVG(cpu_percent), AVG(memory_bytes), MAX(players), AVG(tps), AVG(tick_ms)
		FROM performance_samples WHERE server_id = ? AND recorded_at >= ? AND recorded_at <= ? GROUP BY recorded_at / ? ORDER BY recorded_at / ?`,
		resolution.Milliseconds(), resolution.Milliseconds(), id, result.Start.UnixMilli(), end.UnixMilli(), resolution.Milliseconds(), resolution.Milliseconds())
	if err != nil {
		return PerformanceHistory{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var at int64
		var sample PerformanceSample
		if err := rows.Scan(&at, &sample.Online, &sample.CPUPercent, &sample.MemoryBytes, &sample.Players, &sample.TPS, &sample.TickMS); err != nil {
			return PerformanceHistory{}, err
		}
		sample.Timestamp = time.UnixMilli(at).UTC()
		result.Samples = append(result.Samples, sample)
	}
	return result, rows.Err()
}

var ErrPerformanceWindow = errors.New("range must be 1h, 6h, 24h, or 7d")

func performanceWindow(window string) (time.Duration, time.Duration, error) {
	switch window {
	case "", "1h":
		return time.Hour, 30 * time.Second, nil
	case "6h":
		return 6 * time.Hour, time.Minute, nil
	case "24h":
		return 24 * time.Hour, 5 * time.Minute, nil
	case "7d":
		return PerformanceRetention, 30 * time.Minute, nil
	default:
		return 0, 0, ErrPerformanceWindow
	}
}

func (s *Store) latestSparkReport(ctx context.Context, id string) (*SparkReport, error) {
	var report SparkReport
	var at int64
	err := s.db.QueryRowContext(ctx, `SELECT id, status, started_at, url, error, existing FROM spark_reports WHERE server_id = ?`, id).
		Scan(&report.ID, &report.Status, &at, &report.URL, &report.Error, &report.Existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	report.StartedAt = time.UnixMilli(at).UTC()
	if report.Status == "capturing" && time.Since(report.StartedAt) > 100*time.Second {
		report.Status, report.Error = "failed", "The capture was interrupted or timed out. Try again."
	}
	return &report, nil
}

// CaptureSparkReport claims a single capture per server transactionally before
// starting background work. Its status survives page navigation and restarts.
func (s *Store) CaptureSparkReport(ctx context.Context, id string) (SparkReport, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return SparkReport{}, err
	}
	if srv.State != StateRunning {
		return SparkReport{}, ErrServerNotRunning
	}
	if isDockerNil(s.docker) || srv.ContainerID == "" {
		return SparkReport{}, ErrServerNotRunning
	}
	support, err := s.sparkSupport(ctx, srv)
	if err != nil {
		return SparkReport{}, err
	}
	if !support.Available {
		return SparkReport{}, ErrSparkUnavailable
	}
	report := SparkReport{ID: uuid.NewString(), Status: "capturing", StartedAt: time.Now().UTC()}
	res, err := s.db.ExecContext(ctx, `INSERT INTO spark_reports (server_id, id, status, started_at) VALUES (?, ?, 'capturing', ?)
		ON CONFLICT(server_id) DO UPDATE SET id=excluded.id, status='capturing', started_at=excluded.started_at, url='', error='', existing=0
		WHERE spark_reports.status != 'capturing' OR spark_reports.started_at < ?`, id, report.ID, report.StartedAt.UnixMilli(), report.StartedAt.Add(-100*time.Second).UnixMilli())
	if err != nil {
		return SparkReport{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return SparkReport{}, err
	}
	if n == 0 {
		return SparkReport{}, ErrSparkBusy
	}
	go s.captureSparkReport(srv, report)
	return report, nil
}

var sparkURLRE = regexp.MustCompile(`https://spark\.lucko\.me/[A-Za-z0-9]+(?:\?[A-Za-z0-9_=&%-]+)?`)

func sparkFailure(out string) bool {
	out = strings.ToLower(ansiRE.ReplaceAllString(out, ""))
	return strings.Contains(out, "unknown command") || strings.Contains(out, "an error occurred") ||
		strings.Contains(out, "profiling is not supported") || strings.Contains(out, "profiler is not supported") || strings.Contains(out, "no permission")
}

func (s *Store) captureSparkReport(srv Server, report SparkReport) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	existing := func(out string) bool {
		out = strings.ToLower(out)
		return strings.Contains(out, "already") && (strings.Contains(out, "running") || strings.Contains(out, "started") || strings.Contains(out, "active"))
	}
	out, err := s.performanceCommand(ctx, srv, sparkCommand(srv.ServerType)+" profiler start --timeout 60", 85*time.Second, func(out string) bool {
		return sparkURLRE.MatchString(out) || sparkFailure(out) || existing(out)
	})
	if existing(out) {
		report.Existing = true
		out, err = s.performanceCommand(ctx, srv, sparkCommand(srv.ServerType)+" profiler open", 10*time.Second, func(out string) bool { return sparkURLRE.MatchString(out) || sparkFailure(out) })
	}
	url := sparkURLRE.FindString(ansiRE.ReplaceAllString(out, ""))
	status, message := "completed", ""
	if url == "" || sparkFailure(out) {
		status, message = "failed", "Spark did not return a report link. Check the server console and try again."
		if err != nil {
			message = "Could not capture a spark report. Check that spark is loaded and logs/latest.log or RCON is available, then try again."
		}
	}
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer updateCancel()
	_, _ = s.db.ExecContext(updateCtx, `UPDATE spark_reports SET status=?, url=?, error=?, existing=? WHERE server_id=? AND id=?`, status, url, message, report.Existing, srv.ID, report.ID)
}
