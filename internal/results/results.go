// Package results aggregates Harbor job results into the evals result file
// (evals/results/<date>-<agent>.json), renders it as Markdown, and compares
// a result file with a model's baseline.
package results

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nimbusxr/axx-evals/internal/ratelimit"
)

// SchemaVersion of the result file.
const SchemaVersion = 1

// Kind marks result files.
const Kind = "axx-evals-results"

// File is a result file (and a baseline: a result file promoted as-is).
type File struct {
	SchemaVersion int      `json:"schemaVersion"`
	Kind          string   `json:"kind"`
	Date          string   `json:"date"`
	Agent         string   `json:"agent"`
	Model         string   `json:"model,omitempty"`
	Axx           string   `json:"axx,omitempty"`
	Harbor        string   `json:"harbor,omitempty"`
	Conditions    []string `json:"conditions"`
	Tasks         []Task   `json:"tasks"`
	// Skipped are tasks that did not run, with the reason (e.g. a missing pack).
	Skipped []Skipped `json:"skipped,omitempty"`
	// Scores maps a condition to its score: the mean reward over the tasks
	// that ran, in percent (0-100).
	Scores map[string]float64 `json:"scores"`
	// CoreScores are the same from the core reward: only what tests of any
	// kind are held to, so every condition, the plain one too, compares on it.
	CoreScores map[string]float64 `json:"coreScores,omitempty"`
}

// Task is one task's results per condition.
type Task struct {
	ID       string             `json:"id"`
	Title    string             `json:"title,omitempty"`
	Category string             `json:"category,omitempty"`
	Results  map[string]*Result `json:"results"`
}

// Result is a task's outcome under one condition.
type Result struct {
	// Reward is the mean reward over the trials (0-1).
	Reward float64 `json:"reward"`
	// Core is the mean core reward over the trials (0-1).
	Core float64 `json:"core"`
	// Trials are the scored trials; Reward and Core are their means.
	Trials int `json:"trials"`
	Passed int `json:"passed"`
	// CorePassed are the scored trials that passed every core check.
	CorePassed int `json:"corePassed"`
	// Work is what the agents of the scored trials did; Stumbles, the
	// troubles they ran into with axx, by kind; Misses, every scored trial
	// that did not get reward 1, and why.
	Work     *Work              `json:"work,omitempty"`
	Stumbles map[string]Stumble `json:"stumbles,omitempty"`
	Misses   []Miss             `json:"misses,omitempty"`
	// Errors are scored trials that ended in an exception of the agent's own
	// (it timed out or its process failed); the verifier's reward counts.
	Errors int `json:"errors,omitempty"`
	// Unscored are trials cut off by something other than the agent (see
	// NotScored), by exception type: they are left out of Trials and the
	// rewards, and the run reports them.
	Unscored map[string]int `json:"unscored,omitempty"`
	// MutantsCaught / MutantsTotal from the verifier, summed over trials.
	MutantsCaught int `json:"mutantsCaught,omitempty"`
	MutantsTotal  int `json:"mutantsTotal,omitempty"`
	// AgentSeconds is the agents' working time, summed over the scored
	// trials: the agent's run less what its requests waited on the model
	// provider's rate limit.
	AgentSeconds float64 `json:"agentSeconds,omitempty"`
	// RateLimitWaitSeconds and RateLimitRefused: what the trials' requests
	// spent on the provider's rate limit (internal/ratelimit), left out of
	// AgentSeconds and of the time budget.
	RateLimitWaitSeconds float64 `json:"rateLimitWaitSeconds,omitempty"`
	RateLimitRefused     int     `json:"rateLimitRefused,omitempty"`
	// Models counts the trials' model requests by model, from the rate-limit
	// proxy, every trial included.
	Models map[string]int `json:"models,omitempty"`
	// CostUSD and tokens, summed over trials when the agent reports them.
	CostUSD      float64 `json:"costUsd,omitempty"`
	InputTokens  int64   `json:"inputTokens,omitempty"`
	OutputTokens int64   `json:"outputTokens,omitempty"`
}

// RateLimitFile is the file in a Harbor job directory where `evals run`
// writes what each trial's requests spent on the model provider's rate limit.
const RateLimitFile = "rate-limit.json"

