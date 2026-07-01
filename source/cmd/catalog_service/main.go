package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/adamhoof/MOBv3/internal/catalog"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	store, err := catalog.OpenTurso(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mqttClient, err := startProductLookupSubscriber(ctx, cfg, store)
	if err != nil {
		log.Fatal(err)
	}
	defer mqttClient.Disconnect(250)

	tlsConfig, err := serverTLSConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.ImportEndpoint, handleImport(newImportRunner(store), cfg))

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown failed: %s", err)
		}
	}()

	log.Printf("catalog_service listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServeTLS(cfg.CatalogServerCertPath, cfg.CatalogServerKeyPath); err != nil && err != http.ErrServerClosed {
		log.Fatal(fmt.Errorf("serve https: %w", err))
	}
}
