package main

import (
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
	"github.com/mcm-panel/mcm/internal/servers"
	"github.com/mcm-panel/mcm/internal/spindown"
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

	dockerMgr, err := docker.New(cfg.DockerHost)
	if err != nil {
		logger.Fatalf("create docker client: %v", err)
	}

	jarResolver := jars.NewResolver()
	serverStore := servers.NewStore(handle, dockerMgr, jarResolver, cfg.PortRange.Start, cfg.PortRange.End, cfg.DataDir)
	dnsService := dns.New(handle.DB)
	serverStore.SetDNS(dnsService)
	backupStore := backups.New(handle.DB, backups.S3Config{
		Endpoint:  cfg.S3.Endpoint,
		AccessKey: cfg.S3.AccessKey,
		SecretKey: cfg.S3.SecretKey,
		Bucket:    cfg.S3.Bucket,
		Region:    cfg.S3.Region,
	}, cfg.DataDir)
	backupScheduler := backups.NewScheduler(backupStore, handle.DB, logger, 1*time.Minute)
	backupScheduler.Start()
	defer backupScheduler.Stop()

	spinService := spindown.New(serverStore, serverStore, logger, 30*time.Minute)
	spinService.Start()
	defer spinService.Stop()

	handler := api.New(api.Options{
		Cfg:      cfg,
		DB:       handle,
		Servers:  serverStore,
		Backups:  backupStore,
		Users:    users,
		Sessions: sessions,
		Passkeys: passkeys,
		WebAuthn: webAuthn,
		Jars:     jarResolver,
		DNS:      dnsService,
		Spin:     spinService,
		Logger:   logger,
	})

	logger.Printf("listening on %s (data dir %s)", cfg.Addr, cfg.DataDir)
	if err := http.ListenAndServe(cfg.Addr, handler); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server exited: %v", err)
	}
}