// RateLimit is RateLimitFile.
type RateLimit struct {
	// Trials by trial directory name.
	Trials map[string]ratelimit.Wait `json:"trials"`
	// Unattributed: requests the proxy could not tie to a trial.
	Unattributed ratelimit.Wait `json:"unattributed"`
}

// readRateLimit reads a job's RateLimitFile; none means no waits.
func readRateLimit(dir string) RateLimit {
	var rl RateLimit
	if b, err := os.ReadFile(filepath.Join(dir, RateLimitFile)); err == nil {
		_ = json.Unmarshal(b, &rl)
	}
	return rl
}

// NotScored are the exceptions that end a trial for reasons other than the
// agent: its containers failed to build or start (RuntimeError, which Harbor
// raises plain only for infrastructure, EnvironmentStartTimeoutError,
// HealthcheckError) or the model provider's rate limit cut it off
// (ApiRateLimitError). Harbor matches exact type names. `evals run` has Harbor
// retry these; a trial that still ends in one is not scored, since its reward
// says nothing about the agent. What the agent does ends in other types
// (NonZeroAgentExitCodeError, AgentTimeoutError), which are scored.
var NotScored = []string{"RuntimeError", "EnvironmentStartTimeoutError", "HealthcheckError", "ApiRateLimitError"}

func notScored(exceptionType string) bool {
	for _, t := range NotScored {
		if t == exceptionType {
			return true
		}
	}
	return false
}

// UnscoredCount is the number of a result's unscored trials.
func (r *Result) UnscoredCount() int {
	n := 0
	for _, c := range r.Unscored {
		n += c
	}
	return n
}

// Unscored lists each task and condition with trials that were not scored,
// and why. Empty when every trial was scored.
func (f *File) Unscored() []string {
	var out []string
	for _, t := range f.Tasks {
		for _, c := range f.Conditions {
			r := t.Results[c]
			if r == nil || r.UnscoredCount() == 0 {
				continue
			}
			types := make([]string, 0, len(r.Unscored))
			for ty, n := range r.Unscored {
				types = append(types, fmt.Sprintf("%s ×%d", ty, n))
			}
			sort.Strings(types)
			out = append(out, fmt.Sprintf("%s (%s): %d trial(s) not scored: %s", t.ID, c, r.UnscoredCount(), strings.Join(types, ", ")))
		}
	}
	return out
}

// OtherModels lists each task and condition whose trials called a model
// other than the one under test (File.Model, without its "openrouter/"
// prefix, as the requests name it). Empty when only that model was called,
// or when the run did not go through the proxy that counts them.
func (f *File) OtherModels() []string {
	if f.Model == "" {
		return nil
	}
	want := strings.TrimPrefix(f.Model, "openrouter/")
	var out []string
	for _, t := range f.Tasks {
		for _, c := range f.Conditions {
			r := t.Results[c]
			if r == nil {
				continue
			}
			var others []string
			for m, n := range r.Models {
				if m != want {
					others = append(others, fmt.Sprintf("%s ×%d", m, n))
				}
			}
			if len(others) > 0 {
				sort.Strings(others)
				out = append(out, fmt.Sprintf("%s (%s): %s", t.ID, c, strings.Join(others, ", ")))
			}
		}
	}
	return out
}

// Skipped is a task that did not run under a condition.
type Skipped struct {
	ID string `json:"id"`
	// Condition is the one it did not run under.
	Condition string `json:"condition,omitempty"`
	Reason    string `json:"reason"`
	// ByDesign: the task does not apply to the condition (it has no version
	// without axx), rather than missing something it needs.
	ByDesign bool `json:"byDesign,omitempty"`
}

