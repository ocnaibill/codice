package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Gate keeps the heavy reads of the catalog (the list, the counters, the search) from taking all the database connections
// of the process. Without it a burst of them, as many as the screens ask when a library is being imported (every open tab
// asks the whole home again for each work that is processed), holds the whole pool: then the sign-in, the health check and
// the saving of a reading position wait behind them, and the sign-in of a person timed out after 35 s on a real library.
//
// At most `limit` requests are inside at once; up to `backlog` more wait, for at most `wait`; the rest, and the ones that
// waited too long, are answered at once with 503 and Retry-After, instead of hanging. A `limit` of zero or less turns it off.
// It goes before the authentication, which itself reads the database: a request that waits must not hold a connection.
func Gate(limit, backlog int, wait time.Duration) func(http.Handler) http.Handler {
	if limit <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	slots := make(chan struct{}, limit)
	queue := make(chan struct{}, backlog)
	busy := func(w http.ResponseWriter) {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(wait/time.Second)/4)))
		http.Error(w, "The server is busy; try again in a moment", http.StatusServiceUnavailable)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case slots <- struct{}{}:
			default:
				// No slot: wait in the queue, if there is room in it.
				select {
				case queue <- struct{}{}:
				default:
					busy(w)
					return
				}
				timer := time.NewTimer(wait)
				defer timer.Stop()
				select {
				case slots <- struct{}{}:
					<-queue
				case <-timer.C:
					<-queue
					busy(w)
					return
				case <-r.Context().Done():
					<-queue
					return
				}
			}
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		})
	}
}

// DefaultCatalogConcurrency is how many heavy reads of the catalog run at once when CODICE_CATALOG_CONCURRENCY says nothing:
// a third of the pool of connections (25), so that the other two thirds are always there for the sign-in, the health check,
// the reading position and the dictionary.
const DefaultCatalogConcurrency = 8

// ParseCatalogConcurrency reads CODICE_CATALOG_CONCURRENCY: empty is the default, 0 turns the limit off, and anything that is
// not a whole number from 0 to 100 is an error (the API refuses to start, like with the other settings).
func ParseCatalogConcurrency(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultCatalogConcurrency, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > 100 {
		return 0, fmt.Errorf("CODICE_CATALOG_CONCURRENCY must be a whole number from 0 to 100 (0 turns the limit off), got %q", value)
	}
	return n, nil
}
