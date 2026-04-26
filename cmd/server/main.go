package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sakh1l/quizhub/internal/db"
	"github.com/sakh1l/quizhub/internal/handlers"
	"github.com/sakh1l/quizhub/internal/middleware"
	"github.com/sakh1l/quizhub/internal/ws"
	"github.com/sakh1l/quizhub/web"
)

func main() {
	dbPath := os.Getenv("QUIZHUB_DB")
	if dbPath == "" {
		dbPath = "quizhub.db"
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	hub := ws.NewHub()
	go hub.Run()

	h := handlers.New(database, hub)

	mux := http.NewServeMux()
	h.Register(mux)

	// Serve embedded static files
	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatalf("Failed to load static files: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	handler := middleware.Chain(
		mux,
		middleware.Recover,
		middleware.Logger,
		middleware.CORS,
		middleware.SecurityHeaders,
	)

	port := os.Getenv("QUIZHUB_PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("QuizHub v1.0.0 running on http://localhost:%s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-done
	fmt.Println("\nShutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Shutdown error: %v", err)
	}

	fmt.Println("Server stopped.")
}
