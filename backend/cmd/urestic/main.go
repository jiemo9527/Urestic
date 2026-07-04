package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

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
	if handleCommand(cfg) {
		return
	}

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

func handleCommand(cfg config.Config) bool {
	if len(os.Args) < 2 {
		return false
	}
	switch os.Args[1] {
	case "reset-admin-password":
		if err := resetAdminPassword(cfg, os.Args[2:]); err != nil {
			log.Fatalf("failed to reset admin password: %v", err)
		}
		fmt.Println("admin password reset successfully")
		return true
	case "help", "--help", "-h":
		printUsage()
		return true
	default:
		return false
	}
}

func resetAdminPassword(cfg config.Config, args []string) error {
	var password string
	switch len(args) {
	case 1:
		if args[0] == "--stdin" {
			value, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			password = strings.TrimRight(string(value), "\r\n")
		} else {
			password = args[0]
		}
	default:
		return errors.New("usage: urestic reset-admin-password <new-password> or urestic reset-admin-password --stdin")
	}
	_, err := auth.ResetPassword(cfg.DataDir, password)
	return err
}

func printUsage() {
	fmt.Println(`Usage:
  urestic
  urestic reset-admin-password <new-password>
  urestic reset-admin-password --stdin`)
}
