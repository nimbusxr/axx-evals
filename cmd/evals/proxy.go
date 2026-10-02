package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/nimbusxr/axx-evals/internal/ratelimit"
	"github.com/nimbusxr/axx-evals/internal/results"
)

// openRouter is where the agents' model requests go.
const openRouter = "https://openrouter.ai"

// agentTimeoutMultiplier gives an agent held by the provider's rate limit the
// wall-clock time to wait; the results hold it to the task's budget on its
// own working time (results.FromHarborJob).
const agentTimeoutMultiplier = "4"

// modelProxy is the rate-limit proxy of a run (internal/ratelimit) and the
// OpenCode configuration that sends the agents through it.
type modelProxy struct {
	proxy  *ratelimit.Proxy
	server *http.Server
	// provider is OpenCode's configuration of OpenRouter (the "provider"
	// of opencode.json): its base URL is the proxy, and every request carries
	// the container's hostname, which the proxy maps to the trial.
	provider map[string]any
}

// startModelProxy serves the proxy where the agents' containers reach the
// host (host.docker.internal): the Docker bridge's gateway on Linux, the
// loopback with Docker Desktop.
func startModelProxy() (*modelProxy, error) {
	host := "127.0.0.1"
	if runtime.GOOS == "linux" {
		out, err := exec.CommandContext(context.Background(), "docker", "network", "inspect", "bridge", "--format", "{{(index .IPAM.Config 0).Gateway}}").Output()
		if err != nil {
			return nil, fmt.Errorf("finding the Docker bridge's gateway: %w", err)
		}
		host = strings.TrimSpace(string(out))
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return nil, err
	}
	target, _ := url.Parse(openRouter)
	p := &ratelimit.Proxy{Target: target, Resolve: trialOfContainer}
	srv := &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	port := ln.Addr().(*net.TCPAddr).Port
	provider := map[string]any{
		"openrouter": map[string]any{
			"options": map[string]any{
				"baseURL": fmt.Sprintf("http://host.docker.internal:%d/api/v1", port),
				"headers": map[string]string{ratelimit.TrialHeader: "{env:HOSTNAME}"},
			},
		},
	}
	return &modelProxy{proxy: p, server: srv, provider: provider}, nil
}

func (m *modelProxy) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = m.server.Shutdown(ctx)
}

// harborContainer is the name Harbor gives a trial's agent container:
// <trial>__env-main-1, the trial in lower case.
var harborContainer = regexp.MustCompile(`^/?(.+)__env-main-\d+$`)

// trialOfContainer maps a container's hostname (its id, by default) to the
// trial it runs, in lower case; "" when Docker does not know it.
func trialOfContainer(hostname string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Name}}", hostname).Output()
	if err != nil {
		return ""
	}
	if m := harborContainer.FindStringSubmatch(strings.TrimSpace(string(out))); m != nil {
		return m[1]
	}
	return ""
}

// writeWaits writes what the trials of a Harbor job spent on the provider's
// rate limit to <job>/rate-limit.json (results.RateLimitFile).
func (m *modelProxy) writeWaits(jobDir string) error {
	entries, err := os.ReadDir(jobDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	all := m.proxy.Waits()
	f := results.RateLimit{Trials: map[string]ratelimit.Wait{}}
	for _, e := range entries {
		if w, ok := all[strings.ToLower(e.Name())]; ok && e.IsDir() {
			f.Trials[e.Name()] = w
		}
	}
	for name, w := range all {
		if !strings.Contains(name, "__") { // not resolved to a trial
			f.Unattributed.Seconds += w.Seconds
			f.Unattributed.Refused += w.Refused
			f.Unattributed.Requests += w.Requests
		}
	}
	b, _ := json.MarshalIndent(f, "", "  ")
	return os.WriteFile(filepath.Join(jobDir, results.RateLimitFile), b, 0o644)
}

// cmdProxy serves the proxy on its own, for trying it out: requests to
// http://ADDR/api/v1/... go to OpenRouter; Ctrl-C prints the waits.
func cmdProxy(args []string) error {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8899", "where to listen")
	_ = fs.Parse(args)
	target, _ := url.Parse(openRouter)
	p := &ratelimit.Proxy{Target: target}
	srv := &http.Server{Addr: *addr, Handler: p, ReadHeaderTimeout: 30 * time.Second}
	fmt.Printf("proxy to %s on http://%s\n", openRouter, *addr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() { <-ctx.Done(); _ = srv.Close() }()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	b, _ := json.MarshalIndent(p.Waits(), "", "  ")
	fmt.Println(string(b))
	return nil
}
