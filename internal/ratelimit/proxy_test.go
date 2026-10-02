package ratelimit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// clock is a fake clock: sleeping moves it forward.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) sleep(_ context.Context, d time.Duration) error {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
	return nil
}

// provider answers each request with the next status in answers (200 once
// they run out); a 429 carries OpenRouter's error body, its reset at reset.
type provider struct {
	mu      sync.Mutex
	answers []int
	reset   func() time.Time
	seen    []*http.Request
}

func (f *provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.seen = append(f.seen, r)
	status := http.StatusOK
	if len(f.answers) > 0 {
		status, f.answers = f.answers[0], f.answers[1:]
	}
	f.mu.Unlock()
	if status == http.StatusTooManyRequests {
		w.WriteHeader(status)
		ms := ""
		if f.reset != nil {
			ms = strconv.FormatInt(f.reset().UnixMilli(), 10)
		}
		_, _ = io.WriteString(w, `{"error":{"message":"Rate limit exceeded","code":429,"metadata":{"headers":{"X-RateLimit-Limit":"20","X-RateLimit-Remaining":"0","X-RateLimit-Reset":"`+ms+`"},"limit_source":"openrouter_new_account"}}}`)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, "data: {\"ok\":true}\n\n")
}

func newProxy(t *testing.T, f *provider, c *clock) *Proxy {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return &Proxy{Target: u, now: c.now, sleep: c.sleep}
}

func send(t *testing.T, p *Proxy, tag string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions?x=1", strings.NewReader(`{"model":"m"}`))
	if tag != "" {
		r.Header.Set(TrialHeader, tag)
	}
	r.Header.Set("Authorization", "Bearer k")
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	return w
}

// With no limit in sight, requests go straight through: same path, query and
// authorization, no tag, no wait.
func TestPassesThrough(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	f := &provider{}
	p := newProxy(t, f, c)
	w := send(t, p, "abc123")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	got := f.seen[0]
	if got.URL.Path != "/api/v1/chat/completions" || got.URL.RawQuery != "x=1" || got.Header.Get("Authorization") != "Bearer k" || got.Header.Get(TrialHeader) != "" {
		t.Fatalf("forwarded %s?%s auth=%q tag=%q", got.URL.Path, got.URL.RawQuery, got.Header.Get("Authorization"), got.Header.Get(TrialHeader))
	}
	if wt := p.Waits()["abc123"]; wt != (Wait{Requests: 1}) {
		t.Fatalf("waits = %+v", wt)
	}
}

// A 429 holds the request until the reset OpenRouter names, sends it again,
// and the agent gets the answer that went through; the wait is the trial's.
func TestWaitsOutTheLimit(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	reset := c.now().Add(40 * time.Second)
	f := &provider{answers: []int{http.StatusTooManyRequests}, reset: func() time.Time { return reset }}
	p := newProxy(t, f, c)
	p.Resolve = func(tag string) string { return "trial-" + tag }
	w := send(t, p, "abc123")
	if w.Code != http.StatusOK || len(f.seen) != 2 {
		t.Fatalf("got %d after %d attempts", w.Code, len(f.seen))
	}
	wt := p.Waits()["trial-abc123"]
	if wt.Refused != 1 || wt.Requests != 1 || wt.Seconds < 40 || wt.Seconds > 41 {
		t.Fatalf("waits = %+v", wt)
	}

	// The limit holds every request, not only the refused one: once a 429
	// names a reset, another trial's request waits for it without being
	// sent, and the wait is that trial's.
	p.refused(http.Header{}, []byte(`{"error":{"metadata":{"headers":{"X-RateLimit-Reset":"`+strconv.FormatInt(c.now().Add(30*time.Second).UnixMilli(), 10)+`"}}}}`))
	before := len(f.seen)
	w = send(t, p, "def456")
	if w.Code != http.StatusOK || len(f.seen) != before+1 {
		t.Fatalf("got %d after %d attempts", w.Code, len(f.seen)-before)
	}
	if wt := p.Waits()["trial-def456"]; wt.Refused != 0 || wt.Requests != 1 || wt.Seconds < 30 || wt.Seconds > 31 {
		t.Fatalf("waits = %+v", wt)
	}
}

// Without a reset time, Retry-After or a doubling backoff decides the wait.
func TestBacksOffWithoutAReset(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	f := &provider{answers: []int{http.StatusTooManyRequests, http.StatusTooManyRequests}}
	p := newProxy(t, f, c)
	start := c.now()
	if w := send(t, p, "t"); w.Code != http.StatusOK || len(f.seen) != 3 {
		t.Fatalf("got %d after %d attempts", w.Code, len(f.seen))
	}
	// 2 s, then 4 s, each a quarter second past.
	if got := c.now().Sub(start); got != 6500*time.Millisecond {
		t.Fatalf("waited %s, want 6.5s", got)
	}
	h := http.Header{"Retry-After": {"7"}}
	if r, ok := resetTime(h, nil, start); !ok || r.Sub(start) != 7*time.Second {
		t.Fatalf("Retry-After: %s %v", r.Sub(start), ok)
	}
}

// A request is held at most MaxWait; then the agent gets the 429.
func TestGivesUpAfterMaxWait(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	f := &provider{answers: []int{429, 429, 429, 429, 429, 429, 429, 429, 429, 429}}
	f.reset = func() time.Time { return c.now().Add(time.Minute) }
	p := newProxy(t, f, c)
	p.MaxWait = 150 * time.Second
	w := send(t, p, "t")
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "Rate limit exceeded") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	if wt := p.Waits()["t"]; wt.Seconds < 150 || wt.Refused < 3 {
		t.Fatalf("waits = %+v", wt)
	}
}
