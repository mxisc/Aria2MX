package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aria2mx/internal/server"
	"aria2mx/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	cfgPath := getenv("ARIA2MX_CONFIG", "aria2mx.json")
	cfg, err := server.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	app, err := server.New(server.Options{
		ConfigPath: cfgPath,
		Config:     cfg,
		Assets:     web.Assets(),
	})
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer func() {
		if err := app.Close(); err != nil {
			log.Printf("stop managed aria2: %v", err)
		}
	}()

	addr := getenv("ARIA2MX_ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	log.Printf("Aria2MX listening on %s", addr)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("shutdown: %w", err)
		}
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("listen: %w", err)
		}
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
