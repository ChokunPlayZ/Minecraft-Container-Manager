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
	"github.com/mcm-panel/mcm/internal/gateway"
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

	// The gateway owns each server's public port and wakes on connect. It is
	// enabled per MCM_GATEWAY; in auto mode it tracks the gateway_enabled
	// setting (tied to spin-down), read live on each reconcile.
	gatewayEnabled := func(ctx context.Context) (bool, error) {
		switch cfg.Gateway {
		case "on":
			return true, nil
		case "off":
			return false, nil
		default: // auto
			en, err := serverStore.GatewayEnabled(ctx)
			if err != nil {
				return false, err
			}
			return en, nil
		}
	}
	gatewayMgr := gateway.New(gateway.Options{
		Logger:  logger,
		Store:   serverStore,
		Docker:  dockerMgr,
		Waker:   spinService,
		Enabled: gatewayEnabled,
	})
	gatewayMgr.Start()
	defer gatewayMgr.Stop()

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

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		if cfg.TLSRedirect {
			redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				u := *r.URL
				u.Scheme = "https"
				u.Host = r.Host
				http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
			})
			go func() {
				logger.Printf("redirecting HTTP %s to HTTPS %s", cfg.TLSRedirectAddr, cfg.Addr)
				if err := http.ListenAndServe(cfg.TLSRedirectAddr, redirect); err != nil && err != http.ErrServerClosed {
					logger.Fatalf("http redirect server exited: %v", err)
				}
			}()
		}
		logger.Printf("listening on https://%s (data dir %s)", cfg.Addr, cfg.DataDir)
		if err := http.ListenAndServeTLS(cfg.Addr, cfg.TLSCert, cfg.TLSKey, handler); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server exited: %v", err)
		}
		return
	}
	logger.Printf("listening on %s (data dir %s)", cfg.Addr, cfg.DataDir)
	if err := http.ListenAndServe(cfg.Addr, handler); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server exited: %v", err)
	}
}
