package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"modular-api/internal/modules/announcements"
	"modular-api/internal/modules/billing"
	"modular-api/internal/modules/complaints"
	"modular-api/internal/modules/documents"
	"modular-api/internal/modules/feedback"
	"modular-api/internal/modules/hub"
	"modular-api/internal/modules/property"
	"modular-api/internal/modules/visitors"
	"modular-api/internal/platform/config"
	"modular-api/internal/platform/database"
	"modular-api/internal/platform/httpserver"
	"modular-api/internal/platform/seed"
	"modular-api/internal/platform/storage"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("auto migrate: %v", err)
	}

	if err := seed.Run(db); err != nil {
		log.Fatalf("seed database: %v", err)
	}

	fileStorage := storage.NewLocalStorage(cfg.StorageRoot, cfg.PublicBaseURL)
	propertyModule := property.NewModule(db)
	billingModule := billing.NewModule(db, billing.GatewayConfig{
		BillplzAPIBaseURL:      cfg.BillplzAPIBaseURL,
		BillplzAPIKey:          cfg.BillplzAPIKey,
		BillplzXSignatureKey:   cfg.BillplzXSignatureKey,
		BillplzCollectionID:    cfg.BillplzCollectionID,
		BillplzCallbackBaseURL: cfg.BillplzCallbackBaseURL,
	})
	announcementsModule := announcements.NewModule(db, fileStorage)
	documentsModule := documents.NewModule(db, fileStorage)
	complaintsModule := complaints.NewModule(db, fileStorage)
	feedbackModule := feedback.NewModule(db, fileStorage)
	hubModule := hub.NewModule(db, fileStorage)
	visitorsModule := visitors.NewModule(db)

	apiRouter := httpserver.NewRouter(
		propertyModule.Handler(),
		billingModule.Handler(),
		announcementsModule.Handler(),
		documentsModule.Handler(),
		complaintsModule.Handler(),
		feedbackModule.Handler(),
		hubModule.Handler(),
		visitorsModule.Handler(),
	)

	rootMux := http.NewServeMux()
	rootMux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir(cfg.StorageRoot))))
	rootMux.Handle("/", apiRouter)

	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           rootMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("api listening on %s", cfg.HTTPAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen and serve: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown server: %v", err)
	}
}