// Unexpected lists, for a run whose every trial should get reward want (the
// oracle agent 1, the nop agent 0), each task and condition that did not, and
// the tasks that did not run. Empty when the run went as expected.
func (f *File) Unexpected(want float64) []string {
	var out []string
	for _, s := range f.Skipped {
		if !s.ByDesign {
			out = append(out, fmt.Sprintf("%s: skipped (%s)", s.ID, s.Reason))
		}
	}
	byDesign := map[[2]string]bool{}
	for _, s := range f.Skipped {
		if s.ByDesign {
			byDesign[[2]string{s.ID, s.Condition}] = true
		}
	}
	for _, t := range f.Tasks {
		for _, c := range f.Conditions {
			r := t.Results[c]
			switch {
			case (r == nil || r.Trials == 0 && r.UnscoredCount() == 0) && byDesign[[2]string{t.ID, c}]:
			case r != nil && r.UnscoredCount() > 0:
				out = append(out, fmt.Sprintf("%s (%s): %d trial(s) not scored", t.ID, c, r.UnscoredCount()))
			case r == nil || r.Trials == 0:
				out = append(out, fmt.Sprintf("%s (%s): no trial ran", t.ID, c))
			case r.Errors > 0:
				out = append(out, fmt.Sprintf("%s (%s): %d trial(s) ended in an exception", t.ID, c, r.Errors))
			case r.Reward != want:
				out = append(out, fmt.Sprintf("%s (%s): reward %g, want %g", t.ID, c, r.Reward, want))
			case r.Core != want:
				out = append(out, fmt.Sprintf("%s (%s): core reward %g, want %g", t.ID, c, r.Core, want))
			}
		}
	}
	return out
}

// Read loads a result file.
func Read(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Kind != Kind {
		return nil, fmt.Errorf("%s: not an evals result file (kind %q)", path, f.Kind)
	}
	if f.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%s: schemaVersion %d, want %d", path, f.SchemaVersion, SchemaVersion)
	}
	return &f, nil
}

// Write saves the result file as indented JSON.
func (f *File) Write(path string) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Score computes the scores over the given task IDs (all tasks when nil).
func (f *File) Score(only map[string]bool) map[string]float64 {
	return f.score(only, func(r *Result) float64 { return r.Reward })
}

// CoreScore is Score from the core reward.
func (f *File) CoreScore(only map[string]bool) map[string]float64 {
	return f.score(only, func(r *Result) float64 { return r.Core })
}

func (f *File) score(only map[string]bool, reward func(*Result) float64) map[string]float64 {
	out := map[string]float64{}
	for _, c := range f.Conditions {
		sum, n := 0.0, 0
		for _, t := range f.Tasks {
			if only != nil && !only[t.ID] {
				continue
			}
			if r := t.Results[c]; r != nil && r.Trials > 0 {
				sum += reward(r)
				n++
			}
		}
		if n > 0 {
			out[c] = math.Round(sum/float64(n)*1000) / 10
		}
	}
	return out
}

// ---- Harbor job results ----

type trialResult struct {
	TaskName string `json:"task_name"`
	TaskID   struct {
		Path string `json:"path"`
	} `json:"task_id"`
	AgentInfo struct {
		Name      string `json:"name"`
		ModelInfo *struct {
			Name     string `json:"name"`
			Provider string `json:"provider"`
		} `json:"model_info"`
	} `json:"agent_info"`
	AgentResult *struct {
		InputTokens  *int64   `json:"n_input_tokens"`
		OutputTokens *int64   `json:"n_output_tokens"`
		CostUSD      *float64 `json:"cost_usd"`
	} `json:"agent_result"`
	VerifierResult *struct {
		Rewards map[string]float64 `json:"rewards"`
	} `json:"verifier_result"`
	ExceptionInfo *struct {
		Type string `json:"exception_type"`
	} `json:"exception_info"`
	AgentExecution *struct {
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
	} `json:"agent_execution"`
}

// overBudget is the exception of a trial whose agent worked longer than the
// task's budget, its waits on the rate limit left out (FromHarborJob).
const overBudget = "AgentOverBudget"

// waitedOut is the exception of a trial that Harbor timed out although its
// agent's own working time was within the budget: the waits on the rate
// limit took the time, so it is not scored.
const waitedOut = "RateLimitWaits"

