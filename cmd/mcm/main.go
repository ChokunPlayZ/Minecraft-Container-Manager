package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/mcm-panel/mcm/internal/api"
	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/config"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/dns"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/notifications"
	"github.com/mcm-panel/mcm/internal/schedules"
	"github.com/mcm-panel/mcm/internal/servers"
	"github.com/mcm-panel/mcm/internal/tlsconfig"
)

func main() {
	logger := log.New(os.Stdout, "[mcm] ", log.LstdFlags)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("invalid config: %v", err)
	}
	if os.Getenv(config.EnvSessionSecret) == "" {
		logger.Println("warning: MCM_SESSION_SECRET is empty; using an ephemeral secret, sessions will not survive restarts")
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
	backupScheduler := backups.NewScheduler(backupStore, handle.DB, logger, 1*time.Minute)
	backupScheduler.Start()
	defer backupScheduler.Stop()
	scheduleStore := schedules.NewStore(handle.DB)
	taskScheduler := schedules.NewScheduler(scheduleStore, serverStore.ExecuteScheduledTask, logger)
	taskScheduler.Start()
	defer taskScheduler.Stop()

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
	})

	if !cfg.HTTPOnly {
		tlsConfig, err := tlsconfig.Load(cfg)
		if err != nil {
			logger.Fatalf("configure HTTPS: %v", err)
		}
		if cfg.TLSCert == "" {
			logger.Printf("using self-signed TLS certificate in %s/tls; trust cert.pem in your browser or provide MCM_TLS_CERT and MCM_TLS_KEY", cfg.DataDir)
		}
		logger.Printf("listening on https://%s (data dir %s)", cfg.Addr, cfg.DataDir)
		server := &http.Server{Addr: cfg.Addr, Handler: handler, TLSConfig: tlsConfig}
		if err := tlsconfig.ListenAndServe(server, cfg.TLSRedirect); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server exited: %v", err)
		}
		return
	}
	logger.Printf("listening on http://%s (data dir %s)", cfg.Addr, cfg.DataDir)
	if err := http.ListenAndServe(cfg.Addr, handler); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server exited: %v", err)
	}
}
