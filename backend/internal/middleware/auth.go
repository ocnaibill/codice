package middleware

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GetJWTSecret returns the signing secret. There is no built-in default in any
// environment: a public fallback key would let anyone forge tokens. It panics
// when JWT_SECRET is unset, so call it once at startup to fail fast.
func GetJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET must be set")
	}
	// The example file ships a placeholder. It is public, so tokens signed with it could
	// be forged by anyone who has read the repository.
	if strings.HasPrefix(secret, "CHANGE_THIS") {
		panic("JWT_SECRET is still the placeholder from .env.example: generate one with `openssl rand -hex 32`")
	}
	return []byte(secret)
}

type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	UserRoleKey  contextKey = "user_role"
	SessionIDKey contextKey = "session_id"
)

// Token kinds (claim "typ"). A resource token can never be used as a bearer
// session token, nor the other way around.
const (
	tokenSession  = "session"
	tokenResource = "resource"
)

// Resource token scopes.
const (
	ScopeAssets = "assets" // GET/HEAD of covers, files and pages, for <img>/<audio> URLs
	ScopeWS     = "ws"     // opening the WebSocket
)

// ErrInvalidCredentials is returned by a BasicVerifier when the username or
// password does not match. Any other error means verification itself failed.
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInvalidSession is returned by a SessionChecker when the session is
// unknown, revoked, expired, or belongs to a blocked account.
var ErrInvalidSession = errors.New("invalid session")

// BasicVerifier checks a username and secret (an app token) and returns the
// account's id and role. It must return ErrInvalidCredentials on mismatch.
type BasicVerifier func(ctx context.Context, username, password string) (id, role string, err error)

// SessionChecker confirms a session is live and returns its account and current
// role. It must return ErrInvalidSession when it is not.
type SessionChecker func(ctx context.Context, sessionID string) (userID, role string, err error)

// Authenticator authenticates requests. There is no anonymous access: without
// a live session (or, where allowed, a valid app token or resource token) every
// request is answered 401, in every APP_ENV.
type Authenticator struct {
	Sessions SessionChecker // required; without it every token is refused
	Basic    BasicVerifier  // optional; used by WithBasic and AssetsWithBasic
}

// Middleware accepts only "Authorization: Bearer <session token>".
func (a Authenticator) Middleware(next http.Handler) http.Handler {
	return a.build(false, false)(next)
}

// WithBasic also accepts HTTP Basic with an app token as the password (OPDS).
func (a Authenticator) WithBasic(next http.Handler) http.Handler {
	return a.build(true, false)(next)
}

// Assets also accepts a short-lived "assets" resource token in ?rt= on GET and
// HEAD, for URLs the browser loads without headers (<img>, <audio>).
func (a Authenticator) Assets(next http.Handler) http.Handler {
	return a.build(false, true)(next)
}

// AssetsWithBasic is Assets plus Basic, for files and covers that OPDS clients
// download.
func (a Authenticator) AssetsWithBasic(next http.Handler) http.Handler {
	return a.build(true, true)(next)
}

func (a Authenticator) build(allowBasic, allowResource bool) func(http.Handler) http.Handler {
	challenge := allowBasic && a.Basic != nil
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")

			if header == "" {
				rt := r.URL.Query().Get("rt")
				if allowResource && rt != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
					sid, ok := parseTokenSession(rt, tokenResource, ScopeAssets)
					a.finish(w, r, next, sid, ok, challenge)
					return
				}
				deny(w, challenge, "Access denied: Authentication required")
				return
			}

			switch {
			case strings.HasPrefix(header, "Basic "):
				if !allowBasic || a.Basic == nil {
					deny(w, challenge, "Access denied: Basic authentication is not accepted here")
					return
				}
				id, role, err := verifyBasic(r.Context(), header, a.Basic)
				if errors.Is(err, ErrInvalidCredentials) {
					deny(w, true, "Access denied: Invalid credentials")
					return
				}
				if err != nil {
					http.Error(w, "Authentication unavailable", http.StatusInternalServerError)
					return
				}
				next.ServeHTTP(w, withIdentity(r, id, role, ""))
			case strings.HasPrefix(header, "Bearer "):
				sid, ok := parseTokenSession(strings.TrimPrefix(header, "Bearer "), tokenSession, "")
				a.finish(w, r, next, sid, ok, challenge)
			default:
				deny(w, challenge, "Access denied: Unsupported authorization method")
			}
		})
	}
}