// FromHarborJob reads every trial of a Harbor job directory and returns the
// per-task results for one condition. budgets are the agents' time budgets
// by task id, in seconds: an agent's working time is its run less what its
// requests waited on the model provider's rate limit (RateLimitFile), and an
// agent that worked longer than its budget failed, as if timed out, while one
// that Harbor timed out within its budget, because of the waits, is not
// scored.
func FromHarborJob(dir string, budgets map[string]float64) (map[string]*Result, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]*Result{}, nil // the job never started: no trials
	}
	if err != nil {
		return nil, err
	}
	out := map[string]*Result{}
	sums, coreSums := map[string]float64{}, map[string]float64{}
	rl := readRateLimit(dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name(), "result.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var tr trialResult
		if err := json.Unmarshal(b, &tr); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		id := filepath.Base(tr.TaskID.Path)
		if id == "" || id == "." {
			id = tr.TaskName
		}
		r := out[id]
		if r == nil {
			r = &Result{}
			out[id] = r
		}
		if a := tr.AgentResult; a != nil {
			if a.CostUSD != nil {
				r.CostUSD += *a.CostUSD
			}
			if a.InputTokens != nil {
				r.InputTokens += *a.InputTokens
			}
			if a.OutputTokens != nil {
				r.OutputTokens += *a.OutputTokens
			}
		}
		wait := rl.Trials[e.Name()]
		r.RateLimitWaitSeconds += wait.Seconds
		r.RateLimitRefused += wait.Refused
		for m, n := range wait.Models {
			if r.Models == nil {
				r.Models = map[string]int{}
			}
			r.Models[m] += n
		}
		working := -1.0
		if a := tr.AgentExecution; a != nil && !a.StartedAt.IsZero() && !a.FinishedAt.IsZero() {
			working = max(a.FinishedAt.Sub(a.StartedAt).Seconds()-wait.Seconds, 0)
		}
		budget := budgets[id]
		exception := ""
		if tr.ExceptionInfo != nil {
			exception = tr.ExceptionInfo.Type
		}
		if exception == "AgentTimeoutError" && budget > 0 && working >= 0 && working < budget {
			exception = waitedOut
		}
		if exception != "" && (notScored(exception) || exception == waitedOut) {
			if r.Unscored == nil {
				r.Unscored = map[string]int{}
			}
			r.Unscored[exception]++
			continue
		}
		r.Trials++
		if working >= 0 {
			r.AgentSeconds += working
		}
		reward, core := 0.0, 0.0
		if tr.VerifierResult != nil {
			reward = tr.VerifierResult.Rewards["reward"]
			var ok bool
			if core, ok = tr.VerifierResult.Rewards["core"]; !ok {
				core = reward // a verifier from before the core reward
			}
		}
		if budget > 0 && working > budget && exception == "" {
			// Over its time budget: a timeout, whatever the verifier says.
			reward, core, exception = 0, 0, overBudget
		}
		in := analyzeTrial(filepath.Join(dir, e.Name()), wait.Requests)
		if r.Work == nil {
			r.Work = &Work{}
		}
		r.Work.add(in.work)
		for k, n := range in.stumbles {
			if r.Stumbles == nil {
				r.Stumbles = map[string]Stumble{}
			}
			st := r.Stumbles[k]
			st.Trials++
			st.Times += n
			r.Stumbles[k] = st
		}
		if core >= 1 {
			r.CorePassed++
		}
		if reward < 1 {
			r.Misses = append(r.Misses, missOf(e.Name(), exception, in.miss))
		}
		caught, total := mutantCounts(filepath.Join(dir, e.Name(), "verifier", "verify.json"))
		r.MutantsCaught += caught
		r.MutantsTotal += total
		if exception != "" {
			r.Errors++
		}
		if reward >= 1 {
			r.Passed++
		}
		sums[id] += reward
		coreSums[id] += core
	}
	for id, r := range out {
		if r.Trials > 0 {
			r.Reward = sums[id] / float64(r.Trials)
			r.Core = coreSums[id] / float64(r.Trials)
		}
	}
	return out, nil
}

// missOf is why a scored trial missed: the exception that ended it, else the
// first check the verifier failed.
func missOf(trial, exception string, verified *Miss) Miss {
	m := Miss{Trial: trial, Why: missOther, Core: true}
	switch {
	case exception == overBudget:
		m.Why = missOverBudget
	case exception == "AgentTimeoutError":
		m.Why = missTimedOut
	case exception != "" && verified == nil:
		m.Why, m.Detail = missCrashed, exception
	case verified != nil:
		m = *verified
		m.Trial = trial
	}
	return m
}

// mutantCounts reads the caught and total mutants from a verifier report
// (verify.json); a missing report counts nothing.
func mutantCounts(path string) (caught, total int) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	var rep struct {
		Mutants map[string]struct {
			Killed bool `json:"killed"`
		} `json:"mutants"`
		Total int `json:"mutantsTotal"`
	}
	if json.Unmarshal(b, &rep) != nil {
		return 0, 0
	}
	for _, m := range rep.Mutants {
		if m.Killed {
			caught++
		}
	}
	total = rep.Total
	if total < len(rep.Mutants) {
		total = len(rep.Mutants)
	}
	return caught, total
}

