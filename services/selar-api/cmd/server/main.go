// Package main is the entry point for the SELAR API server.
// It initialises the database connection pool, registers Chi routes with
// JWT auth middleware and CORS, and starts the HTTP server with graceful shutdown.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/cors"
	"github.com/selar-dev/selar-api/internal/allowlist"
	"github.com/selar-dev/selar-api/internal/analytics"
	"github.com/selar-dev/selar-api/internal/dbconfig"
	"github.com/selar-dev/selar-api/internal/googleauth"
	"github.com/selar-dev/selar-api/internal/handler"
	"github.com/selar-dev/selar-api/internal/linktoken"
	"github.com/selar-dev/selar-api/internal/middleware"
	"github.com/selar-dev/selar-api/internal/notify"
	"github.com/selar-dev/selar-api/internal/recaptcha"
	"github.com/selar-dev/selar-api/internal/storage"
	"github.com/selar-dev/selar-api/internal/store"
	"github.com/selar-dev/selar-api/internal/workertrigger"
)

const developmentJWTSecret = "dev-secret-change-in-production"

func main() {
	loadEnv()
	port := getEnv("PORT", "8080")
	appEnv := getEnv("APP_ENV", "development")
	dbURL := getEnv("DATABASE_URL", "postgres://selar:***@localhost:5432/selar?sslmode=disable")
	jwtSecret := getEnv("JWT_SECRET", developmentJWTSecret)
	corsOrigins, err := parseCORSOrigins(getEnv("CORS_ORIGIN", "http://localhost:3000"))
	if err != nil {
		log.Fatal(err)
	}
	if err := validateRuntimeConfig(appEnv, jwtSecret); err != nil {
		log.Fatal(err)
	}
	if err := validateDatabaseURL(appEnv, dbURL); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database connection pool with serverless-safe bounds (few connections,
	// short idle/lifetime, bounded connect timeout). Migrations never run at
	// startup; apply them with cmd/migrate as a separate deploy step.
	poolConfig, err := dbconfig.PoolConfig(dbURL, os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatal("unable to configure database pool")
	}
	defer pool.Close()

	// A cold start must not hang on an unreachable database: ping with a
	// short deadline and keep serving; later queries retry through the pool.
	pingCtx, pingCancel := context.WithTimeout(ctx, dbconfig.DefaultConnectTimeout)
	if err := pool.Ping(pingCtx); err != nil {
		log.Println("database ping failed at startup; requests will retry the pool")
	} else {
		log.Println("connected to database")
	}
	pingCancel()

	// Store & handlers
	st := store.New(pool)
	h := handler.New(st)
	uploads, err := storage.FromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("invalid storage configuration: %v", err)
	}
	h.SetStorage(uploads)
	trigger := workertrigger.FromEnv(os.Getenv)
	h.SetWorkerTrigger(trigger)
	log.Printf("worker trigger enabled: %t", trigger.Enabled())
	log.Printf("upload storage backend: %s", uploads.Backend())
	auth := middleware.NewAuth(jwtSecret)
	auth.SetSessionVersionLookup(handler.SessionVersionLookup(st))
	h.SetAuth(auth)
	adminEmails := analytics.ParseEmailList(os.Getenv("ADMIN_EMAILS"))
	h.SetAdminEmails(adminEmails)
	h.SetAnalyticsEnabled(analyticsEnabled(os.Getenv("ANALYTICS_ENABLED")))
	log.Printf("analytics enabled: %t; ADMIN_EMAILS entries: %d", h.AnalyticsEnabled(), len(adminEmails))
	// Quiz admin routes share the users.role check (ADMIN_EMAILS entries are
	// promoted on first use), so /api/admin/quizzes and /api/admin/* agree.
	h.SetAdminChecker(h.RoleAdminChecker())
	h.SetCaptcha(recaptcha.FromEnv(os.Getenv))
	log.Printf("recaptcha on registration enabled: %t", h.CaptchaEnabled())
	// Sign in with Google is enabled only when both GOOGLE_CLIENT_ID and
	// GOOGLE_CLIENT_SECRET are set.
	if google := googleauth.FromEnv(os.Getenv); google != nil {
		h.SetGoogle(google)
	}
	log.Printf("google sign-in enabled: %t", h.GoogleEnabled())
	// Optional quiz-window calendar feed (#113): off unless CALENDAR_FEED_ENABLED=true
	// AND the account is in CALENDAR_FEED_ALLOWLIST.
	calendarGate := allowlist.FromEnv(os.Getenv("CALENDAR_FEED_ENABLED"), os.Getenv("CALENDAR_FEED_ALLOWLIST"))
	h.SetCalendarConfig(handler.CalendarConfig{
		Gate:         calendarGate,
		Signer:       linktoken.New(linkTokenSecret(jwtSecret)),
		PublicAPIURL: os.Getenv("PUBLIC_API_URL"),
		ConsoleURL:   getEnv("PUBLIC_CONSOLE_URL", firstOrigin(corsOrigins)),
	})
	log.Printf("calendar feed: %s", calendarGate.Describe())
	// Optional email notices (#114): dry-run unless NOTIFY_EMAIL_ENABLED=true, the
	// address is in NOTIFY_EMAIL_ALLOWLIST and SMTP is fully configured.
	notifier := buildNotifier(st, linktoken.New(linkTokenSecret(jwtSecret)), getEnv("PUBLIC_CONSOLE_URL", firstOrigin(corsOrigins)))
	h.SetNotifier(notifier, os.Getenv("WORKER_TRIGGER_SECRET"))
	log.Printf("email notices: %s; transport: %s", notifier.Mode(), notifier.TransportMode())

	// Router
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(cors.New(cors.Options{
		AllowedOrigins:   corsOrigins,
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
	r.Post("/auth/google", h.GoogleSignIn)
	// Token-authenticated calendar feed (no session; see handler/calendar.go).
	r.Get("/calendar/{token}", h.ServeCalendarFeed)
	// Signed unsubscribe links and the secret-authenticated scheduler hook.
	h.MountNotificationPublic(r)

	// Protected API routes
	r.Route("/api", func(r chi.Router) {
		r.Use(auth.Verify)

		// Private generated practice is separate from the fixed formal quiz system.
		r.Post("/practice", h.Practice)
		// Users
		r.Get("/users/me", h.GetCurrentUser)
		r.Patch("/users/me/preferences", h.UpdatePreferences)
		r.Get("/users/me/settings", h.GetSettings)
		r.Patch("/users/me/settings", h.UpdateSettings)
		r.Patch("/users/me/profile", h.UpdateProfile)
		r.Post("/users/me/email", h.ChangeEmail)
		r.Post("/users/me/password", h.ChangePassword)
		r.Get("/users/me/export", h.ExportMyData)
		r.Post("/users/me/delete", h.DeleteAccount)
		h.MountCalendarRoutes(r)
		h.MountNotificationRoutes(r)

		// Documents
		r.Get("/documents", h.ListDocuments)
		r.Post("/documents", h.CreateDocument)
		r.Post("/documents/add", h.AddContent)
		r.Post("/documents/upload", h.UploadDocument)
		r.Post("/documents/upload-url", h.CreateUploadURL)
		r.Post("/documents/upload-complete", h.CompleteUpload)
		r.Post("/documents/import/drive", h.ImportDriveFile)
		r.Get("/documents/stats", h.GetDocumentStats)
		r.Get("/documents/{id}", h.GetDocument)
		r.Get("/documents/{id}/content", h.GetDocumentContent)
		r.Post("/documents/{id}/retry", h.RetryDocumentIngestion)
		r.Get("/documents/{id}/assets/{assetId}", h.ServeAsset)
		r.Get("/documents/{docId}/pdf", h.ServeDocument)
		r.Delete("/documents/{id}", h.DeleteDocument)

		// Managed source origins and auditable ingestion runs
		r.Get("/sources", h.ListSources)
		r.Post("/sources/{id}/refresh", h.RefreshSource)
		r.Get("/sources/{id}/runs", h.ListIngestionRuns)
		r.Delete("/sources/{id}", h.ArchiveSource)

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
		r.Post("/graph/concepts/{id}/respond", h.RespondToConcept)
		r.Get("/documents/{id}/mental-model", h.GetDocumentMentalModel)
		r.Get("/mental-model-links", h.ListMentalModelLinks)
		r.Get("/mental-model-links/{id}/preview", h.PreviewMentalModelLink)
		r.Post("/mental-model-links/{id}/respond", h.RespondToMentalModelLink)
		r.Get("/learner-state", h.ListLearnerConceptState)

		// Provenance-aware research assertions (issue #9): proposed, then owner-reviewed.
		r.Get("/research-assertions", h.ListResearchAssertions)
		r.Post("/research-assertions", h.ProposeResearchAssertion)
		r.Post("/research-assertions/{id}/respond", h.RespondToResearchAssertion)

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

		// Quizzes (learner + admin; admin routes check AdminChecker)
		h.MountQuizRoutes(r)

		// Reading Sessions
		r.Post("/sessions/start", h.StartSession)
		r.Post("/sessions/{id}/end", h.EndSession)

		// First-party analytics: consent, batched console beacon, delete-my-data
		h.MountUserAnalytics(r)

		// Admin-only (role re-read from the database on every request)
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.RequireAdmin(h.RoleLookup()))
			h.MountAdmin(r)
			h.MountNotificationAdmin(r)
		})
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

// analyticsEnabled parses ANALYTICS_ENABLED. Recording is on unless it is
// explicitly set to a false value; per-user events still require consent.
func analyticsEnabled(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// linkTokenSecret is the key for signed links (calendar feeds, unsubscribe).
// LINK_TOKEN_SECRET lets it rotate independently; by default it is derived
// from JWT_SECRET (linktoken separates the keys per purpose).
func linkTokenSecret(jwtSecret string) string {
	if v := strings.TrimSpace(os.Getenv("LINK_TOKEN_SECRET")); v != "" {
		return v
	}
	return jwtSecret
}

// buildNotifier reads the NOTIFY_* environment. Real sending needs all of
// NOTIFY_EMAIL_ENABLED=true, a non-empty NOTIFY_EMAIL_ALLOWLIST and complete
// NOTIFY_SMTP_* settings; anything less keeps every message in dry-run.
func buildNotifier(st *store.Store, signer *linktoken.Signer, consoleURL string) *notify.Notifier {
	cfg := notify.Config{
		Live:         allowlist.Flag(os.Getenv("NOTIFY_EMAIL_ENABLED")),
		Allow:        allowlist.Parse(os.Getenv("NOTIFY_EMAIL_ALLOWLIST")),
		Signer:       signer,
		PublicAPIURL: os.Getenv("PUBLIC_API_URL"),
		ConsoleURL:   consoleURL,
	}
	if tz := os.Getenv("NOTIFY_TIMEZONE"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			cfg.Location = loc
		} else {
			log.Printf("NOTIFY_TIMEZONE %q not recognised; using UTC", tz)
		}
	} else if loc, err := time.LoadLocation("Asia/Colombo"); err == nil {
		cfg.Location = loc
	}
	if cfg.Live {
		port, _ := strconv.Atoi(os.Getenv("NOTIFY_SMTP_PORT"))
		smtpT, err := notify.NewSMTP(notify.SMTPConfig{
			Host: os.Getenv("NOTIFY_SMTP_HOST"), Port: port,
			Username: os.Getenv("NOTIFY_SMTP_USERNAME"), Password: os.Getenv("NOTIFY_SMTP_PASSWORD"),
			From: os.Getenv("NOTIFY_EMAIL_FROM"), ReplyTo: os.Getenv("NOTIFY_EMAIL_REPLY_TO"),
		})
		if err != nil {
			log.Printf("NOTIFY_EMAIL_ENABLED is set but SMTP is incomplete (%v); staying in dry-run", err)
		} else {
			cfg.Real = smtpT
		}
	}
	return notify.New(st, cfg)
}

func firstOrigin(origins []string) string {
	if len(origins) == 0 {
		return ""
	}
	return origins[0]
}

func validateRuntimeConfig(appEnv, jwtSecret string) error {
	if strings.EqualFold(strings.TrimSpace(appEnv), "production") && jwtSecret == developmentJWTSecret {
		return errors.New("JWT_SECRET must be replaced before running SELAR in production")
	}
	return nil
}

// parseCORSOrigins accepts a comma-separated list of exact http(s) origins.
// Wildcards are rejected because the API allows credentials.
func parseCORSOrigins(raw string) ([]string, error) {
	var origins []string
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSuffix(strings.TrimSpace(part), "/")
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || strings.Contains(origin, "*") || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.User != nil {
			return nil, fmt.Errorf("CORS_ORIGIN entry %q must be an exact origin such as https://selar.example.com", origin)
		}
		origins = append(origins, parsed.Scheme+"://"+parsed.Host)
	}
	if len(origins) == 0 {
		return nil, errors.New("CORS_ORIGIN must list at least one exact origin")
	}
	return origins, nil
}

// validateDatabaseURL requires TLS for remote databases in production.
func validateDatabaseURL(appEnv, databaseURL string) error {
	if !strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		return nil
	}
	return dbconfig.RequireTLSForRemote(databaseURL)
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
