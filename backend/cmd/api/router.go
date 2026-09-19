package main

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/ocnaibill/codice/backend/internal/handlers"
	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"github.com/redis/go-redis/v9"
)

type routerDeps struct {
	DB          *sql.DB
	RedisClient *redis.Client
	Sessions    *sessions.Store
	Auth        appMiddleware.Authenticator
	WS          *handlers.WsHandler
	StoragePath string
}

// newRouter wires every HTTP route. Keeping it separate from main lets tests
// check authorization on the real route table without a database.
func newRouter(d routerDeps) http.Handler {
	db := d.DB

	libHandler := &handlers.LibraryHandler{DB: db}
	uploadHandler := &handlers.UploadHandler{DB: db, RedisClient: d.RedisClient}
	authHandler := &handlers.AuthHandler{DB: db, Sessions: d.Sessions}
	appTokensHandler := &handlers.AppTokensHandler{Sessions: d.Sessions}
	usersHandler := &handlers.UsersHandler{DB: db}
	favoritesHandler := &handlers.FavoritesHandler{DB: db}
	notesHandler := &handlers.NotesHandler{DB: db}
	statsHandler := &handlers.StatsHandler{DB: db}

	r := chi.NewRouter()
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)

	// PERF-03: Gzip compression middleware
	r.Use(chiMiddleware.Compress(5, "text/html", "text/css", "text/javascript", "application/json", "application/javascript", "image/svg+xml"))

	allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:5173"
	}

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{allowedOrigin},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("📚 Códice API is online!"))
	})

	// Rate limiting for auth endpoints (SEC-09): 10 requests per minute per IP
	authRateLimit := httprate.LimitByIP(10, 1*time.Minute)

	// Public Auth & Setup Endpoints
	r.With(authRateLimit).Get("/auth/setup-status", authHandler.GetSetupStatus)
	r.With(authRateLimit).Post("/auth/setup", authHandler.SetupMasterAdmin)
	r.With(authRateLimit).Post("/auth/register", authHandler.Register)
	r.With(authRateLimit).Post("/auth/login", authHandler.Login)

	// Session-backed authentication: a bearer token is only good while its
	// session row is live and the account is not blocked.
	auth := d.Auth.Middleware
	assets := d.Auth.Assets // also accepts a short-lived ?rt= token, GET/HEAD only
	// Catalog administration: owner and admin only (DEC-003, RF-007).
	staff := func(next http.Handler) http.Handler {
		return auth(appMiddleware.RequireStaff(next))
	}
	// Instance-level actions: owner only (DEC-056).
	owner := func(next http.Handler) http.Handler {
		return auth(appMiddleware.RequireOwner(next))
	}

	// Protected Application Endpoints (any authenticated user)
	r.With(auth).Get("/works", libHandler.GetWorks)
	r.With(auth).Get("/works/{id}", libHandler.GetWorkByID)
	r.With(auth).Patch("/works/{id}/progress", libHandler.UpdateProgress)
	r.With(auth).Post("/works/{id}/reading-heartbeat", libHandler.ReadingHeartbeat)

	// Catalog administration
	r.With(staff).Put("/works/{id}", libHandler.UpdateWork)
	r.With(staff).Delete("/works/{id}", libHandler.DeleteWork)
	r.With(staff).Post("/works/{id}/restore", libHandler.RestoreWork)
	r.With(staff).Post("/upload", uploadHandler.HandleUpload)
	r.With(staff).Post("/works/bulk-import", uploadHandler.HandleBulkImport)

	// Session, resource tokens and app tokens
	r.With(auth).Post("/auth/logout", authHandler.Logout)
	r.With(auth).Post("/auth/resource-token", authHandler.ResourceToken)
	r.With(auth).Post("/auth/app-tokens", appTokensHandler.Create)
	r.With(auth).Get("/auth/app-tokens", appTokensHandler.List)
	r.With(auth).Delete("/auth/app-tokens/{id}", appTokensHandler.Revoke)

	// Account roles
	r.With(owner).Put("/users/{id}/role", usersHandler.UpdateRole)

	// Favorites, notes/quotes, and dashboard stats
	r.With(auth).Post("/works/{id}/favorite", favoritesHandler.AddFavorite)
	r.With(auth).Delete("/works/{id}/favorite", favoritesHandler.RemoveFavorite)
	r.With(auth).Get("/favorites", favoritesHandler.GetFavorites)
	r.With(auth).Post("/works/{id}/notes", notesHandler.CreateNote)
	r.With(auth).Get("/notes", notesHandler.ListNotes)
	r.With(auth).Delete("/notes/{id}", notesHandler.DeleteNote)
	r.With(auth).Get("/stats", statsHandler.GetStats)

	// Page streaming endpoints (CBZ/CBR)
	pageHandler := &handlers.PageHandler{DB: db}
	r.With(assets).Get("/works/{id}/pages", pageHandler.GetPages)
	r.With(assets).Get("/works/{id}/pages/{page}", pageHandler.ServePage)
	r.With(assets).Get("/works/{id}/pages/{page}/thumbnail", pageHandler.ServePageThumbnail)

	// Text file serving (TXT, MD)
	mediaHandler := &handlers.MediaHandler{DB: db}
	r.With(assets).Get("/works/{id}/text", mediaHandler.ServeText)
	r.With(assets).Get("/works/{id}/audio", mediaHandler.ServeAudio)

	// OPDS 1.2 Catalog (Basic Auth for mobile apps like KOReader, Moon+ Reader)
	opdsHandler := &handlers.OPDSHandler{DB: db, Auth: d.Auth}
	r.With(opdsHandler.OpdsAuth).Get("/opds/v1.2/catalog", opdsHandler.RootCatalog)
	r.With(opdsHandler.OpdsAuth).Get("/opds/v1.2/recent", opdsHandler.RecentFeed)
	r.With(opdsHandler.OpdsAuth).Get("/opds/v1.2/search", opdsHandler.SearchFeed)

	// Metadata search (proxies to worker HTTP server)
	r.With(auth).Get("/metadata/search", libHandler.SearchMetadata)

	// WebSocket (auth handled inside handler for upgrade)
	r.Get("/ws", d.WS.HandleWS)

	// Ensure covers directory exists
	coversPath := filepath.Join(d.StoragePath, "covers")
	os.MkdirAll(coversPath, 0755)

	// Covers and files also accept an app token over Basic (OPDS clients cannot
	// send a bearer token) and the short-lived ?rt= token used by <img>/<a>.
	authWithBasic := d.Auth.AssetsWithBasic

	// Helper to serve static files with Cache-Control headers (PERF-04)
	fsCovers := http.StripPrefix("/covers/", http.FileServer(http.Dir(coversPath)))
	r.With(authWithBasic).Get("/covers/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cache images for 7 days, revalidate
		w.Header().Set("Cache-Control", "public, max-age=604800, must-revalidate")
		if strings.HasSuffix(r.URL.Path, ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		fsCovers.ServeHTTP(w, r)
	}))

	fsFiles := http.StripPrefix("/files/", http.FileServer(http.Dir(d.StoragePath)))
	r.With(authWithBasic).Get("/files/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do not cache original files (could be large, user might delete)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		fsFiles.ServeHTTP(w, r)
	}))

	return r
}
