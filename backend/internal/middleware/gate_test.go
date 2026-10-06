package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// held is a handler that stays inside until it is released, and counts how many are inside.
type held struct {
	release chan struct{}
	inside  atomic.Int32
	peak    atomic.Int32
}

func newHeld() *held { return &held{release: make(chan struct{})} }

func (h *held) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := h.inside.Add(1)
	for {
		p := h.peak.Load()
		if n <= p || h.peak.CompareAndSwap(p, n) {
			break
		}
	}
	<-h.release
	h.inside.Add(-1)
	w.WriteHeader(http.StatusOK)
}

func get(handler http.Handler) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/works", nil))
	return rec
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestGate_NeverHasMoreThanTheLimitInside(t *testing.T) {
	h := newHeld()
	gate := Gate(3, 20, 2*time.Second)(h)
	var wg sync.WaitGroup
	codes := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- get(gate).Code }()
	}
	waitFor(t, "3 inside", func() bool { return h.inside.Load() == 3 })
	time.Sleep(30 * time.Millisecond) // the other seven wait: they do not come in
	if got := h.inside.Load(); got != 3 {
		t.Fatalf("inside = %d, want 3", got)
	}
	close(h.release)
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != http.StatusOK {
			t.Errorf("a request that waited its turn got %d, want 200", c)
		}
	}
	if h.peak.Load() > 3 {
		t.Errorf("peak inside = %d, want at most 3", h.peak.Load())
	}
}

func TestGate_ARequestThatFindsTheQueueFullIsAnsweredAtOnceWithBusy(t *testing.T) {
	h := newHeld()
	gate := Gate(1, 1, 5*time.Second)(h)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); get(gate) }() // inside
	waitFor(t, "one inside", func() bool { return h.inside.Load() == 1 })
	go func() { defer wg.Done(); get(gate) }() // in the queue
	time.Sleep(30 * time.Millisecond)

	start := time.Now()
	rec := get(gate) // no slot and no room in the queue
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("a busy answer must say when to try again")
	}
	if time.Since(start) > time.Second {
		t.Errorf("the busy answer took %v: it must not wait", time.Since(start))
	}
	close(h.release)
	wg.Wait()
}

func TestGate_ARequestThatWaitsTooLongIsAnsweredBusy(t *testing.T) {
	h := newHeld()
	gate := Gate(1, 5, 60*time.Millisecond)(h)
	done := make(chan struct{})
	go func() { get(gate); close(done) }()
	waitFor(t, "one inside", func() bool { return h.inside.Load() == 1 })

	start := time.Now()
	rec := get(gate)
	waited := time.Since(start)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503 after the wait", rec.Code)
	}
	if waited < 50*time.Millisecond || waited > time.Second {
		t.Errorf("waited %v, want about 60ms", waited)
	}
	close(h.release)
	<-done
}

func TestGate_AWaitingRequestGetsTheSlotWhenOneIsFreed(t *testing.T) {
	h := newHeld()
	gate := Gate(1, 5, 2*time.Second)(h)
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- get(gate) }()
	waitFor(t, "one inside", func() bool { return h.inside.Load() == 1 })
	second := make(chan *httptest.ResponseRecorder, 1)
	go func() { second <- get(gate) }()
	time.Sleep(30 * time.Millisecond)
	h.release <- struct{}{} // the first leaves
	if rec := <-first; rec.Code != http.StatusOK {
		t.Errorf("first = %d", rec.Code)
	}
	waitFor(t, "the second inside", func() bool { return h.inside.Load() == 1 })
	h.release <- struct{}{}
	if rec := <-second; rec.Code != http.StatusOK {
		t.Errorf("second = %d, want 200", rec.Code)
	}
}

func TestGate_AGoneClientLeavesTheQueue(t *testing.T) {
	h := newHeld()
	gate := Gate(1, 1, 5*time.Second)(h)
	done := make(chan struct{})
	go func() { get(gate); close(done) }()
	waitFor(t, "one inside", func() bool { return h.inside.Load() == 1 })

	ctx, cancel := contextWithCancel()
	req := httptest.NewRequest(http.MethodGet, "/works", nil).WithContext(ctx)
	gone := make(chan struct{})
	go func() { gate.ServeHTTP(httptest.NewRecorder(), req); close(gone) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-gone:
	case <-time.After(time.Second):
		t.Fatal("a client that left kept its place in the queue")
	}
	// and its place in the queue is free again: a new one can wait (it is not answered busy at once)
	late := make(chan int, 1)
	go func() { late <- get(gate).Code }()
	select {
	case code := <-late:
		t.Fatalf("answered %d at once: the place of the client that left was not freed", code)
	case <-time.After(60 * time.Millisecond):
	}
	close(h.release)
	<-done
	if code := <-late; code != http.StatusOK {
		t.Errorf("late = %d, want 200", code)
	}
}

