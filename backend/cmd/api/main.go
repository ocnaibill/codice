package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq" // Underscore initializes the driver anonymously
	"github.com/ocnaibill/codice/backend/internal/backup"
	"github.com/ocnaibill/codice/backend/internal/config"
	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/handlers"
	"github.com/ocnaibill/codice/backend/internal/jobs"
	"github.com/ocnaibill/codice/backend/internal/ldapauth"
	"github.com/ocnaibill/codice/backend/internal/logins"
	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/performance"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"github.com/ocnaibill/codice/backend/internal/storage"
	"github.com/ocnaibill/codice/backend/internal/version"
)

func main() {
	// 0. Load environment variables from .env
	config.Load()

	// Fail fast: the API must not start without a JWT secret (no default exists).
	appMiddleware.GetJWTSecret()

	// 1. Connection with PostgreSQL (PERF-02: connection pooling)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}
	defer db.Close()

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Ping database to verify active connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Database did not respond to ping: %v", err)
	}
	log.Println("✅ Successfully connected to PostgreSQL!")

	// Run automatic database migrations on startup
	if err := database.Migrate(db); err != nil {
		log.Fatalf("❌ Database auto-migrations failed: %v", err)
	}

	// 2. Connection with Redis
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}

	redisClient, err := connectRedis(context.Background(), redisURL)
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()

	// LDAP is optional. A configuration that would be unsafe or unusable stops the
	// API at startup, with the reason, rather than silently disabling sign-in.
	var directory ldapauth.Directory
	var directoryHost, directoryBase string
	if cfg, on := ldapauth.ConfigFromEnv(); on {
		client, err := ldapauth.New(cfg)
		if err != nil {
			log.Fatalf("LDAP configuration: %v", err)
		}
		directory, directoryHost, directoryBase = client, client.Host(), client.BaseDN()
		log.Printf("LDAP sign-in enabled: %s", directoryHost)
	}

	// Behind a reverse proxy (nginx, a tunnel) every client would look like the proxy
	// and share one login rate limit. Naming the proxies lets their X-Forwarded-For be
	// believed, and only theirs.
	trustedProxies, err := appMiddleware.ParseTrustedProxies(os.Getenv("CODICE_TRUSTED_PROXIES"))
	if err != nil {
		log.Fatal(err)
	}
	if len(trustedProxies) > 0 {
		log.Printf("Trusting X-Forwarded-For from %d proxy range(s)", len(trustedProxies))
	}

	// The address people reach the app at, for the links of the OPDS catalog (#80).
	publicURL, err := appMiddleware.ParsePublicURL(os.Getenv("CODICE_PUBLIC_URL"))
	if err != nil {
		log.Fatal(err)
	}
	if publicURL != "" {
		log.Printf("Public address for OPDS links: %s", publicURL)
	}

	sourceURL, err := version.SourceURL(os.Getenv("CODICE_SOURCE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	sessionStore := &sessions.Store{DB: db}
	loginRecord := &logins.Recorder{DB: db}
	authenticator := appMiddleware.Authenticator{
		Sessions:       sessionStore.CheckSession,
		Basic:          sessionStore.VerifyAppToken,
		OnBasicFailure: handlers.BasicFailureRecorder(db, loginRecord),
	}
	startLoginPurge(context.Background(), loginRecord)

	wsHandler := &handlers.WsHandler{RedisClient: redisClient, Auth: authenticator}

	// Start Redis PubSub listener in background goroutine
	go wsHandler.ListenToRedis()

	// Define base storage directory (fallback to ./uploads).
	// Try to resolve relative paths from the project root by walking up from CWD.
	storagePath := os.Getenv("CODICE_STORAGE_PATH")
	if storagePath == "" {
		storagePath = "./uploads"
	}

	// If the path is relative, resolve it relative to the project root.
	// We find the project root by looking for a known marker file/dir.
	if !filepath.IsAbs(storagePath) {
		cwd, _ := os.Getwd()

		// Walk up from CWD looking for the backend/ directory or go.mod
		// The project root is the parent of backend/
		candidate := cwd
		for i := 0; i < 5; i++ {
			// Check if we're in the backend directory
			if filepath.Base(candidate) == "backend" {
				candidate = filepath.Dir(candidate) // go up to project root
				break
			}
			// Check if go.mod exists (project root)
			if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(candidate)
			if parent == candidate {
				break // reached root, stop
			}
			candidate = parent
		}
		storagePath = filepath.Join(candidate, storagePath)
	}

	log.Printf("📂 Storage path resolved to: %s", storagePath)

	// File jobs (organizing, and later scanning and transferring) run in this
	// process; the Python worker only takes ingestion.
	mover := &storage.Mover{DB: db, Root: storagePath}
	backupPanel := backup.Panel{
		DB: db, DatabaseURL: os.Getenv("DATABASE_URL"), StorageRoot: storagePath,
		Dir: os.Getenv("CODICE_BACKUP_DIR"), PassphraseFile: os.Getenv("CODICE_BACKUP_PASSPHRASE_FILE"),
	}
	if warning, err := backupPanel.Validate(); err != nil {
		log.Fatal(err)
	} else if warning != "" {
		log.Print(warning)
	}
	exportDir := os.Getenv("CODICE_EXPORT_DIR")
	if exportDir == "" {
		exportDir = filepath.Join(os.TempDir(), "codice-exports")
	}
	dataExport := &handlers.DataExportHandler{DB: db, Dir: exportDir}
	startExportPurge(context.Background(), dataExport)
	fileHandlers := fileJobHandlers(db, mover, backupPanel, dataExport)
	// The comparison of possible duplicates is the heavy one, and it only reads and proposes: it has a runner of its own, with
	// the number the owner chose (see below), so that it neither makes the backup and the organizing wait nor is held to
	// one at a time. Everything else that touches the files stays one at a time, as it always was.
	types := make([]string, 0, len(fileHandlers))
	for t := range fileHandlers {
		if t != jobDedupe {
			types = append(types, t)
		}
	}
	startFileJobs(context.Background(), &jobs.Runner{
		DB: db, Owner: jobs.NewOwnerName("api"), Types: types, MaxRunning: 1, LeaseSeconds: 300, Handlers: fileHandlers,
	}, mover, &storage.Trash{DB: db, Root: storagePath})

	startIdentitySweep(context.Background(), db, directory)

	catalogLimit, err := appMiddleware.ParseCatalogConcurrency(os.Getenv("CODICE_CATALOG_CONCURRENCY"))
	if err != nil {
		log.Fatal(err)
	}
	if catalogLimit > 0 {
		log.Printf("Heavy reads of the catalog: at most %d at once", catalogLimit)
	}

	// What the owner tunes from the administration (internal/performance): it starts with what is stored and follows each
	// change at once, without a restart.
	machine := performance.ThisMachine()
	tuning := performance.NewLive(performance.Defaults(catalogLimit))
	if overrides, err := performance.Overrides(context.Background(), db, machine); err != nil {
		log.Printf("performance: could not read what the owner chose (using the defaults): %v", err)
	} else {
		tuning.Apply(overrides)
	}
	catalogGate := appMiddleware.NewGate(0, 0, 8*time.Second)
	tuning.OnChange(func(s performance.Settings) {
		limit := s[performance.CatalogReads]
		if catalogLimit == 0 && !tuning.Overridden(performance.CatalogReads) {
			limit = 0 // the installation turned the limit off and the owner has not chosen one
		}
		catalogGate.SetLimit(limit)
	})
	dedupeRunner := &jobs.Runner{
		DB: db, Owner: jobs.NewOwnerName("api-dedupe"), Types: []string{jobDedupe}, LeaseSeconds: 300, Handlers: fileHandlers,
		MaxRunningFn: func() int { return tuning.Get(performance.DedupeJobs) },
	}
	go dedupeRunner.LoopPool(context.Background(), 5*time.Second, func() int { return tuning.Get(performance.DedupeJobs) })

	// 4. Configure Router
	r := newRouter(routerDeps{
		CatalogGate: catalogGate.Middleware(),
		Performance: &handlers.PerformanceHandler{DB: db, Defaults: performance.Defaults(catalogLimit), Machine: machine, Live: tuning},
		DB:          db,
		RedisClient: redisClient,
		Sessions:    sessionStore,
		Auth:        authenticator,
		WS:          wsHandler,
		StoragePath: storagePath,
		Mover:       mover,

		TrustedProxies: trustedProxies,
		PublicURL:      publicURL,
		Logins:         loginRecord,
		DataExport:     dataExport,
		BackupPanel:    backupPanel,
		Version:        version.Version,
		SourceURL:      sourceURL,
		Directory:      directory,
		DirectoryHost:  directoryHost,
		DirectoryBase:  directoryBase,
	})

	// 5. Start HTTP Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Go server running on port %s", port)
	// No ReadTimeout or WriteTimeout: uploads, downloads and the websocket are long on
	// purpose. The header timeout is what stops a client that opens connections and
	// dribbles a request line, one byte at a time, to hold them open.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	log.Fatalf("HTTP server stopped: %v", srv.ListenAndServe())
}
