package main

import (
	"database/sql"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/ocnaibill/codice/backend/internal/backup"
	"github.com/ocnaibill/codice/backend/internal/handlers"
	"github.com/ocnaibill/codice/backend/internal/ldapauth"
	"github.com/ocnaibill/codice/backend/internal/logins"
	appMiddleware "github.com/ocnaibill/codice/backend/internal/middleware"
	"github.com/ocnaibill/codice/backend/internal/sessions"
	"github.com/ocnaibill/codice/backend/internal/storage"
	"github.com/redis/go-redis/v9"
)

type routerDeps struct {
	DB          *sql.DB
	RedisClient *redis.Client
	Sessions    *sessions.Store
	Auth        appMiddleware.Authenticator
	WS          *handlers.WsHandler
	StoragePath string
	Mover       *storage.Mover
	// TrustedProxies are the reverse proxies whose X-Forwarded-For is believed when
	// telling clients apart for rate limits (CODICE_TRUSTED_PROXIES).
	TrustedProxies []netip.Prefix
	// PublicURL is CODICE_PUBLIC_URL, already checked, or empty.
	PublicURL string
	// Version and SourceURL are what the "Sobre" screen shows (see internal/version).
	Version, SourceURL string
	// Logins is the record of sign-ins; nil records nothing.
	Logins *logins.Recorder
	// DataExport is "Exportar meus dados".
	DataExport *handlers.DataExportHandler
	// BackupPanel is what the owner's backup buttons run; the zero value is "not set up".
	BackupPanel backup.Panel
	// Directory is the LDAP directory, nil when it is not configured.
	Directory     ldapauth.Directory
	DirectoryHost string
	DirectoryBase string
	// CatalogGate limits how many heavy reads of the catalog (the list, the counters, the search, the favorites) run at once;
	// nil is no limit. See appMiddleware.Gate.
	CatalogGate func(http.Handler) http.Handler
	// Performance is the owner's tuning (the "Desempenho" part of the System tab).
	Performance *handlers.PerformanceHandler
	// Optional overrides, so route tests can run without a database.
	FileLookup  handlers.FileLookup
	CoverLookup handlers.FileLookup
}