// ---- Markdown ----

// Markdown renders the task x condition table.
func (f *File) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# axx evals: %s", f.Agent)
	if f.Model != "" {
		fmt.Fprintf(&b, " (%s)", f.Model)
	}
	fmt.Fprintf(&b, ", %s\n\n", f.Date)
	if f.Axx != "" || f.Harbor != "" {
		fmt.Fprintf(&b, "axx %s, Harbor %s.\n\n", orDash(f.Axx), orDash(f.Harbor))
	}
	if other := f.OtherModels(); len(other) > 0 {
		fmt.Fprintf(&b, "**Other models called:** the agents' requests went to models besides %s, so these trials are not that model's alone.\n\n", f.Model)
		for _, line := range other {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	if bad := f.Unscored(); len(bad) > 0 {
		b.WriteString("**Incomplete run:** some trials were not scored, so the scores below rest on fewer trials than the run asked for. Rerun them before comparing conditions.\n\n")
		for _, line := range bad {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	f.withAndWithout(&b)
	b.WriteString("## Scores by task\n\nA cell is the share of trials whose verifier gave reward 1; `err` counts scored trials that ended in an exception of the agent's own (a timeout, a crash); `not scored` counts trials cut off by the infrastructure or the model provider's rate limit, which the scores leave out.\n\n")
	if len(f.CoreScores) > 0 {
		b.WriteString("The **core score** holds every condition to the same checks: the tests pass against the correct service, twice, and against its correct variants (the same service as it may differ within its contract), and fail against every planted bug, with no cheating. The **score** adds what only an axx suite has (`axx validate`, the features' readability, `axx lint` where the task asks); without axx (`plain`) the two are the same.\n\n")
	}
	b.WriteString("| Task | Category |")
	for _, c := range f.Conditions {
		fmt.Fprintf(&b, " %s |", c)
	}
	b.WriteString("\n| --- | --- |")
	for range f.Conditions {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	tasks := append([]Task(nil), f.Tasks...)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	for _, t := range tasks {
		fmt.Fprintf(&b, "| %s | %s |", t.ID, orDash(t.Category))
		for _, c := range f.Conditions {
			fmt.Fprintf(&b, " %s |", cell(t.Results[c]))
		}
		b.WriteString("\n")
	}
	for _, row := range []struct {
		name   string
		scores map[string]float64
	}{{"Core score", f.CoreScores}, {"Score", f.Scores}} {
		if row.scores == nil {
			continue
		}
		fmt.Fprintf(&b, "| **%s** | |", row.name)
		for _, c := range f.Conditions {
			if s, ok := row.scores[c]; ok {
				fmt.Fprintf(&b, " **%.1f** |", s)
			} else {
				b.WriteString(" - |")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	f.axxOnlyMisses(&b)
	f.howTheyWorked(&b)
	f.stumbled(&b)
	f.everyMiss(&b)
	if wait, refused := f.rateLimitWaits(); wait > 0 || refused > 0 {
		fmt.Fprintf(&b, "The model provider's rate limit held the agents' requests for %.1f minutes in all (%d refused and sent again). Agent minutes and the time budget leave those waits out.\n", wait/60, refused)
	}
	if len(f.Skipped) > 0 {
		b.WriteString("\nNot run:\n\n")
		for _, s := range f.Skipped {
			if s.Condition != "" {
				fmt.Fprintf(&b, "- `%s` (%s): %s\n", s.ID, s.Condition, s.Reason)
			} else {
				fmt.Fprintf(&b, "- `%s`: %s\n", s.ID, s.Reason)
			}
		}
	}
	return b.String()
}

// rateLimitWaits sums the run's waits on the rate limit: seconds, refusals.
func (f *File) rateLimitWaits() (float64, int) {
	secs, refused := 0.0, 0
	for _, t := range f.Tasks {
		for _, r := range t.Results {
			secs += r.RateLimitWaitSeconds
			refused += r.RateLimitRefused
		}
	}
	return secs, refused
}

func cell(r *Result) string {
	if r == nil || r.Trials == 0 && r.UnscoredCount() == 0 {
		return "-"
	}
	if r.Trials == 0 {
		return fmt.Sprintf("not scored (%d)", r.UnscoredCount())
	}
	var s string
	if r.Trials == 1 {
		s = map[bool]string{true: "pass", false: "fail"}[r.Passed == 1]
	} else {
		s = fmt.Sprintf("%d/%d", r.Passed, r.Trials)
	}
	if r.Errors > 0 {
		s += fmt.Sprintf(" (%d err)", r.Errors)
	}
	if n := r.UnscoredCount(); n > 0 {
		s += fmt.Sprintf(" (%d not scored)", n)
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---- comparison ----

// Delta is one condition's change between a baseline and a result.
type Delta struct {
	Condition string  `json:"condition"`
	Baseline  float64 `json:"baseline"`
	Current   float64 `json:"current"`
	Change    float64 `json:"change"`
	Tasks     int     `json:"tasks"`
	Regressed bool    `json:"regressed"`
}

// Comparison is the result of Compare.
type Comparison struct {
	MaxDrop float64 `json:"maxDrop"`
	Deltas  []Delta `json:"deltas"`
	// TaskChanges lists tasks that went from pass to fail or back.
	TaskChanges []string `json:"taskChanges,omitempty"`
	Regressed   bool     `json:"regressed"`
}

// Compare scores both files over the tasks they have in common and flags
// every condition whose score dropped by more than maxDrop points.
func Compare(baseline, current *File, maxDrop float64) (*Comparison, error) {
	if baseline.Agent != current.Agent {
		return nil, fmt.Errorf("the baseline is for agent %q, the results for %q", baseline.Agent, current.Agent)
	}
	common := map[string]bool{}
	bt := map[string]Task{}
	for _, t := range baseline.Tasks {
		bt[t.ID] = t
	}
	for _, t := range current.Tasks {
		if _, ok := bt[t.ID]; ok {
			common[t.ID] = true
		}
	}
	if len(common) == 0 {
		return nil, errors.New("the baseline and the results have no task in common")
	}
	bs, cs := baseline.Score(common), current.Score(common)
	c := &Comparison{MaxDrop: maxDrop}
	for _, cond := range current.Conditions {
		b, okb := bs[cond]
		cur, okc := cs[cond]
		if !okb || !okc {
			continue
		}
		d := Delta{Condition: cond, Baseline: b, Current: cur, Change: math.Round((cur-b)*10) / 10, Tasks: len(common)}
		d.Regressed = b-cur > maxDrop
		c.Regressed = c.Regressed || d.Regressed
		c.Deltas = append(c.Deltas, d)
	}
	for _, t := range current.Tasks {
		if !common[t.ID] {
			continue
		}
		for _, cond := range current.Conditions {
			br, cr := bt[t.ID].Results[cond], t.Results[cond]
			if br == nil || cr == nil {
				continue
			}
			switch {
			case br.Reward >= 1 && cr.Reward < 1:
				c.TaskChanges = append(c.TaskChanges, fmt.Sprintf("%s/%s: pass -> fail", t.ID, cond))
			case br.Reward < 1 && cr.Reward >= 1:
				c.TaskChanges = append(c.TaskChanges, fmt.Sprintf("%s/%s: fail -> pass", t.ID, cond))
			}
		}
	}
	sort.Strings(c.TaskChanges)
	return c, nil
}

// String renders a comparison for a terminal or a PR comment.
func (c *Comparison) String() string {
	var b strings.Builder
	b.WriteString("| Condition | Baseline | Current | Change | |\n| --- | --- | --- | --- | --- |\n")
	for _, d := range c.Deltas {
		mark := "ok"
		if d.Regressed {
			mark = fmt.Sprintf("REGRESSION (more than %.0f points)", c.MaxDrop)
		}
		fmt.Fprintf(&b, "| %s | %.1f | %.1f | %+.1f | %s |\n", d.Condition, d.Baseline, d.Current, d.Change, mark)
	}
	if len(c.TaskChanges) > 0 {
		b.WriteString("\nChanged tasks:\n\n")
		for _, t := range c.TaskChanges {
			fmt.Fprintf(&b, "- %s\n", t)
		}
	}
	return b.String()
}
