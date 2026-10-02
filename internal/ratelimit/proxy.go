// Package ratelimit is the proxy `evals run` puts between the agents and the
// model provider (OpenRouter). It sets no rate of its own: requests go through
// as they come, and only when the provider answers 429 does it hold every
// request until the provider says its limit resets (X-RateLimit-Reset, or
// Retry-After), then send the refused one again. So no agent is cut off by a
// rate limit, a run goes as fast as the provider allows, and it goes at full
// speed again as soon as the provider lifts the limit.
//
// It records, per trial, how long that trial's requests were held and how
// many the provider refused, so the results can leave the waits out of the
// agent's time. Agents tag their requests with TrialHeader (their container's
// hostname); the proxy removes it before forwarding.
package ratelimit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TrialHeader carries the agent's tag (its container's hostname).
const TrialHeader = "X-Evals-Trial"

// Wait is what a trial's requests spent on the provider's rate limit.
type Wait struct {
	// Seconds the trial's requests were held, summed: from the moment a
	// request reached the proxy until the attempt that went through was sent.
	Seconds float64 `json:"waitSeconds"`
	// Refused is the number of attempts the provider answered with 429 (each
	// was sent again).
	Refused int `json:"refused"`
	// Requests is the number of the trial's requests.
	Requests int `json:"requests"`
}

// Proxy forwards requests to Target, waiting out the provider's rate limit.
type Proxy struct {
	// Target is the provider's base, like https://openrouter.ai.
	Target *url.URL
	// Transport sends the requests (default http.DefaultTransport).
	Transport http.RoundTripper
	// Resolve maps an agent's tag to its trial's name (default: the tag).
	Resolve func(tag string) string
	// MaxWait bounds how long one request is held (default 10 minutes);
	// after that the provider's 429 goes to the agent.
	MaxWait time.Duration

	now   func() time.Time
	sleep func(context.Context, time.Duration) error

	once    sync.Once
	mu      sync.Mutex
	until   time.Time // no request is sent before
	backoff time.Duration
	names   map[string]string
	waits   map[string]*Wait
}

// hopHeaders are not forwarded by a proxy.
var hopHeaders = []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

func (p *Proxy) init() {
	p.once.Do(func() {
		if p.Transport == nil {
			p.Transport = http.DefaultTransport
		}
		if p.MaxWait == 0 {
			p.MaxWait = 10 * time.Minute
		}
		if p.now == nil {
			p.now = time.Now
		}
		if p.sleep == nil {
			p.sleep = sleepCtx
		}
		p.names = map[string]string{}
		p.waits = map[string]*Wait{}
	})
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ServeHTTP forwards one request, holding it while the provider's limit
// lasts.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.init()
	trial := p.trial(r.Header.Get(TrialHeader))
	r.Header.Del(TrialHeader)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "evals proxy: reading the request: "+err.Error(), http.StatusBadGateway)
		return
	}
	arrived := p.now()
	refused, held := 0, false
	var waited time.Duration
	defer func() { p.record(trial, waited, refused) }()
	for {
		if wait := p.waitFor(); wait > 0 {
			held = true
			if left := p.MaxWait - p.now().Sub(arrived); wait > left {
				wait = left
			}
			if err := p.sleep(r.Context(), wait); err != nil {
				waited = p.now().Sub(arrived) // the agent gave up waiting
				return
			}
		}
		sentAt := p.now()
		resp, err := p.Transport.RoundTrip(p.outgoing(r, body))
		if err != nil {
			waited = ifHeld(held, sentAt.Sub(arrived))
			http.Error(w, "evals proxy: "+err.Error(), http.StatusBadGateway)
			return
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			p.succeeded()
			waited = ifHeld(held, sentAt.Sub(arrived))
			copyResponse(w, resp)
			return
		}
		refused++
		held = true
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		p.refused(resp.Header, rb)
		if p.now().Sub(arrived) >= p.MaxWait {
			// Held as long as allowed: the agent gets the provider's answer.
			waited = p.now().Sub(arrived)
			copyHeader(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(rb)
			return
		}
	}
}

// ifHeld is a request's wait: none unless it was held.
func ifHeld(held bool, d time.Duration) time.Duration {
	if !held {
		return 0
	}
	return d
}

