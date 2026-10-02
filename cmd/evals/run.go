package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nimbusxr/axx-evals/internal/results"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// cmdRun runs every task under each condition for one agent: it builds the
// images, prepares one Harbor dataset per condition, runs `harbor run` for
// each, and writes the result file and table.
// harborRetries is the Harbor job configuration (`harbor run -c`) that runs a
// trial again when something other than the agent ended it: its containers
// failed to build or start, or the model provider's rate limit cut the agent
// off (results.NotScored). Up to twice, after 60 s and then 120 s, so a
// per-minute limit has passed. The agent's own failures (its process failing,
// a timeout) are never retried: no attempt gets a second chance.
func harborRetries() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"retry": map[string]any{
			"max_retries":        2,
			"include_exceptions": results.NotScored,
			"min_wait_sec":       60,
			"wait_multiplier":    2,
			"max_wait_sec":       300,
		},
	}, "", "  ")
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	root := fs.String("root", "", "the evals directory")
	agent := fs.String("agent", "", "Harbor agent: opencode, the one the evals compare (oracle and nop check the plumbing)")
	model := fs.String("model", "", "model for the agent, e.g. openrouter/openai/gpt-6-luna")
	condList := fs.String("conditions", "none,skills,mcp,both,plain", "conditions from conditions.toml")
	only := fs.String("tasks", "", "comma-separated task ids (default: all)")
	attempts := fs.Int("attempts", 1, "trials per task and condition (harbor -k)")
	concurrency := fs.Int("concurrency", 2, "concurrent trials (harbor -n); each trial runs its own databases")
	pending := fs.Bool("include-pending", false, "also run tasks whose required packs are missing")
	skipImages := fs.Bool("skip-images", false, "do not rebuild the images")
	harbor := fs.String("harbor", "harbor", "the harbor command")
	jobsDir := fs.String("jobs-dir", "", "where Harbor writes jobs (default .work/jobs)")
	out := fs.String("out", "", "result file without extension (default results/<date>-<agent>)")
	dry := fs.Bool("dry-run", false, "prepare the datasets and print the harbor commands without running them")
	envFile := fs.String("env-file", "", "a .env file with the agent's API keys (harbor --env-file)")
	expect := fs.Float64("expect-reward", -1, "fail unless every task gets this reward under every condition (1 for the oracle agent, 0 for nop)")
	var agentKwargs multiFlag
	fs.Var(&agentKwargs, "ak", "agent key=value option passed to harbor (repeatable)")
	_ = fs.Parse(args)
	if *agent == "" {
		return exitError{2, "--agent is required"}
	}
	dir, err := evalsRoot(*root)
	if err != nil {
		return err
	}
	conds, err := loadConditions(dir)
	if err != nil {
		return err
	}
	names := splitList(*condList)
	for _, n := range names {
		if _, ok := conds[n]; !ok {
			return fmt.Errorf("unknown condition %q (see conditions.toml)", n)
		}
	}
	if !*skipImages && !*dry {
		if err := buildImages(dir, false); err != nil {
			return err
		}
	}
	packs, axxVersion := map[string]bool{}, ""
	if !*pending {
		if packs, axxVersion, err = imagePacks(); err != nil {
			return err
		}
	}
	date := time.Now().UTC().Format("2006-01-02")
	runID := fmt.Sprintf("%s-%s-%s", date, sanitize(*agent), time.Now().UTC().Format("150405"))
	if *jobsDir == "" {
		*jobsDir = filepath.Join(dir, ".work", "jobs")
	}
	retries, err := harborRetries()
	if err != nil {
		return err
	}
	retryConfig := filepath.Join(dir, ".work", "datasets", runID+"-harbor.json")
	if err := os.MkdirAll(filepath.Dir(retryConfig), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(retryConfig, retries, 0o644); err != nil {
		return err
	}
	jobs := map[string]string{}
	var skipped []results.Skipped
	var failed []string // conditions whose harbor run failed
	for _, n := range names {
		c := conds[n]
		dataset := filepath.Join(dir, ".work", "datasets", runID, n)
		_, sk, err := prepareDataset(dir, c, dataset, splitList(*only), packs, *pending)
		if err != nil {
			return err
		}
		skipped = append(skipped, sk...)
		jobName := runID + "-" + n
		hargs := []string{
			"run", "-p", dataset, "-a", *agent, "-o", *jobsDir, "--job-name", jobName,
			"-n", strconv.Itoa(*concurrency), "-k", strconv.Itoa(*attempts), "-y",
		}
		hargs = append(hargs, "-c", retryConfig)
		if *model != "" {
			hargs = append(hargs, "-m", *model)
		}
		if c.MCP {
			hargs = append(hargs, "--mcp-config", filepath.Join(dir, "conditions", "mcp.json"))
		}
		if *envFile != "" {
			hargs = append(hargs, "--env-file", *envFile)
		}
		for _, kv := range agentKwargs {
			hargs = append(hargs, "--ak", kv)
		}
		fmt.Printf("== condition %s: %s %s\n", n, *harbor, strings.Join(hargs, " "))
		jobs[n] = filepath.Join(*jobsDir, jobName)
		if *dry {
			continue
		}
		// A failed condition does not stop the others: its finished trials
		// still count, and the run reports the failure once the results are
		// written.
		if err := runStreaming(*harbor, hargs...); err != nil {
			fmt.Fprintf(os.Stderr, "harbor run for condition %s failed: %v\n", n, err)
			failed = append(failed, n)
		}
	}
	if *dry {
		return nil
	}
	if *out == "" {
		*out = filepath.Join(dir, "results", date+"-"+sanitize(*agent))
	}
	f, err := buildResults(dir, *agent, *model, axxVersion, date, names, jobs, skipped)
	if err != nil {
		return err
	}
	if err := writeResults(f, *out); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("harbor run failed for condition(s) %s; the results hold their finished trials", strings.Join(failed, ", "))
	}
	if bad := f.Unscored(); len(bad) > 0 {
		return fmt.Errorf("the run is incomplete: some trials were not scored, even after Harbor's retries:\n  %s", strings.Join(bad, "\n  "))
	}
	if *expect >= 0 {
		if bad := f.Unexpected(*expect); len(bad) > 0 {
			return fmt.Errorf("not every trial got reward %g:\n  %s", *expect, strings.Join(bad, "\n  "))
		}
		fmt.Printf("every trial got reward %g, as expected\n", *expect)
	}
	return nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		}
		return '-'
	}, s)
}

func harborVersion() string {
	out, err := exec.CommandContext(context.Background(), "harbor", "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func writeResults(f *results.File, out string) error {
	if err := f.Write(out + ".json"); err != nil {
		return err
	}
	if err := os.WriteFile(out+".md", []byte(f.Markdown()), 0o644); err != nil {
		return err
	}
	fmt.Print(f.Markdown())
	fmt.Printf("\nwrote %s.json and %s.md\n", out, out)
	return nil
}
