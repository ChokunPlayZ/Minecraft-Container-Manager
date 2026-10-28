package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/mcm-panel/mcm/internal/api"
	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/buildinfo"
	"github.com/mcm-panel/mcm/internal/config"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/dns"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/notifications"
	"github.com/mcm-panel/mcm/internal/panelupdate"
	"github.com/mcm-panel/mcm/internal/schedules"
	"github.com/mcm-panel/mcm/internal/servers"
	"github.com/mcm-panel/mcm/internal/tlsconfig"
)

func main() {
	logger := log.New(os.Stdout, "[mcm] ", log.LstdFlags)
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--update-worker" {
		if len(os.Args) != 5 {
			logger.Fatal("invalid update worker arguments")
		}
		if err := panelupdate.RunWorker(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			logger.Fatal(err)
		}
		return
	}
	build := buildinfo.Current()
	logger.Printf("build commit: %s (modified: %t, built: %s)", build.Commit, build.Dirty, build.BuiltAt)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("invalid config: %v", err)
	}

	handle, err := db.Open(cfg.DBPath)
	if err != nil {
		logger.Fatalf("open database: %v", err)
	}
	defer handle.Close()

	users := auth.NewUsers(handle.DB)
	sessions := auth.NewManager(handle.DB)
	passkeys := auth.NewPasskeys(handle.DB)

	webAuthn, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.WebAuthn.RPID,
		RPDisplayName: cfg.WebAuthn.RPDisplayName,
		RPOrigins:     cfg.WebAuthn.RPOrigins,
	})
	if err != nil {
		logger.Fatalf("configure webauthn: %v", err)
	}

	dockerMgr, err := docker.NewWithRuntime(cfg.ContainerRuntime, cfg.DockerHost, cfg.ServerImage)
	if err != nil {
		logger.Fatalf("create container runtime client: %v", err)
	}
	logger.Printf("container runtime: %s (%s)", cfg.ContainerRuntime, cfg.DockerHost)
	defer dockerMgr.Close()

	jarResolver := jars.NewResolver()
	serverStore := servers.NewStore(handle, dockerMgr, jarResolver, cfg.PortRange.Start, cfg.PortRange.End, cfg.DataDir, cfg.DataDirHost)
	notificationService := notifications.New(handle.DB, logger)
	notificationCtx, stopNotifications := context.WithCancel(context.Background())
	defer stopNotifications()
	serverStore.SetNotifier(notificationService)
	go notificationService.Run(notificationCtx)
	performanceCtx, stopPerformance := context.WithCancel(context.Background())
	defer stopPerformance()
	go serverStore.RunPerformanceMonitor(performanceCtx, logger)
	go serverStore.RunUpdateChecker(performanceCtx, logger)
	dnsService := dns.New(handle.DB)
	serverStore.SetDNS(dnsService)
	go serverStore.RunStatusMonitor(notificationCtx, logger)
	backupStore := backups.New(handle.DB, backups.S3Config{
		Endpoint:  cfg.S3.Endpoint,
		AccessKey: cfg.S3.AccessKey,
		SecretKey: cfg.S3.SecretKey,
		Bucket:    cfg.S3.Bucket,
		Region:    cfg.S3.Region,
	}, cfg.DataDir, cfg.BackupDir)
	backupStore.SetNotifier(notificationService)
	backupStore.SetDataCoordinator(serverStore.WithStoppedData)
	backupScheduler := backups.NewScheduler(backupStore, handle.DB, logger, 1*time.Minute)
	backupScheduler.Start()
	scheduleStore := schedules.NewStore(handle.DB)
	taskScheduler := schedules.NewScheduler(scheduleStore, serverStore.ExecuteScheduledTask, logger)
	taskScheduler.Start()

	handler := api.New(api.Options{
		Cfg:           cfg,
		DB:            handle,
		Servers:       serverStore,
		Backups:       backupStore,
		Schedules:     scheduleStore,
		Users:         users,
		Sessions:      sessions,
		Passkeys:      passkeys,
		WebAuthn:      webAuthn,
		Jars:          jarResolver,
		DNS:           dnsService,
		Logger:        logger,
		Notifications: notificationService,
		PanelUpdates:  panelupdate.New(),
	})

	server := &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	serving := make(chan error, 1)
	if !cfg.HTTPOnly {
		tlsConfig, err := tlsconfig.Load(cfg)
		if err != nil {
			logger.Fatalf("configure HTTPS: %v", err)
		}
		server.TLSConfig = tlsConfig
		logger.Printf("listening on https://%s (data dir %s)", cfg.Addr, cfg.DataDir)
		go func() { serving <- tlsconfig.ListenAndServe(server, cfg.TLSRedirect) }()
	} else {
		logger.Printf("listening on http://%s (data dir %s)", cfg.Addr, cfg.DataDir)
		go func() { serving <- server.ListenAndServe() }()
	}
	select {
	case err := <-serving:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("server exited: %v", err)
		}
	case <-signalCtx.Done():
		logger.Println("shutting down; draining active requests")
		stopPerformance()
		stopNotifications()
		drainCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		workersDone := make(chan struct{})
		go func() {
			backupScheduler.Stop()
			taskScheduler.Stop()
			_ = serverStore.WaitProvisioning(drainCtx)
			close(workersDone)
		}()

		if err := server.Shutdown(drainCtx); err != nil {
			logger.Printf("shutdown: %v", err)
			server.Close()
		}
		select {
		case <-workersDone:
		case <-drainCtx.Done():
			logger.Println("worker shutdown deadline reached")
		}

	}
}