// newRouter wires every HTTP route. Keeping it separate from main lets tests
// check authorization on the real route table without a database.
func newRouter(d routerDeps) http.Handler {
	db := d.DB

	libHandler := &handlers.LibraryHandler{DB: db, Trash: &storage.Trash{DB: db, Root: d.StoragePath}}
	uploadHandler := &handlers.UploadHandler{DB: db, RedisClient: d.RedisClient}
	healthHandler := &handlers.HealthHandler{DB: db, Redis: d.RedisClient}
	sessionsHandler := &handlers.SessionsHandler{DB: db, Sessions: d.Sessions}
	authHandler := &handlers.AuthHandler{DB: db, Sessions: d.Sessions, Directory: d.Directory, Logins: d.Logins}
	loginsAdmin := &handlers.LoginsAdminHandler{DB: db, Logins: d.Logins}
	dataExport := d.DataExport
	backupAdmin := &handlers.BackupAdminHandler{DB: db, StoragePath: d.StoragePath, Panel: d.BackupPanel}
	ldapAdmin := &handlers.LDAPAdminHandler{DB: db, Directory: d.Directory, Host: d.DirectoryHost, BaseDN: d.DirectoryBase}
	appTokensHandler := &handlers.AppTokensHandler{Sessions: d.Sessions}
	invitesHandler := &handlers.InvitationsHandler{DB: db, Sessions: d.Sessions, Logins: d.Logins}
	resetsHandler := &handlers.PasswordResetsHandler{DB: db, Disconnect: d.WS.DisconnectUser}
	ownershipHandler := &handlers.OwnershipHandler{DB: db}
	usersHandler := &handlers.UsersHandler{DB: db, Disconnect: d.WS.DisconnectUser}
	jobsHandler := &handlers.JobsHandler{DB: db, RedisClient: d.RedisClient, StoragePath: d.StoragePath}
	dupesHandler := &handlers.DuplicatesHandler{DB: db}
	versionsHandler := &handlers.VersionsHandler{DB: db}
	peopleHandler := &handlers.PeopleHandler{DB: db}
	providersHandler := &handlers.ProvidersHandler{DB: db}
	trashHandler := &handlers.TrashHandler{Trash: &storage.Trash{DB: db, Root: d.StoragePath}}
	storageHandler := &handlers.StorageHandler{Mover: d.Mover, DB: db, StoragePath: d.StoragePath}
	favoritesHandler := &handlers.FavoritesHandler{DB: db}
	progressHandler := &handlers.ProgressHandler{DB: db}
	searchHandler := &handlers.SearchHandler{DB: db}
	equivalenceHandler := &handlers.EquivalenceHandler{DB: db}
	embeddingsAdmin := &handlers.EmbeddingsAdminHandler{DB: db}
	dictionaryAdmin := &handlers.DictionaryAdminHandler{DB: db, RedisClient: d.RedisClient}
	dictionaryLookup := &handlers.DictionaryLookupHandler{DB: db}
	ocrAdmin := &handlers.OCRHandler{DB: db}
	ocrSettings := &handlers.OCRSettingsHandler{DB: db}
	notesHandler := &handlers.NotesHandler{DB: db}
	graphHandler := &handlers.GraphHandler{DB: db}
	statsHandler := &handlers.StatsHandler{DB: db}

	r := chi.NewRouter()
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(appMiddleware.WithClientIP(d.TrustedProxies))
	r.Use(appMiddleware.LimitBody(appMiddleware.MaxJSONBody))

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
	authRateLimit := authRateLimiter(d.TrustedProxies, d.Logins)

	// Public Auth & Setup Endpoints
	// Public, and answers only ok/down per component: for containers and load balancers.
	r.Get("/healthz", healthHandler.Get)

	// A question the app asks at every load, and that tells nothing a person could use to guess a password: it has a limit of
	// its own, so that reloading the page never uses up the attempts of the sign-in.
	r.With(statusRateLimiter(d.TrustedProxies)).Get("/auth/setup-status", authHandler.GetSetupStatus)
	r.With(authRateLimit).Post("/auth/setup", authHandler.SetupMasterAdmin)
	r.With(authRateLimit).Post("/auth/register", authHandler.Register)
	r.With(authRateLimit).Post("/auth/login", authHandler.Login)
	r.With(authRateLimit).Post("/auth/link", authHandler.Link)
	// Invitations are the way in while public registration is off (DEC-050).
	r.With(authRateLimit).Get("/auth/invitation", invitesHandler.Check)
	r.With(authRateLimit).Post("/auth/redeem", invitesHandler.Redeem)
	// Forgotten password without e-mail (DEC-063): ask, get approved, use the link.
	r.With(authRateLimit).Post("/auth/reset-request", resetsHandler.Request)
	r.With(authRateLimit).Get("/auth/reset", resetsHandler.Check)
	r.With(authRateLimit).Post("/auth/reset", resetsHandler.Redeem)

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

	// The heavy reads of the catalog go through the gate before the authentication, which reads the database itself.
	catalog := d.CatalogGate
	if catalog == nil {
		catalog = func(next http.Handler) http.Handler { return next }
	}

	// Protected Application Endpoints (any authenticated user)
	r.With(catalog, auth).Get("/works", libHandler.GetWorks)
	r.With(auth).Get("/works/{id}", libHandler.GetWorkByID)
	r.With(catalog, auth).Get("/search", searchHandler.Search)
	r.With(auth).Patch("/works/{id}/progress", libHandler.UpdateProgress)
	r.With(auth).Get("/progress/files/{id}", progressHandler.Get)
	r.With(auth).Put("/progress/files/{id}", progressHandler.Put)
	r.With(auth).Put("/progress/files/{id}/completion", progressHandler.SetCompletion)
	r.With(auth).Post("/progress/files/{id}/opened", progressHandler.Opened)
	r.With(auth).Put("/progress/works/{id}/finished", progressHandler.SetWorkFinished)
	r.With(auth).Get("/progress/files/{id}/equivalent", equivalenceHandler.Find)
	r.With(auth).Post("/progress/files/{id}/equivalent/accept", equivalenceHandler.Accept)
	r.With(auth).Post("/works/{id}/reading-heartbeat", libHandler.ReadingHeartbeat)

	// Catalog administration
	r.With(staff).Put("/works/{id}", libHandler.UpdateWork)
	r.With(staff).Delete("/works/{id}", libHandler.DeleteWork)
	r.With(staff).Post("/works/{id}/restore", libHandler.RestoreWork)
	r.With(staff).Get("/admin/suggestions", libHandler.SuggestionQueue)
	r.With(staff).Get("/works/{id}/candidates", libHandler.ListCandidates)
	r.With(staff).Post("/works/{id}/candidates/{candidateID}/accept", libHandler.AcceptCandidate)
	r.With(staff).Post("/works/{id}/candidates/{candidateID}/reject", libHandler.RejectCandidate)
	r.With(staff).Post("/upload", uploadHandler.HandleUpload)
	r.With(staff).Post("/works/bulk-import", uploadHandler.HandleBulkImport)

	// Session, resource tokens and app tokens
	r.With(auth).Get("/about", (&handlers.AboutHandler{Version: d.Version, SourceURL: d.SourceURL}).Get)
	r.With(auth).Get("/auth/logins", loginsAdmin.Own)
	r.With(auth).Get("/auth/export", dataExport.Latest)
	r.With(auth).Post("/auth/export", dataExport.Request)
	r.With(auth).Get("/auth/export/{id}/download", dataExport.Download)
	r.With(auth).Delete("/auth/export/{id}", dataExport.Delete)
	r.With(auth).Get("/auth/sessions", sessionsHandler.List)
	r.With(auth).Post("/auth/sessions/revoke-others", sessionsHandler.RevokeOthers)
	r.With(auth).Delete("/auth/sessions/{id}", sessionsHandler.Revoke)
	r.With(auth).Get("/auth/me", authHandler.Me)
	r.With(auth, authRateLimit).Post("/auth/password", authHandler.ChangePassword)
	r.With(auth).Post("/auth/notices/{id}/ack", handlers.AckNotice(db))

	// Ownership changes hands in two steps (RF-038). Recovery when the owner is
	// unavailable is not a route: it is a command run on the server (DEC-057).
	r.With(auth).Get("/ownership/transfer", ownershipHandler.Get)
	r.With(owner, authRateLimit).Post("/ownership/transfer", ownershipHandler.Start)
	r.With(owner).Delete("/ownership/transfer", ownershipHandler.Cancel)
	r.With(auth, authRateLimit).Post("/ownership/transfer/accept", ownershipHandler.Accept)
	r.With(auth).Post("/ownership/transfer/decline", ownershipHandler.Decline)
	r.With(auth).Post("/auth/logout", authHandler.Logout)
	r.With(auth).Post("/auth/resource-token", authHandler.ResourceToken)
	r.With(auth).Post("/auth/app-tokens", appTokensHandler.Create)
	r.With(auth).Get("/auth/app-tokens", appTokensHandler.List)
	r.With(auth).Delete("/auth/app-tokens/{id}", appTokensHandler.Revoke)

	// Jobs administration (RF-020)
	r.With(staff).Get("/admin/jobs", jobsHandler.List)
	r.With(staff).Post("/admin/jobs/rerun-failed", jobsHandler.RerunFailed)
	r.With(staff).Post("/admin/jobs/{id}/rerun", jobsHandler.Rerun)
	r.With(staff).Post("/admin/jobs/{id}/cancel", jobsHandler.Cancel)
	r.With(staff).Post("/admin/works/{id}/extract-text", jobsHandler.ExtractText)

	// Storage administration (RF-044): preview, then confirm that exact preview
	r.With(staff).Get("/admin/storage/reorganize", storageHandler.PreviewReorganize)
	r.With(staff).Post("/admin/storage/reorganize", storageHandler.Reorganize)

	// Referenced library: only the owner authorises directories (DEC-035); owner
	// and admin catalogue inside them.
	r.With(staff).Get("/admin/storage/roots", storageHandler.ListRoots)
	r.With(owner).Post("/admin/storage/roots", storageHandler.AddRoot)
	r.With(owner).Delete("/admin/storage/roots/{id}", storageHandler.RemoveRoot)
	r.With(staff).Get("/admin/storage/referenced", storageHandler.ListReferenced)
	r.With(staff).Get("/admin/storage/transfers", storageHandler.Transfers)
	r.With(staff).Post("/admin/library/scan", storageHandler.Scan)
	r.With(staff).Post("/admin/library/move-to-managed", storageHandler.MoveToManaged)
	r.With(staff).Get("/admin/storage/cleanups", storageHandler.ListCleanups)
	r.With(staff).Post("/admin/storage/cleanups/retry", storageHandler.RetryCleanups)

	// Trash (RF-045): staff can look, restore and empty; only the owner sets the
	// automatic cleanup. Anything that destroys bytes asks for confirm=true.
	r.With(staff).Get("/admin/trash", trashHandler.List)
	r.With(staff).Post("/admin/trash/{id}/restore", trashHandler.Restore)
	r.With(staff).Delete("/admin/trash/{id}", trashHandler.Delete)
	r.With(staff).Post("/admin/trash/empty", trashHandler.Empty)
	r.With(staff).Get("/admin/trash/policy", trashHandler.GetPolicy)
	r.With(owner).Put("/admin/trash/policy", trashHandler.SetPolicy)
	r.With(owner).Get("/admin/trash/policy/preview", trashHandler.PreviewPolicy)
	r.With(owner).Post("/admin/trash/policy/apply", trashHandler.ApplyPolicy)
	r.With(staff).Get("/admin/logins", loginsAdmin.List)
	r.With(owner).Put("/admin/logins/settings", loginsAdmin.SetRetention)
	r.With(staff).Get("/admin/backup", backupAdmin.Get)
	// The owner makes and checks a package from the panel (DEC-123): the password is asked again, so the
	// attempts have the limit of the sign-in. Restoring and downloading stay with codice-admin.
	r.With(owner, authRateLimit).Post("/admin/backup/run", backupAdmin.Run)
	r.With(owner, authRateLimit).Post("/admin/backup/verify", backupAdmin.Verify)
	r.With(staff).Get("/admin/storage/orphans", trashHandler.Orphans)
	r.With(staff).Post("/admin/storage/orphans/trash", trashHandler.TrashOrphans)

	// Possible duplicates (DEC-029): the system proposes, an admin decides.
	r.With(staff).Get("/admin/duplicates", dupesHandler.List)
	r.With(staff).Post("/admin/duplicates/scan", dupesHandler.Scan)
	r.With(staff).Post("/admin/duplicates/{id}/dismiss", dupesHandler.Dismiss)
	r.With(staff).Post("/admin/duplicates/{id}/link", dupesHandler.Link)
	// Putting the files of one book under one work by hand, and taking them out again (#37).
	r.With(staff).Post("/admin/works/{id}/join", versionsHandler.Join)
	r.With(staff).Post("/admin/works/{id}/not-same-as", versionsHandler.NotTheSame)
	r.With(staff).Post("/admin/editions/{id}/split", versionsHandler.Split)
	// People who may be the same person: the system proposes, an admin decides (#36).
	r.With(staff).Get("/admin/people/merges", peopleHandler.List)
	r.With(staff).Post("/admin/people/merges/{id}/merge", peopleHandler.Merge)
	r.With(staff).Post("/admin/people/merges/{id}/dismiss", peopleHandler.Dismiss)
	r.With(staff).Put("/admin/people/{id}/name", peopleHandler.SetName)
	// How names are shown (#64): each account chooses, the owner sets the library's default.
	r.With(auth).Get("/auth/preferences", peopleHandler.GetPreferences)
	r.With(auth).Put("/auth/preferences", peopleHandler.SetPreferences)
	r.With(owner).Put("/admin/name-order", peopleHandler.SetLibraryOrder)
	r.With(staff).Get("/admin/metadata-providers", providersHandler.List)
	r.With(owner).Put("/admin/metadata-providers/{id}", providersHandler.Set)

	// Accounts: owner and admins list and block (the policy decides who may act on
	// whom); only the owner changes roles.
	r.With(staff).Get("/users", usersHandler.List)
	r.With(staff).Get("/users/{id}/sessions", sessionsHandler.ListFor)
	r.With(staff).Delete("/users/{id}/sessions", sessionsHandler.RevokeAllFor)
	r.With(staff).Delete("/users/{id}/sessions/{sid}", sessionsHandler.RevokeFor)
	r.With(staff).Post("/users/{id}/block", usersHandler.Block)
	r.With(staff).Post("/users/{id}/unblock", usersHandler.Unblock)
	r.With(staff).Delete("/users/{id}", usersHandler.Delete)
	r.With(owner).Put("/users/{id}/role", usersHandler.UpdateRole)
	r.With(owner).Get("/admin/ldap", ldapAdmin.Get)
	r.With(owner).Put("/admin/ldap/policy", ldapAdmin.SetPolicy)
	r.With(owner).Post("/admin/ldap/check", ldapAdmin.Check)
	// Dictionaries (#109, DEC-115): third-party data. The staff sees which there are; only the owner installs, cancels or
	// removes one (installing makes the server download it).
	// Whoever reads can look a word up (what was installed is the owner's choice).
	r.With(auth).Get("/dictionary", dictionaryLookup.Lookup)
	r.With(auth).Get("/dictionary/languages", dictionaryLookup.Languages)
	r.With(auth).Get("/dictionary/others", dictionaryLookup.Others)
	r.With(staff).Get("/admin/dictionaries", dictionaryAdmin.List)
	r.With(owner).Post("/admin/dictionaries/{id}/install", dictionaryAdmin.Install)
	r.With(owner).Post("/admin/dictionaries/{id}/cancel", dictionaryAdmin.Cancel)
	r.With(owner).Delete("/admin/dictionaries/{id}", dictionaryAdmin.Remove)
	r.With(owner).Get("/admin/embeddings", embeddingsAdmin.Get)
	r.With(owner).Put("/admin/embeddings", embeddingsAdmin.Set)
	r.With(staff).Get("/admin/ocr", ocrAdmin.List)
	r.With(staff).Post("/admin/works/{id}/ocr/retry", ocrAdmin.Retry)
	r.With(staff).Post("/admin/files/{fileId}/ocr/language", ocrAdmin.SetLanguage)
	r.With(owner).Get("/admin/ocr/settings", ocrSettings.Get)
	r.With(owner).Put("/admin/ocr/settings", ocrSettings.Set)
	performanceHandler := d.Performance
	if performanceHandler == nil {
		performanceHandler = &handlers.PerformanceHandler{DB: db}
	}
	r.With(staff).Get("/admin/performance", performanceHandler.Get)
	r.With(owner).Put("/admin/performance", performanceHandler.Set)
	r.With(staff).Get("/invitations", invitesHandler.List)
	r.With(staff).Post("/invitations", invitesHandler.Create)
	r.With(staff).Delete("/invitations/{id}", invitesHandler.Revoke)
	r.With(staff).Get("/password-resets", resetsHandler.List)
	r.With(staff).Post("/password-resets/{id}/approve", resetsHandler.Approve)
	r.With(staff).Post("/password-resets/{id}/reject", resetsHandler.Reject)

	// Favorites, notes/quotes, and dashboard stats
	r.With(auth).Post("/works/{id}/favorite", favoritesHandler.AddFavorite)
	r.With(auth).Delete("/works/{id}/favorite", favoritesHandler.RemoveFavorite)
	r.With(catalog, auth).Get("/favorites", favoritesHandler.GetFavorites)
	r.With(auth).Post("/works/{id}/notes", notesHandler.CreateNote)
	r.With(auth).Get("/notes", notesHandler.ListNotes)
	r.With(auth).Get("/notes/facets", notesHandler.NoteFacets)
	r.With(auth).Get("/notes/export", notesHandler.ExportNotes)
	r.With(auth).Patch("/notes/{id}", notesHandler.UpdateNote)
	r.With(auth).Delete("/notes/{id}", notesHandler.DeleteNote)

	// The manual graph: a person's concepts and the relations they draw (DEC-109). All personal.
	r.With(auth).Get("/graph/types", graphHandler.Types)
	r.With(auth).Get("/concepts", graphHandler.ListConcepts)
	r.With(auth).Post("/concepts", graphHandler.CreateConcept)
	r.With(auth).Get("/concepts/resolve", graphHandler.ResolveConcept)
	r.With(auth).Get("/concepts/{id}", graphHandler.GetConcept)
	r.With(auth).Patch("/concepts/{id}", graphHandler.UpdateConcept)
	r.With(auth).Delete("/concepts/{id}", graphHandler.DeleteConcept)
	r.With(auth).Get("/relations", graphHandler.ListRelations)
	r.With(auth).Post("/relations", graphHandler.CreateRelation)
	r.With(auth).Patch("/relations/{id}", graphHandler.UpdateRelation)
	r.With(auth).Delete("/relations/{id}", graphHandler.DeleteRelation)
	r.With(catalog, auth).Get("/stats", statsHandler.GetStats)

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
	opdsHandler := &handlers.OPDSHandler{DB: db, Auth: d.Auth, PublicURL: d.PublicURL, TrustedProxies: d.TrustedProxies}
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

	// Files are served only when the catalog owns the path, and a retired work's
	// files only to owner and admin. Covers cache for a week (PERF-04); original
	// files are never cached (they can be large and can be removed).
	fileLookup, coverLookup := d.FileLookup, d.CoverLookup
	if fileLookup == nil {
		fileLookup = handlers.NewFileLookup(db)
	}
	if coverLookup == nil {
		coverLookup = handlers.NewCoverLookup(db)
	}
	// The stand-in for a work with no cover. It is part of the program, not of the library:
	// a fresh installation has no such file on disk.
	r.With(authWithBasic).Get("/covers/placeholder.svg", handlers.CoverPlaceholder)
	r.With(authWithBasic).Method("GET", "/covers/*", &handlers.FilesHandler{
		Root: coversPath, Lookup: coverLookup,
		CacheHeader: "public, max-age=604800, must-revalidate",
	})
	r.With(authWithBasic).Method("GET", "/file/{id}", &handlers.FileByIDHandler{DB: db, StorageRoot: d.StoragePath})
	r.With(authWithBasic).Method("GET", "/files/*", &handlers.FilesHandler{
		Root: d.StoragePath, Lookup: fileLookup,
		CacheHeader: "no-cache, no-store, must-revalidate",
	})

	return r
}