// outgoing builds the request to the provider.
func (p *Proxy) outgoing(r *http.Request, body []byte) *http.Request {
	u := *p.Target
	u.Path = strings.TrimRight(p.Target.Path, "/") + r.URL.Path
	u.RawQuery = r.URL.RawQuery
	out, _ := http.NewRequestWithContext(r.Context(), r.Method, u.String(), bytes.NewReader(body))
	copyHeader(out.Header, r.Header)
	out.Host = p.Target.Host
	return out
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		dst[k] = append([]string(nil), vs...)
	}
	for _, h := range hopHeaders {
		dst.Del(h)
	}
}

// copyResponse streams the provider's answer to the agent, flushing as it
// goes (the answers are server-sent events).
func copyResponse(w http.ResponseWriter, resp *http.Response) {
	defer resp.Body.Close()
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	f, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if f != nil {
				f.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// waitFor is how long requests must still be held.
func (p *Proxy) waitFor() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.until.Sub(p.now())
}

func (p *Proxy) succeeded() {
	p.mu.Lock()
	p.backoff = 0
	p.mu.Unlock()
}

// refused holds every request until the provider's limit resets: the time
// X-RateLimit-Reset names (in the headers or, for OpenRouter, in the error's
// metadata), else Retry-After, else a backoff that doubles from 2 s to 60 s.
func (p *Proxy) refused(h http.Header, body []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	until, ok := resetTime(h, body, now)
	if !ok {
		switch {
		case p.backoff == 0:
			p.backoff = 2 * time.Second
		case p.backoff < 30*time.Second:
			p.backoff *= 2
		default:
			p.backoff = time.Minute
		}
		until = now.Add(p.backoff)
	}
	// A little past the reset, so the provider's clock has turned too.
	until = until.Add(250 * time.Millisecond)
	if until.After(p.until) {
		p.until = until
	}
}

// resetTime reads when the provider's limit resets.
func resetTime(h http.Header, body []byte, now time.Time) (time.Time, bool) {
	reset := h.Get("X-RateLimit-Reset")
	if reset == "" {
		var e struct {
			Error struct {
				Metadata struct {
					Headers map[string]string `json:"headers"`
				} `json:"metadata"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil {
			for k, v := range e.Error.Metadata.Headers {
				if strings.EqualFold(k, "X-RateLimit-Reset") {
					reset = v
				}
			}
		}
	}
	if n, err := strconv.ParseFloat(strings.TrimSpace(reset), 64); err == nil && n > 0 {
		var t time.Time
		switch {
		case n > 1e12: // epoch milliseconds (OpenRouter)
			t = time.UnixMilli(int64(n))
		case n > 1e9: // epoch seconds
			t = time.Unix(int64(n), 0)
		default: // seconds from now
			t = now.Add(time.Duration(n * float64(time.Second)))
		}
		return t, true
	}
	if ra := strings.TrimSpace(h.Get("Retry-After")); ra != "" {
		if s, err := strconv.Atoi(ra); err == nil {
			return now.Add(time.Duration(s) * time.Second), true
		}
		if t, err := http.ParseTime(ra); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// trial names the trial a tag belongs to, resolving each tag once.
func (p *Proxy) trial(tag string) string {
	if tag == "" {
		return ""
	}
	p.mu.Lock()
	name, ok := p.names[tag]
	p.mu.Unlock()
	if ok {
		return name
	}
	name = tag
	if p.Resolve != nil {
		if n := p.Resolve(tag); n != "" {
			name = n
		}
	}
	p.mu.Lock()
	p.names[tag] = name
	p.mu.Unlock()
	return name
}

func (p *Proxy) record(trial string, waited time.Duration, refused int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	wt := p.waits[trial]
	if wt == nil {
		wt = &Wait{}
		p.waits[trial] = wt
	}
	wt.Requests++
	wt.Refused += refused
	wt.Seconds += waited.Seconds()
}

// Waits returns what each trial spent on the rate limit so far, by trial
// name; requests without a tag are under "".
func (p *Proxy) Waits() map[string]Wait {
	p.init()
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]Wait, len(p.waits))
	for k, v := range p.waits {
		out[k] = *v
	}
	return out
}