// finish resolves a parsed token's session against the database and either
// serves the request or answers 401/500.
func (a Authenticator) finish(w http.ResponseWriter, r *http.Request, next http.Handler, sid string, ok, challenge bool) {
	if !ok || a.Sessions == nil {
		deny(w, challenge, "Access denied: Invalid or expired token")
		return
	}
	userID, role, err := a.Sessions(r.Context(), sid)
	if errors.Is(err, ErrInvalidSession) {
		deny(w, challenge, "Access denied: Invalid or expired token")
		return
	}
	if err != nil {
		http.Error(w, "Authentication unavailable", http.StatusInternalServerError)
		return
	}
	next.ServeHTTP(w, withIdentity(r, userID, role, sid))
}

// AuthenticateWS authenticates a WebSocket upgrade: a "ws" resource token in
// ?ticket=, or a session token in the Authorization header (non-browser
// clients). Query strings never carry a session token.
func (a Authenticator) AuthenticateWS(r *http.Request) (userID, role string, err error) {
	var sid string
	var ok bool
	if ticket := r.URL.Query().Get("ticket"); ticket != "" {
		sid, ok = parseTokenSession(ticket, tokenResource, ScopeWS)
	} else if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		sid, ok = parseTokenSession(strings.TrimPrefix(h, "Bearer "), tokenSession, "")
	}
	if !ok || a.Sessions == nil {
		return "", "", ErrInvalidSession
	}
	return a.Sessions(r.Context(), sid)
}

// IssueSessionToken signs the bearer token for a session. It carries no role:
// the role is read from the database on every request.
func IssueSessionToken(sessionID, userID string, expires time.Time) (string, error) {
	return sign(jwt.MapClaims{
		"typ": tokenSession, "sid": sessionID, "sub": userID, "exp": expires.Unix(),
	})
}

// IssueResourceToken signs a short-lived token tied to a session and limited to
// one scope. Revoking the session invalidates it.
func IssueResourceToken(sessionID, userID, scope string, ttl time.Duration) (string, time.Time, error) {
	expires := time.Now().Add(ttl)
	tok, err := sign(jwt.MapClaims{
		"typ": tokenResource, "sid": sessionID, "sub": userID, "scope": scope, "exp": expires.Unix(),
	})
	return tok, expires, err
}

func sign(claims jwt.MapClaims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(GetJWTSecret())
}

// parseTokenSession verifies signature, expiry, kind and (for resource tokens)
// scope, and returns the session id it points to.
func parseTokenSession(tokenString, wantType, wantScope string) (string, bool) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		// SEC-10: Validate algorithm
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return GetJWTSecret(), nil
	}, jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return "", false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	if typ, _ := claims["typ"].(string); typ != wantType {
		return "", false
	}
	if wantScope != "" {
		if scope, _ := claims["scope"].(string); scope != wantScope {
			return "", false
		}
	}
	sid, _ := claims["sid"].(string)
	return sid, sid != ""
}

func verifyBasic(ctx context.Context, authHeader string, verify BasicVerifier) (id, role string, err error) {
	payload, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
	if decodeErr != nil {
		return "", "", ErrInvalidCredentials
	}
	user, pass, found := strings.Cut(string(payload), ":")
	if !found || user == "" || pass == "" {
		return "", "", ErrInvalidCredentials
	}
	return verify(ctx, user, pass)
}

func withIdentity(r *http.Request, id, role, sessionID string) *http.Request {
	ctx := context.WithValue(r.Context(), UserIDKey, id)
	ctx = context.WithValue(ctx, UserRoleKey, role)
	if sessionID != "" {
		ctx = context.WithValue(ctx, SessionIDKey, sessionID)
	}
	return r.WithContext(ctx)
}

// deny writes a 401. When challenge is true it also asks Basic clients to
// prompt for credentials again.
func deny(w http.ResponseWriter, challenge bool, msg string) {
	if challenge {
		w.Header().Set("WWW-Authenticate", `Basic realm="Codice"`)
	}
	http.Error(w, msg, http.StatusUnauthorized)
}
