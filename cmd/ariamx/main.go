package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"ariamx/internal/server"
	"ariamx/internal/web"
)

func main() {
	cfgPath := getenv("ARIAMX_CONFIG", "ariamx.json")
	cfg, err := server.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	app, err := server.New(server.Options{
		ConfigPath: cfgPath,
		Config:     cfg,
		Assets:     web.Assets(),
	})
	if err != nil {
		log.Fatalf("start server: %v", err)
	}

	addr := getenv("ARIAMX_ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("AriaMX listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