func TestGate_ZeroLimitTurnsItOff(t *testing.T) {
	h := newHeld()
	close(h.release)
	gate := Gate(0, 0, time.Second)(h)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := get(gate); rec.Code != http.StatusOK {
				t.Errorf("code = %d", rec.Code)
			}
		}()
	}
	wg.Wait()
}

func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func TestParseCatalogConcurrency(t *testing.T) {
	good := map[string]int{"": DefaultCatalogConcurrency, "  ": DefaultCatalogConcurrency, "8": 8, "0": 0, "1": 1, "100": 100, " 12 ": 12}
	for in, want := range good {
		got, err := ParseCatalogConcurrency(in)
		if err != nil || got != want {
			t.Errorf("ParseCatalogConcurrency(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"-1", "101", "eight", "8.5", "1e1"} {
		if _, err := ParseCatalogConcurrency(in); err == nil {
			t.Errorf("ParseCatalogConcurrency(%q) should be an error", in)
		}
	}
	if DefaultCatalogConcurrency >= 25 || DefaultCatalogConcurrency < 1 {
		t.Errorf("the default (%d) must leave connections of the pool of 25 for the rest", DefaultCatalogConcurrency)
	}
}

// A place in the queue is given back whichever way it was left: by getting the slot, or by waiting too long.
func TestGate_ThePlaceInTheQueueIsGivenBack(t *testing.T) {
	t.Run("when the request gets the slot", func(t *testing.T) {
		h := newHeld()
		gate := Gate(1, 1, 5*time.Second)(h)
		done := make(chan struct{}, 3)
		go func() { get(gate); done <- struct{}{} }() // A, inside
		waitFor(t, "A inside", func() bool { return h.inside.Load() == 1 })
		go func() { get(gate); done <- struct{}{} }() // B, in the queue
		time.Sleep(30 * time.Millisecond)
		h.release <- struct{}{} // A leaves; B comes in
		<-done
		waitFor(t, "B inside", func() bool { return h.inside.Load() == 1 })
		late := make(chan int, 1)
		go func() { late <- get(gate).Code }() // C: the queue has room again, so it waits
		select {
		case code := <-late:
			t.Fatalf("C was answered %d at once: B's place in the queue was not given back", code)
		case <-time.After(60 * time.Millisecond):
		}
		close(h.release)
		<-done
		if code := <-late; code != http.StatusOK {
			t.Errorf("C = %d, want 200", code)
		}
	})
	t.Run("when the request waits too long", func(t *testing.T) {
		h := newHeld()
		gate := Gate(1, 1, 50*time.Millisecond)(h)
		done := make(chan struct{})
		go func() { get(gate); close(done) }()
		waitFor(t, "A inside", func() bool { return h.inside.Load() == 1 })
		if rec := get(gate); rec.Code != http.StatusServiceUnavailable { // B waits 50ms, gives up
			t.Fatalf("B = %d, want 503", rec.Code)
		}
		start := time.Now()
		rec := get(gate) // C: the queue has room again, so it waits as long as B did, and not less
		if rec.Code != http.StatusServiceUnavailable || time.Since(start) < 40*time.Millisecond {
			t.Errorf("C = %d after %v: it should have waited its turn, not been refused at once", rec.Code, time.Since(start))
		}
		close(h.release)
		<-done
	})
}

// The limit can be changed while the gate works: what the owner does from the administration.
func TestGate_RaisingTheLimitLetsTheOnesThatWaitIn(t *testing.T) {
	h := newHeld()
	g := NewGate(1, 10, 5*time.Second)
	gate := g.Middleware()(h)
	done := make(chan *httptest.ResponseRecorder, 4)
	for i := 0; i < 4; i++ {
		go func() { done <- get(gate) }()
	}
	waitFor(t, "one inside", func() bool { return h.inside.Load() == 1 })
	time.Sleep(30 * time.Millisecond)
	if got := h.inside.Load(); got != 1 {
		t.Fatalf("inside = %d, want 1", got)
	}
	g.SetLimit(3)
	waitFor(t, "three inside", func() bool { return h.inside.Load() == 3 })
	time.Sleep(30 * time.Millisecond)
	if got := h.inside.Load(); got != 3 {
		t.Errorf("inside after raising to 3 = %d, want 3 (one still waits)", got)
	}
	if g.Limit() != 3 {
		t.Errorf("Limit() = %d, want 3", g.Limit())
	}
	close(h.release)
	for i := 0; i < 4; i++ {
		if rec := <-done; rec.Code != http.StatusOK {
			t.Errorf("code = %d, want 200", rec.Code)
		}
	}
}

func TestGate_LoweringTheLimitDoesNotCutWhatIsInsideButHoldsTheNext(t *testing.T) {
	h := newHeld()
	g := NewGate(3, 30, 5*time.Second)
	gate := g.Middleware()(h)
	done := make(chan *httptest.ResponseRecorder, 4)
	for i := 0; i < 3; i++ {
		go func() { done <- get(gate) }()
	}
	waitFor(t, "three inside", func() bool { return h.inside.Load() == 3 })
	g.SetLimit(1)
	go func() { done <- get(gate) }() // a fourth: it must wait until fewer than 1 are inside
	time.Sleep(30 * time.Millisecond)
	if got := h.inside.Load(); got != 3 {
		t.Fatalf("lowering the limit cut what was inside: %d, want 3", got)
	}
	h.release <- struct{}{} // 2 inside: still not below the limit of 1
	h.release <- struct{}{} // 1 inside
	time.Sleep(30 * time.Millisecond)
	if got := h.inside.Load(); got != 1 {
		t.Fatalf("inside = %d, want 1: the waiting one came in while the limit of 1 was full", got)
	}
	h.release <- struct{}{} // 0 inside: now the waiting one comes in
	waitFor(t, "the waiting one inside", func() bool { return h.inside.Load() == 1 })
	close(h.release)
	for i := 0; i < 4; i++ {
		if rec := <-done; rec.Code != http.StatusOK {
			t.Errorf("code = %d, want 200", rec.Code)
		}
	}
}

func TestGate_ALimitOfZeroLetsEveryoneIn_AndTheLineKeepsItsProportion(t *testing.T) {
	h := newHeld()
	g := NewGate(1, 8, 5*time.Second) // 8 places in the line for each one inside
	gate := g.Middleware()(h)
	done := make(chan *httptest.ResponseRecorder, 40)
	go func() { done <- get(gate) }()
	waitFor(t, "one inside", func() bool { return h.inside.Load() == 1 })
	for i := 0; i < 8; i++ { // the line of the limit of 1 is full with 8
		go func() { done <- get(gate) }()
	}
	time.Sleep(50 * time.Millisecond)
	if rec := get(gate); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("the ninth in the line = %d, want 503", rec.Code)
	}
	g.SetLimit(2) // the line is now of 16
	for i := 0; i < 7; i++ {
		go func() { done <- get(gate) }()
	}
	time.Sleep(50 * time.Millisecond)
	probe := make(chan int, 1)
	go func() { probe <- get(gate).Code }()
	select {
	case code := <-probe:
		t.Errorf("with a limit of 2 the line holds 16: the request was answered %d at once, it should wait", code)
	case <-time.After(60 * time.Millisecond): // it waits in the line, as it should
	}
	g.SetLimit(0) // no limit: everybody inside
	waitFor(t, "everybody inside", func() bool { return h.inside.Load() >= 16 })
	close(h.release)
}

func TestGate_ARequestThatGivesUpWhileGrantedDoesNotLeakItsPlace(t *testing.T) {
	h := newHeld()
	g := NewGate(1, 5, 5*time.Second)
	gate := g.Middleware()(h)
	first := make(chan struct{})
	go func() { get(gate); close(first) }()
	waitFor(t, "one inside", func() bool { return h.inside.Load() == 1 })
	for i := 0; i < 20; i++ { // waiters that leave at about the moment their place comes
		ctx, cancel := contextWithCancel()
		req := httptest.NewRequest(http.MethodGet, "/works", nil).WithContext(ctx)
		go gate.ServeHTTP(httptest.NewRecorder(), req)
		time.Sleep(time.Millisecond)
		cancel()
	}
	time.Sleep(50 * time.Millisecond)
	h.release <- struct{}{}
	<-first
	// After all of that the gate must be empty: a new request goes in at once.
	next := make(chan *httptest.ResponseRecorder, 1)
	go func() { next <- get(gate) }()
	waitFor(t, "the new one inside", func() bool { return h.inside.Load() == 1 })
	close(h.release)
	if rec := <-next; rec.Code != http.StatusOK {
		t.Errorf("new request = %d, want 200", rec.Code)
	}
}
