// Package main is the entry point for the SELAR API server.
// It initialises the database connection pool, registers Chi routes with
// JWT auth middleware and CORS, and starts the HTTP server with graceful shutdown.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/cors"
	"github.com/selar-dev/selar-api/internal/handler"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/store"
)

func main() {
	loadEnv()
	port := getEnv("PORT", "8080")
	dbURL := getEnv("DATABASE_URL", "postgres://selar:selar_dev@localhost:5432/selar?sslmode=disable")
	jwtSecret := getEnv("JWT_SECRET", "dev-secret-change-in-production")
	corsOrigin := getEnv("CORS_ORIGIN", "http://localhost:3000")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database connection pool
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("unable to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("unable to ping database: %v", err)
	}
	log.Println("connected to database")

	// Store & handlers
	st := store.New(pool)
	h := handler.New(st)
	auth := middleware.NewAuth(jwtSecret)
	h.SetAuth(auth)

	// Router
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(cors.New(cors.Options{
		AllowedOrigins:   []string{corsOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}).Handler)

	// Public routes
	r.Get("/healthz", h.Health)
	r.Head("/healthz", h.Health)
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)

	// Protected API routes
	r.Route("/api", func(r chi.Router) {
		r.Use(auth.Verify)

		// Users
		r.Get("/users/me", h.GetCurrentUser)
		r.Patch("/users/me/preferences", h.UpdatePreferences)

		// Documents
		r.Get("/documents", h.ListDocuments)
		r.Post("/documents", h.CreateDocument)
		r.Post("/documents/upload", h.UploadDocument)
		r.Get("/documents/stats", h.GetDocumentStats)
		r.Get("/documents/{id}", h.GetDocument)
		r.Get("/documents/{docId}/pdf", h.ServeDocument)
		r.Delete("/documents/{id}", h.DeleteDocument)

		// Suggestions (per document)
		r.Get("/documents/{id}/suggestions", h.ListSuggestions)
		r.Post("/suggestions/{id}/respond", h.RespondToSuggestion)

		// Annotations (per document)
		r.Get("/documents/{id}/annotations", h.ListAnnotations)
		r.Post("/annotations", h.CreateAnnotation)
		r.Delete("/annotations/{id}", h.DeleteAnnotation)

		// Concepts & Graph
		r.Get("/concepts", h.ListConcepts)
		r.Get("/graph", h.GetGraph)
		r.Post("/graph/replay", h.ReplayAdaptiveGraph)
		r.Post("/graph/lifecycle", h.ApplyGraphLifecycle)
		r.Post("/graph/edges/{id}/respond", h.RespondToConceptEdge)
		r.Get("/documents/{id}/mental-model", h.GetDocumentMentalModel)
		r.Get("/mental-model-links", h.ListMentalModelLinks)
		r.Post("/mental-model-links/{id}/respond", h.RespondToMentalModelLink)
		r.Get("/learner-state", h.ListLearnerConceptState)

		// Grounded Chat
		r.Get("/chat/threads", h.ListChatThreads)
		r.Post("/chat/threads", h.CreateChatThread)
		r.Get("/chat/threads/{id}/messages", h.ListChatMessages)
		r.Post("/chat/threads/{id}/messages", h.CreateChatMessage)
		r.Delete("/chat/threads/{id}", h.DeleteChatThread)
		r.Post("/chat/messages/{id}/feedback", h.RecordChatFeedback)
		r.Post("/chat/citations/{id}/open", h.RecordCitationOpen)

		// Deterministic learner signals and local evaluation metrics
		r.Post("/learner-signals", h.RecordLearnerSignal)
		r.Get("/evaluation/metrics", h.GetEvaluationMetrics)

		// Reading Sessions
		r.Post("/sessions/start", h.StartSession)
		r.Post("/sessions/{id}/end", h.EndSession)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down server...")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Fatalf("server shutdown failed: %v", err)
		}
	}()

	log.Printf("selar-api listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	fmt.Println("server stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadEnv() {
	paths := []string{".env", "../../.env", "../../.env.development", "../../../.env", "../../../.env.development"}
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
					(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
					value = value[1 : len(value)-1]
				}
				if os.Getenv(key) == "" {
					os.Setenv(key, value)
				}
			}
		}
		log.Printf("Loaded environment from %s", path)
		break
	}
}
