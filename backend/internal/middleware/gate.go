package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Gate keeps the heavy reads of the catalog (the list, the counters, the search) from taking all the database connections
// of the process. Without it a burst of them, as many as the screens ask when a library is being imported (every open tab
// asks the whole home again for each work that is processed), holds the whole pool: then the sign-in, the health check and
// the saving of a reading position wait behind them, and the sign-in of a person timed out after 35 s on a real library.
//
// At most `limit` requests are inside at once; up to `backlog` more wait, for at most `wait`; the rest, and the ones that
// waited too long, are answered at once with 503 and Retry-After, instead of hanging. A `limit` of zero or less turns it off.
// The limit can be changed while it runs (SetLimit): the owner does it from the administration, and it holds from the next
// request on. It goes before the authentication, which itself reads the database: a request that waits must not hold a
// connection.
type DynamicGate struct {
	mu         sync.Mutex
	limit      int
	backlogPer int // places in the line for each one that may be inside
	fixedBack  int // the line when the gate was made with a limit of zero
	wait       time.Duration
	inside     int
	queue      []*gateWaiter
}

type gateWaiter struct {
	ready   chan struct{}
	granted bool
}

// NewGate makes the gate. `backlog` is the line for the limit given; when the limit changes the line keeps the same
// proportion (a limit of 8 with a line of 64 is eight places for each one that may be inside).
func NewGate(limit, backlog int, wait time.Duration) *DynamicGate {
	per := 8
	if limit > 0 && backlog > 0 {
		per = max(1, backlog/limit)
	}
	return &DynamicGate{limit: limit, backlogPer: per, fixedBack: backlog, wait: wait}
}

// Gate is the middleware of a gate that does not change.
func Gate(limit, backlog int, wait time.Duration) func(http.Handler) http.Handler {
	return NewGate(limit, backlog, wait).Middleware()
}

// SetLimit changes how many may be inside at once. Raising it lets the ones that wait in at once.
func (g *DynamicGate) SetLimit(limit int) {
	g.mu.Lock()
	g.limit = limit
	g.dispatch()
	g.mu.Unlock()
}

// Limit is how many may be inside at once now (zero or less: no limit).
func (g *DynamicGate) Limit() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.limit
}

func (g *DynamicGate) backlog() int {
	if g.limit > 0 {
		return g.limit * g.backlogPer
	}
	return g.fixedBack
}

// dispatch gives the free places to the ones that wait, in order. The caller holds the lock.
func (g *DynamicGate) dispatch() {
	for len(g.queue) > 0 && (g.limit <= 0 || g.inside < g.limit) {
		w := g.queue[0]
		g.queue = g.queue[1:]
		w.granted = true
		g.inside++
		close(w.ready)
	}
}

func (g *DynamicGate) remove(w *gateWaiter) {
	for i, other := range g.queue {
		if other == w {
			g.queue = append(g.queue[:i], g.queue[i+1:]...)
			return
		}
	}
}

type gateOutcome int

const (
	gateIn gateOutcome = iota
	gateBusy
	gateGone
)

func (g *DynamicGate) enter(r *http.Request) gateOutcome {
	g.mu.Lock()
	if g.limit <= 0 || g.inside < g.limit {
		g.inside++
		g.mu.Unlock()
		return gateIn
	}
	if len(g.queue) >= g.backlog() {
		g.mu.Unlock()
		return gateBusy
	}
	w := &gateWaiter{ready: make(chan struct{})}
	g.queue = append(g.queue, w)
	wait := g.wait
	g.mu.Unlock()

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-w.ready:
		return gateIn
	case <-timer.C:
	case <-r.Context().Done():
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if w.granted { // the place came just as the wait ended: it is this request's
		if r.Context().Err() != nil {
			g.inside--
			g.dispatch()
			return gateGone
		}
		return gateIn
	}
	g.remove(w)
	if r.Context().Err() != nil {
		return gateGone
	}
	return gateBusy
}

func (g *DynamicGate) leave() {
	g.mu.Lock()
	g.inside--
	g.dispatch()
	g.mu.Unlock()
}

// Middleware is the gate as a chi middleware.
func (g *DynamicGate) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch g.enter(r) {
			case gateBusy:
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(g.wait/time.Second)/4)))
				http.Error(w, "The server is busy; try again in a moment", http.StatusServiceUnavailable)
				return
			case gateGone:
				return
			}
			defer g.leave()
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
