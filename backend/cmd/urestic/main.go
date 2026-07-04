package main

import (
	"log"

	"github.com/urestic/urestic/backend/internal/auth"
	"github.com/urestic/urestic/backend/internal/config"
	"github.com/urestic/urestic/backend/internal/db"
	"github.com/urestic/urestic/backend/internal/httpapi"
	"github.com/urestic/urestic/backend/internal/notifications"
	"github.com/urestic/urestic/backend/internal/repositories"
	"github.com/urestic/urestic/backend/internal/secrets"
	"github.com/urestic/urestic/backend/internal/settings"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	secretManager, err := secrets.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to open secret manager: %v", err)
	}
	authManager, err := auth.Open(cfg)
	if err != nil {
		log.Fatalf("failed to initialize auth: %v", err)
	}

	repositoryStore := repositories.NewStore(database)
	notificationStore := notifications.NewStore(database)
	settingsStore := settings.NewStore(database)
	router := httpapi.NewRouter(cfg, repositoryStore, notificationStore, settingsStore, secretManager, authManager)

	log.Printf("starting urestic on %s", cfg.Addr)
	if err := router.Run(cfg.Addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
