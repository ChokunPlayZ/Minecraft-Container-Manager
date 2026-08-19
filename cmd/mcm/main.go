package main

import (
	"log"
	"net/http"
	"os"

	"github.com/mcm-panel/mcm/internal/api"
	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/config"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
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

	dockerMgr, err := docker.New(cfg.DockerHost)
	if err != nil {
		logger.Fatalf("create docker client: %v", err)
	}

	jarResolver := jars.NewResolver()
	serverStore := servers.NewStore(handle, dockerMgr, jarResolver, cfg.PortRange.Start, cfg.PortRange.End, cfg.DataDir)

	handler := api.New(api.Options{
		Cfg:      cfg,
		DB:       handle,
		Servers:  serverStore,
		Users:    users,
		Sessions: sessions,
		Jars:     jarResolver,
		Logger:   logger,
	})

	logger.Printf("listening on %s (data dir %s)", cfg.Addr, cfg.DataDir)
	if err := http.ListenAndServe(cfg.Addr, handler); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server exited: %v", err)
	}
}
