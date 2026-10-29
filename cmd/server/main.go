package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/auth"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/handlers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/middleware"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/proxy"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/scheduler"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/migrations"
)

func main() {
	healthcheckFlag := flag.Bool("healthcheck", false, "Execute health check probe and exit")
	flag.Parse()

	port := getEnv("PORT", "8080")

	// If health check probe flag is set, ping /healthz and exit 0 (healthy) or 1 (unhealthy)
	if *healthcheckFlag {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", port))
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	databaseURL := getEnv("DATABASE_URL", "postgres://manhua_user:manhua_secure_password_change_me@localhost:5432/manhua_pro?sslmode=disable")
	redisURL := getEnv("REDIS_URL", "redis://:redis_secure_password_change_me@localhost:6379/0")
	jwtSecret := getEnv("JWT_SECRET", "super_secret_jwt_hmac_sha256_minimum_32_characters_key")
	refreshSecret := getEnv("REFRESH_SECRET", "super_secret_refresh_jwt_minimum_32_characters_key")
	googleClientID := getEnv("GOOGLE_CLIENT_ID", "")

	log.Println("[INFO] Initializing PostgreSQL connection pool...")
	pgDB, err := repository.NewPostgresDB(databaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Failed to connect to PostgreSQL: %v", err)
	}
	defer pgDB.Close()

	log.Println("[INFO] Running embedded database migrations...")
	if err := pgDB.RunMigrationsFS(migrations.FS, "."); err != nil {
		log.Fatalf("[FATAL] Failed to run database migrations: %v", err)
	}
	log.Println("[INFO] Database migrations applied successfully.")

	log.Println("[INFO] Initializing Redis connection pool...")
	redisClient, err := repository.NewRedisClient(redisURL)
	if err != nil {
		log.Printf("[WARN] Failed to connect to Redis: %v (continuing with degraded leaderboard caching)", err)
	} else {
		defer redisClient.Close()
		log.Println("[INFO] Redis connection pool active.")
	}

	// Initialize Services & Handlers
	userRepo := repository.NewPostgresUserRepository(pgDB.DB)
	canonicalRepo := repository.NewPostgresCanonicalRepository(pgDB.DB)

	// Seed canonical manhuas if empty
	if err := repository.SeedInitialCanonicalManhuas(context.Background(), canonicalRepo); err != nil {
		log.Printf("[WARN] Failed to seed initial canonical manhuas: %v", err)
	}

	jwtService := auth.NewJWTService(jwtSecret, refreshSecret, 15*time.Minute, 7*24*time.Hour)
	googleVerifier := auth.NewGoogleVerifier(googleClientID)
	authService := auth.NewAuthService(userRepo, jwtService, googleVerifier)
	authHandler := handlers.NewAuthHandler(authService)
	authMiddleware := middleware.NewAuthMiddleware(jwtService)
	catalogHandler := handlers.NewCatalogHandler(canonicalRepo)

	libraryRepo := repository.NewPostgresLibraryRepository(pgDB.DB)
	libraryService := service.NewLibraryService(libraryRepo, canonicalRepo)
	libraryHandler := handlers.NewLibraryHandler(libraryService)

	scraperWorkerURL := getEnv("SCRAPER_WORKER_URL", "http://localhost:50051")
	extractorService := service.NewExtractorService(scraperWorkerURL, nil)

	ssrfGuard := proxy.NewSSRFGuard(proxy.DefaultAllowedDomains)
	streamer := proxy.NewStreamer(ssrfGuard)
	readerHandler := handlers.NewReaderHandler(streamer, canonicalRepo, extractorService)

	batchExtractor := func(ctx context.Context, m *domain.CanonicalManhua, chapter int) ([]domain.BatchChapterImage, error) {
		urls, err := extractorService.ExtractChapterImages(ctx, m, chapter)
		if err != nil {
			log.Printf("[WARN] Batch extraction failed for %s ch %d: %v", m.Slug, chapter, err)
			return nil, err
		}
		res := make([]domain.BatchChapterImage, len(urls))
		for i, u := range urls {
			res[i] = domain.BatchChapterImage{
				Index:    i + 1,
				URL:      u,
				ProxyURL: "/api/reader/proxy-image?url=" + url.QueryEscape(u),
			}
		}
		return res, nil
	}
	batchService := service.NewBatchService(canonicalRepo, 5, batchExtractor)
	batchHandler := handlers.NewBatchHandler(batchService)

	ratingRepo := repository.NewPostgresRatingRepository(pgDB.DB, redisClient)
	rankingService := service.NewRankingService(ratingRepo, canonicalRepo, redisClient)
	rankingHandler := handlers.NewRankingHandler(rankingService)

	notifRepo := repository.NewPostgresNotificationRepository(pgDB.DB)
	sched := scheduler.NewScheduler(pgDB.DB, canonicalRepo, notifRepo, func(ctx context.Context, m *domain.CanonicalManhua) (int, error) {
		current := 0
		if m.LastChapter != nil {
			current = *m.LastChapter
		}
		return current + 1, nil
	})
	notifHandler := handlers.NewNotificationsHandler(notifRepo, sched.RunRoutineCheck)

	providerReg := providers.NewProviderRegistry()
	providerReg.Register(providers.NewAsuraScansScraper())
	providerReg.Register(providers.NewRavenScansScraper())
	providerReg.Register(providers.NewMangaKScraper())
	providerReg.Register(providers.NewManhuaTopScraper())

	importService := service.NewImportService(providerReg, redisClient, canonicalRepo, libraryService)
	importHandler := handlers.NewImportHandler(importService, jwtService)

	if redisClient != nil {
		if err := rankingService.WarmUp(context.Background()); err != nil {
			log.Printf("[WARN] Failed to warm up ranking cache: %v", err)
		}
		rankingService.StartBackgroundSync(context.Background(), 15*time.Minute)
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `"}`))
	})

	// Root welcome / status
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"app":"Manhua Central API","status":"online","version":"1.0.0"}`))
	})

	// Auth Endpoints
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.HandleFunc("/api/auth/google", authHandler.GoogleAuth)
	mux.HandleFunc("/api/auth/refresh", authHandler.Refresh)

	// Catalog Endpoints
	catalogHandler.RegisterRoutes(mux)

	// Library Endpoints (Protected)
	libraryHandler.RegisterRoutes(mux, authMiddleware)

	// Reader Endpoints
	readerHandler.RegisterRoutes(mux)

	// Batch Chapter Download Endpoints
	batchHandler.RegisterRoutes(mux)

	// Ranking & Ratings Endpoints
	rankingHandler.RegisterRoutes(mux, authMiddleware)

	// Notifications & Routine Endpoints
	notifHandler.RegisterRoutes(mux, authMiddleware)

	// Import Endpoints
	importHandler.RegisterRoutes(mux, authMiddleware)

	// User Endpoints (Protected)
	mux.Handle("/api/users/me", authMiddleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.GetCurrentUser(w, r)
		case http.MethodDelete:
			authHandler.DeleteCurrentUser(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})))

	handlerWithCORS := corsMiddleware(mux)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handlerWithCORS,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("[INFO] API Gateway server listening on port :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[INFO] Shutting down API Gateway gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("[ERROR] Server forced to shutdown: %v", err)
	}

	log.Println("[INFO] API Gateway exited cleanly.")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}
