package middleware

import (
	"mime"
	"net/http"
)

// MaxJSONBody is the most any request that is not a file upload may send. The
// largest legitimate body is a list of paths or ids, far below this.
const MaxJSONBody = 1 << 20

// LimitBody caps the body of every request but a multipart one (the upload, which
// streams to disk and enforces its own limit). Without it a handler that decodes
// JSON reads whatever the client sends, and the unauthenticated ones (login,
// setup, register, password reset) would let anyone fill the server's memory.
// A handler that wants a tighter limit still sets its own.
func LimitBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err == nil && mt == "multipart/form-data" {
				next.ServeHTTP(w, r)
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