// authRateLimiter is the limit on login and the other unauthenticated auth calls
// (SEC-09): 10 requests per minute per client. "Client" is the connection's address
// unless the connection comes from a trusted proxy, in which case it is the address
// that proxy reports (see middleware.ClientIP).
// StatusRequestsPerMinute is how many times a client may ask whether the server has been set up: the app asks at every
// load (more than once), so it is far above the attempts of a sign-in, and far below what would let it be used to load the
// server.
const StatusRequestsPerMinute = 120

// statusRateLimiter limits GET /auth/setup-status apart from the sign-in, per client like it.
func statusRateLimiter(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return httprate.LimitBy(StatusRequestsPerMinute, 1*time.Minute, appMiddleware.RateLimitKey(trusted))
}

func authRateLimiter(trusted []netip.Prefix, rec *logins.Recorder) func(http.Handler) http.Handler {
	return httprate.LimitBy(10, 1*time.Minute, appMiddleware.RateLimitKey(trusted),
		// A refused request still goes into the record of sign-ins (DEC-121): someone being stopped is exactly what the
		// owner wants to see. Same answer as before.
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			rec.Record(r.Context(), logins.Event{Result: logins.RateLimited, Method: logins.Local, IP: appMiddleware.RequestClientIP(r), UserAgent: r.UserAgent()})
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		}))
}
